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
