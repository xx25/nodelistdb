package web

import (
	"fmt"
	"html/template"
	"log"
	"path/filepath"
	"strings"

	"nodelistdb/internal/flags"
)

// loadTemplates loads HTML templates from files
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "stats", "sysop_search", "node_history", "api_help", "nodelist_download", "analytics"}

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
		"replaceUnderscores": func(s string) string {
			return strings.ReplaceAll(s, "_", " ")
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
