package web

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
)

// loadTemplates loads HTML templates from files
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "stats", "sysop_search", "node_history", "api_help"}
	
	// Create function map for template functions
	funcMap := template.FuncMap{
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



