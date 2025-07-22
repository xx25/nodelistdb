package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
)

// AnalyticsHandler handles the analytics main page
func (s *Server) AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "FidoNet Analytics",
	}
	
	s.templates["analytics"].Execute(w, data)
}

// V34AnalyticsHandler handles V.34 modem analysis
func (s *Server) V34AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	
	report, err := s.storage.GetV34ModemReport()
	if err != nil {
		data := struct {
			Title string
			Error string
		}{
			Title: "V.34 Modem Analysis",
			Error: fmt.Sprintf("Failed to generate V.34 analysis: %v", err),
		}
		s.templates["v34_analytics"].Execute(w, data)
		return
	}
	
	data := struct {
		Title     string
		Report    *database.V34ModemReport
		QueryTime time.Duration
	}{
		Title:     "V.34 Modem Analysis",
		Report:    report,
		QueryTime: time.Since(startTime),
	}
	
	s.templates["v34_analytics"].Execute(w, data)
}

// BinkpAnalyticsHandler handles Binkp protocol analysis
func (s *Server) BinkpAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	
	report, err := s.storage.GetBinkpReport()
	if err != nil {
		data := struct {
			Title string
			Error string
		}{
			Title: "Binkp Protocol Analysis",
			Error: fmt.Sprintf("Failed to generate Binkp analysis: %v", err),
		}
		s.templates["binkp_analytics"].Execute(w, data)
		return
	}
	
	data := struct {
		Title     string
		Report    *database.BinkpReport
		QueryTime time.Duration
	}{
		Title:     "Binkp Protocol Analysis",
		Report:    report,
		QueryTime: time.Since(startTime),
	}
	
	s.templates["binkp_analytics"].Execute(w, data)
}

// NetworkLifecycleHandler handles network lifecycle analysis
func (s *Server) NetworkLifecycleHandler(w http.ResponseWriter, r *http.Request) {
	var zone, net int
	var err error
	
	// Try to parse from query parameters first (/analytics/network/?zone=2&net=5001)
	zoneStr := r.URL.Query().Get("zone")
	netStr := r.URL.Query().Get("net")
	
	if zoneStr != "" && netStr != "" {
		zone, err = strconv.Atoi(zoneStr)
		if err != nil {
			http.Error(w, "Invalid zone parameter", http.StatusBadRequest)
			return
		}
		
		net, err = strconv.Atoi(netStr)
		if err != nil {
			http.Error(w, "Invalid net parameter", http.StatusBadRequest)
			return
		}
	} else {
		// Fall back to URL path parsing /analytics/network/{zone}/{net}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 5 {
			http.Error(w, "Invalid network address. Use /analytics/network/?zone=X&net=Y or /analytics/network/X/Y", http.StatusBadRequest)
			return
		}
		
		zone, err = strconv.Atoi(parts[3])
		if err != nil {
			http.Error(w, "Invalid zone in URL path", http.StatusBadRequest)
			return
		}
		
		net, err = strconv.Atoi(parts[4])
		if err != nil {
			http.Error(w, "Invalid net in URL path", http.StatusBadRequest)
			return
		}
	}
	
	startTime := time.Now()
	report, err := s.storage.GetNetworkLifecycleReport(zone, net)
	if err != nil {
		data := struct {
			Title string
			Error string
			Zone  int
			Net   int
		}{
			Title: "Network Lifecycle Analysis",
			Error: fmt.Sprintf("Failed to generate network analysis: %v", err),
			Zone:  zone,
			Net:   net,
		}
		s.templates["network_lifecycle"].Execute(w, data)
		return
	}
	
	data := struct {
		Title     string
		Report    *database.NetworkLifecycleReport
		QueryTime time.Duration
	}{
		Title:     "Network Lifecycle Analysis",
		Report:    report,
		QueryTime: time.Since(startTime),
	}
	
	s.templates["network_lifecycle"].Execute(w, data)
}

// SysopNamesHandler handles sysop name analysis
func (s *Server) SysopNamesHandler(w http.ResponseWriter, r *http.Request) {
	// Get year parameter, default to current year
	yearStr := r.URL.Query().Get("year")
	year := time.Now().Year()
	if yearStr != "" {
		if parsedYear, err := strconv.Atoi(yearStr); err == nil && parsedYear > 1980 && parsedYear <= time.Now().Year() {
			year = parsedYear
		}
	}
	
	startTime := time.Now()
	report, err := s.storage.GetSysopNameReportByYear(year)
	if err != nil {
		data := struct {
			Title string
			Error string
			Year  int
		}{
			Title: "Sysop Name Analysis",
			Error: fmt.Sprintf("Failed to generate sysop name analysis: %v", err),
			Year:  year,
		}
		s.templates["sysop_analytics"].Execute(w, data)
		return
	}
	
	data := struct {
		Title     string
		Report    *database.SysopNameReport
		QueryTime time.Duration
	}{
		Title:     "Sysop Name Analysis",
		Report:    report,
		QueryTime: time.Since(startTime),
	}
	
	s.templates["sysop_analytics"].Execute(w, data)
}

// ProtocolTrendHandler handles protocol adoption trend analysis
func (s *Server) ProtocolTrendHandler(w http.ResponseWriter, r *http.Request) {
	protocol := r.URL.Query().Get("protocol")
	if protocol == "" {
		protocol = "V34" // Default
	}
	
	startTime := time.Now()
	report, err := s.storage.GetProtocolAdoptionTrend(protocol)
	if err != nil {
		data := struct {
			Title    string
			Error    string
			Protocol string
		}{
			Title:    "Protocol Adoption Trends",
			Error:    fmt.Sprintf("Failed to generate protocol trend analysis: %v", err),
			Protocol: protocol,
		}
		s.templates["protocol_trend"].Execute(w, data)
		return
	}
	
	data := struct {
		Title     string
		Report    *database.ProtocolAdoptionReport
		QueryTime time.Duration
	}{
		Title:     "Protocol Adoption Trends",
		Report:    report,
		QueryTime: time.Since(startTime),
	}
	
	s.templates["protocol_trend"].Execute(w, data)
}