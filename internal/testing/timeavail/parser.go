package timeavail

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	tflagRegex = regexp.MustCompile(`^T([A-Z]+)$`)
	nnRegex    = regexp.MustCompile(`^#(\d{2})$`)
)

type Parser struct {
	defaultZone int
}

func NewParser(defaultZone int) *Parser {
	return &Parser{
		defaultZone: defaultZone,
	}
}

func (p *Parser) ParseNodeFlags(flags []string, zone int, phoneNumber string) (*NodeAvailability, error) {
	availability := &NodeAvailability{
		Windows:     []TimeWindow{},
		HasPSTN:     phoneNumber != "" && phoneNumber != "-Unpublished-",
		PhoneNumber: phoneNumber,
		RawFlags:    flags,
	}

	var hasZMH bool
	var timWindows []TimeWindow

	for _, flag := range flags {
		flag = strings.TrimSpace(strings.ToUpper(flag))

		if flag == "CM" {
			availability.IsCM = true
			continue
		}

		if flag == "ICM" {
			availability.IsICM = true
			continue
		}

		if flag == "ZMH" {
			hasZMH = true
			continue
		}

		if window := p.parseTFlag(flag); window != nil {
			timWindows = append(timWindows, *window)
			continue
		}

		if window := p.parseNumberFlag(flag); window != nil {
			timWindows = append(timWindows, *window)
			continue
		}
	}

	if hasZMH {
		if zmhWindow := GetZMHWindow(zone); zmhWindow != nil {
			timWindows = append(timWindows, *zmhWindow)
		}
	}

	if len(timWindows) > 0 {
		availability.Windows = OptimizeWindows(timWindows)
	}

	return availability, nil
}

func (p *Parser) parseTFlag(flag string) *TimeWindow {
	matches := tflagRegex.FindStringSubmatch(flag)
	if len(matches) != 2 {
		return nil
	}

	letters := matches[1]
	windows, err := ParseTIMSString(letters)
	if err != nil || len(windows) == 0 {
		return nil
	}

	merger := NewWindowMerger()
	for _, w := range windows {
		merger.AddWindow(*w)
	}

	merged := merger.Merge()
	if len(merged) > 0 {
		return &merged[0]
	}

	return nil
}

func (p *Parser) parseNumberFlag(flag string) *TimeWindow {
	matches := nnRegex.FindStringSubmatch(flag)
	if len(matches) != 2 {
		return nil
	}

	hours, err := strconv.Atoi(matches[1])
	if err != nil || hours < 0 || hours > 23 {
		return nil
	}

	return &TimeWindow{
		StartUTC: time.Date(2024, 1, 1, hours, 0, 0, 0, time.UTC),
		EndUTC:   time.Date(2024, 1, 1, hours+1, 0, 0, 0, time.UTC),
		Source:   SourceNumber,
		Days:     allDays(),
	}
}

func ParseAvailability(flags []string, zone int, phoneNumber string) (*NodeAvailability, error) {
	parser := NewParser(zone)
	return parser.ParseNodeFlags(flags, zone, phoneNumber)
}

func FormatTimeWindow(window TimeWindow) string {
	startTime := window.StartUTC.Format("15:04")
	endTime := window.EndUTC.Format("15:04")

	dayStr := ""
	if len(window.Days) > 0 && len(window.Days) < 7 {
		dayNames := []string{}
		for _, day := range window.Days {
			dayNames = append(dayNames, day.String()[:3])
		}
		dayStr = fmt.Sprintf(" (%s)", strings.Join(dayNames, ","))
	}

	return fmt.Sprintf("%s-%s%s [%s]", startTime, endTime, dayStr, window.Source)
}

func FormatAvailability(availability *NodeAvailability) string {
	if availability.IsCM {
		return "CM (24/7 PSTN)"
	}

	if availability.IsICM {
		return "ICM (24/7 IP only)"
	}

	if len(availability.Windows) == 0 {
		return "No time restrictions"
	}

	windowStrs := []string{}
	for _, window := range availability.Windows {
		windowStrs = append(windowStrs, FormatTimeWindow(window))
	}

	return strings.Join(windowStrs, ", ")
}