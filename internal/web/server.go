package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"nodelistdb/internal/storage"
)

// Server represents the web server
type Server struct {
	storage   *storage.Storage
	templates map[string]*template.Template
}

// New creates a new web server
func New(storage *storage.Storage) *Server {
	server := &Server{
		storage:   storage,
		templates: make(map[string]*template.Template),
	}
	
	server.loadTemplates()
	return server
}

// loadTemplates loads HTML templates
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "node", "stats", "sysop_search", "node_history", "api_help", "analytics", "v34_analytics", "binkp_analytics", "sysop_analytics", "network_lifecycle"}
	
	// Create function map for template functions
	funcMap := template.FuncMap{
		"getZoneDescription": getZoneDescription,
		"add": func(a, b int) int {
			return a + b
		},
		"float64": func(i interface{}) float64 {
			switch v := i.(type) {
			case int:
				return float64(v)
			case float64:
				return v
			default:
				return 0
			}
		},
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"printf": func(format string, args ...interface{}) string {
			return fmt.Sprintf(format, args...)
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
	
	for _, tmpl := range templates {
		s.templates[tmpl] = template.Must(template.New(tmpl).Funcs(funcMap).Parse(s.getTemplate(tmpl)))
	}
}

// SetupRoutes configures the HTTP routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.IndexHandler)
	mux.HandleFunc("/search", s.SearchHandler)
	mux.HandleFunc("/search/sysop", s.SysopSearchHandler)
	mux.HandleFunc("/stats", s.StatsHandler)
	mux.HandleFunc("/analytics", s.AnalyticsHandler)
	mux.HandleFunc("/analytics/v34", s.V34AnalyticsHandler)
	mux.HandleFunc("/analytics/binkp", s.BinkpAnalyticsHandler)
	mux.HandleFunc("/analytics/network/", s.NetworkLifecycleHandler)
	mux.HandleFunc("/analytics/sysops", s.SysopNamesHandler)
	mux.HandleFunc("/analytics/trends", s.ProtocolTrendHandler)
	mux.HandleFunc("/api/help", s.APIHelpHandler)
	mux.HandleFunc("/node/", s.NodeHistoryHandler)
	
	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}

