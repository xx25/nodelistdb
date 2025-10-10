package timeavail

import (
	"fmt"
	"time"
)

type TIMSEntry struct {
	Letter      rune
	UTC         bool
	StartHour   int
	StartMinute int
	EndHour     int
	EndMinute   int
	Days        []time.Weekday
}

var TIMSTable = map[rune]TIMSEntry{
	'A': {Letter: 'A', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 6, EndMinute: 0, Days: allDays()},
	'B': {Letter: 'B', UTC: false, StartHour: 6, StartMinute: 0, EndHour: 12, EndMinute: 0, Days: allDays()},
	'C': {Letter: 'C', UTC: false, StartHour: 12, StartMinute: 0, EndHour: 18, EndMinute: 0, Days: allDays()},
	'D': {Letter: 'D', UTC: false, StartHour: 18, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: allDays()},
	'E': {Letter: 'E', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 12, EndMinute: 0, Days: weekends()},
	'F': {Letter: 'F', UTC: false, StartHour: 12, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: weekends()},
	'G': {Letter: 'G', UTC: false, StartHour: 19, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: weekdays()},
	'H': {Letter: 'H', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: weekends()},
	'J': {Letter: 'J', UTC: false, StartHour: 22, StartMinute: 30, EndHour: 8, EndMinute: 30, Days: allDays()},
	'K': {Letter: 'K', UTC: false, StartHour: 20, StartMinute: 0, EndHour: 7, EndMinute: 0, Days: allDays()},
	'L': {Letter: 'L', UTC: false, StartHour: 21, StartMinute: 0, EndHour: 9, EndMinute: 0, Days: weekdays()},
	'M': {Letter: 'M', UTC: false, StartHour: 8, StartMinute: 0, EndHour: 20, EndMinute: 0, Days: weekdays()},
	'N': {Letter: 'N', UTC: false, StartHour: 20, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: allDays()},
	'O': {Letter: 'O', UTC: false, StartHour: 19, StartMinute: 0, EndHour: 7, EndMinute: 0, Days: allDays()},
	'P': {Letter: 'P', UTC: false, StartHour: 20, StartMinute: 0, EndHour: 10, EndMinute: 0, Days: weekends()},
	'Q': {Letter: 'Q', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: allDays()},
	'R': {Letter: 'R', UTC: false, StartHour: 20, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: weekdays()},
	'S': {Letter: 'S', UTC: false, StartHour: 8, StartMinute: 0, EndHour: 20, EndMinute: 0, Days: weekends()},
	'T': {Letter: 'T', UTC: false, StartHour: 18, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: allDays()},
	'U': {Letter: 'U', UTC: false, StartHour: 19, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: allDays()},
	'V': {Letter: 'V', UTC: false, StartHour: 1, StartMinute: 0, EndHour: 7, EndMinute: 0, Days: allDays()},
	'W': {Letter: 'W', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: weekdays()},
	'X': {Letter: 'X', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: allDays()},
	'Y': {Letter: 'Y', UTC: false, StartHour: 0, StartMinute: 0, EndHour: 8, EndMinute: 0, Days: weekends()},
	'Z': {Letter: 'Z', UTC: false, StartHour: 21, StartMinute: 0, EndHour: 24, EndMinute: 0, Days: weekends()},
}

func allDays() []time.Weekday {
	return []time.Weekday{
		time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	}
}

func weekdays() []time.Weekday {
	return []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday,
	}
}

func weekends() []time.Weekday {
	return []time.Weekday{time.Saturday, time.Sunday}
}

func ParseTIMSLetter(letter rune) (*TimeWindow, error) {
	entry, ok := TIMSTable[letter]
	if !ok {
		return nil, fmt.Errorf("unknown TIMS letter: %c", letter)
	}

	start := time.Date(2024, 1, 1, entry.StartHour, entry.StartMinute, 0, 0, time.UTC)
	end := time.Date(2024, 1, 1, entry.EndHour, entry.EndMinute, 0, 0, time.UTC)

	if entry.EndHour < entry.StartHour || (entry.EndHour == entry.StartHour && entry.EndMinute < entry.StartMinute) {
		end = end.AddDate(0, 0, 1)
	}

	if entry.EndHour == 24 && entry.EndMinute == 0 {
		end = time.Date(2024, 1, 1, 23, 59, 59, 0, time.UTC)
	}

	return &TimeWindow{
		StartUTC: start,
		EndUTC:   end,
		Source:   SourceTFlag,
		Days:     entry.Days,
	}, nil
}

func ParseTIMSString(timsString string) ([]*TimeWindow, error) {
	var windows []*TimeWindow

	for _, char := range timsString {
		if char == ' ' || char == ',' {
			continue
		}

		window, err := ParseTIMSLetter(char)
		if err != nil {
			return nil, fmt.Errorf("parsing TIMS string %s: %w", timsString, err)
		}
		windows = append(windows, window)
	}

	return windows, nil
}