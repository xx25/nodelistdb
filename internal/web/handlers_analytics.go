package web

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// analyticsParams holds common query parameters for analytics handlers
type analyticsParams struct {
	Days             int
	Limit            int
	IncludeZeroNodes bool
	ValidationError  string
}

// parseAnalyticsParams extracts common analytics parameters from HTTP request
func parseAnalyticsParams(r *http.Request) analyticsParams {
	query := r.URL.Query()
	var validationError string

	// Days parameter (default: 30, max: 365)
	days := 30
	if daysStr := query.Get("days"); daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		} else {
			validationError = "Invalid 'days' parameter (must be 1-365)"
		}
	}

	// Limit parameter (default: 1000, max: 1000)
	limit := 1000
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		} else if validationError == "" {
			validationError = "Invalid 'limit' parameter (must be 1-1000)"
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	return analyticsParams{
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		ValidationError:  validationError,
	}
}

// protocolAnalyticsData holds template data for protocol analytics pages
type protocolAnalyticsData struct {
	Title            string
	ActivePage       string
	Version          string
	ProtocolNodes    []storage.NodeTestResult
	Days             int
	Limit            int
	IncludeZeroNodes bool
	Error            error
}

// protocolNodesFetcher is a function type for fetching protocol-specific nodes
type protocolNodesFetcher func(limit, days int, includeZeroNodes bool) ([]storage.NodeTestResult, error)

// renderProtocolAnalytics is a generic handler for protocol analytics pages
func (s *Server) renderProtocolAnalytics(
	w http.ResponseWriter,
	r *http.Request,
	title string,
	templateName string,
	fetcher protocolNodesFetcher,
) {
	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch protocol nodes
	protocolNodes, err := fetcher(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] %s: Error fetching nodes: %v", title, err)
		protocolNodes = []storage.NodeTestResult{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := protocolAnalyticsData{
		Title:            title,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    protocolNodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
	}

	// Check template exists before rendering
	tmpl, exists := s.templates[templateName]
	if !exists {
		log.Printf("[ERROR] %s: Template '%s' not found", title, templateName)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] %s: Error executing template: %v", title, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// AnalyticsHandler shows the analytics page
func (s *Server) AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AnalyticsFlagHandler handles flag analytics requests
func (s *Server) AnalyticsFlagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/analytics", http.StatusSeeOther)
		return
	}

	flag := r.FormValue("flag")

	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Flag:       flag,
		Version:    version.GetVersionInfo(),
	}

	if flag == "" {
		data.Error = fmt.Errorf("Flag cannot be empty")
	} else {
		// Get first appearance
		firstAppearance, err := s.storage.AnalyticsOps().GetFlagFirstAppearance(flag)
		if err != nil {
			data.Error = fmt.Errorf("Failed to get first appearance: %v", err)
		} else {
			data.FirstAppearance = firstAppearance
		}

		// Get yearly usage
		if data.Error == nil {
			yearlyUsage, err := s.storage.AnalyticsOps().GetFlagUsageByYear(flag)
			if err != nil {
				data.Error = fmt.Errorf("Failed to get yearly usage: %v", err)
			} else {
				data.YearlyUsage = yearlyUsage
			}
		}
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AnalyticsNetworkHandler handles network analytics requests
func (s *Server) AnalyticsNetworkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/analytics", http.StatusSeeOther)
		return
	}

	network := r.FormValue("network")

	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Network:    network,
		Version:    version.GetVersionInfo(),
	}

	if network == "" {
		data.Error = fmt.Errorf("Please enter a network address (e.g., 2:5000)")
	} else {
		// Parse network address (zone:net)
		var zone, net int
		_, err := fmt.Sscanf(network, "%d:%d", &zone, &net)
		if err != nil {
			data.Error = fmt.Errorf("Invalid network format. Use zone:net (e.g., 2:5000)")
		} else {
			// Get network history
			history, err := s.storage.AnalyticsOps().GetNetworkHistory(zone, net)
			if err != nil {
				data.Error = fmt.Errorf("Failed to fetch network history: %v", err)
			} else if history == nil {
				data.Error = fmt.Errorf("Network %s not found", network)
			} else {
				data.NetworkHistory = history
			}
		}
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ipv6AnalyticsData holds template data for IPv6 analytics pages
type ipv6AnalyticsData struct {
	Title            string
	ActivePage       string
	Version          string
	IPv6Nodes        []storage.NodeTestResult
	Days             int
	Limit            int
	IncludeZeroNodes bool
	Error            error
}

// renderIPv6Analytics is a generic handler for IPv6 analytics pages
func (s *Server) renderIPv6Analytics(
	w http.ResponseWriter,
	r *http.Request,
	title string,
	templateName string,
	fetcher protocolNodesFetcher,
) {
	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch IPv6 nodes
	ipv6Nodes, err := fetcher(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] %s: Error fetching nodes: %v", title, err)
		ipv6Nodes = []storage.NodeTestResult{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := ipv6AnalyticsData{
		Title:            title,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		IPv6Nodes:        ipv6Nodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
	}

	// Check template exists before rendering
	tmpl, exists := s.templates[templateName]
	if !exists {
		log.Printf("[ERROR] %s: Template '%s' not found", title, templateName)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] %s: Error executing template: %v", title, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// IPv6AnalyticsHandler shows IPv6 enabled nodes analytics
func (s *Server) IPv6AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderIPv6Analytics(w, r, "IPv6 Enabled Nodes", "ipv6_analytics", s.storage.GetIPv6EnabledNodes)
}

// IPv6NonWorkingAnalyticsHandler shows IPv6 nodes with non-working services
func (s *Server) IPv6NonWorkingAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderIPv6Analytics(w, r, "IPv6 Non-Working Nodes", "ipv6_nonworking_analytics", s.storage.GetIPv6NonWorkingNodes)
}

// BinkPAnalyticsHandler shows BinkP enabled nodes analytics
func (s *Server) BinkPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderProtocolAnalytics(w, r, "BinkP Enabled Nodes", "binkp_analytics", s.storage.GetBinkPEnabledNodes)
}

// IfcicoAnalyticsHandler shows IFCICO enabled nodes analytics
func (s *Server) IfcicoAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderProtocolAnalytics(w, r, "IFCICO Enabled Nodes", "ifcico_analytics", s.storage.GetIfcicoEnabledNodes)
}

// BinkPSoftwareHandler shows BinkP software distribution analytics
func (s *Server) BinkPSoftwareHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "BinkP Software Distribution",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["binkp_software"].Execute(w, data); err != nil {
		log.Printf("Error executing BinkP software template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// IfcicoSoftwareHandler shows IFCICO software distribution analytics
func (s *Server) IfcicoSoftwareHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "IFCICO Software Distribution",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["ifcico_software"].Execute(w, data); err != nil {
		log.Printf("Error executing IFCICO software template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TelnetAnalyticsHandler shows Telnet enabled nodes analytics
func (s *Server) TelnetAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderProtocolAnalytics(w, r, "Telnet Enabled Nodes", "telnet_analytics", s.storage.GetTelnetEnabledNodes)
}

// VModemAnalyticsHandler shows VModem enabled nodes analytics
func (s *Server) VModemAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderProtocolAnalytics(w, r, "VModem Enabled Nodes", "vmodem_analytics", s.storage.GetVModemEnabledNodes)
}

// FTPAnalyticsHandler shows FTP enabled nodes analytics
func (s *Server) FTPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	s.renderProtocolAnalytics(w, r, "FTP Enabled Nodes", "ftp_analytics", s.storage.GetFTPEnabledNodes)
}
