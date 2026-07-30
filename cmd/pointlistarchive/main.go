// pointlistarchive builds the publishable pointlist download tree from the
// fidohist corpus.
//
// The corpus stores weeklies as ZIP archives named NAME.Z##, where ## is the
// day-of-year MOD 100 — the true day only exists inside the archive. The web
// download surface (internal/web/pointlist_download.go) lists a file only when
// its pre-.gz extension is a 3-digit day, under
// <pointlist_root>/<network>/<source>/<year>/. So the corpus cannot be copied
// to a server as-is; it has to be extracted, renamed and recompressed. This
// tool does that, deliberately reusing parser.DerivePointlistSource so the
// published layout can never disagree with what the importer filed in
// ClickHouse.
//
// Usage:
//
//	pointlistarchive -corpus <fidohist-pntlist> -out <pointlists root> [-dry-run]
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/nodelistfs"
	"github.com/nodelistdb/internal/parser"
)

var (
	// dayExtRe matches a 3-digit day-of-year extension (the shape the web
	// download page requires).
	dayExtRe = regexp.MustCompile(`^[0-9]{3}$`)
	// zipExtRe matches the corpus' ZIP naming: .zip and the .Z## weeklies.
	zipExtRe = regexp.MustCompile(`(?i)^\.(zip|z[0-9]{2})$`)
	// lhaExtRe matches LHA/LZH archives. They occur only in the diff series
	// this tool skips, so an unpacker dependency is not worth taking on.
	lhaExtRe = regexp.MustCompile(`(?i)^\.(lha|lzh|l[0-9]{2})$`)
	// yearPathRe finds the year component of a corpus path. Both layouts
	// occur: <region>/<year>/<format>/ and the inverted <region>/<format>/<year>/.
	yearPathRe = regexp.MustCompile(`(19[89][0-9]|20[0-5][0-9])`)
	// headerDayRe reads the authoritative day from a list header line, e.g.
	// ";A Fidonet R24 pointlist for Friday 05-Jan-2024 -- Day number 005".
	headerDayRe = regexp.MustCompile(`(?i)day\s*number\s*[: ]?\s*([0-9]{1,3})`)
	// headerYearRe rescues the ~7 corpus files that sit at a region root with
	// no year directory; their headers carry a full date.
	headerYearRe = regexp.MustCompile(`(19[89][0-9]|20[0-5][0-9])`)
)

type stats struct {
	archives  int
	written   int
	collided  int
	skipped   int
	unknown   int
	bytesOut  int64
	bytesRead int64
}

// origin remembers which corpus file produced a published target, so a
// collision report names both sides.
type origin struct {
	source string // corpus path
	member string // member name inside the archive ("" if the file was plain)
}

type builder struct {
	corpus  string
	out     string
	network string
	dryRun  bool
	report  *os.File

	written map[string]origin
	st      stats
}

func main() {
	corpus := flag.String("corpus", "", "Corpus root (the fidohist-pntlist directory)")
	out := flag.String("out", "", "Output pointlists root; the tree is written to <out>/<network>/<source>/<year>/")
	network := flag.String("network", "fidonet", "FTN network the corpus belongs to")
	reportPath := flag.String("report", "", "Write the skip/collision report here (default <out>/build-report.txt)")
	dryRun := flag.Bool("dry-run", false, "Resolve and report without writing any file")
	flag.Parse()

	if *corpus == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-corpus and -out are required")
		os.Exit(2)
	}
	if !nodelistfs.ValidNetworkName(*network) {
		fmt.Fprintf(os.Stderr, "invalid network name %q: the download routes accept only [a-z0-9_-]+\n", *network)
		os.Exit(2)
	}

	rp := *reportPath
	if rp == "" {
		rp = filepath.Join(*out, "build-report.txt")
	}
	if err := os.MkdirAll(filepath.Dir(rp), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create report directory: %v\n", err)
		os.Exit(1)
	}
	rf, err := os.Create(rp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create report: %v\n", err)
		os.Exit(1)
	}
	defer rf.Close()

	b := &builder{
		corpus:  *corpus,
		out:     *out,
		network: *network,
		dryRun:  *dryRun,
		report:  rf,
		written: make(map[string]origin),
	}

	if err := b.run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\narchives/files read: %d (%.1f MB)\n", b.st.archives, float64(b.st.bytesRead)/(1<<20))
	fmt.Printf("published:          %d (%.1f MB gzip)\n", b.st.written, float64(b.st.bytesOut)/(1<<20))
	fmt.Printf("collisions:         %d (first-wins, all logged)\n", b.st.collided)
	fmt.Printf("unknown family:     %d\n", b.st.unknown)
	fmt.Printf("skipped:            %d\n", b.st.skipped)
	fmt.Printf("report:             %s\n", rp)
}

