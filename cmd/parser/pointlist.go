package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/parser"
	"github.com/nodelistdb/internal/storage"
)

// pointlistOptions carries the pointlist-mode CLI flags
type pointlistOptions struct {
	Domain         string
	ListSource     string // empty = derive from filename family
	SourcePriority int    // -1 = derive from filename family (default regional)
	Format         string
	Year           int
	DefaultZone    int
	Charset        string
	Reimport       bool
	Force          bool
	ShrinkCheck    string // "fail" (refuse <50% shrink) or "warn" (import anyway)
	Recursive      bool
	Verbose        bool
	Quiet          bool
}

// runPointlistImport imports one file or a directory of pointlist files.
// Returns the number of files that failed.
func runPointlistImport(storageLayer *storage.Storage, path string, opts pointlistOptions) int {
	files, err := findPointlistFiles(path, opts.Recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		if !opts.Quiet {
			fmt.Printf("No pointlist files found in: %s\n", path)
		}
		return 0
	}

	if !opts.Quiet {
		fmt.Printf("Found %d pointlist file(s) to process\n\n", len(files))
	}

	imported, skipped, quarantined, failed := 0, 0, 0, 0
	totalPoints := 0
	startTime := time.Now()

	for i, filePath := range files {
		if !opts.Quiet {
			fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(files), filePath)
		}
		points, err := importPointlistFile(storageLayer, filePath, opts)
		switch {
		case err == errPointlistSkipped:
			skipped++
		case err == errPointlistQuarantined:
			quarantined++
		case err != nil:
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			failed++
		default:
			imported++
			totalPoints += points
		}
	}

	if !opts.Quiet {
		fmt.Printf("\nPointlist import completed in %v\n", time.Since(startTime).Round(time.Millisecond))
		fmt.Printf("Files imported: %d, skipped: %d, quarantined: %d, failed: %d\n", imported, skipped, quarantined, failed)
		fmt.Printf("Total points imported: %d\n", totalPoints)
	}

	return failed
}

// errPointlistSkipped marks a file intentionally not imported (already gated).
var errPointlistSkipped = fmt.Errorf("pointlist file skipped")

// errPointlistQuarantined marks a file held back because its series cannot be
// derived from the filename — it needs an explicit -list-source decision.
var errPointlistQuarantined = fmt.Errorf("pointlist file quarantined")

