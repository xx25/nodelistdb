// fsxnetarchive builds a publishable nodelist download tree from a git-hosted
// nodelist repository — fsxNet's (github.com/fsxnet/nodelist) being the one
// this exists for.
//
// The data was imported long ago; the files never were. NodelistDB held 408
// fsxNet nodelist dates and published exactly one file, because the only thing
// that writes into the archive is sync_nodelists.sh, and its EXTRA_NETWORKS
// list was never turned on. Backfilling through that path is not possible even
// with it on: it skips any date already in the database before it reaches the
// archive write, which is every date here.
//
// A checkout alone will not do either. The repository keeps only the current
// week at its root and rotates older ones into old/<year>/ as NAME.Z## ZIP
// archives, and it stopped rotating after 2024 — every 2025-and-later weekly
// exists solely as a blob in git history, renamed away one commit later. So
// this tool reads three places: the working tree, old/<year>/, and every blob
// any commit ever placed at a nodelist-shaped path.
//
// Dates come from the nodelist header, which states both:
//
//	;A fsxNet Nodelist for Friday, October 17, 2025 -- Day number 290 : 46769
//
// That is the only source that needs no guessing. A ZIP's NAME.Z## name holds
// the day only modulo 100, and a commit's timestamp merely sits near the
// publication date. Both are used as fallbacks, and every fallback is logged.
//
// Output matches what internal/nodelistfs scans and what sync_nodelists.sh
// writes for any other network: <out>/<network>/<year>/<network>.DDD.gz.
//
// Usage:
//
//	fsxnetarchive -repo <clone> -out <nodelist root> [-network fsxnet] [-dry-run]
package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/nodelistfs"
)

var (
	// nodelistNameRe matches both shapes a weekly is stored under: the plain
	// NAME.DDD nodelist and the NAME.Z## ZIP holding it.
	nodelistNameRe = regexp.MustCompile(`(?i)^([a-z0-9_-]+)\.(?:([0-9]{3})|z([0-9]{2}))$`)
	// headerDayRe reads the day number a nodelist header states.
	headerDayRe = regexp.MustCompile(`(?i)day\s*number\s*[: ]?\s*([0-9]{1,3})`)
	// headerYearRe reads the year from the date the same header states.
	headerYearRe = regexp.MustCompile(`\b(19[89][0-9]|20[0-9]{2})\b`)
	// yearDirRe matches an old/<year>/ directory component.
	yearDirRe = regexp.MustCompile(`^(19[89][0-9]|20[0-9]{2})$`)
)

// candidate is one file that might hold a weekly, before its contents are read.
type candidate struct {
	path   string    // path within the repository
	commit string    // commit to read it from ("" = the working tree)
	when   time.Time // commit time, for the year fallback
	dirs   string    // directory component, for the old/<year>/ year fallback
	tier   int       // tierCurated or tierHistory; lower wins
}

// Precedence when the same weekly turns up more than once. A checkout's
// old/<year>/NAME.Z## is the copy the network itself archived and is taken as
// final. Everything else is a blob some commit happened to carry, where the
// newest revision of a given week wins instead: a second commit touching a week
// already published is a correction to it.
const (
	tierCurated = iota
	tierHistory
)

// resolved is a weekly that has been read, dated and chosen over any rival.
type resolved struct {
	year, day int
	content   []byte
	from      candidate
}

// origin describes where a published file came from, so a collision names both
// sides.
func (c candidate) origin() string {
	if c.commit == "" {
		return c.path + " (working tree)"
	}
	return fmt.Sprintf("%s (%s)", c.path, c.commit[:min(8, len(c.commit))])
}

type stats struct {
	candidates  int
	written     int
	duplicates  int
	conflicts   int
	unreadable  int
	undated     int
	headerDated int
	guessed     int
	bytesOut    int64
}

type builder struct {
	repo    string
	out     string
	network string
	dryRun  bool
	report  *os.File

	resolved map[string]*resolved // "<year>/<day>" -> the weekly to publish
	st       stats
}

