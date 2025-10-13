package parser

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// extractDateFromLine extracts the nodelist date from a header comment line.
// It handles multiple date formats used in FidoNet nodelists across different eras.
func (p *Parser) extractDateFromLine(line string) (time.Time, int, error) {
	// Try various date patterns in header comments
	patterns := []struct {
		regex   *regexp.Regexp
		handler func([]string) (time.Time, int, error)
	}{
		// Modern format with 4-digit year: "Friday, 1 July 2022 -- Day number 182"
		{
			regexp.MustCompile(`(\w+),?\s+(\d{1,2})\s+(\w+)\s+(\d{4})\s+--\s+Day\s+number\s+(\d+)`),
			func(matches []string) (time.Time, int, error) {
				year, _ := strconv.Atoi(matches[4])
				day, _ := strconv.Atoi(matches[2])
				dayNum, _ := strconv.Atoi(matches[5])
				month := p.parseMonth(matches[3])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[3])
				}
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
		// Format with year in comment: "Day number 002 : Friday, January 2, 1998"
		{
			regexp.MustCompile(`Day\s+number\s+(\d+)\s*:\s*\w+,?\s+(\w+)\s+(\d{1,2}),?\s+(\d{4})`),
			func(matches []string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				day, _ := strconv.Atoi(matches[3])
				year, _ := strconv.Atoi(matches[4])
				month := p.parseMonth(matches[2])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[2])
				}
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
		// Old format without year: "Friday, 4 August 1989 -- Day number 216"
		{
			regexp.MustCompile(`(\w+),?\s+(\d{1,2})\s+(\w+)\s+--\s+Day\s+number\s+(\d+)`),
			func(matches []string) (time.Time, int, error) {
				day, _ := strconv.Atoi(matches[2])
				dayNum, _ := strconv.Atoi(matches[4])
				month := p.parseMonth(matches[3])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[3])
				}

				// Extract year from comment or filename context
				year := 1989 // Default for old nodelists
				if yearMatch := regexp.MustCompile(`(\d{4})`).FindStringSubmatch(line); len(yearMatch) > 1 {
					if y, err := strconv.Atoi(yearMatch[1]); err == nil && y > 1980 && y < 2100 {
						year = y
					}
				}

				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(line); len(matches) > 0 {
			return pattern.handler(matches)
		}
	}

	// Fallback: look for day number only
	if matches := regexp.MustCompile(`[Dd]ay\s+(?:number\s+)?(\d+)`).FindStringSubmatch(line); len(matches) > 1 {
		dayNum, _ := strconv.Atoi(matches[1])

		// Try to find year in the line
		year := 1989 // Default
		if yearMatch := regexp.MustCompile(`(\d{4})`).FindStringSubmatch(line); len(yearMatch) > 1 {
			if y, err := strconv.Atoi(yearMatch[1]); err == nil && y > 1980 && y < 2100 {
				year = y
			}
		}

		// Calculate date from day number
		date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
		return date, dayNum, nil
	}

	return time.Time{}, 0, fmt.Errorf("no date pattern found in line")
}

// extractDateFromFile extracts the nodelist date from the filename or file header.
// It tries multiple filename patterns and falls back to reading the file header.
func (p *Parser) extractDateFromFile(filePath string) (time.Time, int, error) {
	filename := filepath.Base(filePath)

	// Try various filename patterns
	patterns := []struct {
		regex   *regexp.Regexp
		handler func([]string, string) (time.Time, int, error)
	}{
		// NODELIST.nnn format (where nnn is day of year)
		{
			regexp.MustCompile(`(?i)nodelist\.(\d{3})`),
			func(matches []string, path string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				year := p.extractYearFromPath(path)
				if year == 0 {
					year = 1989 // Default for old nodelists
				}
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
		// z1-nnn.yy format (zone-day.year)
		{
			regexp.MustCompile(`(?i)z\d+-(\d{3})\.(\d{2})`),
			func(matches []string, path string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				year, _ := strconv.Atoi(matches[2])
				if year < 50 {
					year += 2000
				} else {
					year += 1900
				}
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
		// nodelist_yyyy_ddd format
		{
			regexp.MustCompile(`(?i)nodelist[_-](\d{4})[_-](\d{3})`),
			func(matches []string, path string) (time.Time, int, error) {
				year, _ := strconv.Atoi(matches[1])
				dayNum, _ := strconv.Atoi(matches[2])
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(filename); len(matches) > 0 {
			return pattern.handler(matches, filePath)
		}
	}

	// Fallback: read file header
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer file.Close()

	// Create reader that handles both regular and gzipped files
	var reader io.Reader = file
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("failed to create gzip reader for %s: %w", filePath, err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	scanner := bufio.NewScanner(reader)
	lineCount := 0
	for scanner.Scan() && lineCount < 20 {
		lineCount++
		line := scanner.Text()
		if strings.HasPrefix(line, ";A") || strings.HasPrefix(line, ";S") {
			if date, dayNum, err := p.extractDateFromLine(line); err == nil {
				return date, dayNum, nil
			}
		}
	}

	return time.Time{}, 0, fmt.Errorf("no date found in filename or header")
}

// extractYearFromPath extracts a 4-digit year from the file path.
// It looks for patterns like "1989", "2024", etc. in the path.
func (p *Parser) extractYearFromPath(filePath string) int {
	// Look for 4-digit year in path
	yearRe := regexp.MustCompile(`\b(19[8-9]\d|20[0-5]\d)\b`)
	if matches := yearRe.FindStringSubmatch(filePath); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		return year
	}

	// Look for 2-digit year in path
	year2Re := regexp.MustCompile(`\b([89]\d|[0-5]\d)\b`)
	if matches := year2Re.FindStringSubmatch(filepath.Base(filePath)); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		if year < 50 {
			return 2000 + year
		}
		return 1900 + year
	}

	return 0
}

// parseMonth converts a month name (full or abbreviated) to its numeric value (1-12).
func (p *Parser) parseMonth(monthStr string) int {
	monthStr = strings.ToLower(monthStr)
	months := map[string]int{
		"january": 1, "jan": 1,
		"february": 2, "feb": 2,
		"march": 3, "mar": 3,
		"april": 4, "apr": 4,
		"may":  5,
		"june": 6, "jun": 6,
		"july": 7, "jul": 7,
		"august": 8, "aug": 8,
		"september": 9, "sep": 9,
		"october": 10, "oct": 10,
		"november": 11, "nov": 11,
		"december": 12, "dec": 12,
	}
	return months[monthStr]
}
