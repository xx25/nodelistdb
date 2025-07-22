package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
)

// IndexHandler handles the main page
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "FidoNet Nodelist Database",
	}
	
	s.templates["index"].Execute(w, data)
}

// StatsHandler handles the statistics page
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	// Get available dates for the date picker
	availableDates, err := s.storage.GetAvailableDates()
	if err != nil || len(availableDates) == 0 {
		// No data at all
		data := struct {
			Title          string
			Stats          *database.NetworkStats
			Error          string
			NoData         bool
			AvailableDates []time.Time
			SelectedDate   time.Time
			RequestedDate  string
			DateMessage    string
		}{
			Title:          "Network Statistics",
			Stats:          nil,
			Error:          "No nodelist data available in the database",
			NoData:         true,
			AvailableDates: []time.Time{},
			SelectedDate:   time.Time{},
			RequestedDate:  "",
			DateMessage:    "",
		}
		s.templates["stats"].Execute(w, data)
		return
	}

	// Default to latest date
	selectedDate := availableDates[0]
	requestedDate := ""
	dateMessage := ""

	// Check if a specific date was requested
	if dateParam := r.URL.Query().Get("date"); dateParam != "" {
		requestedDate = dateParam
		if parsedDate, err := time.Parse("2006-01-02", dateParam); err == nil {
			// Find closest available date
			if closestDate, err := s.storage.GetClosestAvailableDate(parsedDate); err == nil {
				selectedDate = closestDate
				if !selectedDate.Equal(parsedDate) {
					dateMessage = fmt.Sprintf("Requested date %s not available. Showing closest available date: %s", 
						parsedDate.Format("2006-01-02"), selectedDate.Format("2006-01-02"))
				}
			}
		} else {
			dateMessage = fmt.Sprintf("Invalid date format '%s'. Please use YYYY-MM-DD format.", dateParam)
		}
	}
	
	stats, err := s.storage.GetStats(selectedDate)
	if err != nil {
		data := struct {
			Title          string
			Stats          *database.NetworkStats
			Error          string
			NoData         bool
			AvailableDates []time.Time
			SelectedDate   time.Time
			RequestedDate  string
			DateMessage    string
		}{
			Title:          "Network Statistics",
			Stats:          nil,
			Error:          fmt.Sprintf("Failed to retrieve statistics: %v", err),
			NoData:         false,
			AvailableDates: availableDates,
			SelectedDate:   selectedDate,
			RequestedDate:  requestedDate,
			DateMessage:    dateMessage,
		}
		s.templates["stats"].Execute(w, data)
		return
	}
	
	data := struct {
		Title          string
		Stats          *database.NetworkStats
		Error          string
		NoData         bool
		AvailableDates []time.Time
		SelectedDate   time.Time
		RequestedDate  string
		DateMessage    string
	}{
		Title:          "Network Statistics",
		Stats:          stats,
		Error:          "",
		NoData:         false,
		AvailableDates: availableDates,
		SelectedDate:   selectedDate,
		RequestedDate:  requestedDate,
		DateMessage:    dateMessage,
	}
	
	s.templates["stats"].Execute(w, data)
}

// NodeHistoryHandler handles node history page
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /node/{zone}/{net}/{node}
	path := r.URL.Path[6:] // Remove "/node/"
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid node address format", http.StatusBadRequest)
		return
	}
	
	zone, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}
	
	net, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}
	
	node, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}
	
	// Get node history
	history, err := s.storage.GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	
	// Get date range
	firstDate, lastDate, _ := s.storage.GetNodeDateRange(zone, net, node)
	
	// Parse filter options from query parameters
	query := r.URL.Query()
	filter := storage.ChangeFilter{
		IgnoreFlags:              query.Get("noflags") == "1",
		IgnorePhone:              query.Get("nophone") == "1",
		IgnoreSpeed:              query.Get("nospeed") == "1",
		IgnoreStatus:             query.Get("nostatus") == "1",
		IgnoreLocation:           query.Get("nolocation") == "1",
		IgnoreName:               query.Get("noname") == "1",
		IgnoreSysop:              query.Get("nosysop") == "1",
		IgnoreConnectivity:       query.Get("noconnectivity") == "1",
		IgnoreInternetProtocols:  query.Get("nointernetprotocols") == "1",
		IgnoreInternetHostnames:  query.Get("nointernethostnames") == "1",
		IgnoreInternetPorts:      query.Get("nointernetports") == "1",
		IgnoreInternetEmails:     query.Get("nointernetemails") == "1",
		IgnoreModemFlags:         query.Get("nomodemflags") == "1",
	}
	
	// Get changes
	changes, err := s.storage.GetNodeChanges(zone, net, node, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Determine if node is currently active by checking if last history entry is from the most recent nodelist
	var currentlyActive bool
	if len(history) > 0 {
		// Get the most recent nodelist date efficiently
		maxDate, err := s.storage.GetMaxNodelistDate()
		if err == nil {
			currentlyActive = history[len(history)-1].NodelistDate.Equal(maxDate)
		}
	}
	
	data := struct {
		Title           string
		Address         string
		Zone            int
		Net             int
		Node            int
		History         []database.Node
		Changes         []database.NodeChange
		FirstDate       time.Time
		LastDate        time.Time
		CurrentlyActive bool
		Filter          storage.ChangeFilter
	}{
		Title:           "Node History",
		Address:         fmt.Sprintf("%d:%d/%d", zone, net, node),
		Zone:            zone,
		Net:             net,
		Node:            node,
		History:         history,
		Changes:         changes,
		FirstDate:       firstDate,
		LastDate:        lastDate,
		CurrentlyActive: currentlyActive,
		Filter:          filter,
	}
	
	s.templates["node_history"].Execute(w, data)
}