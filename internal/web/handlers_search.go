package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// SearchHandler handles unified node and sysop search
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var points []storage.PointSummary
	var count int
	var searchErr error
	var sysopName string
	isRootPage := r.URL.Path == "/"

	// Only perform search on POST
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			searchErr = fmt.Errorf("Failed to parse form: %v", err)
		} else {
			// A 4-D address (2:5001/100.7) is a point — route it to the point
			// page instead of discarding the point suffix.
			if fullAddress := r.FormValue("full_address"); fullAddress != "" {
				if zone, net, node, point, hasPoint, err := parseFullAddress(fullAddress); err == nil && hasPoint {
					target := fmt.Sprintf("/points/%d/%d/%d/%d", zone, net, node, point)
					// Only an EXPLICIT domain is forwarded — forwarding the
					// switcher cookie would pin the point page to a network
					// the point may not exist in; with no param the point
					// page resolves the network itself (cookie preferred,
					// cross-network fallback).
					if domain := explicitDomain(r); domain != "" {
						target += "?domain=" + domain
					}
					http.Redirect(w, r, target, http.StatusSeeOther)
					return
				}
			}

			// Check if sysop_name field is filled
			sysopName = r.FormValue("sysop_name")

			if sysopName != "" {
				// Perform sysop search, scoped to the selected network
				nodes, searchErr = s.storage.SearchNodesBySysop(r.Context(), sysopName, 100, requestDomain(r))
				count = len(nodes)
			} else {
				// Perform node search
				nodes, count, searchErr = s.performNodeSearchWithLifetime(r)
			}

			// Same criteria against the points table, shown as a separate
			// labeled section. A points-side failure must not kill the node
			// results, but it is a real error, not "0 points" — log it.
			if searchErr == nil {
				if pointFilter, ok := buildPointFilterFromForm(r); ok {
					var pointErr error
					points, pointErr = s.storage.SearchPointsWithLifetime(r.Context(), pointFilter)
					if pointErr != nil {
						if clientGone("Search: points", pointErr) {
							return
						}
						logging.Warnf("point search failed: %v", pointErr)
					}
				}
			}

			// The node search reports failure by returning an error the page
			// then shows. A cancelled request is not a failed search: the tab
			// is gone, so there is nobody to show it to.
			if searchErr != nil && clientGone("Search: nodes", searchErr) {
				return
			}
		}
	}

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Points     []storage.PointSummary
		PointCount int
		Count      int
		Error      error
		SysopName  string
		IsRootPage bool
		Version    string
	}{
		Title:      "Search",
		ActivePage: "search",
		Nodes:      nodes,
		Points:     points,
		PointCount: len(points),
		Count:      count,
		Error:      searchErr,
		SysopName:  sysopName,
		IsRootPage: isRootPage,
		Version:    version.GetVersionInfo(),
	}

	if isRootPage {
		data.Title = "NodelistDB"
	}

	s.render(w, "search", data)
}

// performNodeSearchWithLifetime handles the actual node search logic and returns NodeSummary with lifetime info
func (s *Server) performNodeSearchWithLifetime(r *http.Request) ([]storage.NodeSummary, int, error) {
	if r.Method != "POST" {
		return nil, 0, nil
	}

	if err := r.ParseForm(); err != nil {
		return nil, 0, fmt.Errorf("Failed to parse form: %v", err)
	}

	var filter database.NodeFilter
	var err error

	// Check if full address was provided
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		filter, err = buildNodeFilterFromAddress(fullAddress)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid address format: %v", err)
		}
		// A full address is looked up across ALL networks (the result rows
		// carry a per-network badge); only an explicit ?domain= scopes it.
		if domain := explicitDomain(r); domain != "" {
			filter.Domain = &domain
		}
	} else {
		// Build filter from individual fields
		filter = buildNodeFilterFromForm(r)

		// Check if search would be too resource-intensive
		if filter.Limit == 0 {
			return nil, 0, fmt.Errorf("Search requires more specific criteria. Please specify zone or net along with node number, or search by system name/location (minimum 2 characters)")
		}
	}

	nodes, err := s.storage.SearchNodesWithLifetime(r.Context(), filter)
	if err != nil {
		return nil, 0, err
	}

	return nodes, len(nodes), nil
}

// NodeHistoryHandler handles node history view
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, err := parseNodeURLPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve the network: explicit ?domain= wins, then the global switcher,
	// then the network(s) the address actually exists in
	availableDomains, _ := s.storage.GetNodeDomains(r.Context(), zone, net, node)
	domain := resolveEntityDomain(r, availableDomains)
	history, err := s.storage.GetNodeHistory(r.Context(), zone, net, node, domain)
	if err != nil {
		httpStorageError(w, "Error retrieving node history", "Error retrieving node history", err)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get all node changes within the requested network
	changes, err := s.storage.GetNodeChanges(r.Context(), zone, net, node, domain)
	if err != nil {
		httpStorageError(w, "Error retrieving node changes", "Error retrieving node changes", err)
		return
	}

	activityInfo := analyzeNodeActivity(history)

	resolvedDomain := domain
	if len(history) > 0 && history[0].Domain != "" {
		resolvedDomain = history[0].Domain
	}

	// Pointlist snapshot under this boss (empty for the vast majority of nodes)
	points, _ := s.storage.GetPointsByBoss(r.Context(), resolvedDomain, zone, net, node, nil)

	data := struct {
		Title            string
		Address          string
		Domain           string
		AvailableDomains []string
		History          []database.Node
		Changes          []database.NodeChange
		Points           []database.Point
		FirstDate        time.Time
		LastDate         time.Time
		CurrentlyActive  bool
		FlagDescriptions map[string]flags.FlagInfo
		Version          string
		ActivePage       string
	}{
		Title:            "Node History",
		Address:          fmt.Sprintf("%d:%d/%d", zone, net, node),
		Domain:           resolvedDomain,
		AvailableDomains: availableDomains,
		History:          history,
		Changes:          changes,
		Points:           points,
		FirstDate:        activityInfo.FirstDate,
		LastDate:         activityInfo.LastDate,
		CurrentlyActive:  activityInfo.CurrentlyActive,
		FlagDescriptions: flags.GetFlagDescriptions(),
		Version:          version.GetVersionInfo(),
		ActivePage:       "",
	}

	s.render(w, "node_history", data)
}
