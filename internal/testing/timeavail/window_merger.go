package timeavail

import (
	"sort"
	"time"
)

type WindowMerger struct {
	windows []TimeWindow
}

func NewWindowMerger() *WindowMerger {
	return &WindowMerger{
		windows: make([]TimeWindow, 0),
	}
}

func (wm *WindowMerger) AddWindow(window TimeWindow) {
	wm.windows = append(wm.windows, window)
}

func (wm *WindowMerger) AddWindows(windows []TimeWindow) {
	wm.windows = append(wm.windows, windows...)
}

func (wm *WindowMerger) Merge() []TimeWindow {
	if len(wm.windows) <= 1 {
		return wm.windows
	}

	sort.Slice(wm.windows, func(i, j int) bool {
		return wm.windows[i].StartUTC.Before(wm.windows[j].StartUTC)
	})

	merged := []TimeWindow{}
	current := wm.windows[0]

	for i := 1; i < len(wm.windows); i++ {
		next := wm.windows[i]

		if canMerge(current, next) {
			current = mergeWindows(current, next)
		} else {
			merged = append(merged, current)
			current = next
		}
	}

	merged = append(merged, current)
	return merged
}

func canMerge(w1, w2 TimeWindow) bool {
	if w1.Source != w2.Source {
		return false
	}

	if !daysOverlap(w1.Days, w2.Days) {
		return false
	}

	if w1.EndUTC.After(w2.StartUTC) || w1.EndUTC.Equal(w2.StartUTC) {
		return true
	}

	if w2.EndUTC.After(w1.StartUTC) || w2.EndUTC.Equal(w1.StartUTC) {
		return true
	}

	return false
}

func mergeWindows(w1, w2 TimeWindow) TimeWindow {
	start := w1.StartUTC
	if w2.StartUTC.Before(start) {
		start = w2.StartUTC
	}

	end := w1.EndUTC
	if w2.EndUTC.After(end) {
		end = w2.EndUTC
	}

	days := mergeDays(w1.Days, w2.Days)

	return TimeWindow{
		StartUTC: start,
		EndUTC:   end,
		Source:   w1.Source,
		Days:     days,
	}
}

func daysOverlap(days1, days2 []time.Weekday) bool {
	if len(days1) == 0 || len(days2) == 0 {
		return true
	}

	dayMap := make(map[time.Weekday]bool)
	for _, d := range days1 {
		dayMap[d] = true
	}

	for _, d := range days2 {
		if dayMap[d] {
			return true
		}
	}

	return false
}

func mergeDays(days1, days2 []time.Weekday) []time.Weekday {
	if len(days1) == 0 {
		return days2
	}
	if len(days2) == 0 {
		return days1
	}

	dayMap := make(map[time.Weekday]bool)
	for _, d := range days1 {
		dayMap[d] = true
	}
	for _, d := range days2 {
		dayMap[d] = true
	}

	result := make([]time.Weekday, 0, len(dayMap))
	for d := range dayMap {
		result = append(result, d)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})

	return result
}

func OptimizeWindows(windows []TimeWindow) []TimeWindow {
	if len(windows) <= 1 {
		return windows
	}

	merger := NewWindowMerger()
	merger.AddWindows(windows)
	return merger.Merge()
}