// getTemplate returns HTML templates as string literals
func (s *Server) getTemplate(name string) string {
	switch name {
	case "index":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Comprehensive FidoNet historical data and analytics</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/" class="active">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <p style="font-size: 1.1rem; color: var(--text-secondary); margin-bottom: 2rem;">
                    Welcome to the FidoNet Nodelist Database. This system provides comprehensive access to historical and current FidoNet node information, featuring advanced search capabilities and detailed historical analysis.
                </p>
                
                <h2 style="margin-bottom: 1.5rem; color: var(--text-primary);">Quick Actions</h2>
                
                <div class="stats-grid">
                    <div class="stat-card">
                        <h3>🔍</h3>
                        <p style="color: var(--text-primary); font-size: 1rem; margin-top: 1rem;">
                            <a href="/search" class="btn" style="text-decoration: none; display: inline-block; margin-top: 0.5rem;">Search Nodes</a>
                        </p>
                        <small style="color: var(--text-secondary);">Find nodes by zone, net, system name, or location</small>
                    </div>
                    
                    <div class="stat-card">
                        <h3>👤</h3>
                        <p style="color: var(--text-primary); font-size: 1rem; margin-top: 1rem;">
                            <a href="/search/sysop" class="btn" style="text-decoration: none; display: inline-block; margin-top: 0.5rem;">Search Sysops</a>
                        </p>
                        <small style="color: var(--text-secondary);">Find all nodes operated by a specific sysop</small>
                    </div>
                    
                    <div class="stat-card">
                        <h3>📊</h3>
                        <p style="color: var(--text-primary); font-size: 1rem; margin-top: 1rem;">
                            <a href="/stats" class="btn" style="text-decoration: none; display: inline-block; margin-top: 0.5rem;">Statistics</a>
                        </p>
                        <small style="color: var(--text-secondary);">View network statistics and trends</small>
                    </div>
                </div>
                
                <div class="alert alert-success" style="margin-top: 2rem;">
                    <strong>✨ New Feature:</strong> Historical node tracking with timeline visualization and change filtering is now available!
                </div>
            </div>
        </div>
    </div>
</body>
</html>`

	case "search":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Find FidoNet nodes by address, system name, or location</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search" class="active">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <form method="post" class="search-form">
                    <div class="form-group" style="grid-column: span 2;">
                        <label for="full_address">Full Node Address:</label>
                        <input type="text" id="full_address" name="full_address" placeholder="e.g. 2:5001/100 or 1:234/56.7" style="font-family: monospace;">
                        <small style="color: var(--text-secondary); margin-top: 0.25rem; display: block;">
                            💡 Enter complete address like "2:5001/100" or use individual fields below
                        </small>
                    </div>
                    
                    <div class="form-group">
                        <label for="zone">Zone:</label>
                        <select id="zone" name="zone">
                            <option value="">All Zones</option>
                            <option value="1">Zone 1 - United States and Canada</option>
                            <option value="2">Zone 2 - Europe, Former Soviet Union, and Israel</option>
                            <option value="3">Zone 3 - Australasia (includes former Zone 6 nodes)</option>
                            <option value="4">Zone 4 - Latin America (except Puerto Rico)</option>
                            <option value="5">Zone 5 - Africa</option>
                            <option value="6">Zone 6 - Asia (removed July 2007, nodes moved to Zone 3)</option>
                        </select>
                    </div>
                    
                    <div class="form-group">
                        <label for="net">Net:</label>
                        <input type="number" id="net" name="net" placeholder="e.g. 234" min="0">
                    </div>
                    
                    <div class="form-group">
                        <label for="node">Node:</label>
                        <input type="number" id="node" name="node" placeholder="e.g. 56" min="0">
                    </div>
                    
                    <div class="form-group">
                        <label for="system_name">System Name:</label>
                        <input type="text" id="system_name" name="system_name" placeholder="e.g. Example BBS">
                    </div>
                    
                    <div class="form-group">
                        <label for="location">Location:</label>
                        <input type="text" id="location" name="location" placeholder="e.g. New York, NY">
                    </div>
                    
                    <div class="form-group" style="align-self: end;">
                        <button type="submit" class="btn">🔍 Search Nodes</button>
                    </div>
                </form>
                
                <div style="margin-top: 1rem; padding: 1rem; background: #f8fafc; border-radius: var(--radius); font-size: 0.9rem; color: var(--text-secondary);">
                    💡 <strong>Tip:</strong> You can search by any combination of fields. Leave fields empty to search more broadly.
                </div>
            </div>
            
            {{if .Error}}
                <div class="alert alert-error">
                    <strong>Error:</strong> {{.Error}}
                </div>
            {{end}}
            
            {{if .Nodes}}
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1rem;">
                        Search Results 
                        <span class="badge badge-info" style="font-size: 0.8rem; margin-left: 0.5rem;">
                            {{.Count}} nodes found
                        </span>
                    </h2>
                    
                    <div class="table-container">
                        <table>
                            <thead>
                                <tr>
                                    <th>Address</th>
                                    <th>System Name</th>
                                    <th>Location</th>
                                    <th>Sysop</th>
                                    <th>Type</th>
                                    <th>Date</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {{range .Nodes}}
                                <tr>
                                    <td>
                                        <strong>
                                            <a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}">{{.Zone}}:{{.Net}}/{{.Node}}</a>
                                        </strong>
                                    </td>
                                    <td>{{if .SystemName}}{{.SystemName}}{{else}}<em>-</em>{{end}}</td>
                                    <td>{{if .Location}}{{.Location}}{{else}}<em>-</em>{{end}}</td>
                                    <td>{{if .SysopName}}{{.SysopName}}{{else}}<em>-</em>{{end}}</td>
                                    <td>
                                        {{if eq .NodeType "Zone"}}<span class="badge badge-error">Zone</span>
                                        {{else if eq .NodeType "Region"}}<span class="badge badge-warning">Region</span>
                                        {{else if eq .NodeType "Host"}}<span class="badge badge-info">Host</span>
                                        {{else if eq .NodeType "Hub"}}<span class="badge badge-success">Hub</span>
                                        {{else if eq .NodeType "Pvt"}}<span class="badge badge-warning">Pvt</span>
                                        {{else if eq .NodeType "Down"}}<span class="badge badge-error">Down</span>
                                        {{else if eq .NodeType "Hold"}}<span class="badge badge-warning">Hold</span>
                                        {{else}}{{.NodeType}}{{end}}
                                    </td>
                                    <td style="font-size: 0.9rem; color: var(--text-secondary);">
                                        {{.NodelistDate.Format "2006-01-02"}}
                                    </td>
                                    <td>
                                        <a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem;">
                                            📈 History
                                        </a>
                                    </td>
                                </tr>
                                {{end}}
                            </tbody>
                        </table>
                    </div>
                </div>
            {{end}}
        </div>
    </div>
    <script src="/static/app.js"></script>
</body>
</html>`

	case "stats":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">FidoNet network statistics and metrics</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats" class="active">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if not .NoData}}
                <!-- Date Selection Form -->
                <div class="card" style="margin-bottom: 2rem;">
                    <h3 style="margin-bottom: 1rem; color: var(--text-primary);">📅 Select Date for Statistics</h3>
                    <form method="GET" action="/stats" style="display: flex; gap: 1rem; align-items: end; flex-wrap: wrap;">
                        <div style="flex: 1; min-width: 200px;">
                            <label for="date" style="display: block; margin-bottom: 0.5rem; font-weight: 600; color: var(--text-primary);">Choose Date:</label>
                            <input 
                                type="date" 
                                id="date" 
                                name="date" 
                                value="{{if .RequestedDate}}{{.RequestedDate}}{{else}}{{.SelectedDate.Format "2006-01-02"}}{{end}}"
                                style="padding: 0.75rem; border: 1px solid var(--border-color); border-radius: var(--radius); font-size: 1rem; width: 100%;"
                            />
                        </div>
                        <div>
                            <button type="submit" class="btn btn-primary" style="padding: 0.75rem 1.5rem;">
                                📊 View Statistics
                            </button>
                        </div>
                    </form>
                    {{if .DateMessage}}
                        <div style="margin-top: 1rem; padding: 0.75rem; background: var(--accent-color); border-left: 4px solid var(--primary-color); border-radius: var(--radius); color: var(--text-primary);">
                            ℹ️ {{.DateMessage}}
                        </div>
                    {{end}}
                    {{if gt (len .AvailableDates) 0}}
                        <div style="margin-top: 1rem; font-size: 0.9rem; color: var(--text-secondary);">
                            <strong>📊 {{len .AvailableDates}} nodelist{{if ne (len .AvailableDates) 1}}s{{end}} available</strong>
                            {{if gt (len .AvailableDates) 1}}
                                <br>Date range: {{(index .AvailableDates 0).Format "2006-01-02"}} (newest) to older dates
                            {{end}}
                        </div>
                    {{end}}
                </div>
            {{end}}
            
            {{if .NoData}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Data Available</strong><br>
                        {{.Error}}
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        To populate statistics, please import nodelist files using the parser tool:<br>
                        <code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem;">./bin/parser -path /path/to/nodelists -db ./nodelist.duckdb</code>
                    </p>
                </div>
            {{else if .Error}}
                <div class="alert alert-error">
                    <strong>Error:</strong> {{.Error}}
                </div>
            {{else if .Stats}}
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">
                        📊 Network Statistics for {{.Stats.Date.Format "January 2, 2006"}}
                    </h2>
                    
                    <div class="stats-grid">
                        <div class="stat-card">
                            <h3>{{.Stats.TotalNodes}}</h3>
                            <p>Total Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.ActiveNodes}}</h3>
                            <p>Active Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.CMNodes}}</h3>
                            <p>CM Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.BinkpNodes}}</h3>
                            <p>Binkp Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.InternetNodes}}</h3>
                            <p>Internet Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.DownNodes}}</h3>
                            <p>Down Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.HoldNodes}}</h3>
                            <p>Hold Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Stats.PvtNodes}}</h3>
                            <p>Private Nodes</p>
                        </div>
                    </div>
                </div>
                
                {{if .Stats.ZoneDistribution}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌍 Zone Distribution</h3>
                        
                        <div class="table-container">
                            <table>
                                <thead>
                                    <tr>
                                        <th>Zone</th>
                                        <th>Description</th>
                                        <th>Node Count</th>
                                        <th>Percentage</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range $zone, $count := .Stats.ZoneDistribution}}
                                    <tr>
                                        <td><strong>Zone {{$zone}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{getZoneDescription $zone}}</td>
                                        <td>{{$count}} nodes</td>
                                        <td style="color: var(--text-secondary);">
                                            {{printf "%.1f%%" (div (mul $count 100) $.Stats.TotalNodes)}}
                                        </td>
                                        <td>
                                            <div style="background: #e2e8f0; height: 8px; border-radius: 4px; overflow: hidden;">
                                                <div style="background: var(--primary-color); height: 100%; width: {{printf "%.1f%%" (div (mul $count 100) $.Stats.TotalNodes)}}; transition: width 0.3s ease;"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                {{if .Stats.LargestRegions}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🏛️ Largest Regions</h3>
                        
                        <div class="table-container">
                            <table>
                                <thead>
                                    <tr>
                                        <th>Region</th>
                                        <th>Name</th>
                                        <th>Zone</th>
                                        <th>Node Count</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .Stats.LargestRegions}}
                                    <tr>
                                        <td><strong>Region {{.Region}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{if .Name}}{{.Name}}{{else}}<em>-</em>{{end}}</td>
                                        <td>Zone {{.Zone}}</td>
                                        <td>{{.NodeCount}} nodes</td>
                                        <td>
                                            <div style="background: #e2e8f0; height: 8px; border-radius: 4px; overflow: hidden;">
                                                <div style="background: var(--primary-color); height: 100%; width: {{printf "%.1f%%" (div (mul .NodeCount 100) $.Stats.TotalNodes)}}; transition: width 0.3s ease;"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                {{if .Stats.LargestNets}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌐 Largest Networks</h3>
                        
                        <div class="table-container">
                            <table>
                                <thead>
                                    <tr>
                                        <th>Network</th>
                                        <th>Network Description</th>
                                        <th>Node Count</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .Stats.LargestNets}}
                                    <tr>
                                        <td><strong>{{.Zone}}:{{.Net}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{if .Name}}{{.Name}}{{else}}<em>-</em>{{end}}</td>
                                        <td>{{.NodeCount}} nodes</td>
                                        <td>
                                            <div style="background: #e2e8f0; height: 8px; border-radius: 4px; overflow: hidden;">
                                                <div style="background: var(--success-color); height: 100%; width: {{printf "%.1f%%" (div (mul .NodeCount 100) $.Stats.TotalNodes)}}; transition: width 0.3s ease;"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <div class="alert alert-success">
                    <strong>💡 Did you know?</strong> This data represents the current state of the FidoNet network. Use the historical node search to explore how individual nodes have changed over time!
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "sysop_search":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Find all nodes operated by a specific sysop</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop" class="active">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <form method="post" class="search-form">
                    <div class="form-group" style="grid-column: span 2;">
                        <label for="sysop_name">Sysop Name:</label>
                        <input type="text" id="sysop_name" name="sysop_name" value="{{.SysopName}}" placeholder="e.g. John Doe or John_Doe" required>
                        <small style="color: var(--text-secondary); margin-top: 0.25rem; display: block;">
                            💡 You can use either spaces or underscores - both "John Doe" and "John_Doe" will work
                        </small>
                    </div>
                    
                    <div class="form-group" style="align-self: end;">
                        <button type="submit" class="btn">👤 Search Sysops</button>
                    </div>
                </form>
                
                <div style="margin-top: 1rem; padding: 1rem; background: #f8fafc; border-radius: var(--radius); font-size: 0.9rem; color: var(--text-secondary);">
                    💡 <strong>Tip:</strong> Search will find all nodes that have ever been operated by this sysop, including historical records.
                </div>
            </div>
            
            {{if .Error}}
                <div class="alert alert-error">
                    <strong>Error:</strong> {{.Error}}
                </div>
            {{end}}
            
            {{if .Nodes}}
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1rem;">
                        Search Results 
                        <span class="badge badge-info" style="font-size: 0.8rem; margin-left: 0.5rem;">
                            {{.Count}} nodes found
                        </span>
                    </h2>
                    
                    <div class="table-container">
                        <table>
                            <thead>
                                <tr>
                                    <th>Address</th>
                                    <th>System Name</th>
                                    <th>Location</th>
                                    <th>Sysop</th>
                                    <th>Active Period</th>
                                    <th>Status</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {{range .Nodes}}
                                <tr>
                                    <td>
                                        <strong>
                                            <a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}">{{.Zone}}:{{.Net}}/{{.Node}}</a>
                                        </strong>
                                    </td>
                                    <td>{{if .SystemName}}{{.SystemName}}{{else}}<em>-</em>{{end}}</td>
                                    <td>{{if .Location}}{{.Location}}{{else}}<em>-</em>{{end}}</td>
                                    <td>{{if .SysopName}}{{.SysopName}}{{else}}<em>-</em>{{end}}</td>
                                    <td style="font-size: 0.9rem; color: var(--text-secondary);">
                                        {{.FirstDate.Format "2006-01-02"}} - {{if .CurrentlyActive}}now{{else}}{{.LastDate.Format "2006-01-02"}}{{end}}
                                    </td>
                                    <td>
                                        {{if .CurrentlyActive}}
                                            <span class="badge badge-success">Active</span>
                                        {{else}}
                                            <span class="badge badge-warning">Inactive</span>
                                        {{end}}
                                    </td>
                                    <td>
                                        <a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem;">
                                            📈 History
                                        </a>
                                    </td>
                                </tr>
                                {{end}}
                            </tbody>
                        </table>
                    </div>
                </div>
            {{end}}
        </div>
    </div>
    <script src="/static/app.js"></script>
