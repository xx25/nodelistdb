package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/nodelistdb/internal/flags"
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
	Config           ProtocolPageConfig  // Configuration for the page
	ProcessedInfo    []template.HTML     // Processed InfoText with days substituted
}

// protocolNodesFetcher is a function type for fetching protocol-specific nodes
type protocolNodesFetcher func(limit, days int, includeZeroNodes bool) ([]storage.NodeTestResult, error)

// renderProtocolAnalytics is a generic handler for protocol analytics pages
// Updated to use ProtocolPageConfig for configuration-driven rendering
func (s *Server) renderProtocolAnalytics(
	w http.ResponseWriter,
	r *http.Request,
	config ProtocolPageConfig,
	fetcher protocolNodesFetcher,
) {
	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch protocol nodes
	protocolNodes, err := fetcher(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] %s: Error fetching nodes: %v", config.PageTitle, err)
		protocolNodes = []storage.NodeTestResult{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := protocolAnalyticsData{
		Title:            config.PageTitle,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    protocolNodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
		Config:           config,
		ProcessedInfo:    config.processInfoText(params.Days),
	}

	// Use unified template
	tmpl, exists := s.templates["unified_analytics"]
	if !exists {
		log.Printf("[ERROR] %s: Template 'unified_analytics' not found", config.PageTitle)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] %s: Error executing template: %v", config.PageTitle, err)
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

// IPv6PageConfig holds page-specific configuration for IPv6 analytics pages
type IPv6PageConfig struct {
	PageTitle       string        // Page title (e.g., "IPv6 Enabled Nodes")
	PageSubtitle    template.HTML // HTML subtitle text
	StatsHeading    string        // Text for "Found X [StatsHeading] Nodes"
	TableLayout     string        // "standard" or "dual-protocol"
	ProtocolColumn  string        // "ipv6", "ipv4", or "both"
	InfoText        []string      // Paragraphs for info box (can use %d for days)
	EmptyStateTitle string        // Title for empty state
	EmptyStateDesc  string        // Description for empty state
}

// processInfoText converts InfoText strings to template.HTML, substituting %d with days
func (c *IPv6PageConfig) processInfoText(days int) []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
		// Only substitute %d if the text contains it, otherwise use text as-is
		// This prevents "EXTRA int" errors when InfoText strings don't have %d
		var processed string
		if containsFormatVerb(text) {
			processed = fmt.Sprintf(text, days)
		} else {
			processed = text
		}
		result[i] = template.HTML(processed)
	}
	return result
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
	Config           IPv6PageConfig
	ProcessedInfo    []template.HTML // Processed InfoText with days substituted
}

// renderIPv6Analytics is a generic handler for IPv6 analytics pages
func (s *Server) renderIPv6Analytics(
	w http.ResponseWriter,
	r *http.Request,
	config IPv6PageConfig,
	templateName string,
	fetcher protocolNodesFetcher,
) {
	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch IPv6 nodes
	ipv6Nodes, err := fetcher(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] %s: Error fetching nodes: %v", config.PageTitle, err)
		ipv6Nodes = []storage.NodeTestResult{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := ipv6AnalyticsData{
		Title:            config.PageTitle,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		IPv6Nodes:        ipv6Nodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
		Config:           config,
		ProcessedInfo:    config.processInfoText(params.Days),
	}

	// Check template exists before rendering
	tmpl, exists := s.templates[templateName]
	if !exists {
		log.Printf("[ERROR] %s: Template '%s' not found", config.PageTitle, templateName)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] %s: Error executing template: %v", config.PageTitle, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// IPv6AnalyticsHandler shows IPv6 enabled nodes analytics
func (s *Server) IPv6AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := IPv6PageConfig{
		PageTitle:    "IPv6 Enabled Nodes",
		PageSubtitle: `<p class="subtitle">Nodes that have been successfully tested with IPv6 connectivity</p>`,
		StatsHeading: "IPv6 Enabled",
		TableLayout:  "standard",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have IPv6 addresses resolved and have been successfully tested with at least one protocol over the last %d days. All listed nodes have working IPv6 connectivity.`,
		},
		EmptyStateTitle: "No IPv6 enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with IPv6 addresses were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderIPv6Analytics(w, r, config, "ipv6_analytics_generic", s.storage.GetIPv6EnabledNodes)
}

// IPv6NonWorkingAnalyticsHandler shows IPv6 nodes with non-working services
func (s *Server) IPv6NonWorkingAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := IPv6PageConfig{
		PageTitle:    "IPv6 Non-Working Nodes",
		PageSubtitle: `<p class="subtitle">Nodes with IPv6 addresses but no working IPv6 services</p>`,
		StatsHeading: "IPv6 Non-Working",
		TableLayout:  "standard",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have IPv6 addresses configured but have not responded successfully to any IPv6 protocol tests over the last %d days. This may indicate connectivity issues, firewall problems, or incomplete IPv6 deployment.`,
		},
		EmptyStateTitle: "No IPv6 non-working nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that all nodes with IPv6 addresses have at least one working IPv6 service, or no IPv6 nodes were tested during this period.",
	}
	s.renderIPv6Analytics(w, r, config, "ipv6_analytics_generic", s.storage.GetIPv6NonWorkingNodes)
}

