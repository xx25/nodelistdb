package web

import (
	"net/http"
	"strconv"
	"strings"

	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
)

// SearchHandler handles the node search page
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []database.Node
	var err error
	
	if r.Method == http.MethodPost {
		// Handle search form submission
		latestOnly := true
		filter := database.NodeFilter{
			Limit:      100,
			LatestOnly: &latestOnly,
		}
		
		// Parse full address first (takes precedence over individual fields)
		if fullAddress := r.FormValue("full_address"); fullAddress != "" {
			if zone, net, node, err := parseNodeAddress(fullAddress); err == nil {
				filter.Zone = &zone
				filter.Net = &net
				filter.Node = &node
			}
		}
		
		// Individual fields override full address if provided
		if zone := r.FormValue("zone"); zone != "" {
			if z, parseErr := strconv.Atoi(zone); parseErr == nil {
				filter.Zone = &z
			}
		}
		
		if net := r.FormValue("net"); net != "" {
			if n, parseErr := strconv.Atoi(net); parseErr == nil {
				filter.Net = &n
			}
		}
		
		if node := r.FormValue("node"); node != "" {
			if n, parseErr := strconv.Atoi(node); parseErr == nil {
				filter.Node = &n
			}
		}
		
		if systemName := r.FormValue("system_name"); systemName != "" {
			filter.SystemName = &systemName
		}
		
		if location := r.FormValue("location"); location != "" {
			filter.Location = &location
		}
		
		nodes, err = s.storage.GetNodes(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	data := struct {
		Title   string
		Nodes   []database.Node
		Count   int
		Error   string
	}{
		Title: "Search Nodes",
		Nodes: nodes,
		Count: len(nodes),
	}
	
	if err != nil {
		data.Error = err.Error()
	}
	
	s.templates["search"].Execute(w, data)
}

// SysopSearchHandler handles sysop name search page
func (s *Server) SysopSearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var sysopName string
	var err error
	
	if r.Method == http.MethodPost {
		sysopName = r.FormValue("sysop_name")
		if sysopName != "" {
			// Convert spaces to underscores as that's how data is stored in nodelist database
			searchName := strings.ReplaceAll(sysopName, " ", "_")
			nodes, err = s.storage.SearchNodesBySysop(searchName, 50)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	
	data := struct {
		Title     string
		Nodes     []storage.NodeSummary
		Count     int
		SysopName string
		Error     string
	}{
		Title:     "Search by Sysop Name",
		Nodes:     nodes,
		Count:     len(nodes),
		SysopName: sysopName,
	}
	
	if err != nil {
		data.Error = err.Error()
	}
	
	s.templates["sysop_search"].Execute(w, data)
}