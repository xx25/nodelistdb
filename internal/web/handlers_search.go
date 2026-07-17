package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// SearchHandler handles unified node and sysop search
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var count int
	var searchErr error
	var sysopName string
	isRootPage := r.URL.Path == "/"

	// Only perform search on POST
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			searchErr = fmt.Errorf("Failed to parse form: %v", err)
		} else {
			// Check if sysop_name field is filled
			sysopName = r.FormValue("sysop_name")

			if sysopName != "" {
				// Perform sysop search (scoped to the selected network, if any)
				searchDomain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
				nodes, searchErr = s.storage.SearchNodesBySysop(sysopName, 100, searchDomain)
				count = len(nodes)
			} else {
				// Perform node search
				nodes, count, searchErr = s.performNodeSearchWithLifetime(r)
			}
		}
	}

	networks, _ := s.storage.GetDomains()

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Count      int
		Error      error
		SysopName  string
		IsRootPage bool
		Version    string
		Networks   []storage.DomainInfo
	}{
		Title:      "Search",
		ActivePage: "search",
		Nodes:      nodes,
		Count:      count,
		Error:      searchErr,
		SysopName:  sysopName,
		IsRootPage: isRootPage,
		Version:    version.GetVersionInfo(),
		Networks:   networks,
	}

	if isRootPage {
		data.Title = "NodelistDB"
	}

	if err := s.templates["search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
		// Honor the network selector for full-address searches too
		if domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain"))); domain != "" {
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

	nodes, err := s.storage.SearchNodesWithLifetime(filter)
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

	// Resolve the network: explicit ?domain= wins; otherwise use the only
	// network the address exists in, preferring fidonet when ambiguous
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	availableDomains, _ := s.storage.NodeOps().GetNodeDomains(zone, net, node)
	if domain == "" {
		switch len(availableDomains) {
		case 0:
			domain = database.DefaultDomain
		case 1:
			domain = availableDomains[0]
		default:
			domain = availableDomains[0]
			for _, d := range availableDomains {
				if d == database.DefaultDomain {
					domain = d
					break
				}
			}
		}
	}
	history, err := s.storage.GetNodeHistory(zone, net, node, domain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get all node changes within the requested network
	changes, err := s.storage.GetNodeChanges(zone, net, node, domain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node changes: %v", err), http.StatusInternalServerError)
		return
	}

	activityInfo := analyzeNodeActivity(history)

	resolvedDomain := domain
	if len(history) > 0 && history[0].Domain != "" {
		resolvedDomain = history[0].Domain
	}

	data := struct {
		Title            string
		Address          string
		Domain           string
		AvailableDomains []string
		History          []database.Node
		Changes          []database.NodeChange
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
		FirstDate:        activityInfo.FirstDate,
		LastDate:         activityInfo.LastDate,
		CurrentlyActive:  activityInfo.CurrentlyActive,
		FlagDescriptions: flags.GetFlagDescriptions(),
		Version:          version.GetVersionInfo(),
		ActivePage:       "",
	}

	if err := s.templates["node_history"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
