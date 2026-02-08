package timeavail

import (
	"sort"
	"time"
)

type CallSchedule struct {
	Node       string
	Zone       int
	NextWindow *TimeWindow
	NextCall   time.Time
	IsCallable bool
	Reason     string
}

type Scheduler struct {
	currentTime time.Time
}

func NewScheduler(currentTime time.Time) *Scheduler {
	return &Scheduler{
		currentTime: currentTime,
	}
}

func (s *Scheduler) GetNextCallTime(availability *NodeAvailability) *CallSchedule {
	if availability.IsCM {
		return &CallSchedule{
			IsCallable: true,
			NextCall:   s.currentTime,
			Reason:     "CM - Always available",
		}
	}

	if len(availability.Windows) == 0 {
		return &CallSchedule{
			IsCallable: true,
			NextCall:   s.currentTime,
			Reason:     "No time restrictions",
		}
	}

	for _, window := range availability.Windows {
		if s.isInWindow(window) {
			return &CallSchedule{
				IsCallable: true,
				NextCall:   s.currentTime,
				NextWindow: &window,
				Reason:     "Currently in call window",
			}
		}
	}

	nextWindow, nextTime := s.findNextWindow(availability.Windows)
	if nextWindow != nil {
		return &CallSchedule{
			IsCallable: false,
			NextCall:   nextTime,
			NextWindow: nextWindow,
			Reason:     "Outside call window",
		}
	}

	return &CallSchedule{
		IsCallable: false,
		Reason:     "No available call windows",
	}
}

func (s *Scheduler) isInWindow(window TimeWindow) bool {
	currentWeekday := s.currentTime.Weekday()
	previousDay := (currentWeekday + 6) % 7
	currentHour := s.currentTime.Hour()
	currentMinute := s.currentTime.Minute()
	currentTimeMinutes := currentHour*60 + currentMinute

	startMinutes := window.StartUTC.Hour()*60 + window.StartUTC.Minute()
	endMinutes := window.EndUTC.Hour()*60 + window.EndUTC.Minute()

	if startMinutes <= endMinutes {
		// Normal window (same day)
		return window.IncludesDay(currentWeekday) && currentTimeMinutes >= startMinutes && currentTimeMinutes < endMinutes
	}

	// Overnight window (spans midnight)
	if currentTimeMinutes >= startMinutes && window.IncludesDay(currentWeekday) {
		return true
	}
	if currentTimeMinutes < endMinutes && window.IncludesDay(previousDay) {
		return true
	}
	return false
}

func (s *Scheduler) findNextWindow(windows []TimeWindow) (*TimeWindow, time.Time) {
	type windowStart struct {
		window    TimeWindow
		startTime time.Time
	}

	var starts []windowStart

	for _, window := range windows {
		for dayOffset := 0; dayOffset < 7; dayOffset++ {
			checkDate := s.currentTime.AddDate(0, 0, dayOffset)
			checkWeekday := checkDate.Weekday()

			if !window.IncludesDay(checkWeekday) {
				continue
			}

			windowStartTime := time.Date(
				checkDate.Year(), checkDate.Month(), checkDate.Day(),
				window.StartUTC.Hour(), window.StartUTC.Minute(), 0, 0,
				checkDate.Location(),
			)

			if windowStartTime.After(s.currentTime) {
				starts = append(starts, windowStart{
					window:    window,
					startTime: windowStartTime,
				})
			}
		}
	}

	if len(starts) == 0 {
		return nil, time.Time{}
	}

	sort.Slice(starts, func(i, j int) bool {
		return starts[i].startTime.Before(starts[j].startTime)
	})

	return &starts[0].window, starts[0].startTime
}

func (s *Scheduler) SortNodesByAvailability(nodes []NodeSchedule) []NodeSchedule {
	sorted := make([]NodeSchedule, len(nodes))
	copy(sorted, nodes)

	sort.Slice(sorted, func(i, j int) bool {
		sched1 := s.GetNextCallTime(sorted[i].Availability)
		sched2 := s.GetNextCallTime(sorted[j].Availability)

		if sched1.IsCallable != sched2.IsCallable {
			return sched1.IsCallable
		}

		if sched1.IsCallable && sched2.IsCallable {
			return sorted[i].Priority > sorted[j].Priority
		}

		if !sched1.NextCall.IsZero() && !sched2.NextCall.IsZero() {
			return sched1.NextCall.Before(sched2.NextCall)
		}

		return sorted[i].Priority > sorted[j].Priority
	})

	return sorted
}

type NodeSchedule struct {
	NodeAddress  string
	Availability *NodeAvailability
	Priority     int
}