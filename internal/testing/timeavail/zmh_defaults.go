package timeavail

import (
	"time"
)

type ZMHZone struct {
	Zone       int
	StartHour  int
	EndHour    int
	Days       []time.Weekday
}

var ZMHDefaults = []ZMHZone{
	{Zone: 1, StartHour: 9, EndHour: 10, Days: allDays()},
	{Zone: 2, StartHour: 2, EndHour: 3, Days: allDays()},
	{Zone: 3, StartHour: 18, EndHour: 19, Days: allDays()},
	{Zone: 4, StartHour: 8, EndHour: 9, Days: allDays()},
	{Zone: 5, StartHour: 2, EndHour: 3, Days: allDays()},
	{Zone: 6, StartHour: 22, EndHour: 23, Days: allDays()},
}

func GetZMHWindow(zone int) *TimeWindow {
	for _, zmh := range ZMHDefaults {
		if zmh.Zone == zone {
			start := time.Date(2024, 1, 1, zmh.StartHour, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 1, zmh.EndHour, 0, 0, 0, time.UTC)

			if zmh.EndHour < zmh.StartHour {
				end = end.AddDate(0, 0, 1)
			}

			return &TimeWindow{
				StartUTC: start,
				EndUTC:   end,
				Source:   SourceZMH,
				Days:     zmh.Days,
			}
		}
	}

	return &TimeWindow{
		StartUTC: time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC),
		EndUTC:   time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC),
		Source:   SourceZMH,
		Days:     allDays(),
	}
}

func ApplyZMHDefaults(zone int, hasZMHFlag bool) *TimeWindow {
	if !hasZMHFlag {
		return nil
	}
	return GetZMHWindow(zone)
}