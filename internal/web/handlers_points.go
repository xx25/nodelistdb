package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// pointHistoryPeriod is one row of the collapsed history table: a run of
// consecutive issues (in newest-first history order) whose listed content is
// identical.
type pointHistoryPeriod struct {
	Entry     database.Point // newest row of the run; supplies the content columns
	FirstDate time.Time
	LastDate  time.Time
	Count     int      // distinct publications collapsed into this period
	Sources   []string // distinct list_source values, in appearance order
}

// SourcesKey joins the period's sources for the client-side sorter, which
// otherwise reads only the first badge of a cell.
func (p pointHistoryPeriod) SourcesKey() string {
	return strings.Join(p.Sources, ", ")
}

// samePointContent reports whether two rows carry the same listed content
// (issue metadata and source provenance excluded). InternetConfig is part of
// the key because recognized internet tokens (IBN, INA, ...) are stripped out
// of Flags during parsing — an endpoint change is invisible to the flag slices.
func samePointContent(a, b database.Point) bool {
	return a.SystemName == b.SystemName &&
		a.Location == b.Location &&
		a.SysopName == b.SysopName &&
		a.Phone == b.Phone &&
		a.MaxSpeed == b.MaxSpeed &&
		slices.Equal(a.Flags, b.Flags) &&
		slices.Equal(a.ModemFlags, b.ModemFlags) &&
		sameInternetConfig(a.InternetConfig, b.InternetConfig)
}

// sameInternetConfig reports whether two stored configs describe the same
// endpoints. Comparing the raw bytes is not enough: rows written before INA
// became a list hold {"INA":"host"} where newer ones hold {"INA":["host"]}, and
// that reshape alone must not read as a content change. Unparseable values fall
// back to a byte comparison.
func sameInternetConfig(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}

	canonical := func(raw []byte) ([]byte, bool) {
		if len(raw) == 0 {
			return nil, true
		}
		var config database.InternetConfiguration
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, false
		}
		// Map keys are marshalled in sorted order, so this is canonical.
		encoded, err := json.Marshal(config)
		if err != nil {
			return nil, false
		}
		return encoded, true
	}

	canonicalA, okA := canonical(a)
	canonicalB, okB := canonical(b)
	if !okA || !okB {
		return false
	}
	return bytes.Equal(canonicalA, canonicalB)
}

// groupPointHistory collapses the newest-first history into periods of
// unchanged content. Sources interleave (a zone rollup republishes the
// regionals), so a period spans every series that carried the same entry; a
// content change ends the run even if a later issue reverts it.
func groupPointHistory(history []database.Point) []pointHistoryPeriod {
	var periods []pointHistoryPeriod
	for i, h := range history {
		if n := len(periods); n > 0 && samePointContent(periods[n-1].Entry, h) {
			p := &periods[n-1]
			p.FirstDate = h.PointlistDate // history is newest-first
			// Duplicate lines inside one file (conflict_sequence > 0) sort
			// adjacent; count distinct publications, not stored rows.
			if prev := history[i-1]; prev.ListSource != h.ListSource || !prev.PointlistDate.Equal(h.PointlistDate) {
				p.Count++
			}
			if !slices.Contains(p.Sources, h.ListSource) {
				p.Sources = append(p.Sources, h.ListSource)
			}
			continue
		}
		periods = append(periods, pointHistoryPeriod{
			Entry:     h,
			FirstDate: h.PointlistDate,
			LastDate:  h.PointlistDate,
			Count:     1,
			Sources:   []string{h.ListSource},
		})
	}
	return periods
}

// pickSnapshotEntry resolves a point's current entry from its full history
// (newest first) with snapshot semantics: candidates are the rows within the
// staleness window of the newest issue; the lowest source_priority wins,
// newest date then lowest conflict sequence on ties.
func pickSnapshotEntry(history []database.Point) database.Point {
	best := history[0]
	windowStart := history[0].PointlistDate.AddDate(0, 0, -storage.PointSnapshotStalenessDays)
	for _, h := range history[1:] {
		if !h.PointlistDate.After(windowStart) {
			continue
		}
		if h.SourcePriority < best.SourcePriority ||
			(h.SourcePriority == best.SourcePriority && h.PointlistDate.After(best.PointlistDate)) ||
			(h.SourcePriority == best.SourcePriority && h.PointlistDate.Equal(best.PointlistDate) && h.ConflictSequence < best.ConflictSequence) {
			best = h
		}
	}
	return best
}

// parsePointURLPath extracts the 4-D address from /points/{zone}/{net}/{node}/{point}
func parsePointURLPath(path string) (zone, net, node, point int, err error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/points/"), "/"), "/")
	if len(parts) < 4 {
		return 0, 0, 0, 0, fmt.Errorf("invalid point address")
	}

	for i, p := range []struct {
		name string
		dest *int
	}{{"zone", &zone}, {"net", &net}, {"node", &node}, {"point", &point}} {
		v, aerr := strconv.Atoi(parts[i])
		if aerr != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid %s", p.name)
		}
		*p.dest = v
	}
	return zone, net, node, point, nil
}

// PointHistoryHandler renders the point detail page: latest entry plus the
// full history across all pointlist series (rows labeled with list_source).
// Path: /points/{zone}/{net}/{node}/{point}
func (s *Server) PointHistoryHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, point, err := parsePointURLPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve the network like the node page does, but against the points
	// table: the address may exist only in a network the node-level heuristic
	// would not pick.
	availableDomains, _ := s.storage.GetPointDomains(zone, net, node, &point)
	domain := resolveEntityDomain(r, availableDomains)

	history, err := s.storage.GetPointHistory(domain, zone, net, node, point)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving point history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Point not found", http.StatusNotFound)
		return
	}

	// Pick the "current" entry with the same rule the snapshot queries use:
	// among rows within the staleness window of the newest issue, the most
	// authoritative source wins (priority ASC), newest issue on ties —
	// history[0] alone would prefer date over priority and could disagree
	// with the API snapshot.
	latest := pickSnapshotEntry(history)
	firstDate := history[len(history)-1].PointlistDate
	lastDate := history[0].PointlistDate
	currentlyActive := time.Since(lastDate).Hours()/24 <= storage.PointSnapshotStalenessDays

	data := struct {
		Title            string
		Address          string
		BossAddress      string
		Domain           string
		AvailableDomains []string
		History          []database.Point
		HistoryPeriods   []pointHistoryPeriod
		Latest           database.Point
		FirstDate        time.Time
		LastDate         time.Time
		CurrentlyActive  bool
		FlagDescriptions map[string]flags.FlagInfo
		Version          string
		ActivePage       string
	}{
		Title:            "Point History",
		Address:          fmt.Sprintf("%d:%d/%d.%d", zone, net, node, point),
		BossAddress:      fmt.Sprintf("%d:%d/%d", zone, net, node),
		Domain:           domain,
		AvailableDomains: availableDomains,
		History:          history,
		HistoryPeriods:   groupPointHistory(history),
		Latest:           latest,
		FirstDate:        firstDate,
		LastDate:         lastDate,
		CurrentlyActive:  currentlyActive,
		FlagDescriptions: flags.GetFlagDescriptions(),
		Version:          version.GetVersionInfo(),
		ActivePage:       "",
	}

	if err := s.templates["point_history"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
