package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/testing/timeavail"
)

// loadTemplates loads HTML templates from files
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "stats", "node_history", "api_help", "nodelist_download", "analytics", "reachability", "test_detail", "modem_test_detail", "ipv6_analytics_generic", "ipv6_weekly_news", "ipv6_node_list", "unified_analytics", "binkp_software", "ifcico_software", "geo_analytics", "geo_nodes_list", "geo_unified", "pioneers", "on_this_day", "links", "pstn_analytics", "pstn_accessible_analytics", "filerequest_analytics", "aka_mismatch_analytics", "other_networks_analytics", "other_network_nodes", "domain_expiration"}

	// Create function map for template functions
	funcMap := template.FuncMap{
		"getFlagDescription": func(flagDescriptions map[string]flags.FlagInfo, flag string) string {
			if info, exists := flagDescriptions[flag]; exists {
				return info.Description
			}
			// Check if it's a T-flag that needs dynamic generation
			if len(flag) == 3 && flag[0] == 'T' {
				if info, ok := flags.GetTFlagInfo(flag); ok {
					return info.Description
				}
			}
			return ""
		},
		"getFieldDescription": func(field string) string {
			return GetFieldDescription(field)
		},
		"getFieldIcon": func(field string) string {
			return GetFieldIcon(field)
		},
		"renderFlagChange": func(flagDescriptions map[string]flags.FlagInfo, changeValue string) template.HTML {
			// Parse change value like "[MO LO V34] → [MO XA V34]"
			if !strings.Contains(changeValue, "→") {
				return template.HTML(changeValue)
			}

			parts := strings.Split(changeValue, "→")
			if len(parts) != 2 {
				return template.HTML(changeValue)
			}

			oldFlags := strings.TrimSpace(parts[0])
			newFlags := strings.TrimSpace(parts[1])

			// Parse flags from brackets like "[MO LO V34]"
			oldFlagList := parseFlagList(oldFlags)
			newFlagList := parseFlagList(newFlags)

			// Render with tooltips
			oldHTML := renderFlagListWithTooltips(flagDescriptions, oldFlagList)
			newHTML := renderFlagListWithTooltips(flagDescriptions, newFlagList)

			return template.HTML(oldHTML + " → " + newHTML)
		},
		"div": func(a, b interface{}) float64 {
			switch a := a.(type) {
			case int:
				switch b := b.(type) {
				case int:
					if b == 0 {
						return 0
					}
					return float64(a) / float64(b)
				default:
					return 0
				}
			case float64:
				switch b := b.(type) {
				case float64:
					if b == 0 {
						return 0
					}
					return a / b
				case int:
					if b == 0 {
						return 0
					}
					return a / float64(b)
				default:
					return 0
				}
			default:
				return 0
			}
		},
		"q931Cause": func(code any) string {
			// Q.850/Q.931 cause codes (synced with Asterisk 22 causes.h)
			causes := map[int]string{
				0: "Not defined", 1: "Unallocated number", 2: "No route to transit network",
				3: "No route to destination", 5: "Misdialled trunk prefix", 6: "Channel unacceptable",
				7: "Call awarded/delivered", 8: "Pre-empted", 14: "Number ported, not here",
				16: "Normal clearing", 17: "User busy", 18: "No user responding",
				19: "No answer", 20: "Subscriber absent", 21: "Call rejected",
				22: "Number changed", 23: "Redirected to new destination", 26: "Answered elsewhere",
				27: "Destination out of order", 28: "Invalid number format", 29: "Facility rejected",
				30: "Response to status enquiry", 31: "Normal, unspecified",
				34: "No circuit/channel available", 38: "Network out of order",
				41: "Temporary failure", 42: "Switching equipment congestion",
				43: "Access info discarded", 44: "Requested channel not available",
				50: "Facility not subscribed", 52: "Outgoing calls barred", 54: "Incoming calls barred",
				57: "Bearer capability not authorized", 58: "Bearer capability not available",
				65: "Bearer capability not implemented", 66: "Channel not implemented",
				69: "Facility not implemented", 81: "Invalid call reference",
				88: "Incompatible destination", 95: "Invalid message, unspecified",
				96: "Mandatory IE missing", 97: "Message type nonexistent", 98: "Wrong message",
				99: "IE nonexistent", 100: "Invalid IE contents", 101: "Wrong call state",
				102: "Recovery on timer expiry", 103: "Mandatory IE length error",
				111: "Protocol error", 127: "Interworking, unspecified",
			}
			var c int
			switch v := code.(type) {
			case uint8:
				c = int(v)
			case int:
				c = v
			default:
				return fmt.Sprintf("%v", code)
			}
			if desc, ok := causes[c]; ok {
				return fmt.Sprintf("%d (%s)", c, desc)
			}
			return fmt.Sprintf("%d", c)
		},
		"msToSec": func(ms any) string {
			var v float64
			switch ms := ms.(type) {
			case uint32:
				v = float64(ms) / 1000.0
			case uint16:
				v = float64(ms) / 1000.0
			case int:
				v = float64(ms) / 1000.0
			default:
				return "0s"
			}
			return fmt.Sprintf("%.1fs", v)
		},
		"mul": func(a, b interface{}) float64 {
			switch a := a.(type) {
			case int:
				switch b := b.(type) {
				case int:
					return float64(a * b)
				case float64:
					return float64(a) * b
				default:
					return 0
				}
			case float64:
				switch b := b.(type) {
				case float64:
					return a * b
				case int:
					return a * float64(b)
				default:
					return 0
				}
			default:
				return 0
			}
		},
		"formatFileSize": func(size int64) string {
			const (
				KB = 1024
				MB = KB * 1024
				GB = MB * 1024
			)

			switch {
			case size >= GB:
				return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
			case size >= MB:
				return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
			case size >= KB:
				return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
			default:
				return fmt.Sprintf("%d B", size)
			}
		},
		"join": strings.Join,
		"replaceUnderscores": func(s string) string {
			return strings.ReplaceAll(s, "_", " ")
		},
		"hasBinkp": func(config json.RawMessage) bool {
			if len(config) == 0 {
				return false
			}

			var internetConfig database.InternetConfiguration
			if err := json.Unmarshal(config, &internetConfig); err != nil {
				return false
			}

			_, hasIBN := internetConfig.Protocols["IBN"]
			_, hasBND := internetConfig.Protocols["BND"]
			return hasIBN || hasBND
		},
		"getInternetProtocols": func(config json.RawMessage) []string {
			if len(config) == 0 {
				return nil
			}

			var internetConfig database.InternetConfiguration
			if err := json.Unmarshal(config, &internetConfig); err != nil {
				return nil
			}

			var protocols []string
			for proto := range internetConfig.Protocols {
				protocols = append(protocols, proto)
			}
			return protocols
		},
		"getInternetHostnames": func(config json.RawMessage) []string {
			if len(config) == 0 {
				return nil
			}

			var internetConfig database.InternetConfiguration
			if err := json.Unmarshal(config, &internetConfig); err != nil {
				return nil
			}

			hostnameMap := make(map[string]bool)
			for _, details := range internetConfig.Protocols {
				for _, detail := range details {
					if detail.Address != "" {
						hostnameMap[detail.Address] = true
					}
				}
			}

			var hostnames []string
			for hostname := range hostnameMap {
				hostnames = append(hostnames, hostname)
			}
			return hostnames
		},
		"getProtocolAddresses": func(config json.RawMessage, protocol string) []string {
			if len(config) == 0 {
				return nil
			}

			var internetConfig database.InternetConfiguration
			if err := json.Unmarshal(config, &internetConfig); err != nil {
				return nil
			}

			details, ok := internetConfig.Protocols[protocol]
			if !ok {
				return nil
			}

			var addresses []string
			for _, detail := range details {
				addr := detail.Address
				if detail.Port != 0 {
					addr = fmt.Sprintf("%s:%d", addr, detail.Port)
				}
				addresses = append(addresses, addr)
			}
			return addresses
		},
		"getEmails": func(config json.RawMessage) []string {
			if len(config) == 0 {
				return nil
			}

			var internetConfig database.InternetConfiguration
			if err := json.Unmarshal(config, &internetConfig); err != nil {
				return nil
			}

			var emails []string
			for _, emailDetails := range internetConfig.EmailProtocols {
				for _, detail := range emailDetails {
					if detail.Email != "" {
						emails = append(emails, detail.Email)
					}
				}
			}
			return emails
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"dateDuration": func(start, end time.Time) string {
			if end.Before(start) {
				start, end = end, start
			}
			years := end.Year() - start.Year()
			months := int(end.Month()) - int(start.Month())
			days := end.Day() - start.Day()

			if days < 0 {
				months--
				// Get days in previous month
				prevMonth := end.AddDate(0, -1, 0)
				days += time.Date(prevMonth.Year(), prevMonth.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
			}
			if months < 0 {
				years--
				months += 12
			}

			var parts []string
			if years > 0 {
				if years == 1 {
					parts = append(parts, "1 year")
				} else {
					parts = append(parts, fmt.Sprintf("%d years", years))
				}
			}
			if months > 0 {
				if months == 1 {
					parts = append(parts, "1 month")
				} else {
					parts = append(parts, fmt.Sprintf("%d months", months))
				}
			}
			if days > 0 || len(parts) == 0 {
				if days == 1 {
					parts = append(parts, "1 day")
				} else {
					parts = append(parts, fmt.Sprintf("%d days", days))
				}
			}
			return strings.Join(parts, ", ")
		},
		"len": func(v interface{}) int {
			if v == nil {
				return 0
			}
			switch reflect.TypeOf(v).Kind() {
			case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
				return reflect.ValueOf(v).Len()
			default:
				return 0
			}
		},
		"formatPSTNSchedule": func(node storage.PSTNNode) string {
			if node.IsCM {
				return "Continuous Mail — available 24/7"
			}
			avail, err := timeavail.ParseAvailability(node.Flags, node.Zone, node.Phone)
			if err != nil || len(avail.Windows) == 0 {
				// No T-flags found; show ZMH default for the zone
				zmh := timeavail.GetZMHWindow(node.Zone)
				if zmh != nil {
					return fmt.Sprintf("ZMH only: %s-%s UTC daily",
						zmh.StartUTC.Format("15:04"), zmh.EndUTC.Format("15:04"))
				}
				return "Schedule unknown"
			}
			var parts []string
			for _, w := range avail.Windows {
				parts = append(parts, timeavail.FormatTimeWindow(w))
			}
			return strings.Join(parts, "\n")
		},
		"isNotNil": func(v interface{}) bool {
			if v == nil {
				return false
			}
			rv := reflect.ValueOf(v)
			return !rv.IsNil()
		},
		"deref": func(v *bool) bool {
			if v == nil {
				return false
			}
			return *v
		},
		"countryFlag": func(countryCode string) string {
			// Convert ISO 3166-1 alpha-2 country code to flag emoji
			// Each letter is converted to regional indicator symbol (U+1F1E6 to U+1F1FF)
			if len(countryCode) != 2 {
				return ""
			}
			code := strings.ToUpper(countryCode)
			flag := ""
			for _, c := range code {
				if c < 'A' || c > 'Z' {
					return ""
				}
				// Regional indicator symbols start at U+1F1E6 (🇦)
				flag += string(rune(0x1F1E6 + (c - 'A')))
			}
			return flag
		},
	}

	for _, tmplName := range templates {
		tmpl, err := s.loadTemplateFromFile(tmplName, funcMap)
		if err != nil {
			log.Fatalf("Failed to load template %s: %v", tmplName, err)
		}
		s.templates[tmplName] = tmpl
	}
}

// loadTemplateFromFile loads a template from embedded filesystem
func (s *Server) loadTemplateFromFile(name string, funcMap template.FuncMap) (*template.Template, error) {
	templateFile := filepath.Join("templates", name+".html")

	// Read template from embedded filesystem
	content, err := s.templatesFS.ReadFile(templateFile)
	if err != nil {
		return nil, fmt.Errorf("template file %s not found in embedded filesystem: %v", templateFile, err)
	}

	// Create template with function map
	tmpl := template.New(name + ".html")
	if funcMap != nil {
		tmpl = tmpl.Funcs(funcMap)
	}

	// Load base template if exists
	baseContent, err := s.templatesFS.ReadFile("templates/base.html")
	if err == nil {
		tmpl, err = tmpl.Parse(string(baseContent))
		if err != nil {
			return nil, fmt.Errorf("failed to parse base template: %v", err)
		}
	}

	// For main templates (not nav or footer), also load the nav and footer templates
	if name != "nav" && name != "footer" {
		navContent, err := s.templatesFS.ReadFile("templates/nav.html")
		if err == nil {
			// Parse nav template first
			tmpl, err = tmpl.Parse(string(navContent))
			if err != nil {
				return nil, fmt.Errorf("failed to parse nav template: %v", err)
			}
		}

		footerContent, err := s.templatesFS.ReadFile("templates/footer.html")
		if err == nil {
			// Parse footer template
			tmpl, err = tmpl.Parse(string(footerContent))
			if err != nil {
				return nil, fmt.Errorf("failed to parse footer template: %v", err)
			}
		}
	}

	// Load partial templates from partials/ directory
	partialFiles := []string{
		"analytics_filters.html",
		"analytics_table.html",
		"error_display.html",
		"node_address_cell.html",
		"hostname_cell.html",
		"hostname_cell_simple.html",
		"location_cell.html",
		"timestamp_cell.html",
		"action_buttons_cell.html",
		"ipv6_protocols_cell.html",
		"ipv4_protocols_cell.html",
		"ipv6_failed_protocols_cell.html",
	}

	for _, partialFile := range partialFiles {
		partialPath := filepath.Join("templates", "partials", partialFile)
		partialContent, err := s.templatesFS.ReadFile(partialPath)
		if err == nil {
			tmpl, err = tmpl.Parse(string(partialContent))
			if err != nil {
				log.Printf("Warning: failed to parse partial %s: %v", partialFile, err)
				// Continue loading other partials
			}
		}
	}

	// Parse the main template content
	tmpl, err = tmpl.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %v", templateFile, err)
	}

	return tmpl, nil
}