func main() {
	repo := flag.String("repo", "", "Path to a clone of the nodelist git repository")
	out := flag.String("out", "", "Nodelist archive root; files are written to <out>/<network>/<year>/")
	network := flag.String("network", "fsxnet", "FTN network the repository publishes")
	reportPath := flag.String("report", "", "Write the skip/collision report here (default ./<network>-archive-report.txt)")
	noHistory := flag.Bool("no-history", false, "Read only the working tree, not git history")
	dryRun := flag.Bool("dry-run", false, "Resolve and report without writing any file")
	flag.Parse()

	if *repo == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "both -repo and -out are required")
		flag.Usage()
		os.Exit(2)
	}
	if !nodelistfs.ValidNetworkName(*network) {
		fmt.Fprintf(os.Stderr, "invalid network name %q\n", *network)
		os.Exit(2)
	}

	b := &builder{
		repo:     *repo,
		out:      *out,
		network:  strings.ToLower(*network),
		dryRun:   *dryRun,
		resolved: make(map[string]*resolved),
	}

	if err := b.openReport(*reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "cannot open report: %v\n", err)
		os.Exit(1)
	}
	defer b.report.Close()

	candidates, err := b.collect(!*noHistory)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collecting candidates: %v\n", err)
		os.Exit(1)
	}
	b.st.candidates = len(candidates)

	// Resolve every candidate before writing anything: the same weekly appears
	// on average three times across the tree and history, and only the winner
	// should ever reach the archive.
	for _, c := range candidates {
		b.resolve(c)
	}
	b.writeAll()

	b.summarize()
}

func (b *builder) openReport(reportPath string) error {
	if reportPath == "" {
		if b.dryRun {
			b.report = os.Stderr
			return nil
		}
		// Deliberately not inside -out: that tree gets published verbatim over
		// HTTP and FTP, and a build report is not a nodelist. Writing it there
		// puts it in the archive's own directory listing.
		reportPath = b.network + "-archive-report.txt"
	}
	f, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	b.report = f
	return nil
}

func (b *builder) logf(format string, args ...interface{}) {
	fmt.Fprintf(b.report, format+"\n", args...)
}

// collect finds every nodelist-shaped path in the working tree and, unless
// asked not to, every one that any commit ever added or modified.
//
// Ordering decides which copy of a duplicated weekly wins: the working tree and
// old/<year>/ first, because their year is stated by a directory rather than
// inferred, then history from the oldest commit forward, so the blob closest to
// publication wins over any later re-add.
func (b *builder) collect(withHistory bool) ([]candidate, error) {
	var candidates []candidate

	err := filepath.WalkDir(b.repo, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(b.repo, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !b.isNodelistPath(rel) {
			return nil
		}
		candidates = append(candidates, candidate{path: rel, dirs: path.Dir(rel), tier: tierCurated})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })

	if !withHistory {
		return candidates, nil
	}

	historical, err := b.collectHistory()
	if err != nil {
		return nil, err
	}
	return append(candidates, historical...), nil
}

// isNodelistPath reports whether a repository path looks like one of this
// network's weeklies. The name must match the network so a repository holding
// several lists cannot cross-contaminate.
func (b *builder) isNodelistPath(rel string) bool {
	m := nodelistNameRe.FindStringSubmatch(path.Base(rel))
	return m != nil && strings.EqualFold(m[1], b.network)
}