// importPointlistFile runs the per-file import procedure:
// resolve source → parse → sanity → gate check → clean remnants → insert →
// register gate row.
func importPointlistFile(storageLayer *storage.Storage, filePath string, opts pointlistOptions) (int, error) {
	// Resolve list_source + priority: explicit flag wins, else filename family
	listSource := opts.ListSource
	priority := opts.SourcePriority
	derivedSource, derivedPriority, derived := parser.DerivePointlistSource(filePath)
	if listSource == "" {
		if !derived {
			// Greppable marker: the bulk import script collects these lines
			// into its quarantine report.
			fmt.Printf("  QUARANTINED: %s — cannot derive list source from filename (diff file or unknown series); pass -list-source explicitly\n",
				filePath)
			return 0, errPointlistQuarantined
		}
		listSource = derivedSource
	}
	if priority < 0 {
		if derived {
			priority = int(derivedPriority)
		} else {
			priority = int(parser.SourcePriorityRegional)
		}
	}

	// Parse
	pp := parser.NewPointlistParser(opts.Verbose)
	pp.SetDomain(opts.Domain)
	pp.ListSource = listSource
	pp.SourcePriority = uint8(priority)
	pp.Format = opts.Format
	pp.Year = opts.Year
	pp.DefaultZone = opts.DefaultZone
	pp.Charset = opts.Charset

	result, err := pp.ParseFile(filePath)
	if err != nil {
		// Undetectable format needs a human -format decision, exactly like an
		// underivable filename family — quarantine (greppable marker), don't
		// fail the run. Real corpus cases: empty issues (context lines only)
		// and fakenets whose boss mapping exists only in header comments.
		var fe *parser.FileError
		if errors.As(err, &fe) && fe.Op == "detect" {
			fmt.Printf("  QUARANTINED: %s — %v; pass -format explicitly\n", filePath, err)
			return 0, errPointlistQuarantined
		}
		return 0, err
	}
	for _, w := range result.Warnings {
		if !opts.Quiet {
			fmt.Printf("  WARNING: %s\n", w)
		}
	}
	if result.PointlistDate.IsZero() {
		return 0, fmt.Errorf("cannot determine pointlist date for %s (pass -year with a 3-digit day filename)", filePath)
	}

	pointOps := storageLayer.PointOps()

	// Import gate (per domain + source + date)
	alreadyImported, err := pointOps.IsPointlistImported(opts.Domain, listSource, result.PointlistDate)
	if err != nil {
		return 0, fmt.Errorf("gate check failed: %w", err)
	}
	if alreadyImported && !opts.Reimport {
		if !opts.Quiet {
			fmt.Printf("  Already imported (%s/%s/%s), skipping (use -reimport to replace)\n",
				opts.Domain, listSource, result.PointlistDate.Format("2006-01-02"))
		}
		return 0, errPointlistSkipped
	}

	// Sanity thresholds (truncation guard)
	if len(result.Points) == 0 && !opts.Force {
		return 0, fmt.Errorf("file yields 0 points — refusing to import (use -force to override)")
	}
	nearCount, nearDate, found, err := pointOps.NearestPointsCount(opts.Domain, listSource, result.PointlistDate)
	if err != nil {
		return 0, fmt.Errorf("sanity check failed: %w", err)
	}
	if found && nearCount > 0 && len(result.Points)*2 < nearCount && !opts.Force {
		shrinkMsg := fmt.Sprintf("file yields %d points, less than 50%% of the nearest imported %s issue (%d on %s)",
			len(result.Points), listSource, nearCount, nearDate.Format("2006-01-02"))
		// Historical series legitimately collapse (mass purges of dead
		// entries: r20 went 71 → 8 points in one week of 2017), so bulk
		// imports run with -shrink-check=warn; the strict default protects
		// the weekly sync against truncated downloads.
		if opts.ShrinkCheck == "warn" {
			if !opts.Quiet {
				fmt.Printf("  WARNING: %s — importing anyway (-shrink-check=warn)\n", shrinkMsg)
			}
		} else {
			return 0, fmt.Errorf("%s — possible truncation (use -force or -shrink-check=warn to override)", shrinkMsg)
		}
	}

	// Replay / partial-remnant cleanup. A crash between insert and gate
	// registration leaves un-gated rows; deleting by (domain, source, date)
	// before inserting makes the retry idempotent.
	if err := pointOps.DeletePointlistData(opts.Domain, listSource, result.PointlistDate, opts.Reimport && alreadyImported); err != nil {
		return 0, fmt.Errorf("failed to clean existing rows: %w", err)
	}

	// Insert (chunked internally)
	if err := pointOps.InsertPoints(result.Points); err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}

	// Register the gate row LAST: the file only counts as imported now
	if err := pointOps.RegisterPointlistFile(database.PointlistFile{
		Domain:        opts.Domain,
		ListSource:    listSource,
		PointlistDate: result.PointlistDate,
		DayNumber:     result.DayNumber,
		Filename:      filepath.Base(filePath),
		SourceFormat:  result.SourceFormat,
		PointsCount:   uint32(len(result.Points)),
		BossesCount:   uint32(result.BossCount),
	}); err != nil {
		return 0, fmt.Errorf("failed to register import gate: %w", err)
	}

	if !opts.Quiet {
		fmt.Printf("  ✓ Imported %d points under %d bosses (%s, %s, format %s, day %03d)\n",
			len(result.Points), result.BossCount, listSource,
			result.PointlistDate.Format("2006-01-02"), result.SourceFormat, result.DayNumber)
		if result.SkippedLines > 0 {
			fmt.Printf("  Skipped %d non-conforming line(s)\n", result.SkippedLines)
		}
	}

	return len(result.Points), nil
}

