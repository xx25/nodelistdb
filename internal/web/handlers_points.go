package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

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