// collectHistory lists every commit that added or modified a nodelist-shaped
// path, oldest first. Renames are read as adds (--diff-filter=AM with rename
// detection off), which is what recovers the weeklies this repository rotates
// out of its root: each is renamed to the next week's name one commit later and
// so exists nowhere in any checkout.
func (b *builder) collectHistory() ([]candidate, error) {
	cmd := exec.Command("git", "-C", b.repo, "log", "--all", "--reverse",
		"--no-renames", "--diff-filter=AM", "--name-only",
		"--pretty=format:%x00%H %ct")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var candidates []candidate
	var commit string
	var when time.Time

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "\x00") {
			fields := strings.Fields(strings.TrimPrefix(line, "\x00"))
			if len(fields) != 2 {
				continue
			}
			commit = fields[0]
			secs, convErr := strconv.ParseInt(fields[1], 10, 64)
			if convErr != nil {
				continue
			}
			when = time.Unix(secs, 0).UTC()
			continue
		}
		if commit == "" || !b.isNodelistPath(line) {
			continue
		}
		candidates = append(candidates, candidate{
			path:   line,
			commit: commit,
			when:   when,
			dirs:   path.Dir(line),
			tier:   tierHistory,
		})
	}

	return candidates, nil
}

// resolve reads one candidate, dates it, and keeps it if it beats whatever
// already holds its slot.
func (b *builder) resolve(c candidate) {
	raw, err := b.read(c)
	if err != nil {
		b.st.unreadable++
		b.logf("UNREADABLE: %s: %v", c.origin(), err)
		return
	}

	content, err := b.unwrap(c, raw)
	if err != nil {
		b.st.unreadable++
		b.logf("UNREADABLE: %s: %v", c.origin(), err)
		return
	}

	year, day, exact := b.date(c, content)
	if year == 0 {
		b.st.undated++
		b.logf("UNDATED: %s: no header date, no usable fallback", c.origin())
		return
	}
	if exact {
		b.st.headerDated++
	} else {
		b.st.guessed++
		b.logf("GUESSED: %s -> %d day %03d (header unreadable)", c.origin(), year, day)
	}

	key := fmt.Sprintf("%04d/%03d", year, day)
	prev, seen := b.resolved[key]
	if !seen {
		b.resolved[key] = &resolved{year: year, day: day, content: content, from: c}
		return
	}

	b.st.duplicates++
	if bytes.Equal(prev.content, content) {
		return
	}

	// Candidates arrive curated-first, then history oldest-first, so a rival at
	// the same tier is always the newer revision and supersedes.
	b.st.conflicts++
	if c.tier < prev.from.tier || (c.tier == prev.from.tier && c.tier == tierHistory) {
		b.logf("SUPERSEDED: %s: %s replaces %s", key, c.origin(), prev.from.origin())
		b.resolved[key] = &resolved{year: year, day: day, content: content, from: c}
		return
	}
	b.logf("CONFLICT: %s: keeping %s over %s", key, prev.from.origin(), c.origin())
}

// writeAll publishes every resolved weekly.
func (b *builder) writeAll() {
	keys := make([]string, 0, len(b.resolved))
	for key := range b.resolved {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		r := b.resolved[key]
		if err := b.publish(r.year, r.day, r.content); err != nil {
			b.st.unreadable++
			b.logf("WRITE FAILED: %s -> %s: %v", r.from.origin(), key, err)
			continue
		}
		b.st.written++
	}
}

// read returns a candidate's bytes, from the working tree or from git.
func (b *builder) read(c candidate) ([]byte, error) {
	if c.commit == "" {
		return os.ReadFile(filepath.Join(b.repo, filepath.FromSlash(c.path)))
	}
	out, err := exec.Command("git", "-C", b.repo, "show", c.commit+":"+c.path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show: %w", err)
	}
	return out, nil
}

