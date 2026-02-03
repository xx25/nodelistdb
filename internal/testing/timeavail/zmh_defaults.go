package timeavail

import (
	"time"
)

// ZMHZone defines Zone Mail Hour for a specific FidoNet zone.
// ZMH times are in UTC per FidoNet standards.
type ZMHZone struct {
	Zone        int
	StartHour   int
	StartMinute int
	EndHour     int
	EndMinute   int
	Days        []time.Weekday
}

// ZMHDefaults contains the official ZMH windows for each FidoNet zone.
// Per FidoNet standards, these are the times when non-CM nodes must be available.
var ZMHDefaults = []ZMHZone{
	{Zone: 1, StartHour: 9, StartMinute: 0, EndHour: 10, EndMinute: 0, Days: allDays()},   // 09:00-10:00 UTC
	{Zone: 2, StartHour: 2, StartMinute: 30, EndHour: 3, EndMinute: 30, Days: allDays()},  // 02:30-03:30 UTC
	{Zone: 3, StartHour: 18, StartMinute: 0, EndHour: 19, EndMinute: 0, Days: allDays()},  // 18:00-19:00 UTC
	{Zone: 4, StartHour: 8, StartMinute: 0, EndHour: 9, EndMinute: 0, Days: allDays()},    // 08:00-09:00 UTC
	{Zone: 5, StartHour: 2, StartMinute: 30, EndHour: 3, EndMinute: 30, Days: allDays()},  // 02:30-03:30 UTC (same as Zone 2)
	{Zone: 6, StartHour: 22, StartMinute: 0, EndHour: 23, EndMinute: 0, Days: allDays()},  // 22:00-23:00 UTC
}

// GetZMHWindow returns the ZMH time window for a given zone.
// If the zone is not found, returns the Zone 2 default (02:30-03:30 UTC).
func GetZMHWindow(zone int) *TimeWindow {
	for _, zmh := range ZMHDefaults {
		if zmh.Zone == zone {
			start := time.Date(2024, 1, 1, zmh.StartHour, zmh.StartMinute, 0, 0, time.UTC)
			end := time.Date(2024, 1, 1, zmh.EndHour, zmh.EndMinute, 0, 0, time.UTC)

			// Handle overnight windows (EndHour < StartHour)
			if zmh.EndHour < zmh.StartHour || (zmh.EndHour == zmh.StartHour && zmh.EndMinute < zmh.StartMinute) {
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

	// Default to Zone 2 ZMH (02:30-03:30 UTC) for unknown zones
	return &TimeWindow{
		StartUTC: time.Date(2024, 1, 1, 2, 30, 0, 0, time.UTC),
		EndUTC:   time.Date(2024, 1, 1, 3, 30, 0, 0, time.UTC),
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