</body>
</html>`

	case "node_history":
		return `<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}} - {{.Address}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>{{.Title}} - {{.Address}}</h1>
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if .Error}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>Error:</strong> {{.Error}}
                    </div>
                </div>
            {{else if not .HasHistory}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>Node Not Found:</strong> No historical data available for node {{.Address}}.
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        This node may not exist in the current database, or no nodelist data has been imported yet.
                    </p>
                    <a href="/search" class="btn" style="margin-top: 1rem;">🔍 Search for Other Nodes</a>
                </div>
            {{else}}
                <!-- Current Node Information -->
                {{if .CurrentNode}}
                    <div class="card">
                        <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📊 Current Node Information</h2>
                        
                        <div class="node-info">
                            <div class="info-item">
                                <strong>Address:</strong> {{.CurrentNode.Zone}}:{{.CurrentNode.Net}}/{{.CurrentNode.Node}}
                            </div>
                            {{if .CurrentNode.SystemName}}
                                <div class="info-item">
                                    <strong>System Name:</strong> {{.CurrentNode.SystemName}}
                                </div>
                            {{end}}
                            {{if .CurrentNode.SysopName}}
                                <div class="info-item">
                                    <strong>Sysop:</strong> {{.CurrentNode.SysopName}}
                                </div>
                            {{end}}
                            {{if .CurrentNode.Location}}
                                <div class="info-item">
                                    <strong>Location:</strong> {{.CurrentNode.Location}}
                                </div>
                            {{end}}
                            {{if .CurrentNode.Phone}}
                                <div class="info-item">
                                    <strong>Phone:</strong> {{.CurrentNode.Phone}}
                                </div>
                            {{end}}
                            {{if .CurrentNode.Speed}}
                                <div class="info-item">
                                    <strong>Speed:</strong> {{.CurrentNode.Speed}} baud
                                </div>
                            {{end}}
                            <div class="info-item">
                                <strong>Node Type:</strong> 
                                {{if eq .CurrentNode.NodeType "Zone"}}<span class="badge badge-error">Zone</span>
                                {{else if eq .CurrentNode.NodeType "Region"}}<span class="badge badge-warning">Region</span>
                                {{else if eq .CurrentNode.NodeType "Host"}}<span class="badge badge-info">Host</span>
                                {{else if eq .CurrentNode.NodeType "Hub"}}<span class="badge badge-success">Hub</span>
                                {{else if eq .CurrentNode.NodeType "Pvt"}}<span class="badge badge-warning">Pvt</span>
                                {{else if eq .CurrentNode.NodeType "Down"}}<span class="badge badge-error">Down</span>
                                {{else if eq .CurrentNode.NodeType "Hold"}}<span class="badge badge-warning">Hold</span>
                                {{else}}{{.CurrentNode.NodeType}}{{end}}
                            </div>
                            <div class="info-item">
                                <strong>Last Seen:</strong> {{.CurrentNode.NodelistDate.Format "January 2, 2006"}}
                            </div>
                            {{if .CurrentNode.Flags}}
                                <div class="info-item">
                                    <strong>Flags:</strong> 
                                    {{range $index, $flag := .CurrentNode.Flags}}
                                        {{if $index}}, {{end}}<span class="badge badge-info">{{$flag}}</span>
                                    {{end}}
                                </div>
                            {{end}}
                        </div>
                    </div>
                {{end}}
                
                <!-- Filter Panel -->
                <div class="card filter-panel">
                    <h3 style="margin-bottom: 1rem; color: var(--text-primary);">🔍 Filter Historical Changes</h3>
                    
                    <form method="GET" action="/node/{{.Zone}}/{{.Net}}/{{.Node}}">
                        <div class="filter-options">
                            <label>
                                <input type="checkbox" name="show_system_name" value="1" {{if .ShowSystemName}}checked{{end}}>
                                System Name Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_sysop_name" value="1" {{if .ShowSysopName}}checked{{end}}>
                                Sysop Name Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_location" value="1" {{if .ShowLocation}}checked{{end}}>
                                Location Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_phone" value="1" {{if .ShowPhone}}checked{{end}}>
                                Phone Number Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_speed" value="1" {{if .ShowSpeed}}checked{{end}}>
                                Speed Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_flags" value="1" {{if .ShowFlags}}checked{{end}}>
                                Flag Changes
                            </label>
                            <label>
                                <input type="checkbox" name="show_node_type" value="1" {{if .ShowNodeType}}checked{{end}}>
                                Node Type Changes
                            </label>
                        </div>
                        
                        <div style="margin-top: 1rem;">
                            <button type="submit" class="btn">🔍 Apply Filters</button>
                            <a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}" class="btn btn-secondary">📋 Show All</a>
                        </div>
                    </form>
                    
                    <div style="margin-top: 1rem; font-size: 0.9rem; color: var(--text-secondary);">
                        💡 <strong>Tip:</strong> Select change types you want to see in the timeline below. Uncheck all to see all activity.
                    </div>
                </div>
                
                <!-- Historical Timeline -->
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📈 Historical Timeline</h2>
                    
                    {{if .History}}
                        <div class="timeline">
                            {{range .History}}
                                <div class="timeline-entry {{.ChangeType}}">
                                    <div class="timeline-marker"></div>
                                    <div class="timeline-date">
                                        {{.Date.Format "Jan 2, 2006"}}
                                    </div>
                                    <div class="timeline-content">
                                        <h4>
                                            {{if eq .ChangeType "added"}}📥 Node Added
                                            {{else if eq .ChangeType "removed"}}📤 Node Removed
                                            {{else if eq .ChangeType "modified"}}✏️ Node Modified
                                            {{else}}🔄 Node Updated{{end}}
                                        </h4>
                                        
                                        {{if .Changes}}
                                            <div class="change-list">
                                                {{range .Changes}}
                                                    <div class="change-item">
                                                        <strong>{{.Field}}:</strong>
                                                        {{if .OldValue}}
                                                            <span class="change-value">{{.OldValue}}</span> → 
                                                        {{end}}
                                                        <span class="change-value">{{.NewValue}}</span>
                                                    </div>
                                                {{end}}
                                            </div>
                                        {{else if eq .ChangeType "added"}}
                                            <p>Node first appeared in the nodelist</p>
                                        {{else if eq .ChangeType "removed"}}
                                            <p>Node was removed from the nodelist</p>
                                        {{end}}
                                    </div>
                                </div>
                            {{end}}
                        </div>
                    {{else}}
                        <div class="alert alert-error">
                            <strong>No Changes Found:</strong> No historical changes match your current filter settings.
                        </div>
                        <p style="margin-top: 1rem; color: var(--text-secondary);">
                            Try adjusting your filters above to see different types of changes, or remove all filters to see the complete timeline.
                        </p>
                    {{end}}
                </div>
                
                <div class="alert alert-success">
                    <strong>💡 About Historical Data:</strong> This timeline shows how this node's information has changed over time based on nodelist entries. Each entry represents a snapshot from a specific nodelist file.
                </div>
            {{end}}
        </div>
    </div>
    <script src="/static/app.js"></script>