// unwrap returns the nodelist itself, extracting it when the candidate is one
// of the NAME.Z## ZIP archives.
func (b *builder) unwrap(c candidate, raw []byte) ([]byte, error) {
	m := nodelistNameRe.FindStringSubmatch(path.Base(c.path))
	if m == nil || m[3] == "" {
		return raw, nil // plain NAME.DDD
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("not a readable ZIP: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !b.isNodelistPath(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", f.Name, err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("no %s nodelist member", b.network)
}

// date returns the year and day-of-year a nodelist belongs to, and whether that
// came from its own header rather than a fallback.
//
// The header states both and is trusted first. Failing that: the day comes from
// a plain NAME.DDD filename (a NAME.Z## name holds it only modulo 100, which is
// why the ZIP is unwrapped before this runs), and the year from an old/<year>/
// directory or, last, the commit that carried the file — accepting that a
// weekly published near New Year is committed in the following one.
func (b *builder) date(c candidate, content []byte) (year, day int, exact bool) {
	if y, d, ok := headerDate(content); ok {
		return y, d, true
	}

	if m := nodelistNameRe.FindStringSubmatch(path.Base(c.path)); m != nil && m[2] != "" {
		day, _ = strconv.Atoi(m[2])
	}
	if day < 1 || day > 366 {
		return 0, 0, false
	}

	for _, part := range strings.Split(c.dirs, "/") {
		if yearDirRe.MatchString(part) {
			year, _ = strconv.Atoi(part)
		}
	}
	if year == 0 && !c.when.IsZero() {
		year = c.when.Year()
		// A list from the tail of one year is committed at the start of the
		// next; the day number gives it away.
		if day > c.when.YearDay()+7 {
			year--
		}
	}
	if year == 0 {
		return 0, 0, false
	}
	return year, day, false
}

// headerDate reads the year and day a nodelist header states.
func headerDate(content []byte) (year, day int, ok bool) {
	line := content
	if idx := bytes.IndexAny(line, "\r\n"); idx >= 0 {
		line = line[:idx]
	}
	// The header reads "... for <date> -- Day number NNN : <crc>"; splitting on
	// the dashes keeps the CRC away from the year match.
	head, tail, found := strings.Cut(string(line), "--")
	if !found {
		return 0, 0, false
	}

	dm := headerDayRe.FindStringSubmatch(tail)
	ym := headerYearRe.FindStringSubmatch(head)
	if dm == nil || ym == nil {
		return 0, 0, false
	}
	day, _ = strconv.Atoi(dm[1])
	year, _ = strconv.Atoi(ym[1])
	if day < 1 || day > 366 {
		return 0, 0, false
	}
	return year, day, true
}

// publish writes one weekly as <out>/<network>/<year>/<network>.DDD.gz.
func (b *builder) publish(year, day int, content []byte) error {
	name := fmt.Sprintf("%s.%03d.gz", b.network, day)
	dir := filepath.Join(b.out, b.network, strconv.Itoa(year))
	target := filepath.Join(dir, name)

	if b.dryRun {
		b.st.bytesOut += int64(len(content))
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Write to a temporary and rename, so an interrupted run never leaves a
	// truncated nodelist where the download routes will happily serve it.
	tmp, err := os.CreateTemp(dir, ".fsxnetarchive-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	zw := gzip.NewWriter(tmp)
	if _, err := zw.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return err
	}

	b.st.bytesOut += info.Size()
	return nil
}

func (b *builder) summarize() {
	years := make(map[int]int)
	for _, r := range b.resolved {
		years[r.year]++
	}
	ordered := make([]int, 0, len(years))
	for y := range years {
		ordered = append(ordered, y)
	}
	sort.Ints(ordered)

	b.logf("")
	b.logf("=== %s archive ===", b.network)
	for _, y := range ordered {
		b.logf("  %d: %d files", y, years[y])
	}

	summary := fmt.Sprintf(
		"candidates=%d written=%d years=%d duplicates=%d conflicts=%d unreadable=%d undated=%d header-dated=%d guessed=%d bytes=%d",
		b.st.candidates, b.st.written, len(years), b.st.duplicates, b.st.conflicts,
		b.st.unreadable, b.st.undated, b.st.headerDated, b.st.guessed, b.st.bytesOut)

	b.logf("%s", summary)
	fmt.Println(summary)
	if b.report != os.Stderr {
		fmt.Printf("report: %s\n", b.report.Name())
	}
}