// IPv6AdvertisedIPv4OnlyAnalyticsHandler shows nodes that advertise IPv6 but are only accessible via IPv4
func (s *Server) IPv6AdvertisedIPv4OnlyAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := IPv6PageConfig{
		PageTitle:    "IPv6 Advertised, IPv4 Only",
		PageSubtitle: `<p class="subtitle">Nodes advertising IPv6 capability but only accessible via IPv4</p>`,
		StatsHeading: "IPv6 Advertised, IPv4 Only",
		TableLayout:  "dual-protocol",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that advertise IPv6 capability (either in the nodelist or via DNS resolution) but are currently only accessible via IPv4 over the last %d days. These nodes have working IPv4 services (BinkP, IFCICO, or Telnet) but all IPv6 services are non-functional. This may indicate IPv6 misconfiguration, firewall issues, or incomplete IPv6 deployment.`,
		},
		EmptyStateTitle: "No IPv6-advertised IPv4-only nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that all nodes with IPv6 addresses have at least one working IPv6 service, or no nodes meet the criteria of having working IPv4 but failing IPv6 during this period.",
	}
	s.renderIPv6Analytics(w, r, config, "ipv6_analytics_generic", s.storage.GetIPv6AdvertisedIPv4OnlyNodes)
}

// IPv6OnlyNodesHandler shows nodes that have working IPv6 services but NO working IPv4 services
func (s *Server) IPv6OnlyNodesHandler(w http.ResponseWriter, r *http.Request) {
	config := IPv6PageConfig{
		PageTitle:    "IPv6 Only Nodes (Non-Working IPv4)",
		PageSubtitle: `<p class="subtitle">Nodes with working IPv6 services but NO working IPv4 services (IPv4 may be configured but not working)</p>`,
		StatsHeading: "IPv6 Only",
		TableLayout:  "standard",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have working IPv6 connectivity but NO working IPv4 services over the last %d days. These nodes may have IPv4 addresses configured, but the IPv4 protocols failed or were not tested.`,
			`For nodes that ONLY advertise IPv6 addresses (no IPv4 at all), see "Pure IPv6 Only Nodes".`,
		},
		EmptyStateTitle: "No IPv6-only nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that all nodes with working IPv6 also have working IPv4 services, or no IPv6 nodes were tested during this period.",
	}
	s.renderIPv6Analytics(w, r, config, "ipv6_analytics_generic", s.storage.GetIPv6OnlyNodes)
}

// PureIPv6OnlyNodesHandler shows nodes that ONLY advertise IPv6 addresses (no IPv4 addresses at all)
func (s *Server) PureIPv6OnlyNodesHandler(w http.ResponseWriter, r *http.Request) {
	config := IPv6PageConfig{
		PageTitle:    "Pure IPv6 Only Nodes",
		PageSubtitle: `<p class="subtitle">Nodes that ONLY advertise IPv6 addresses (no IPv4 addresses configured)</p>`,
		StatsHeading: "Pure IPv6 Only",
		TableLayout:  "standard",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that ONLY advertise IPv6 addresses (no IPv4 addresses configured at all) over the last %d days. These are true pure IPv6-only nodes.`,
			`This is different from "IPv6 Only Nodes (Non-Working IPv4)" which shows nodes that have IPv4 addresses configured but the IPv4 services don't work.`,
		},
		EmptyStateTitle: "No pure IPv6-only nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that all IPv6 nodes also have IPv4 addresses configured, or no such nodes were tested during this period.",
	}
	s.renderIPv6Analytics(w, r, config, "ipv6_analytics_generic", s.storage.GetPureIPv6OnlyNodes)
}

// BinkPAnalyticsHandler shows BinkP enabled nodes analytics
func (s *Server) BinkPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := ProtocolPageConfig{
		PageTitle:    "BinkP Enabled Nodes",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes that have been successfully tested with BinkP protocol</p>`),
		StatsHeading: "BinkP Enabled",
		ShowVersion:  true,
		VersionField: "BinkPVersion",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have been successfully tested with BinkP protocol over the last %d days. BinkP is a modern, efficient protocol for FidoNet mail exchange over TCP/IP.`,
		},
		EmptyStateTitle: "No BinkP enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with BinkP support were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderProtocolAnalytics(w, r, config, s.storage.GetBinkPEnabledNodes)
}

// IfcicoAnalyticsHandler shows IFCICO enabled nodes analytics
func (s *Server) IfcicoAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := ProtocolPageConfig{
		PageTitle:    "IFCICO Enabled Nodes",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes that have been successfully tested with IFCICO protocol</p>`),
		StatsHeading: "IFCICO Enabled",
		ShowVersion:  true,
		VersionField: "IfcicoMailerInfo",
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have been successfully tested with IFCICO protocol over the last %d days. IFCICO is a traditional FidoNet mailer protocol.`,
		},
		EmptyStateTitle: "No IFCICO enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with IFCICO support were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderProtocolAnalytics(w, r, config, s.storage.GetIfcicoEnabledNodes)
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
	config := ProtocolPageConfig{
		PageTitle:    "Telnet Enabled Nodes",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes that have been successfully tested with Telnet protocol</p>`),
		StatsHeading: "Telnet Enabled",
		ShowVersion:  false,
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have been successfully tested with Telnet protocol over the last %d days. Telnet is commonly used for BBS access in the FidoNet community.`,
		},
		EmptyStateTitle: "No Telnet enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with Telnet support were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderProtocolAnalytics(w, r, config, s.storage.GetTelnetEnabledNodes)
}

// VModemAnalyticsHandler shows VModem enabled nodes analytics
func (s *Server) VModemAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := ProtocolPageConfig{
		PageTitle:    "VModem Enabled Nodes",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes that have been successfully tested with VModem protocol</p>`),
		StatsHeading: "VModem Enabled",
		ShowVersion:  false,
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have been successfully tested with VModem protocol over the last %d days. VModem provides virtual modem emulation for legacy BBS software.`,
		},
		EmptyStateTitle: "No VModem enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with VModem support were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderProtocolAnalytics(w, r, config, s.storage.GetVModemEnabledNodes)
}