</body>
</html>`

	case "api_help":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">REST API Documentation for FidoNet Nodelist Database</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help" class="active">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🚀 Getting Started</h2>
                
                <p style="font-size: 1.1rem; margin-bottom: 1.5rem;">
                    The FidoNet Nodelist Database provides a RESTful API for programmatic access to historical and current FidoNet node data. All API endpoints return JSON responses and support CORS for cross-origin requests.
                </p>
                
                <div class="alert alert-success">
                    <strong>Base URL:</strong> <code>{{.BaseURL}}/api</code>
                </div>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📊 Statistics Endpoint</h2>
                
                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">GET /api/stats</h3>
                <p>Returns comprehensive network statistics for a specific date or the most recent data available.</p>
                
                <h4 style="color: var(--text-primary); margin-top: 1.5rem; margin-bottom: 0.5rem;">Query Parameters:</h4>
                <ul style="margin-bottom: 1.5rem;">
                    <li><code>date</code> (optional) - Date in YYYY-MM-DD format</li>
                </ul>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Requests:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; margin-bottom: 1rem;"><code>curl "{{.BaseURL}}/api/stats"
curl "{{.BaseURL}}/api/stats?date=2024-01-15"</code></pre>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Response:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; font-size: 0.9rem;"><code>{
  "date": "2024-01-15T00:00:00Z",
  "total_nodes": 1542,
  "active_nodes": 1389,
  "cm_nodes": 1124,
  "binkp_nodes": 968,
  "internet_nodes": 1205,
  "down_nodes": 45,
  "hold_nodes": 12,
  "pvt_nodes": 96,
  "zone_distribution": {
    "1": 456,
    "2": 789,
    "3": 234,
    "4": 63
  },
  "largest_regions": [
    {
      "zone": 2,
      "region": 50,
      "name": "GERMANY",
      "node_count": 245
    }
  ],
  "largest_nets": [
    {
      "zone": 2,
      "net": 2452,
      "name": "Planet Internet Germany",
      "node_count": 89
    }
  ]
}</code></pre>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🔍 Node Search Endpoint</h2>
                
                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">GET /api/search/nodes</h3>
                <p>Search for nodes using various criteria including address components, system name, location, and sysop name.</p>
                
                <h4 style="color: var(--text-primary); margin-top: 1.5rem; margin-bottom: 0.5rem;">Query Parameters:</h4>
                <ul style="margin-bottom: 1.5rem;">
                    <li><code>zone</code> (optional) - Zone number (1-6)</li>
                    <li><code>net</code> (optional) - Net number</li>
                    <li><code>node</code> (optional) - Node number</li>
                    <li><code>system_name</code> (optional) - Partial system name search</li>
                    <li><code>location</code> (optional) - Partial location search</li>
                    <li><code>sysop_name</code> (optional) - Partial sysop name search</li>
                    <li><code>limit</code> (optional) - Maximum results to return (default: 100, max: 1000)</li>
                </ul>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Requests:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; margin-bottom: 1rem;"><code>curl "{{.BaseURL}}/api/search/nodes?zone=2&net=2452"
curl "{{.BaseURL}}/api/search/nodes?system_name=BBS&limit=50"
curl "{{.BaseURL}}/api/search/nodes?location=Germany"</code></pre>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Response:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; font-size: 0.9rem;"><code>{
  "nodes": [
    {
      "zone": 2,
      "net": 2452,
      "node": 100,
      "system_name": "Example BBS",
      "sysop_name": "John Doe",
      "location": "Berlin, Germany",
      "phone": "+49-30-12345678",
      "speed": 33600,
      "node_type": "",
      "flags": ["CM", "IBN", "INA:john.doe.example.com"],
      "nodelist_date": "2024-01-15T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 100
}</code></pre>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">👤 Sysop Search Endpoint</h2>
                
                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">GET /api/search/sysops</h3>
                <p>Find all nodes that have ever been operated by a specific sysop, including historical records.</p>
                
                <h4 style="color: var(--text-primary); margin-top: 1.5rem; margin-bottom: 0.5rem;">Query Parameters:</h4>
                <ul style="margin-bottom: 1.5rem;">
                    <li><code>sysop_name</code> (required) - Sysop name to search for</li>
                    <li><code>limit</code> (optional) - Maximum results to return (default: 100, max: 1000)</li>
                </ul>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Requests:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; margin-bottom: 1rem;"><code>curl "{{.BaseURL}}/api/search/sysops?sysop_name=John%20Doe"
curl "{{.BaseURL}}/api/search/sysops?sysop_name=Ward%20Dossche&limit=50"</code></pre>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Response:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; font-size: 0.9rem;"><code>{
  "sysop_name": "John Doe",
  "nodes": [
    {
      "zone": 2,
      "net": 2452,
      "node": 100,
      "system_name": "Example BBS",
      "location": "Berlin, Germany",
      "first_date": "2020-01-15T00:00:00Z",
      "last_date": "2024-01-15T00:00:00Z",
      "currently_active": true
    }
  ],
  "count": 1,
  "limit": 100
}</code></pre>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📈 Node History Endpoint</h2>
                
                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">GET /api/nodes/{zone}/{net}/{node}/history</h3>
                <p>Returns complete historical timeline for a specific node, showing all changes over time.</p>
                
                <h4 style="color: var(--text-primary); margin-top: 1.5rem; margin-bottom: 0.5rem;">URL Parameters:</h4>
                <ul style="margin-bottom: 1.5rem;">
                    <li><code>zone</code> - Zone number</li>
                    <li><code>net</code> - Net number</li>
                    <li><code>node</code> - Node number</li>
                </ul>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Request:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; margin-bottom: 1rem;"><code>curl "{{.BaseURL}}/api/nodes/2/2452/100/history"</code></pre>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Example Response:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; font-size: 0.9rem;"><code>{
  "node_address": "2:2452/100",
  "current_node": {
    "zone": 2,
    "net": 2452,
    "node": 100,
    "system_name": "Example BBS v2.0",
    "sysop_name": "John Doe",
    "location": "Berlin, Germany",
    "phone": "+49-30-12345678",
    "speed": 33600,
    "node_type": "",
    "flags": ["CM", "IBN", "INA:john.doe.example.com"],
    "nodelist_date": "2024-01-15T00:00:00Z"
  },
  "history": [
    {
      "date": "2024-01-15T00:00:00Z",
      "change_type": "modified",
      "changes": [
        {
          "field": "system_name",
          "old_value": "Example BBS",
          "new_value": "Example BBS v2.0"
        }
      ]
    },
    {
      "date": "2020-01-15T00:00:00Z",
      "change_type": "added",
      "changes": []
    }
  ]
}</code></pre>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">⚠️ Error Responses</h2>
                
                <p>All API endpoints return appropriate HTTP status codes and error messages in JSON format when errors occur.</p>
                
                <h4 style="color: var(--text-primary); margin-bottom: 0.5rem;">Common Error Responses:</h4>
                <pre style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); overflow-x: auto; font-size: 0.9rem;"><code>// 400 Bad Request
{
  "error": "Invalid date format. Please use YYYY-MM-DD format."
}

// 404 Not Found
{
  "error": "Node not found: 2:2452/999"
}

// 500 Internal Server Error
{
  "error": "Database connection failed"
}</code></pre>
            </div>
            
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🔧 Usage Notes</h2>
                
                <ul style="list-style-type: disc; margin-left: 1.5rem; line-height: 1.6;">
                    <li><strong>Rate Limiting:</strong> API requests are currently not rate-limited, but please be respectful and avoid excessive requests.</li>
                    <li><strong>CORS:</strong> Cross-origin requests are supported for web applications.</li>
                    <li><strong>Date Formats:</strong> All dates in responses are in ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ).</li>
                    <li><strong>Search Behavior:</strong> Text searches are case-insensitive and support partial matching.</li>
                    <li><strong>Historical Data:</strong> All endpoints return the most current data unless specifically requesting historical information.</li>
                    <li><strong>Node Addressing:</strong> FidoNet addresses use the format Zone:Net/Node (e.g., 2:2452/100).</li>
                </ul>
            </div>
            
            <div class="alert alert-success">
                <strong>💡 Need Help?</strong> If you encounter any issues or need additional API endpoints, please check the project documentation or submit an issue on the project repository.
            </div>
        </div>
    </div>
</body>
</html>`

	case "analytics":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Advanced FidoNet network analysis and insights</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics" class="active">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🔬 Network Analytics Dashboard</h2>
                
                <p style="font-size: 1.1rem; color: var(--text-secondary); margin-bottom: 2rem;">
                    Explore detailed analytics and trends in the FidoNet network. These tools provide deep insights into protocol adoption, network evolution, and operational patterns.
                </p>
                
                <div class="analytics-grid">
                    <div class="analytics-card">
                        <h3>📞 V.34 Modem Analysis</h3>
                        <p>Analyze V.34 modem adoption across different zones and time periods. Track the evolution of high-speed dialup technology in FidoNet.</p>
                        <div class="analytics-links">
                            <a href="/analytics/v34" class="btn">📊 View V.34 Analytics</a>
                        </div>
                    </div>
                    
                    <div class="analytics-card">
                        <h3>🌐 Binkp Protocol Analysis</h3>
                        <p>Study Binkp protocol adoption patterns, geographic distribution, and correlation with internet connectivity flags.</p>
                        <div class="analytics-links">
                            <a href="/analytics/binkp" class="btn">📈 View Binkp Analytics</a>
                        </div>
                    </div>
                    
                    <div class="analytics-card">
                        <h3>🏛️ Network Lifecycle</h3>
                        <p>Examine the birth, growth, and evolution of FidoNet networks. Track how different nets have changed over time.</p>
                        <div class="analytics-links">
                            <form method="GET" action="/analytics/network/" style="display: inline;">
                                <input type="number" name="zone" placeholder="Zone" style="width: 60px; margin-right: 0.5rem;" required>
                                <input type="number" name="net" placeholder="Net" style="width: 80px; margin-right: 0.5rem;" required>
                                <button type="submit" class="btn">🔍 Analyze Network</button>
                            </form>
                        </div>
                    </div>
                    
                    <div class="analytics-card">
                        <h3>👥 Sysop Analysis</h3>
                        <p>Discover patterns in sysop names, popular naming conventions, and multi-node operators across the network.</p>
                        <div class="analytics-links">
                            <a href="/analytics/sysops" class="btn">👤 View Sysop Analytics</a>
                        </div>
                    </div>
                    
                    <div class="analytics-card">
                        <h3>📡 Protocol Trends</h3>
                        <p>Track the adoption trends of different protocols and technologies over time across the entire FidoNet.</p>
                        <div class="analytics-links">
                            <a href="/analytics/trends" class="btn">📊 View Protocol Trends</a>
                        </div>
                    </div>
                    
                    <div class="analytics-card">
                        <h3>🔜 More Analytics</h3>
                        <p>Additional analytics modules are planned, including geographic distribution analysis, speed evolution tracking, and node type distribution studies.</p>
                        <div class="analytics-links">
                            <a href="#" class="btn btn-disabled">Coming Soon</a>
                        </div>
                    </div>
                </div>
            </div>
            
            <div class="alert alert-success">
                <strong>💡 About Analytics:</strong> These analytics are generated from historical nodelist data and provide insights into the evolution and characteristics of the FidoNet network over time. Data availability depends on the nodelist files that have been imported into the database.
            </div>
        </div>
    </div>
