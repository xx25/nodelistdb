package web

import (
	"fmt"
	"html/template"
	"strings"
)

// basePageConfig is the page copy every config-driven analytics page carries.
// Five configs used to declare these six fields verbatim and carry an
// identical processInfoText alongside them.
type basePageConfig struct {
	PageTitle       string        // e.g., "BinkP Enabled Nodes"
	PageSubtitle    template.HTML // HTML subtitle displayed below page title
	StatsHeading    string        // e.g., "BinkP Enabled" (used in "Found X {StatsHeading} Nodes")
	InfoText        []string      // Info paragraphs (can use %d for days substitution)
	EmptyStateTitle string        // Title when no results found
	EmptyStateDesc  string        // Description when no results found
}

// base returns the shared fields, and by being promoted onto every config that
// embeds it, it is also what makes those configs interchangeable at the one
// renderer they share.
func (c basePageConfig) base() basePageConfig { return c }

// analyticsPageConfig is satisfied by any config embedding basePageConfig.
type analyticsPageConfig interface {
	base() basePageConfig
}

// processInfoText converts InfoText strings to template.HTML, substituting %d
// with days. This allows info text to dynamically include the current time
// range.
func (c basePageConfig) processInfoText(days int) []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
		// Only substitute when the text actually carries a verb, or Sprintf
		// appends "%!(EXTRA int=...)" to every paragraph that does not.
		processed := text
		if containsFormatVerb(text) {
			processed = fmt.Sprintf(text, days)
		}
		result[i] = template.HTML(processed)
	}
	return result
}

// containsFormatVerb checks if a string contains a format verb like %d
func containsFormatVerb(s string) bool {
	return strings.Contains(s, "%d") || strings.Contains(s, "%s") ||
		strings.Contains(s, "%v") || strings.Contains(s, "%f")
}

// ProtocolPageConfig configures the protocol analytics pages (BinkP, IFCICO,
// Telnet, VModem, FTP), which share unified_analytics.html.
type ProtocolPageConfig struct {
	basePageConfig
	ShowVersion   bool   // Show version column (true for BinkP, IFCICO)
	VersionField  string // Field name: "BinkPVersion", "IfcicoVersion"
	ShowAnonLogin bool   // Show anonymous login column (FTP only)
}

// IPv6PageConfig configures the five IPv6 reports, which share
// ipv6_analytics_generic.html.
type IPv6PageConfig struct {
	basePageConfig
	TableLayout string // "standard" or "dual-protocol"
}

// AKAMismatchPageConfig configures the page showing nodes whose announced AKA
// does not match their nodelist address.
type AKAMismatchPageConfig struct {
	basePageConfig
}

// VModemUnavailablePageConfig configures the page showing nodes that announce
// IVM but are not confirmed as genuine VMODEM.
type VModemUnavailablePageConfig struct {
	basePageConfig
}

// OtherNetworksPageConfig configures the pages showing nodes that announce
// AKAs in non-FidoNet networks (tqwnet, fsxnet, ...).
type OtherNetworksPageConfig struct {
	basePageConfig
}

// GeoPageConfig configures the country, provider and domain node listings,
// which share geo_unified.html.
type GeoPageConfig struct {
	basePageConfig
	ViewType     string // "country", "provider" or "domain"
	CountryCode  string // ISO country code (for country view)
	CountryName  string // Full country name (for country view)
	ProviderName string // ISP/provider name (for provider and domain views)
	BackURL      string // Back-link target (domain view)
	BackLabel    string // Back-link text (domain view)
	Days         int    // Time range in days
}

// SoftwarePageConfig holds page-specific configuration for software analytics
// pages (BinkP and IFCICO software distribution). It shares no fields with the
// node-listing configs above: the page is charts and API endpoints, not a
// table of nodes.
type SoftwarePageConfig struct {
	PageTitle            string  // e.g., "BinkP Software Distribution"
	PageSubtitle         string  // Plain text subtitle
	APIEndpoint          string  // e.g., "/api/software/binkp"
	InfoNote             string  // Optional info note shown above stats
	HasDetailSection     bool    // Whether to show detail breakdown section
	DetailSectionTitle   string  // e.g., "Binkd Detailed Analysis"
	DetailSectionDesc    string  // Optional description below detail section title
	DetailLayout         string  // "dual" (version+OS side by side) or "single" (version only)
	DetailAPIEndpoint    string  // Separate API for detail data (e.g., "/api/software/binkd")
	DetailChartTitle     string  // e.g., "Binkd Version Distribution"
	DetailListTitle      string  // e.g., "Binkd Versions"
	DetailChart2Title    string  // e.g., "Binkd Operating Systems" (dual layout only)
	DetailList2Title     string  // e.g., "Operating Systems" (dual layout only)
	DetailChartType      string  // "pie" or "bar" (for single layout)
	DetailSoftwareFilter string  // Software name to filter version_breakdown (single layout)
	DetailShowThreshold  float64 // Show detail section if software percentage > this (single layout)
}
