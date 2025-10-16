package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// SearchHandler handles node search
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	nodes, count, searchErr := s.performNodeSearchWithLifetime(r)

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Count      int
		Error      error
		Version    string
	}{
		Title:      "Search Nodes",
		ActivePage: "search",
		Nodes:      nodes,
		Count:      count,
		Error:      searchErr,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// performNodeSearch handles the actual node search logic
func (s *Server) performNodeSearch(r *http.Request) ([]database.Node, int, error) {
	if r.Method != "POST" {
		return nil, 0, nil
	}

	_ = r.ParseForm()

	var filter database.NodeFilter
	var err error

	// Check if full address was provided
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		filter, err = buildNodeFilterFromAddress(fullAddress)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid address format: %v", err)
		}
	} else {
		// Build filter from individual fields
		filter = buildNodeFilterFromForm(r)

		// Check if search would be too resource-intensive
		if filter.Limit == 0 {
			return nil, 0, fmt.Errorf("Search requires more specific criteria. Please specify zone or net along with node number, or search by system name/location (minimum 2 characters)")
		}
	}

	nodes, err := s.storage.NodeOps().GetNodes(filter)
	if err != nil {
		return nil, 0, err
	}

	return nodes, len(nodes), nil
}

// performNodeSearchWithLifetime handles the actual node search logic and returns NodeSummary with lifetime info
func (s *Server) performNodeSearchWithLifetime(r *http.Request) ([]storage.NodeSummary, int, error) {
	if r.Method != "POST" {
		return nil, 0, nil
	}

	_ = r.ParseForm()

	var filter database.NodeFilter
	var err error

	// Check if full address was provided
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		filter, err = buildNodeFilterFromAddress(fullAddress)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid address format: %v", err)
		}
	} else {
		// Build filter from individual fields
		filter = buildNodeFilterFromForm(r)

		// Check if search would be too resource-intensive
		if filter.Limit == 0 {
			return nil, 0, fmt.Errorf("Search requires more specific criteria. Please specify zone or net along with node number, or search by system name/location (minimum 2 characters)")
		}
	}

	nodes, err := s.storage.SearchOps().SearchNodesWithLifetime(filter)
	if err != nil {
		return nil, 0, err
	}

	return nodes, len(nodes), nil
}

// SysopSearchHandler handles sysop search
func (s *Server) SysopSearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var count int
	var searchErr error
	var sysopName string

	if r.Method == "POST" {
		r.ParseForm()
		sysopName = r.FormValue("sysop_name")

		if sysopName != "" {
			nodes, searchErr = s.storage.SearchOps().SearchNodesBySysop(sysopName, 100)
			count = len(nodes)
		}
	}

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Count      int
		Error      error
		SysopName  string
		Version    string
	}{
		Title:      "Search by Sysop",
		ActivePage: "sysop",
		Nodes:      nodes,
		Count:      count,
		Error:      searchErr,
		SysopName:  sysopName,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["sysop_search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// NodeHistoryHandler handles node history view
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, err := parseNodeURLPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.NodeOps().GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get all node changes without filtering
	changes, err := s.storage.SearchOps().GetNodeChanges(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node changes: %v", err), http.StatusInternalServerError)
		return
	}

	activityInfo := analyzeNodeActivity(history)

	data := struct {
		Title            string
		Address          string
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