// importNodelistPoints stores the inline "Point," lines of one parsed
// nodelist, gated by pointlist_files (domain, "nodelist", date). alwaysGate
// registers a gate row even for 0 points ("extracted, nothing found") so
// backfill re-runs can skip the file cheaply. The normal import path passes
// false: it registers a 0-point gate row only when the domain's 'nodelist'
// series already exists — a nodelist that DROPS its inline points must
// supersede the previous issue in snapshot queries (which pick the series'
// latest gate row), while domains that never had inline points stay free of
// daily gate-row litter. Returns the number of points actually inserted.
func importNodelistPoints(storageLayer *storage.Storage, domain string, result *parser.ParseResult, alwaysGate, quiet bool) (int, error) {
	pointOps := storageLayer.PointOps()

	if result.NodelistDate.IsZero() {
		if len(result.Points) == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("cannot gate inline points: nodelist date unknown")
	}
	if len(result.Points) == 0 && !alwaysGate {
		_, _, seriesExists, err := pointOps.NearestPointsCount(domain, parser.NodelistPointSource, result.NodelistDate)
		if err != nil {
			return 0, fmt.Errorf("gate check failed: %w", err)
		}
		if !seriesExists {
			return 0, nil
		}
	}
	imported, err := pointOps.IsPointlistImported(domain, parser.NodelistPointSource, result.NodelistDate)
	if err != nil {
		return 0, fmt.Errorf("gate check failed: %w", err)
	}
	if imported {
		return 0, nil
	}

	// Idempotent retry: clean un-gated remnants of a crashed extraction
	if err := pointOps.DeletePointlistData(domain, parser.NodelistPointSource, result.NodelistDate, false); err != nil {
		return 0, fmt.Errorf("failed to clean existing rows: %w", err)
	}
	if len(result.Points) > 0 {
		if err := pointOps.InsertPoints(result.Points); err != nil {
			return 0, fmt.Errorf("insert failed: %w", err)
		}
	}

	bosses := make(map[string]bool)
	for i := range result.Points {
		p := &result.Points[i]
		bosses[fmt.Sprintf("%d:%d/%d", p.Zone, p.Net, p.Node)] = true
	}

	if err := pointOps.RegisterPointlistFile(database.PointlistFile{
		Domain:        domain,
		ListSource:    parser.NodelistPointSource,
		PointlistDate: result.NodelistDate,
		DayNumber:     result.DayNumber,
		Filename:      filepath.Base(result.FilePath),
		SourceFormat:  parser.NodelistPointSource,
		PointsCount:   uint32(len(result.Points)),
		BossesCount:   uint32(len(bosses)),
	}); err != nil {
		return 0, fmt.Errorf("failed to register import gate: %w", err)
	}

	if !quiet && len(result.Points) > 0 {
		fmt.Printf("  ✓ Extracted %d inline point(s) under %d boss(es)\n", len(result.Points), len(bosses))
	}
	return len(result.Points), nil
}

// runExtractPoints re-reads archived nodelist files and imports only their
// inline "Point," lines (backfill for nodelists imported before inline point
// extraction existed). The nodelist gate is bypassed by design; idempotency
// comes from the pointlist_files gate keyed (domain, "nodelist", date).
// Returns the number of files that failed.
func runExtractPoints(storageLayer *storage.Storage, path string, domain string, pattern *regexp.Regexp, recursive, verbose, quiet bool) int {
	files, err := findNodelistFiles(path, recursive, pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		if !quiet {
			fmt.Printf("No nodelist files found in: %s\n", path)
		}
		return 0
	}

	if !quiet {
		fmt.Printf("Found %d nodelist file(s) to scan for inline points\n\n", len(files))
	}

	nodelistParser := parser.NewAdvanced(verbose)
	nodelistParser.SetDomain(domain)
	nodelistParser.CollectPoints = true

	pointOps := storageLayer.PointOps()
	processed, skipped, extracted, failed := 0, 0, 0, 0

	for i, filePath := range files {
		if !quiet {
			fmt.Printf("[%d/%d] Scanning: %s\n", i+1, len(files), filePath)
		}

		// Cheap pre-check: when the filename yields the date, an already-gated
		// file is skipped without parsing.
		if date, _, err := nodelistParser.ExtractDateFromFilename(filePath); err == nil && !date.IsZero() {
			if imported, err := pointOps.IsPointlistImported(domain, parser.NodelistPointSource, date); err == nil && imported {
				skipped++
				continue
			}
		}

		result, err := nodelistParser.ParseFileWithCRC(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			failed++
			continue
		}
		n, err := importNodelistPoints(storageLayer, domain, result, true, quiet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			failed++
			continue
		}
		processed++
		extracted += n
	}

	if !quiet {
		fmt.Printf("\nExtract-points completed: %d file(s) processed, %d skipped (already extracted), %d failed\n",
			processed, skipped, failed)
		fmt.Printf("Total inline points imported: %d\n", extracted)
	}
	return failed
}

// findPointlistFiles lists candidate files. Unlike nodelists there is no
// single filename regex gate (series names are too varied) — only obviously
// non-pointlist files (files.bbs) are excluded here; per-file source
// derivation quarantines the rest.
func findPointlistFiles(path string, recursive bool) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var files []string
	walkFunc := func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !recursive && filePath != path {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Base(filePath), "files.bbs") {
			return nil
		}
		files = append(files, filePath)
		return nil
	}

	if err := filepath.Walk(path, walkFunc); err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}
	return files, nil
}