// Helper functions for flag change rendering
func parseFlagList(flagString string) []string {
	// Remove brackets and parse space-separated flags
	flagString = strings.Trim(flagString, "[]")
	if flagString == "" {
		return []string{}
	}
	return strings.Fields(flagString)
}

func renderFlagListWithTooltips(flagDescriptions map[string]flags.FlagInfo, flagList []string) string {
	if len(flagList) == 0 {
		return "[]"
	}

	var result strings.Builder
	result.WriteString("[")

	for i, flag := range flagList {
		if i > 0 {
			result.WriteString(" ")
		}

		// Check static descriptions first
		if desc, exists := flagDescriptions[flag]; exists && desc.Description != "" {
			// Render with tooltip
			result.WriteString(fmt.Sprintf(`<span class="flag-tooltip"><span class="badge badge-info" style="margin: 0 1px;">%s</span><span class="tooltip-text">%s</span></span>`, flag, desc.Description))
		} else if len(flag) == 3 && flag[0] == 'T' {
			// Check if it's a T-flag that needs dynamic generation
			if info, ok := flags.GetTFlagInfo(flag); ok && info.Description != "" {
				// Render with tooltip
				result.WriteString(fmt.Sprintf(`<span class="flag-tooltip"><span class="badge badge-info" style="margin: 0 1px;">%s</span><span class="tooltip-text">%s</span></span>`, flag, info.Description))
			} else {
				// Render without tooltip
				result.WriteString(fmt.Sprintf(`<span class="badge badge-info" style="margin: 0 1px;">%s</span>`, flag))
			}
		} else {
			// Render without tooltip
			result.WriteString(fmt.Sprintf(`<span class="badge badge-info" style="margin: 0 1px;">%s</span>`, flag))
		}
	}

	result.WriteString("]")
	return result.String()
}
