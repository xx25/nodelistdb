package web

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"

	"nodelistdb/internal/flags"
)

// loadTemplates loads HTML templates from files
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "stats", "sysop_search", "node_history", "api_help"}
	
	// Create function map for template functions
	funcMap := template.FuncMap{
		"getFlagDescription": func(flagDescriptions map[string]flags.FlagInfo, flag string) string {
			if info, exists := flagDescriptions[flag]; exists {
				return info.Description
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
	}
	
	for _, tmplName := range templates {
		tmpl, err := s.loadTemplateFromFile(tmplName, funcMap)
		if err != nil {
			log.Fatalf("Failed to load template %s: %v", tmplName, err)
		}
		s.templates[tmplName] = tmpl
	}
}

// loadTemplateFromFile loads a template from a file
func (s *Server) loadTemplateFromFile(name string, funcMap template.FuncMap) (*template.Template, error) {
	templateFile := filepath.Join("templates", name+".html")
	
	// Check if file exists
	if _, err := os.Stat(templateFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("template file %s not found", templateFile)
	}
	
	// Parse template file with the correct template name
	tmpl, err := template.New(name+".html").Funcs(funcMap).ParseFiles(templateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %v", templateFile, err)
	}
	
	// Return the template with the expected name
	return tmpl.Lookup(name+".html"), nil
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
		
		if desc, exists := flagDescriptions[flag]; exists && desc.Description != "" {
			// Render with tooltip
			result.WriteString(fmt.Sprintf(`<span class="flag-tooltip"><span class="badge badge-info" style="margin: 0 1px;">%s</span><span class="tooltip-text">%s</span></span>`, flag, desc.Description))
		} else {
			// Render without tooltip
			result.WriteString(fmt.Sprintf(`<span class="badge badge-info" style="margin: 0 1px;">%s</span>`, flag))
		}
	}
	
	result.WriteString("]")
	return result.String()
}