</body>
</html>`

	case "v34_analytics":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">V.34 modem technology adoption in FidoNet</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if .NoData}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Data Available</strong><br>
                        No nodelist data has been imported yet, or no nodes with V.34 modems found.
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        To populate analytics, please import nodelist files using the parser tool:<br>
                        <code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem;">./bin/parser -path /path/to/nodelists -db ./nodelist.duckdb</code>
                    </p>
                </div>
            {{else}}
                <!-- Summary Card -->
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📞 V.34 Modem Analysis Summary</h2>
                    
                    <div class="stats-grid">
                        <div class="stat-card">
                            <h3>{{.TotalV34Nodes}}</h3>
                            <p>Total V.34 Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{printf "%.1f%%" .V34Percentage}}</h3>
                            <p>V.34 Adoption Rate</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.ZoneCount}}</h3>
                            <p>Zones with V.34</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.NetworkCount}}</h3>
                            <p>Networks with V.34</p>
                        </div>
                    </div>
                    
                    <div style="margin-top: 1.5rem; padding: 1rem; background: #f8fafc; border-radius: var(--radius); font-size: 0.9rem; color: var(--text-secondary);">
                        <strong>About V.34:</strong> V.34 was a 33.6k modem standard that represented the pinnacle of analog dialup technology. Nodes advertising V.34 speeds (33600 baud) were considered high-speed for their time.
                    </div>
                </div>
                
                <!-- Zone Distribution -->
                {{if .ZoneDistribution}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌍 V.34 Distribution by Zone</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Zone</th>
                                        <th>Description</th>
                                        <th>V.34 Nodes</th>
                                        <th>Total Nodes</th>
                                        <th>Adoption Rate</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .ZoneDistribution}}
                                    <tr>
                                        <td><strong>Zone {{.Zone}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{getZoneDescription .Zone}}</td>
                                        <td>{{.V34Count}} nodes</td>
                                        <td>{{.TotalCount}} nodes</td>
                                        <td><strong>{{printf "%.1f%%" .AdoptionRate}}</strong></td>
                                        <td>
                                            <div class="progress-bar">
                                                <div style="width: {{printf "%.1f%%" .AdoptionRate}}; background: var(--success-color);"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Top Networks -->
                {{if .TopNetworks}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🏆 Networks with Most V.34 Nodes</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Network</th>
                                        <th>Network Name</th>
                                        <th>V.34 Nodes</th>
                                        <th>Total Nodes</th>
                                        <th>Adoption Rate</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .TopNetworks}}
                                    <tr>
                                        <td><strong><a href="/analytics/network/?zone={{.Zone}}&net={{.Net}}">{{.Zone}}:{{.Net}}</a></strong></td>
                                        <td style="color: var(--text-secondary);">{{if .Name}}{{.Name}}{{else}}<em>Unknown</em>{{end}}</td>
                                        <td>{{.V34Count}} nodes</td>
                                        <td>{{.TotalCount}} nodes</td>
                                        <td><strong>{{printf "%.1f%%" .AdoptionRate}}</strong></td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <div class="alert alert-success">
                    <strong>💡 Historical Context:</strong> V.34 modems (33.6k) were the fastest analog dialup technology available before the transition to digital technologies like ISDN and early broadband. High V.34 adoption often indicated technologically advanced BBSs and networks.
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "binkp_analytics":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Binkp protocol adoption and internet connectivity analysis</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if .NoData}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Data Available</strong><br>
                        No nodelist data has been imported yet, or no nodes with Binkp support found.
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        To populate analytics, please import nodelist files using the parser tool:<br>
                        <code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem;">./bin/parser -path /path/to/nodelists -db ./nodelist.duckdb</code>
                    </p>
                </div>
            {{else}}
                <!-- Summary Card -->
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌐 Binkp Protocol Analysis Summary</h2>
                    
                    <div class="stats-grid">
                        <div class="stat-card">
                            <h3>{{.TotalBinkpNodes}}</h3>
                            <p>Total Binkp Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{printf "%.1f%%" .BinkpPercentage}}</h3>
                            <p>Binkp Adoption Rate</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.InternetCapableNodes}}</h3>
                            <p>Internet-Capable Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{printf "%.1f%%" .InternetPercentage}}</h3>
                            <p>Internet Adoption Rate</p>
                        </div>
                    </div>
                    
                    <div style="margin-top: 1.5rem; padding: 1rem; background: #f8fafc; border-radius: var(--radius); font-size: 0.9rem; color: var(--text-secondary);">
                        <strong>About Binkp:</strong> Binkp is an IP-based protocol for FidoNet mail and file transfers, allowing nodes to connect over the internet instead of traditional dial-up. It represented a major technological shift in FidoNet operations.
                    </div>
                </div>
                
                <!-- Zone Distribution -->
                {{if .ZoneDistribution}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌍 Binkp Distribution by Zone</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Zone</th>
                                        <th>Description</th>
                                        <th>Binkp Nodes</th>
                                        <th>Total Nodes</th>
                                        <th>Adoption Rate</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .ZoneDistribution}}
                                    <tr>
                                        <td><strong>Zone {{.Zone}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{getZoneDescription .Zone}}</td>
                                        <td>{{.BinkpCount}} nodes</td>
                                        <td>{{.TotalCount}} nodes</td>
                                        <td><strong>{{printf "%.1f%%" .AdoptionRate}}</strong></td>
                                        <td>
                                            <div class="progress-bar">
                                                <div style="width: {{printf "%.1f%%" .AdoptionRate}}; background: var(--primary-color);"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Internet vs Dialup -->
                {{if .ConnectivityBreakdown}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">📡 Connectivity Type Analysis</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Connectivity Type</th>
                                        <th>Node Count</th>
                                        <th>Percentage</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .ConnectivityBreakdown}}
                                    <tr>
                                        <td><strong>{{.Type}}</strong></td>
                                        <td>{{.Count}} nodes</td>
                                        <td><strong>{{printf "%.1f%%" .Percentage}}</strong></td>
                                        <td>
                                            <div class="progress-bar">
                                                <div style="width: {{printf "%.1f%%" .Percentage}}; background: {{if eq .Type "Internet + Binkp"}}var(--success-color){{else if eq .Type "Internet Only"}}var(--primary-color){{else if eq .Type "Binkp Only"}}var(--warning-color){{else}}var(--text-secondary){{end}};"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Top Networks -->
                {{if .TopNetworks}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🏆 Networks with Highest Binkp Adoption</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Network</th>
                                        <th>Network Name</th>
                                        <th>Binkp Nodes</th>
                                        <th>Total Nodes</th>
                                        <th>Adoption Rate</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .TopNetworks}}
                                    <tr>
                                        <td><strong><a href="/analytics/network/?zone={{.Zone}}&net={{.Net}}">{{.Zone}}:{{.Net}}</a></strong></td>
                                        <td style="color: var(--text-secondary);">{{if .Name}}{{.Name}}{{else}}<em>Unknown</em>{{end}}</td>
                                        <td>{{.BinkpCount}} nodes</td>
                                        <td>{{.TotalCount}} nodes</td>
                                        <td><strong>{{printf "%.1f%%" .AdoptionRate}}</strong></td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <div class="alert alert-success">
                    <strong>💡 Historical Significance:</strong> The adoption of Binkp protocol marked FidoNet's transition from pure dial-up to internet-based connectivity. Networks with high Binkp adoption were typically more modern and efficient in their operations.
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "network_lifecycle":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}} - {{.NetworkAddress}}</h1>
            <p class="subtitle">Historical lifecycle analysis for FidoNet network {{.NetworkAddress}}</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if .Error}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>Error:</strong> {{.Error}}
                    </div>
                </div>
            {{else if .NoData}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Data Available</strong><br>
                        No historical data found for network {{.NetworkAddress}}, or no nodelist data has been imported.
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        This network may not exist in the database, or you may need to import nodelist files first.<br>
                        <code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem;">./bin/parser -path /path/to/nodelists -db ./nodelist.duckdb</code>
                    </p>
                    <div style="margin-top: 1.5rem;">
                        <a href="/analytics" class="btn">🔬 Back to Analytics</a>
                    </div>
                </div>
            {{else}}
                <!-- Network Summary -->
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📊 Network {{.NetworkAddress}} Summary</h2>
                    
                    <div class="stats-grid">
                        <div class="stat-card">
                            <h3>{{.Summary.CurrentNodes}}</h3>
                            <p>Current Nodes</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Summary.MaxNodes}}</h3>
                            <p>Peak Node Count</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Summary.FirstSeen.Format "2006-01-02"}}</h3>
                            <p>First Appeared</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.Summary.LastSeen.Format "2006-01-02"}}</h3>
                            <p>Last Updated</p>
                        </div>
                    </div>
                    
                    {{if .Summary.NetworkName}}
                        <div style="margin-top: 1.5rem; padding: 1rem; background: #f8fafc; border-radius: var(--radius);">
                            <strong>Network Name:</strong> {{.Summary.NetworkName}}
                        </div>
                    {{end}}
                </div>
                
                <!-- Node Count Timeline -->
                {{if .Timeline}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">📈 Node Count Over Time</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Date</th>
                                        <th>Node Count</th>
                                        <th>Change</th>
                                        <th>Growth Rate</th>
                                        <th>Trend</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range $index, $entry := .Timeline}}
                                    <tr>
                                        <td><strong>{{.Date.Format "2006-01-02"}}</strong></td>
                                        <td>{{.NodeCount}} nodes</td>
                                        <td style="color: {{if gt .Change 0}}var(--success-color){{else if lt .Change 0}}var(--error-color){{else}}var(--text-secondary){{end}};">
                                            {{if gt .Change 0}}+{{.Change}}{{else if lt .Change 0}}{{.Change}}{{else}}0{{end}}
                                        </td>
                                        <td>
                                            {{if .GrowthRate}}{{printf "%.1f%%" .GrowthRate}}{{else}}-{{end}}
                                        </td>
                                        <td>
                                            {{if gt .Change 0}}📈 Growing
                                            {{else if lt .Change 0}}📉 Declining
                                            {{else}}➡️ Stable{{end}}
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Current Active Nodes -->
                {{if .CurrentNodes}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🖥️ Current Active Nodes ({{len .CurrentNodes}})</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Node</th>
                                        <th>System Name</th>
                                        <th>Sysop</th>
                                        <th>Location</th>
                                        <th>Type</th>
                                        <th>Last Seen</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .CurrentNodes}}
                                    <tr>
                                        <td><strong><a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}">{{.Zone}}:{{.Net}}/{{.Node}}</a></strong></td>
                                        <td>{{if .SystemName}}{{.SystemName}}{{else}}<em>-</em>{{end}}</td>
                                        <td>{{if .SysopName}}{{.SysopName}}{{else}}<em>-</em>{{end}}</td>
                                        <td>{{if .Location}}{{.Location}}{{else}}<em>-</em>{{end}}</td>
                                        <td>
                                            {{if eq .NodeType "Zone"}}<span class="badge badge-error">Zone</span>
                                            {{else if eq .NodeType "Region"}}<span class="badge badge-warning">Region</span>
                                            {{else if eq .NodeType "Host"}}<span class="badge badge-info">Host</span>
                                            {{else if eq .NodeType "Hub"}}<span class="badge badge-success">Hub</span>
                                            {{else if eq .NodeType "Pvt"}}<span class="badge badge-warning">Pvt</span>
                                            {{else if eq .NodeType "Down"}}<span class="badge badge-error">Down</span>
                                            {{else if eq .NodeType "Hold"}}<span class="badge badge-warning">Hold</span>
                                            {{else}}{{.NodeType}}{{end}}
                                        </td>
                                        <td style="font-size: 0.9rem; color: var(--text-secondary);">
                                            {{.NodelistDate.Format "2006-01-02"}}
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <div class="alert alert-success">
                    <strong>💡 About Network Lifecycle:</strong> This analysis tracks how FidoNet networks grow, evolve, and change over time. Node count fluctuations often reflect the health and activity level of the network community.
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "sysop_analytics":
		return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}}</h1>
            <p class="subtitle">Analysis of sysop names and multi-node operations</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/analytics">🔬 Analytics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            {{if .NoData}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Data Available</strong><br>
                        No nodelist data has been imported yet.
                    </div>
                    <p style="margin-top: 1rem; color: var(--text-secondary);">
                        To populate analytics, please import nodelist files using the parser tool:<br>
                        <code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem;">./bin/parser -path /path/to/nodelists -db ./nodelist.duckdb</code>
                    </p>
                </div>
            {{else}}
                <!-- Summary Card -->
                <div class="card">
                    <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">👥 Sysop Analysis Summary</h2>
                    
                    <div class="stats-grid">
                        <div class="stat-card">
                            <h3>{{.TotalSysops}}</h3>
                            <p>Unique Sysops</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.MultiNodeSysops}}</h3>
                            <p>Multi-Node Operators</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{printf "%.1f%%" .MultiNodePercentage}}</h3>
                            <p>Multi-Node Rate</p>
                        </div>
                        
                        <div class="stat-card">
                            <h3>{{.MaxNodesPerSysop}}</h3>
                            <p>Max Nodes/Sysop</p>
                        </div>
                    </div>
                </div>
                
                <!-- Top Multi-Node Sysops -->
                {{if .TopMultiNodeSysops}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🏆 Top Multi-Node Operators</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Sysop Name</th>
                                        <th>Node Count</th>
                                        <th>Networks</th>
                                        <th>Zones</th>
                                        <th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .TopMultiNodeSysops}}
                                    <tr>
                                        <td><strong>{{.SysopName}}</strong></td>
                                        <td>{{.NodeCount}} nodes</td>
                                        <td>{{.NetworkCount}} networks</td>
                                        <td>{{.ZoneCount}} zones</td>
                                        <td>
                                            <a href="/search/sysop?sysop_name={{.SysopName}}" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem;">
                                                🔍 View Nodes
                                            </a>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Popular Name Patterns -->
                {{if .NamePatterns}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">📛 Popular Name Patterns</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>First Name</th>
                                        <th>Occurrences</th>
                                        <th>Percentage</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .NamePatterns}}
                                    <tr>
                                        <td><strong>{{.FirstName}}</strong></td>
                                        <td>{{.Count}} sysops</td>
                                        <td>{{printf "%.1f%%" .Percentage}}</td>
                                        <td>
                                            <div class="progress-bar">
                                                <div style="width: {{printf "%.1f%%" .Percentage}}; background: var(--primary-color);"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <!-- Zone Distribution -->
                {{if .ZoneDistribution}}
                    <div class="card">
                        <h3 style="color: var(--text-primary); margin-bottom: 1.5rem;">🌍 Multi-Node Operators by Zone</h3>
                        
                        <div class="table-container">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Zone</th>
                                        <th>Description</th>
                                        <th>Multi-Node Sysops</th>
                                        <th>Total Sysops</th>
                                        <th>Multi-Node Rate</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .ZoneDistribution}}
                                    <tr>
                                        <td><strong>Zone {{.Zone}}</strong></td>
                                        <td style="color: var(--text-secondary);">{{getZoneDescription .Zone}}</td>
                                        <td>{{.MultiNodeSysops}} sysops</td>
                                        <td>{{.TotalSysops}} sysops</td>
                                        <td><strong>{{printf "%.1f%%" .MultiNodeRate}}</strong></td>
                                        <td>
                                            <div class="progress-bar">
                                                <div style="width: {{printf "%.1f%%" .MultiNodeRate}}; background: var(--success-color);"></div>
                                            </div>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                {{end}}
                
                <div class="alert alert-success">
                    <strong>💡 About Sysop Analysis:</strong> Multi-node operators often represented larger BBS operations, networks, or technical experts who managed multiple systems. High multi-node rates in a zone might indicate more commercial or professional FidoNet operations.
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	default:
		return "Template not found"
	}
}