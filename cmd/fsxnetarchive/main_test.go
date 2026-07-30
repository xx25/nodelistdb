package main

import "testing"

// TestHeaderDate covers the function that decides where every reconstructed
// weekly lands. It is load-bearing: the repository's NAME.Z## names carry the
// day only modulo 100 and its old/<year>/ directories are not reliable — the
// corpus really does hold old/2024/FSXNET.Z45 wrapping a day-045 list from
// 2025, and old/2024/FSXNET.Z71 wrapping day 171. Only the header disambiguates.
func TestHeaderDate(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		year, day int
		ok        bool
	}{
		{
			name:    "fsxnet weekly",
			content: ";A fsxNet Nodelist for Friday, October 17, 2025 -- Day number 290 : 46769\n;A\n",
			year:    2025, day: 290, ok: true,
		},
		{
			name:    "single-digit day of month",
			content: ";A fsxNet Nodelist for Friday, April 5, 2024 -- Day number 096 : 09653\n",
			year:    2024, day: 96, ok: true,
		},
		{
			name:    "misfiled under the wrong year directory",
			content: ";A fsxNet Nodelist for Friday, June 20, 2025 -- Day number 171 : 12345\n",
			year:    2025, day: 171, ok: true,
		},
		{
			name:    "day 1",
			content: ";A fsxNet Nodelist for Friday, January 1, 2021 -- Day number 001 : 1\n",
			year:    2021, day: 1, ok: true,
		},
		{
			name:    "CRLF line ending",
			content: ";A fsxNet Nodelist for Friday, July 31, 2026 -- Day number 212 : 51755\r\n;S\r\n",
			year:    2026, day: 212, ok: true,
		},
		{
			// The trailing CRC is a bare number and must never be read as a year.
			name:    "five-digit CRC is not a year",
			content: ";A fsxNet Nodelist for Friday, March 26, 2021 -- Day number 085 : 20211\n",
			year:    2021, day: 85, ok: true,
		},
		{
			name:    "no day number",
			content: ";A fsxNet Nodelist for Friday, October 17, 2025 -- something else\n",
			ok:      false,
		},
		{
			name:    "no dashes to split on",
			content: ";A fsxNet Nodelist Day number 290\n",
			ok:      false,
		},
		{
			name:    "day out of range",
			content: ";A fsxNet Nodelist for Friday, October 17, 2025 -- Day number 400 : 1\n",
			ok:      false,
		},
		{
			name:    "empty",
			content: "",
			ok:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			year, day, ok := headerDate([]byte(tc.content))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if year != tc.year || day != tc.day {
				t.Errorf("= %d day %d, want %d day %d", year, day, tc.year, tc.day)
			}
		})
	}
}

// TestIsNodelistPath verifies a repository holding more than one network's
// lists cannot cross-contaminate the archive, and that both storage shapes are
// recognised.
func TestIsNodelistPath(t *testing.T) {
	b := &builder{network: "fsxnet"}

	for _, tc := range []struct {
		path string
		want bool
	}{
		{"FSXNET.290", true},
		{"FSXNET.Z90", true},
		{"old/2024/FSXNET.Z96", true},
		{"fsxnet.096", true},
		{"NODELIST.290", false},
		{"old/2024/NODELIST.Z96", false},
		{"README.md", false},
		{"FSXNET.txt", false},
		{"FSXNET.2900", false},
		{"FSXNET", false},
	} {
		if got := b.isNodelistPath(tc.path); got != tc.want {
			t.Errorf("isNodelistPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