// FTPAnalyticsHandler shows FTP enabled nodes analytics
func (s *Server) FTPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := ProtocolPageConfig{
		PageTitle:    "FTP Enabled Nodes",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes that have been successfully tested with FTP protocol</p>`),
		StatsHeading:  "FTP Enabled",
		ShowVersion:   false,
		ShowAnonLogin: true,
		InfoText: []string{
			`<strong>Note:</strong> This report shows nodes that have been successfully tested with FTP protocol over the last %d days. FTP is used for file distribution and downloads in the FidoNet network.`,
		},
		EmptyStateTitle: "No FTP enabled nodes found for the selected period.",
		EmptyStateDesc:  "This could mean that either no nodes with FTP support were tested during this period, or none of them responded successfully to protocol tests.",
	}
	s.renderProtocolAnalytics(w, r, config, s.storage.GetFTPEnabledNodes)
}

// akaMismatchAnalyticsData holds template data for AKA mismatch analytics pages
type akaMismatchAnalyticsData struct {
	Title              string
	ActivePage         string
	Version            string
	MismatchNodes      []storage.NodeTestResult
	IPv6IncorrectNodes []storage.AKAIPVersionMismatchNode
	IPv4IncorrectNodes []storage.AKAIPVersionMismatchNode
	Days               int
	Limit              int
	IncludeZeroNodes   bool
	Error              error
	Config             AKAMismatchPageConfig
	ProcessedInfo      []template.HTML
}

// AKAMismatchAnalyticsHandler shows nodes with AKA address mismatches
func (s *Server) AKAMismatchAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := AKAMismatchPageConfig{
		PageTitle:    "Nodes with AKA Mismatch",
		PageSubtitle: template.HTML(`<p class="subtitle">Nodes where the announced AKA address doesn't match the expected nodelist address</p>`),
		StatsHeading: "AKA Mismatch",
		InfoText: []string{
			`<strong>What is an AKA mismatch?</strong> During BinkP or IFCICO handshakes, nodes announce their addresses (AKAs). An AKA mismatch occurs when the expected nodelist address (zone:net/node) is not found in the list of addresses the node announces.`,
			`<strong>Common causes:</strong> Misconfigured mailer software, outdated address lists, node address changes, or AKA consolidation where a node responds for multiple addresses.`,
			`<strong>Note:</strong> This report shows nodes that were operational (responded successfully) but announced different addresses than expected over the last %d days.`,
		},
		EmptyStateTitle: "No AKA mismatches found for the selected period.",
		EmptyStateDesc:  "This means all operational nodes announced their expected addresses correctly, or no nodes met the criteria during this period.",
	}

	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch mismatch nodes
	mismatchNodes, err := s.storage.GetAKAMismatchNodes(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] AKA Mismatch Analytics: Error fetching nodes: %v", err)
		mismatchNodes = []storage.NodeTestResult{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Fetch IPv4/IPv6 AKA discrepancy nodes
	var ipv6IncorrectNodes []storage.AKAIPVersionMismatchNode
	var ipv4IncorrectNodes []storage.AKAIPVersionMismatchNode

	if ipv6Inc, err := s.storage.GetIPv6IncorrectIPv4CorrectNodes(params.Limit, params.Days, params.IncludeZeroNodes); err != nil {
		log.Printf("[ERROR] AKA Mismatch Analytics: Error fetching IPv6 incorrect nodes: %v", err)
	} else {
		ipv6IncorrectNodes = ipv6Inc
	}

	if ipv4Inc, err := s.storage.GetIPv4IncorrectIPv6CorrectNodes(params.Limit, params.Days, params.IncludeZeroNodes); err != nil {
		log.Printf("[ERROR] AKA Mismatch Analytics: Error fetching IPv4 incorrect nodes: %v", err)
	} else {
		ipv4IncorrectNodes = ipv4Inc
	}

	// Build template data
	data := akaMismatchAnalyticsData{
		Title:              config.PageTitle,
		ActivePage:         "analytics",
		Version:            version.GetVersionInfo(),
		MismatchNodes:      mismatchNodes,
		IPv6IncorrectNodes: ipv6IncorrectNodes,
		IPv4IncorrectNodes: ipv4IncorrectNodes,
		Days:               params.Days,
		Limit:              params.Limit,
		IncludeZeroNodes:   params.IncludeZeroNodes,
		Error:              displayError,
		Config:             config,
		ProcessedInfo:      config.processInfoText(params.Days),
	}

	// Use AKA mismatch analytics template
	tmpl, exists := s.templates["aka_mismatch_analytics"]
	if !exists {
		log.Printf("[ERROR] AKA Mismatch Analytics: Template 'aka_mismatch_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] AKA Mismatch Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// otherNetworksAnalyticsData holds template data for the other networks analytics page
type otherNetworksAnalyticsData struct {
	Title            string
	ActivePage       string
	Version          string
	Networks         []storage.OtherNetworkSummary
	Days             int
	Limit            int
	IncludeZeroNodes bool
	Error            error
	Config           OtherNetworksPageConfig
	ProcessedInfo    []template.HTML
}

// OtherNetworksAnalyticsHandler shows networks found in AKA addresses
func (s *Server) OtherNetworksAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	config := OtherNetworksPageConfig{
		PageTitle:    "Other Networks",
		PageSubtitle: template.HTML(`<p class="subtitle">FidoNet nodes that also participate in other FTN-style networks</p>`),
		StatsHeading: "Networks",
		InfoText: []string{
			`<strong>What are other networks?</strong> Many FidoNet sysops also run nodes on other FTN-style networks like FSXNet, TQWNet, WhisperNet, and others. During BinkP or IFCICO handshakes, nodes announce all their addresses (AKAs), including addresses on these other networks.`,
			`<strong>Format:</strong> Other network addresses are announced as <code>zone:net/node@network</code>, for example <code>21:1/100@fsxnet</code> or <code>1337:2/106@tqwnet</code>.`,
			`<strong>Note:</strong> This report shows networks detected from operational nodes over the last %d days. Click on a network name to see all FidoNet nodes that participate in that network.`,
		},
		EmptyStateTitle: "No other networks found for the selected period.",
		EmptyStateDesc:  "This means no operational nodes announced addresses in other networks, or no nodes met the criteria during this period.",
	}

	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch networks summary
	networks, err := s.storage.GetOtherNetworksSummary(params.Days)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] Other Networks Analytics: Error fetching networks: %v", err)
		networks = []storage.OtherNetworkSummary{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := otherNetworksAnalyticsData{
		Title:            config.PageTitle,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		Networks:         networks,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
		Config:           config,
		ProcessedInfo:    config.processInfoText(params.Days),
	}

	// Use other networks analytics template
	tmpl, exists := s.templates["other_networks_analytics"]
	if !exists {
		log.Printf("[ERROR] Other Networks Analytics: Template 'other_networks_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Other Networks Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// otherNetworkNodesData holds template data for the network nodes detail page
type otherNetworkNodesData struct {
	Title            string
	ActivePage       string
	Version          string
	NetworkName      string
	Nodes            []storage.OtherNetworkNode
	Days             int
	Limit            int
	IncludeZeroNodes bool
	Error            error
	Config           OtherNetworksPageConfig
	ProcessedInfo    []template.HTML
}

// OtherNetworkNodesHandler shows nodes in a specific network
func (s *Server) OtherNetworkNodesHandler(w http.ResponseWriter, r *http.Request) {
	networkName := r.URL.Query().Get("network")
	if networkName == "" {
		http.Redirect(w, r, "/analytics/other-networks", http.StatusFound)
		return
	}

	config := OtherNetworksPageConfig{
		PageTitle:    fmt.Sprintf("Nodes in %s", networkName),
		PageSubtitle: template.HTML(fmt.Sprintf(`<p class="subtitle">FidoNet nodes that also participate in the <strong>%s</strong> network</p>`, template.HTMLEscapeString(networkName))),
		StatsHeading: "Nodes",
		InfoText: []string{
			fmt.Sprintf(`These FidoNet nodes announced AKA addresses in the <strong>%s</strong> network during BinkP or IFCICO handshakes over the last %%d days.`, template.HTMLEscapeString(networkName)),
			`The "Network Address" column shows the address(es) announced for this specific network.`,
		},
		EmptyStateTitle: fmt.Sprintf("No nodes found in %s for the selected period.", networkName),
		EmptyStateDesc:  "This means no operational nodes announced addresses in this network during this period.",
	}

	// Parse common parameters
	params := parseAnalyticsParams(r)

	// Fetch nodes in network
	nodes, err := s.storage.GetNodesInNetwork(networkName, params.Limit, params.Days)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] Other Network Nodes: Error fetching nodes for %s: %v", networkName, err)
		nodes = []storage.OtherNetworkNode{}
		displayError = fmt.Errorf("Failed to fetch analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	// Build template data
	data := otherNetworkNodesData{
		Title:            config.PageTitle,
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		NetworkName:      networkName,
		Nodes:            nodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
		Config:           config,
		ProcessedInfo:    config.processInfoText(params.Days),
	}

	// Use other network nodes template
	tmpl, exists := s.templates["other_network_nodes"]
	if !exists {
		log.Printf("[ERROR] Other Network Nodes: Template 'other_network_nodes' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Other Network Nodes: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// PSTNSummaryStats holds summary statistics for PSTN nodes
type PSTNSummaryStats struct {
	TotalCount      int
	CMCount         int
	NonCMCount      int
	ZoneCounts      map[int]int
	SpeedTiers      map[string]int
	ModemFlagCounts map[string]int
}

// computePSTNStats calculates summary statistics from a list of PSTN nodes
func computePSTNStats(nodes []storage.PSTNNode) PSTNSummaryStats {
	stats := PSTNSummaryStats{
		TotalCount:      len(nodes),
		ZoneCounts:      make(map[int]int),
		SpeedTiers:      make(map[string]int),
		ModemFlagCounts: make(map[string]int),
	}
	for _, n := range nodes {
		if n.IsCM {
			stats.CMCount++
		} else {
			stats.NonCMCount++
		}
		stats.ZoneCounts[n.Zone]++
		switch {
		case n.MaxSpeed >= 28800:
			stats.SpeedTiers["28800+"]++
		case n.MaxSpeed >= 14400:
			stats.SpeedTiers["14400"]++
		case n.MaxSpeed >= 9600:
			stats.SpeedTiers["9600"]++
		default:
			stats.SpeedTiers["300-2400"]++
		}
		for _, mf := range n.ModemFlags {
			stats.ModemFlagCounts[mf]++
		}
	}
	return stats
}

// PSTNCMAnalyticsHandler shows all nodes with valid phone numbers from the latest nodelist
func (s *Server) PSTNCMAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit := 5000 // Default limit - high to capture all PSTN nodes
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}

	// Fetch ALL PSTN nodes (CM and non-CM)
	pstnNodes, err := s.storage.GetPSTNNodes(limit, 0)
	var displayError error
	if err != nil {
		log.Printf("[ERROR] PSTN Analytics: Error fetching nodes: %v", err)
		pstnNodes = []storage.PSTNNode{}
		displayError = fmt.Errorf("Failed to fetch PSTN analytics data. Please try again later")
	}

	// Compute summary statistics
	stats := computePSTNStats(pstnNodes)

	// Build template data
	data := struct {
		Title      string
		ActivePage string
		Version    string
		PSTNNodes  []storage.PSTNNode
		Stats      PSTNSummaryStats
		Limit      int
		Error      error
	}{
		Title:      "PSTN Nodes (Phone Access)",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
		PSTNNodes:  pstnNodes,
		Stats:      stats,
		Limit:      limit,
		Error:      displayError,
	}

	// Use PSTN analytics template
	tmpl, exists := s.templates["pstn_analytics"]
	if !exists {
		log.Printf("[ERROR] PSTN Analytics: Template 'pstn_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] PSTN Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ModemAccessibleSpeedTier holds count for a single speed tier (for ordered display)
type ModemAccessibleSpeedTier struct {
	Tier  string
	Count int
}

// ModemAccessibleProtocolCount holds count for a single modem protocol (for ordered display)
type ModemAccessibleProtocolCount struct {
	Protocol string
	Count    int
}

// ModemAccessibleOperatorCount holds count for a single operator (for ordered display)
type ModemAccessibleOperatorCount struct {
	Operator string
	Count    int
}

// ModemAccessibleZoneCount holds count for a single zone (for ordered display)
type ModemAccessibleZoneCount struct {
	Zone  int
	Count int
}

// ModemAccessibleSummaryStats holds summary statistics for modem-accessible nodes
type ModemAccessibleSummaryStats struct {
	TotalCount     int
	SpeedTiers     []ModemAccessibleSpeedTier
	ProtocolCounts []ModemAccessibleProtocolCount
	OperatorCounts []ModemAccessibleOperatorCount
	ZoneCounts     []ModemAccessibleZoneCount
}

// computeModemAccessibleStats calculates summary statistics from modem accessible nodes
func computeModemAccessibleStats(nodes []storage.ModemAccessibleNode) ModemAccessibleSummaryStats {
	stats := ModemAccessibleSummaryStats{
		TotalCount: len(nodes),
	}

	speedMap := make(map[string]int)
	protoMap := make(map[string]int)
	operMap := make(map[string]int)
	zoneMap := make(map[int]int)

	for _, n := range nodes {
		zoneMap[n.Zone]++

		// Speed tier classification (prefer real TX speed from line stats, fall back to connect speed)
		speed := n.ModemTxSpeed
		if speed == 0 {
			speed = n.ModemConnectSpeed
		}
		switch {
		case speed >= 28800:
			speedMap["28800+"]++
		case speed >= 14400:
			speedMap["14400"]++
		case speed >= 9600:
			speedMap["9600"]++
		case speed > 0:
			speedMap["300-2400"]++
		default:
			speedMap["Unknown"]++
		}

		// Protocol distribution
		proto := n.ModemProtocol
		if proto == "" {
			proto = "Unknown"
		}
		protoMap[proto]++

		// Operator distribution
		if n.ModemOperatorName != "" {
			operMap[n.ModemOperatorName]++
		}
	}

	// Convert maps to sorted slices
	// Speed tiers in logical order
	tierOrder := []string{"28800+", "14400", "9600", "300-2400", "Unknown"}
	for _, tier := range tierOrder {
		if count, ok := speedMap[tier]; ok {
			stats.SpeedTiers = append(stats.SpeedTiers, ModemAccessibleSpeedTier{Tier: tier, Count: count})
		}
	}

	// Protocols sorted by count descending
	stats.ProtocolCounts = sortedProtocolCounts(protoMap)

	// Operators sorted by count descending
	stats.OperatorCounts = sortedOperatorCounts(operMap)

	// Zones sorted by zone number
	stats.ZoneCounts = sortedModemZoneCounts(zoneMap)

	return stats
}

// sortedProtocolCounts converts protocol map to sorted slice (by count desc)
func sortedProtocolCounts(m map[string]int) []ModemAccessibleProtocolCount {
	result := make([]ModemAccessibleProtocolCount, 0, len(m))
	for proto, count := range m {
		result = append(result, ModemAccessibleProtocolCount{Protocol: proto, Count: count})
	}
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// sortedOperatorCounts converts operator map to sorted slice (by count desc)
func sortedOperatorCounts(m map[string]int) []ModemAccessibleOperatorCount {
	result := make([]ModemAccessibleOperatorCount, 0, len(m))
	for oper, count := range m {
		result = append(result, ModemAccessibleOperatorCount{Operator: oper, Count: count})
	}
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// sortedModemZoneCounts converts zone map to sorted slice (by zone number)
func sortedModemZoneCounts(m map[int]int) []ModemAccessibleZoneCount {
	result := make([]ModemAccessibleZoneCount, 0, len(m))
	for zone, count := range m {
		result = append(result, ModemAccessibleZoneCount{Zone: zone, Count: count})
	}
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Zone < result[i].Zone {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// ModemAccessibleAnalyticsHandler shows nodes successfully reached via modem calls
func (s *Server) ModemAccessibleAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	params := parseAnalyticsParams(r)

	nodes, err := s.storage.GetModemAccessibleNodes(params.Limit, params.Days, params.IncludeZeroNodes)
	var displayError error
	if err != nil {
		log.Printf("[ERROR] Modem Accessible Analytics: Error fetching nodes: %v", err)
		nodes = []storage.ModemAccessibleNode{}
		displayError = fmt.Errorf("Failed to fetch modem accessible analytics data. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	stats := computeModemAccessibleStats(nodes)

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		Nodes            []storage.ModemAccessibleNode
		Stats            ModemAccessibleSummaryStats
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "PSTN Accessible Nodes (Verified by Modem)",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		Nodes:            nodes,
		Stats:            stats,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
	}

	tmpl, exists := s.templates["pstn_accessible_analytics"]
	if !exists {
		log.Printf("[ERROR] Modem Accessible Analytics: Template 'pstn_accessible_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Modem Accessible Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// FileRequestFlagCount holds count for a single flag (for ordered display)
type FileRequestFlagCount struct {
	Flag  string
	Count int
}

// FileRequestZoneCount holds count for a single zone (for ordered display)
type FileRequestZoneCount struct {
	Zone  int
	Count int
}

// FileRequestSummaryStats holds summary statistics for file request nodes
type FileRequestSummaryStats struct {
	TotalCount      int
	FlagCounts      []FileRequestFlagCount // Ordered by flag name
	ZoneCounts      []FileRequestZoneCount // Ordered by zone number
	CapabilityStats struct {
		BarkFileRequest    int
		BarkUpdateRequest  int
		WaZOOFileRequest   int
		WaZOOUpdateRequest int
	}
}

// computeFileRequestStats calculates summary statistics from file request nodes
func computeFileRequestStats(nodes []storage.FileRequestNode) FileRequestSummaryStats {
	stats := FileRequestSummaryStats{
		TotalCount: len(nodes),
	}

	// Use maps for counting, then convert to ordered slices
	flagCountMap := make(map[string]int)
	zoneCountMap := make(map[int]int)

	for _, n := range nodes {
		flagCountMap[n.FileRequestFlag]++
		zoneCountMap[n.Zone]++

		// Calculate capability stats using existing flags package
		if caps, ok := flags.GetFileRequestCapabilities(n.FileRequestFlag); ok {
			if caps.BarkFileRequest {
				stats.CapabilityStats.BarkFileRequest++
			}
			if caps.BarkUpdateRequest {
				stats.CapabilityStats.BarkUpdateRequest++
			}
			if caps.WaZOOFileRequest {
				stats.CapabilityStats.WaZOOFileRequest++
			}
			if caps.WaZOOUpdateRequest {
				stats.CapabilityStats.WaZOOUpdateRequest++
			}
		}
	}

	// Convert flag counts to ordered slice (using flags.FileRequestFlagList order)
	for _, flag := range flags.FileRequestFlagList {
		if count, ok := flagCountMap[flag]; ok && count > 0 {
			stats.FlagCounts = append(stats.FlagCounts, FileRequestFlagCount{Flag: flag, Count: count})
		}
	}

	// Convert zone counts to ordered slice (sorted by zone number)
	// First collect zones, then sort
	var zones []int
	for zone := range zoneCountMap {
		zones = append(zones, zone)
	}
	// Sort zones (simple insertion sort for small number of zones)
	for i := 1; i < len(zones); i++ {
		for j := i; j > 0 && zones[j] < zones[j-1]; j-- {
			zones[j], zones[j-1] = zones[j-1], zones[j]
		}
	}
	for _, zone := range zones {
		stats.ZoneCounts = append(stats.ZoneCounts, FileRequestZoneCount{Zone: zone, Count: zoneCountMap[zone]})
	}

	return stats
}

// getFileRequestCapabilities is a template-safe wrapper for flags.GetFileRequestCapabilities
// Go templates require functions to return a single value or (value, error), not (value, bool)
func getFileRequestCapabilities(flag string) flags.FileRequestCapabilities {
	caps, _ := flags.GetFileRequestCapabilities(flag)
	return caps
}

// FileRequestAnalyticsHandler shows nodes with file request capabilities (XA-XX)
func (s *Server) FileRequestAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit := 5000 // Default limit - high to capture all file request nodes
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}

	// Fetch file request nodes
	fileRequestNodes, err := s.storage.GetFileRequestNodes(limit)
	var displayError error
	if err != nil {
		log.Printf("[ERROR] File Request Analytics: Error fetching nodes: %v", err)
		fileRequestNodes = []storage.FileRequestNode{}
		displayError = fmt.Errorf("Failed to fetch file request analytics data. Please try again later")
	}

	// Compute summary statistics
	stats := computeFileRequestStats(fileRequestNodes)

	// Build template data
	data := struct {
		Title            string
		ActivePage       string
		Version          string
		FileRequestNodes []storage.FileRequestNode
		Stats            FileRequestSummaryStats
		CapabilityMatrix []string
		GetCapabilities  func(string) flags.FileRequestCapabilities
		Limit            int
		Error            error
	}{
		Title:            "File Request Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		FileRequestNodes: fileRequestNodes,
		Stats:            stats,
		CapabilityMatrix: flags.FileRequestFlagList,
		GetCapabilities:  getFileRequestCapabilities,
		Limit:            limit,
		Error:            displayError,
	}

	// Use file request analytics template
	tmpl, exists := s.templates["filerequest_analytics"]
	if !exists {
		log.Printf("[ERROR] File Request Analytics: Template 'filerequest_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] File Request Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// IPv6WeeklyNewsHandler shows weekly IPv6 connectivity changes
func (s *Server) IPv6WeeklyNewsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse common parameters
	query := r.URL.Query()
	var validationError string

	// Limit parameter (default: 50, max: 1000)
	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		} else {
			validationError = "Invalid 'limit' parameter (must be 1-1000)"
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Fetch weekly news (uses cached version if CachedStorage is in use)
	news, err := s.storage.GetIPv6WeeklyNews(limit, includeZeroNodes)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] IPv6 Weekly News: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch weekly IPv6 news. Please try again later")
	} else if validationError != "" {
		displayError = fmt.Errorf("%s", validationError)
	}

	// Build template data
	data := struct {
		Title                  string
		ActivePage             string
		Version                string
		NewNodesWorking        []storage.NodeTestResult
		NewNodesNonWorking     []storage.NodeTestResult
		OldNodesLostIPv6       []storage.NodeTestResult
		OldNodesGainedIPv6     []storage.NodeTestResult
		Limit                  int
		IncludeZeroNodes       bool
		Error                  error
	}{
		Title:                  "Weekly IPv6 News",
		ActivePage:             "analytics",
		Version:                version.GetVersionInfo(),
		NewNodesWorking:        []storage.NodeTestResult{},
		NewNodesNonWorking:     []storage.NodeTestResult{},
		OldNodesLostIPv6:       []storage.NodeTestResult{},
		OldNodesGainedIPv6:     []storage.NodeTestResult{},
		Limit:                  limit,
		IncludeZeroNodes:       includeZeroNodes,
		Error:                  displayError,
	}

	if news != nil {
		data.NewNodesWorking = news.NewNodesWorking
		data.NewNodesNonWorking = news.NewNodesNonWorking
		data.OldNodesLostIPv6 = news.OldNodesLostIPv6
		data.OldNodesGainedIPv6 = news.OldNodesGainedIPv6
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["ipv6_weekly_news"]
	if !exists {
		log.Printf("[ERROR] IPv6 Weekly News: Template 'ipv6_weekly_news' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] IPv6 Weekly News: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// GeoHostingAnalyticsHandler shows geographic hosting distribution
func (s *Server) GeoHostingAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse days parameter (default: 365 for current year view)
	daysStr := r.URL.Query().Get("days")
	days := 365
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 3650 {
		days = d
	}

	// Get geo distribution
	dist, err := s.storage.GetGeoHostingDistribution(days)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] Geo Hosting Analytics: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch geo hosting distribution. Please try again later")
	}

	// Build template data
	data := struct {
		Title        string
		ActivePage   string
		Version      string
		Days         int
		Distribution *storage.GeoHostingDistribution
		Updated      string
		Error        error
	}{
		Title:        "Geographic Hosting Distribution",
		ActivePage:   "analytics",
		Version:      version.GetVersionInfo(),
		Days:         days,
		Distribution: dist,
		Error:        displayError,
	}

	if dist != nil {
		data.Updated = dist.LastUpdated.Format("2006-01-02 15:04:05")
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["geo_analytics"]
	if !exists {
		log.Printf("[ERROR] Geo Hosting Analytics: Template 'geo_analytics' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Geo Hosting Analytics: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// GeoCountryNodesHandler shows nodes for a specific country
func (s *Server) GeoCountryNodesHandler(w http.ResponseWriter, r *http.Request) {
	// Get country code from URL query
	countryCode := r.URL.Query().Get("code")
	if countryCode == "" {
		http.Error(w, "Country code is required", http.StatusBadRequest)
		return
	}

	// Parse days parameter (default: 365)
	daysStr := r.URL.Query().Get("days")
	days := 365
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 3650 {
		days = d
	}

	// Get nodes for country
	nodes, err := s.storage.GetNodesByCountry(countryCode, days)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] Geo Country Nodes: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch nodes for country. Please try again later")
		nodes = []storage.NodeTestResult{}
	}

	// Get country name from first node
	countryName := ""
	if len(nodes) > 0 {
		countryName = nodes[0].Country
	}

	// Build page config
	config := GeoPageConfig{
		PageTitle:       fmt.Sprintf("Nodes in %s", countryName),
		PageSubtitle:    template.HTML(fmt.Sprintf(`<p class="subtitle">Operational FidoNet nodes in %s (last %d days)</p>`, countryName, days)),
		StatsHeading:    "Nodes",
		ViewType:        "country",
		CountryCode:     countryCode,
		CountryName:     countryName,
		Days:            days,
		InfoText:        []string{},
		EmptyStateTitle: "No nodes found.",
		EmptyStateDesc:  "No operational nodes found for the selected country and time period.",
	}

	// Build template data
	data := struct {
		Title         string
		ActivePage    string
		Version       string
		Days          int
		GeoNodes      []storage.NodeTestResult
		Error         error
		Config        GeoPageConfig
		ProcessedInfo []template.HTML
	}{
		Title:         config.PageTitle,
		ActivePage:    "analytics",
		Version:       version.GetVersionInfo(),
		Days:          days,
		GeoNodes:      nodes,
		Error:         displayError,
		Config:        config,
		ProcessedInfo: config.processInfoText(),
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["geo_unified"]
	if !exists {
		log.Printf("[ERROR] Geo Country Nodes: Template 'geo_unified' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Geo Country Nodes: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// GeoProviderNodesHandler shows nodes for a specific provider
func (s *Server) GeoProviderNodesHandler(w http.ResponseWriter, r *http.Request) {
	// Get provider from URL query
	provider := r.URL.Query().Get("isp")
	if provider == "" {
		http.Error(w, "Provider is required", http.StatusBadRequest)
		return
	}

	// Parse days parameter (default: 365)
	daysStr := r.URL.Query().Get("days")
	days := 365
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 3650 {
		days = d
	}

	// Get nodes for provider
	nodes, err := s.storage.GetNodesByProvider(provider, days)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] Geo Provider Nodes: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch nodes for provider. Please try again later")
		nodes = []storage.NodeTestResult{}
	}

	// Build page config
	config := GeoPageConfig{
		PageTitle:       provider,
		PageSubtitle:    template.HTML(fmt.Sprintf(`<p class="subtitle">Operational FidoNet nodes hosted by %s (last %d days)</p>`, provider, days)),
		StatsHeading:    "Nodes",
		ViewType:        "provider",
		ProviderName:    provider,
		Days:            days,
		InfoText:        []string{},
		EmptyStateTitle: "No nodes found.",
		EmptyStateDesc:  "No operational nodes found for the selected provider and time period.",
	}

	// Build template data
	data := struct {
		Title         string
		ActivePage    string
		Version       string
		Days          int
		GeoNodes      []storage.NodeTestResult
		Error         error
		Config        GeoPageConfig
		ProcessedInfo []template.HTML
	}{
		Title:         config.PageTitle,
		ActivePage:    "analytics",
		Version:       version.GetVersionInfo(),
		Days:          days,
		GeoNodes:      nodes,
		Error:         displayError,
		Config:        config,
		ProcessedInfo: config.processInfoText(),
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["geo_unified"]
	if !exists {
		log.Printf("[ERROR] Geo Provider Nodes: Template 'geo_unified' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Geo Provider Nodes: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// OnThisDayHandler displays nodes that were first added on this day in previous years
func (s *Server) OnThisDayHandler(w http.ResponseWriter, r *http.Request) {
	// Use current date's month and day
	now := time.Now()
	month := int(now.Month())
	day := now.Day()

	// Parse optional limit parameter (default: 100, 0 = all)
	query := r.URL.Query()
	limit := 100
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed >= 0 {
			if parsed == 0 || parsed > 10000 {
				limit = 0 // 0 means all
			} else {
				limit = parsed
			}
		}
	}

	// Parse active only parameter (default: true)
	activeOnly := query.Get("active") != "0"

	// Fetch on this day nodes
	nodes, err := s.storage.GetOnThisDayNodes(month, day, limit, activeOnly)
	var displayError error

	if err != nil {
		log.Printf("[ERROR] On This Day: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch On This Day data. Please try again later")
		nodes = []storage.OnThisDayNode{}
	}

	// Group nodes by year for better display
	nodesByYear := make(map[int][]storage.OnThisDayNode)
	var years []int
	for _, n := range nodes {
		year := n.FirstAppeared.Year()
		if _, exists := nodesByYear[year]; !exists {
			years = append(years, year)
		}
		nodesByYear[year] = append(nodesByYear[year], n)
	}

	// Sort years in descending order (most recent first)
	for i := 0; i < len(years)-1; i++ {
		for j := i + 1; j < len(years); j++ {
			if years[j] > years[i] {
				years[i], years[j] = years[j], years[i]
			}
		}
	}

	// Build template data
	data := struct {
		Title       string
		ActivePage  string
		Version     string
		Month       int
		Day         int
		MonthName   string
		CurrentYear int
		TotalNodes  int
		NodesByYear map[int][]storage.OnThisDayNode
		Years       []int
		Limit       int
		ActiveOnly  bool
		Error       error
	}{
		Title:       "On This Day",
		ActivePage:  "analytics",
		Version:     version.GetVersionInfo(),
		Month:       month,
		Day:         day,
		MonthName:   now.Month().String(),
		CurrentYear: now.Year(),
		TotalNodes:  len(nodes),
		NodesByYear: nodesByYear,
		Years:       years,
		Limit:       limit,
		ActiveOnly:  activeOnly,
		Error:       displayError,
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["on_this_day"]
	if !exists {
		log.Printf("[ERROR] On This Day: Template 'on_this_day' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] On This Day: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// PioneersHandler displays the first nodes (pioneers) in a FidoNet region
func (s *Server) PioneersHandler(w http.ResponseWriter, r *http.Request) {
	// Get zone and region parameters from query string
	zoneStr := r.URL.Query().Get("zone")
	regionStr := r.URL.Query().Get("region")
	var zone, region int
	var pioneers []storage.PioneerNode
	var err error

	if zoneStr != "" && regionStr != "" {
		var zoneErr, regionErr error
		zone, zoneErr = strconv.Atoi(zoneStr)
		region, regionErr = strconv.Atoi(regionStr)

		if zoneErr != nil || regionErr != nil || zone < 1 || zone > 6 || region < 1 {
			err = fmt.Errorf("invalid zone or region parameters")
		} else {
			// Get pioneers for this zone:region (default to 50)
			pioneers, err = s.storage.GetPioneersByRegion(zone, region, 50)
		}
	}

	var displayError error
	if err != nil {
		log.Printf("[ERROR] Pioneers: Error fetching data: %v", err)
		displayError = fmt.Errorf("Failed to fetch pioneer data. Please try again later")
		pioneers = []storage.PioneerNode{}
	}

	// Build template data
	data := struct {
		Title      string
		ActivePage string
		Version    string
		Zone       int
		Region     int
		Pioneers   []storage.PioneerNode
		Error      error
	}{
		Title:      "FidoNet Region Pioneers",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
		Zone:       zone,
		Region:     region,
		Pioneers:   pioneers,
		Error:      displayError,
	}

	// Check template exists before rendering
	tmpl, exists := s.templates["pioneers"]
	if !exists {
		log.Printf("[ERROR] Pioneers: Template 'pioneers' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render template
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] Pioneers: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// IPv6NodeListHandler shows the IPv6 node list report in Michiel van der Vlist's format.
// Supports ?format=text for plain text export matching Michiel's exact format.
func (s *Server) IPv6NodeListHandler(w http.ResponseWriter, r *http.Request) {
	params := parseAnalyticsParams(r)

	nodes, err := s.storage.GetIPv6NodeList(params.Limit, params.Days, params.IncludeZeroNodes)

	// Handle text format
	if r.URL.Query().Get("format") == "text" {
		if err != nil {
			http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"ipv6_node_list.txt\"")
		renderIPv6NodeListText(w, nodes)
		return
	}

	var displayError error
	if err != nil {
		log.Printf("[ERROR] IPv6 Node List: Error fetching nodes: %v", err)
		nodes = []storage.IPv6NodeListEntry{}
		displayError = fmt.Errorf("Failed to fetch IPv6 node list. Please try again later")
	} else if params.ValidationError != "" {
		displayError = fmt.Errorf("%s", params.ValidationError)
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		Nodes            []storage.IPv6NodeListEntry
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "Michiel van der Vlist's IPv6 Node List",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		Nodes:            nodes,
		Days:             params.Days,
		Limit:            params.Limit,
		IncludeZeroNodes: params.IncludeZeroNodes,
		Error:            displayError,
	}

	tmpl, exists := s.templates["ipv6_node_list"]
	if !exists {
		log.Printf("[ERROR] IPv6 Node List: Template 'ipv6_node_list' not found")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] IPv6 Node List: Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// renderIPv6NodeListText writes the IPv6 node list in Michiel van der Vlist's plain text format.
func renderIPv6NodeListText(w http.ResponseWriter, nodes []storage.IPv6NodeListEntry) {
	now := time.Now()

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "                      List of IPv6 nodes\n")
	fmt.Fprintf(w, "                      Generated by NodelistDB\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "                      Updated %s\n", now.Format("2 Jan 2006"))
	fmt.Fprintf(w, "\n\n")
	fmt.Fprintf(w, "     Node Nr.     Sysop                  Type      Provider              Remark\n")
	fmt.Fprintf(w, "\n")

	for i, node := range nodes {
		address := fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node)
		// Truncate sysop and provider to fit column widths
		sysop := node.SysopName
		if len(sysop) > 22 {
			sysop = sysop[:22]
		}
		provider := node.Provider
		if len(provider) > 21 {
			provider = provider[:21]
		}
		fmt.Fprintf(w, "%3d  %-13s %-22s %-9s %-21s %s\n",
			i+1,
			address,
			sysop,
			node.IPv6Type,
			provider,
			node.Remarks,
		)
	}

	fmt.Fprintf(w, "\n\n")
	fmt.Fprintf(w, "T-6in4             Static 6in4 tunnel\n")
	fmt.Fprintf(w, "T-6to4             6to4 tunnel\n")
	fmt.Fprintf(w, "T-Teredo           Teredo tunnel\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Remarks:\n\n")
	fmt.Fprintf(w, "f     Has a ::f1d0:<zone>:<net>:<node> style host address.\n")
	fmt.Fprintf(w, "INO4  No IPv4 (Node has no working IPv4 connectivity)\n")
	fmt.Fprintf(w, "6UNS  IPv6 unstable (failed more than twice during the last 30 days)\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Note: This list is automatically generated by NodelistDB.\n")
	fmt.Fprintf(w, "Only nodes with verified IPv6 AKA (address validation) are included.\n")
}