// run walks the corpus in the same order as scripts/import_pointlists.sh:
// the boss/poss families of every region first, then the fake/v7 gap-fillers,
// then the z2 rollup. The order is what makes "first target wins" resolve a
// collision the same way the importer resolved it — the fake/v7 flavour of a
// date must never displace the boss list that ClickHouse holds.
func (b *builder) run() error {
	regions, err := os.ReadDir(b.corpus)
	if err != nil {
		return err
	}
	var regionDirs []string
	for _, e := range regions {
		if e.IsDir() {
			regionDirs = append(regionDirs, e.Name())
		}
	}
	sort.Strings(regionDirs)

	for _, pass := range []string{"main", "fakev7", "z2"} {
		for _, region := range regionDirs {
			isZ2 := strings.EqualFold(region, "z2")
			switch pass {
			case "main", "fakev7":
				if isZ2 {
					continue
				}
			case "z2":
				if !isZ2 {
					continue
				}
			}
			wantFakeV7 := pass == "fakev7"
			if err := b.processRegion(filepath.Join(b.corpus, region), wantFakeV7); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *builder) processRegion(regionDir string, wantFakeV7 bool) error {
	var files []string
	err := filepath.WalkDir(regionDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		// The diff and fidouser series are never full issues.
		if strings.Contains(lower, "/diff/") || strings.Contains(lower, "/fidouser/") {
			return nil
		}
		inFakeV7 := strings.Contains(lower, "/fake/") || strings.Contains(lower, "/v7/")
		if inFakeV7 != wantFakeV7 {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		if err := b.processFile(f); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) processFile(path string) error {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if lower == "files.bbs" {
		return nil
	}
	ext := filepath.Ext(lower)

	switch {
	case zipExtRe.MatchString(ext):
		return b.processZip(path)
	case lhaExtRe.MatchString(ext):
		b.logf("SKIP unsupported archive (lha/lzh): %s", b.rel(path))
		b.st.skipped++
		return nil
	case dayExtRe.MatchString(strings.TrimPrefix(ext, ".")), ext == ".pvt", ext == ".lst":
		// Already a plain list file. .pvt/.lst carry no day in the name; the
		// day comes from the header, exactly as the importer resolves it.
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		b.st.archives++
		b.st.bytesRead += info.Size()
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return b.publish(path, "", base, data)
	default:
		b.logf("SKIP unrecognized name: %s", b.rel(path))
		b.st.skipped++
		return nil
	}
}

// processZip extracts one archive with the unzip CLI rather than archive/zip.
// 535 members of this corpus are stored with PKZIP 1.x "Imploding", which Go's
// archive/zip cannot decompress at all (it supports only Store and Deflate) —
// it opens the archive, lists the member, then fails on Open(). Shelling out
// also keeps parity with scripts/import_pointlists.sh, so the published set
// matches what ClickHouse holds. The 7 archives unzip itself refuses are
// truncated multi-part volumes ("cannot find zipfile directory"); the importer
// quarantined those too.
func (b *builder) processZip(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "pointlistarchive-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("unzip", "-qq", "-o", "-j", path, "-d", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		b.logf("SKIP unextractable zip: %s (%v: %s)", b.rel(path), err,
			strings.TrimSpace(strings.ReplaceAll(string(out), "\n", " ")))
		b.st.skipped++
		return nil
	}

	b.st.archives++
	b.st.bytesRead += info.Size()

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	var members []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if name == "files.bbs" || name == "file_id.diz" || strings.HasSuffix(name, ".diz") {
			continue
		}
		members = append(members, e.Name())
	}
	// Deterministic member order so a rebuild resolves ties identically.
	sort.Strings(members)

	for _, member := range members {
		data, err := os.ReadFile(filepath.Join(tmp, member))
		if err != nil {
			b.logf("SKIP unreadable member %s in %s (%v)", member, b.rel(path), err)
			b.st.skipped++
			continue
		}
		if err := b.publish(path, member, member, data); err != nil {
			return err
		}
	}
	return nil
}

// publish resolves one list file's series, year and day, then writes it to the
// download tree. archivePath is the corpus file it came from (used for the
// year and for collision reports); member is "" when the file was already plain.
func (b *builder) publish(archivePath, member, name string, data []byte) error {
	// list_source comes from the FILENAME family, never the directory: the
	// corpus contains strays such as R50PNT.293 inside R56/BOSS/, and
	// DK-POINT.PVT (an r23 series) inside an r20 archive. Filing those by
	// directory would publish them under a series they do not belong to.
	source, _, ok := parser.DerivePointlistSource(name)
	if !ok {
		b.logf("SKIP unknown family or diff: %s (from %s)", name, b.rel(archivePath))
		b.st.unknown++
		return nil
	}

	day, ok := resolveDay(name, data)
	if !ok {
		b.logf("SKIP no day number: %s (from %s)", name, b.rel(archivePath))
		b.st.skipped++
		return nil
	}

	year, ok := resolveYear(archivePath, data)
	if !ok {
		b.logf("SKIP no year: %s (from %s)", name, b.rel(archivePath))
		b.st.skipped++
		return nil
	}
	// A day that overflows its year means the year is wrong: year-end issues
	// get archived under the NEXT year's directory (P28-LIST.366 of leap 2004
	// sits in r28/2005/). Mirrors dateFromYearDay in internal/parser.
	if day > daysInYear(year) && day <= daysInYear(year-1) {
		b.logf("YEAR FIX %s: day %03d does not exist in %d, filing under %d", name, day, year, year-1)
		year--
	}
	if day > daysInYear(year) {
		b.logf("SKIP impossible day: %s day %03d in %d (from %s)", name, day, year, b.rel(archivePath))
		b.st.skipped++
		return nil
	}

	// The page keys off a 3-digit day extension, so .PVT/.LST have to be
	// renamed or they are invisible in every listing despite being on disk.
	stem := strings.ToLower(name)
	if i := strings.LastIndex(stem, "."); i > 0 {
		stem = stem[:i]
	}
	target := filepath.Join(b.network, source, strconv.Itoa(year), fmt.Sprintf("%s.%03d.gz", stem, day))

	if prev, exists := b.written[target]; exists {
		b.logf("COLLISION %s: kept %s%s, dropped %s%s", target,
			b.rel(prev.source), memberSuffix(prev.member), b.rel(archivePath), memberSuffix(member))
		b.st.collided++
		return nil
	}
	b.written[target] = origin{source: archivePath, member: member}

	if b.dryRun {
		b.st.written++
		return nil
	}

	full := filepath.Join(b.out, target)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := zw.Write(data); err != nil {
		zw.Close()
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if info, err := os.Stat(full); err == nil {
		b.st.bytesOut += info.Size()
	}
	b.st.written++
	return nil
}

// resolveDay prefers the filename's 3-digit extension and falls back to the
// header's "Day number", which is the only source for .PVT/.LST issues.
func resolveDay(name string, data []byte) (int, bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if dayExtRe.MatchString(ext) {
		if d, err := strconv.Atoi(ext); err == nil && d >= 1 && d <= 366 {
			return d, true
		}
	}
	for _, line := range headerLines(data) {
		if m := headerDayRe.FindStringSubmatch(line); m != nil {
			if d, err := strconv.Atoi(m[1]); err == nil && d >= 1 && d <= 366 {
				return d, true
			}
		}
	}
	return 0, false
}

// resolveYear takes the year from the corpus path, falling back to the header
// date for the handful of files that sit at a region root with no year dir.
func resolveYear(archivePath string, data []byte) (int, bool) {
	if m := yearPathRe.FindString(archivePath); m != "" {
		y, err := strconv.Atoi(m)
		return y, err == nil
	}
	for _, line := range headerLines(data) {
		if m := headerYearRe.FindString(line); m != "" {
			y, err := strconv.Atoi(m)
			return y, err == nil
		}
	}
	return 0, false
}

// headerLines returns the leading comment block, where the date and day live.
func headerLines(data []byte) []string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() && len(lines) < 40 {
		lines = append(lines, sc.Text())
	}
	return lines
}

func daysInYear(year int) int {
	return time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).YearDay()
}

func memberSuffix(member string) string {
	if member == "" {
		return ""
	}
	return "!" + member
}

func (b *builder) rel(path string) string {
	if r, err := filepath.Rel(b.corpus, path); err == nil {
		return r
	}
	return path
}

func (b *builder) logf(format string, args ...interface{}) {
	fmt.Fprintf(b.report, format+"\n", args...)
}
