package timeavail

import (
	"time"
)

type Source string

const (
	SourceTFlag  Source = "T-flag"
	SourceZMH    Source = "ZMH"
	SourceNumber Source = "#nn"
	SourceCM     Source = "CM"
	SourceICM    Source = "ICM"
)

type TimeWindow struct {
	StartUTC time.Time
	EndUTC   time.Time
	Source   Source
	Days     []time.Weekday
}

type NodeAvailability struct {
	Windows      []TimeWindow
	IsCM         bool
	IsICM        bool
	IsDefaultZMH bool // True if ZMH was applied as default (no CM, no explicit time flags)
	HasPSTN      bool
	PhoneNumber  string
	TimeZone     *time.Location
	RawFlags     []string
}

func (na *NodeAvailability) IsCallableNow(now time.Time) bool {
	if na.IsCM {
		return true
	}

	if len(na.Windows) == 0 {
		return true
	}

	localTime := now
	if na.TimeZone != nil {
		localTime = now.In(na.TimeZone)
	}

	currentWeekday := localTime.Weekday()
	currentTime := localTime.Format("15:04")

	for _, window := range na.Windows {
		if window.IncludesDay(currentWeekday) {
			windowStart := window.StartUTC.In(localTime.Location()).Format("15:04")
			windowEnd := window.EndUTC.In(localTime.Location()).Format("15:04")

			// Use exclusive end time (half-open interval) for consistency with scheduler.go
			if windowStart <= currentTime && currentTime < windowEnd {
				return true
			}
			// Handle overnight windows (end time wraps to next day)
			if windowStart > windowEnd && (currentTime >= windowStart || currentTime < windowEnd) {
				return true
			}
		}
	}

	return false
}

func (tw *TimeWindow) IncludesDay(day time.Weekday) bool {
	if len(tw.Days) == 0 {
		return true
	}

	for _, d := range tw.Days {
		if d == day {
			return true
		}
	}
	return false
}

func (tw *TimeWindow) Overlaps(other *TimeWindow) bool {
	if tw.StartUTC.After(other.EndUTC) || other.StartUTC.After(tw.EndUTC) {
		return false
	}

	for _, day1 := range tw.Days {
		for _, day2 := range other.Days {
			if day1 == day2 {
				return true
			}
		}
	}

	if len(tw.Days) == 0 || len(other.Days) == 0 {
		return true
	}

	return false
}