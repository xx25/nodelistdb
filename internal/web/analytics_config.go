package web

import (
	"fmt"
	"html/template"
	"strings"
)

// ProtocolPageConfig holds page-specific configuration for protocol analytics pages.
// Used for BinkP, IFCICO, Telnet, VModem, FTP analytics pages.
// This enables configuration-driven rendering with a single unified template.
type ProtocolPageConfig struct {
	PageTitle       string        // e.g., "BinkP Enabled Nodes"
	PageSubtitle    template.HTML // HTML subtitle displayed below page title
	StatsHeading    string        // e.g., "BinkP Enabled" (used in "Found X {StatsHeading} Nodes")
	ShowVersion     bool          // Show version column (true for BinkP, IFCICO)
	VersionField    string        // Field name: "BinkPVersion", "IfcicoVersion"
	ShowAnonLogin   bool          // Show anonymous login column (FTP only)
	InfoText        []string      // Info paragraphs (can use %d for days substitution)
	EmptyStateTitle string        // Title when no results found
	EmptyStateDesc  string        // Description when no results found
}

// processInfoText converts InfoText strings to template.HTML, substituting %d with days.
// This allows info text to dynamically include the current time range.
func (c *ProtocolPageConfig) processInfoText(days int) []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
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

// containsFormatVerb checks if a string contains a format verb like %d
func containsFormatVerb(s string) bool {
	return strings.Contains(s, "%d") || strings.Contains(s, "%s") ||
		strings.Contains(s, "%v") || strings.Contains(s, "%f")
}

// AKAMismatchPageConfig holds page-specific configuration for AKA mismatch analytics pages.
// Used for showing nodes where announced AKA doesn't match expected nodelist address.
type AKAMismatchPageConfig struct {
	PageTitle       string        // e.g., "Nodes with AKA Mismatch"
	PageSubtitle    template.HTML // HTML subtitle displayed below page title
	StatsHeading    string        // e.g., "AKA Mismatch" (used in "Found X {StatsHeading} Nodes")
	InfoText        []string      // Info paragraphs (can use %d for days substitution)
	EmptyStateTitle string        // Title when no results found
	EmptyStateDesc  string        // Description when no results found
}

// processInfoText converts InfoText strings to template.HTML, substituting %d with days.
func (c *AKAMismatchPageConfig) processInfoText(days int) []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
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

// OtherNetworksPageConfig holds page-specific configuration for other networks analytics pages.
// Used for showing nodes that announce AKAs in non-FidoNet networks (e.g., tqwnet, fsxnet).
type OtherNetworksPageConfig struct {
	PageTitle       string        // e.g., "Other Networks"
	PageSubtitle    template.HTML // HTML subtitle displayed below page title
	StatsHeading    string        // e.g., "Networks" (used in "Found X {StatsHeading}")
	InfoText        []string      // Info paragraphs (can use %d for days substitution)
	EmptyStateTitle string        // Title when no results found
	EmptyStateDesc  string        // Description when no results found
}

// processInfoText converts InfoText strings to template.HTML, substituting %d with days.
func (c *OtherNetworksPageConfig) processInfoText(days int) []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
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

// SoftwarePageConfig holds page-specific configuration for software analytics pages.
// Used for BinkP and IFCICO software distribution pages.
// This enables configuration-driven rendering with a single unified template.
type SoftwarePageConfig struct {
	PageTitle          string // e.g., "BinkP Software Distribution"
	PageSubtitle       string // Plain text subtitle
	APIEndpoint        string // e.g., "/api/software/binkp"
	InfoNote           string // Optional info note shown above stats
	HasDetailSection   bool   // Whether to show detail breakdown section
	DetailSectionTitle string // e.g., "Binkd Detailed Analysis"
	DetailSectionDesc  string // Optional description below detail section title
	DetailLayout       string // "dual" (version+OS side by side) or "single" (version only)
	DetailAPIEndpoint  string // Separate API for detail data (e.g., "/api/software/binkd")
	DetailChartTitle   string // e.g., "Binkd Version Distribution"
	DetailListTitle    string // e.g., "Binkd Versions"
	DetailChart2Title  string // e.g., "Binkd Operating Systems" (dual layout only)
	DetailList2Title   string // e.g., "Operating Systems" (dual layout only)
	DetailChartType    string // "pie" or "bar" (for single layout)
	DetailSoftwareFilter string // Software name to filter version_breakdown (single layout)
	DetailShowThreshold  float64 // Show detail section if software percentage > this (single layout)
}

// GeoPageConfig holds page-specific configuration for geo-hosting analytics pages.
// Used for country and provider node listing pages.
// This enables configuration-driven rendering with a single unified template.
type GeoPageConfig struct {
	PageTitle       string        // e.g., "Nodes in United States"
	PageSubtitle    template.HTML // HTML subtitle displayed below page title
	StatsHeading    string        // e.g., "Nodes" (used in "Found X {StatsHeading}")
	ViewType        string        // "country" or "provider"
	CountryCode     string        // ISO country code (for country view)
	CountryName     string        // Full country name (for country view)
	ProviderName    string        // ISP/provider name (for provider view)
	Days            int           // Time range in days
	InfoText        []string      // Info paragraphs
	EmptyStateTitle string        // Title when no results found
	EmptyStateDesc  string        // Description when no results found
}

// processInfoText converts InfoText strings to template.HTML, substituting %d with days.
func (c *GeoPageConfig) processInfoText() []template.HTML {
	result := make([]template.HTML, len(c.InfoText))
	for i, text := range c.InfoText {
		var processed string
		if containsFormatVerb(text) {
			processed = fmt.Sprintf(text, c.Days)
		} else {
			processed = text
		}
		result[i] = template.HTML(processed)
	}
	return result
}
