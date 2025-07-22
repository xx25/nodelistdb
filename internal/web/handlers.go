package web

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
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
	templates := []string{"index", "search", "node", "stats", "sysop_search", "node_history"}
	
	for _, tmpl := range templates {
		s.templates[tmpl] = template.Must(template.New(tmpl).Parse(s.getTemplate(tmpl)))
	}
}

// IndexHandler handles the home page
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "FidoNet Nodelist Database",
	}
	
	s.templates["index"].Execute(w, data)
}

// SearchHandler handles node search page
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []database.Node
	var err error
	
	if r.Method == http.MethodPost {
		// Handle search form submission
		filter := database.NodeFilter{
			Limit: 100,
		}
		
		if zone := r.FormValue("zone"); zone != "" {
			if z, parseErr := strconv.Atoi(zone); parseErr == nil {
				filter.Zone = &z
			}
		}
		
		if net := r.FormValue("net"); net != "" {
			if n, parseErr := strconv.Atoi(net); parseErr == nil {
				filter.Net = &n
			}
		}
		
		if node := r.FormValue("node"); node != "" {
			if n, parseErr := strconv.Atoi(node); parseErr == nil {
				filter.Node = &n
			}
		}
		
		if systemName := r.FormValue("system_name"); systemName != "" {
			filter.SystemName = &systemName
		}
		
		if location := r.FormValue("location"); location != "" {
			filter.Location = &location
		}
		
		nodes, err = s.storage.GetNodes(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	data := struct {
		Title   string
		Nodes   []database.Node
		Count   int
		Error   string
	}{
		Title: "Search Nodes",
		Nodes: nodes,
		Count: len(nodes),
	}
	
	if err != nil {
		data.Error = err.Error()
	}
	
	s.templates["search"].Execute(w, data)
}

// StatsHandler handles statistics page
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	date := time.Now().Truncate(24 * time.Hour)
	
	stats, err := s.storage.GetStats(date)
	if err != nil {
		// If no stats for today, try to find any available stats
		stats = &database.NetworkStats{
			Date: date,
		}
	}
	
	data := struct {
		Title string
		Stats *database.NetworkStats
		Error string
	}{
		Title: "Network Statistics",
		Stats: stats,
	}
	
	if err != nil {
		data.Error = err.Error()
	}
	
	s.templates["stats"].Execute(w, data)
}

// SysopSearchHandler handles sysop name search page
func (s *Server) SysopSearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var sysopName string
	var err error
	
	if r.Method == http.MethodPost {
		sysopName = r.FormValue("sysop_name")
		if sysopName != "" {
			nodes, err = s.storage.SearchNodesBySysop(sysopName, 50)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	
	data := struct {
		Title     string
		Nodes     []storage.NodeSummary
		Count     int
		SysopName string
		Error     string
	}{
		Title:     "Search by Sysop Name",
		Nodes:     nodes,
		Count:     len(nodes),
		SysopName: sysopName,
	}
	
	if err != nil {
		data.Error = err.Error()
	}
	
	s.templates["sysop_search"].Execute(w, data)
}

// NodeHistoryHandler handles node history page
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /node/{zone}/{net}/{node}
	path := r.URL.Path[6:] // Remove "/node/"
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid node address format", http.StatusBadRequest)
		return
	}
	
	zone, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}
	
	net, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}
	
	node, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}
	
	// Get node history
	history, err := s.storage.GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	
	// Get date range
	firstDate, lastDate, _ := s.storage.GetNodeDateRange(zone, net, node)
	
	// Parse filter options from query parameters
	query := r.URL.Query()
	filter := storage.ChangeFilter{
		IgnoreFlags:    query.Get("noflags") == "1",
		IgnorePhone:    query.Get("nophone") == "1",
		IgnoreSpeed:    query.Get("nospeed") == "1",
		IgnoreStatus:   query.Get("nostatus") == "1",
		IgnoreLocation: query.Get("nolocation") == "1",
		IgnoreName:     query.Get("noname") == "1",
		IgnoreSysop:    query.Get("nosysop") == "1",
	}
	
	// Get changes
	changes, err := s.storage.GetNodeChanges(zone, net, node, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	data := struct {
		Title     string
		Address   string
		Zone      int
		Net       int
		Node      int
		History   []database.Node
		Changes   []database.NodeChange
		FirstDate time.Time
		LastDate  time.Time
		Filter    storage.ChangeFilter
	}{
		Title:     "Node History",
		Address:   fmt.Sprintf("%d:%d/%d", zone, net, node),
		Zone:      zone,
		Net:       net,
		Node:      node,
		History:   history,
		Changes:   changes,
		FirstDate: firstDate,
		LastDate:  lastDate,
		Filter:    filter,
	}
	
	s.templates["node_history"].Execute(w, data)
}

// SetupRoutes sets up web routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.IndexHandler)
	mux.HandleFunc("/search", s.SearchHandler)
	mux.HandleFunc("/search/sysop", s.SysopSearchHandler)
	mux.HandleFunc("/stats", s.StatsHandler)
	mux.HandleFunc("/node/", s.NodeHistoryHandler)
	
	// Serve static files
	mux.HandleFunc("/static/", s.StaticHandler)
}

// StaticHandler serves static files
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
	// Simple static file serving - in production, use http.FileServer
	path := r.URL.Path[len("/static/"):]
	
	switch filepath.Ext(path) {
	case ".css":
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(s.getCSS()))
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(s.getJS()))
	default:
		http.NotFound(w, r)
	}
}

// Template definitions
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
                <a href="/api/health">⚡ API Health</a>
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
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <form method="post" class="search-form">
                    <div class="form-group">
                        <label for="zone">Zone:</label>
                        <input type="number" id="zone" name="zone" placeholder="e.g. 1" min="1">
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
            </div>
        </nav>
        
        <div class="content">
            {{if .Error}}
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
                                        <th>Node Count</th>
                                        <th>Percentage</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range $zone, $count := .Stats.ZoneDistribution}}
                                    <tr>
                                        <td><strong>Zone {{$zone}}</strong></td>
                                        <td>{{$count}} nodes</td>
                                        <td style="color: var(--text-secondary);">
                                            {{printf "%.1f%%" (div (mul (printf "%.0f" $count) 100.0) (printf "%.0f" $.Stats.TotalNodes))}}
                                        </td>
                                        <td>
                                            <div style="background: #e2e8f0; height: 8px; border-radius: 4px; overflow: hidden;">
                                                <div style="background: var(--primary-color); height: 100%; width: {{printf "%.1f%%" (div (mul (printf "%.0f" $count) 100.0) (printf "%.0f" $.Stats.TotalNodes))}}; transition: width 0.3s ease;"></div>
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
            {{else}}
                <div class="card">
                    <div class="alert alert-error">
                        <strong>📊 No Statistics Available</strong><br>
                        No statistical data is available for the current date. This might mean no nodelist data has been imported yet.
                    </div>
                </div>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "sysop_search":
		return `<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>{{.Title}}</h1>
        <nav>
            <a href="/">Home</a> | 
            <a href="/search">Search Nodes</a> | 
            <a href="/search/sysop">Search by Sysop</a> | 
            <a href="/stats">Statistics</a>
        </nav>
        
        <div class="content">
            <form method="post">
                <div class="form-group">
                    <label for="sysop_name">Sysop Name:</label>
                    <input type="text" id="sysop_name" name="sysop_name" value="{{.SysopName}}" placeholder="e.g. John Doe" required>
                </div>
                
                <button type="submit">Search</button>
            </form>
            
            {{if .Error}}
                <div class="error">Error: {{.Error}}</div>
            {{end}}
            
            {{if .Nodes}}
                <h2>Results ({{.Count}} nodes found)</h2>
                <table>
                    <thead>
                        <tr>
                            <th>Address</th>
                            <th>System Name</th>
                            <th>Location</th>
                            <th>Sysop</th>
                            <th>Active Period</th>
                            <th>Currently Active</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Nodes}}
                        <tr>
                            <td><a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}">{{.Zone}}:{{.Net}}/{{.Node}}</a></td>
                            <td>{{.SystemName}}</td>
                            <td>{{.Location}}</td>
                            <td>{{.SysopName}}</td>
                            <td>{{.FirstDate.Format "2006-01-02"}} - {{if .CurrentlyActive}}now{{else}}{{.LastDate.Format "2006-01-02"}}{{end}}</td>
                            <td>{{if .CurrentlyActive}}Yes{{else}}No{{end}}</td>
                            <td><a href="/node/{{.Zone}}/{{.Net}}/{{.Node}}">View History</a></td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            {{end}}
        </div>
    </div>
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
            <a href="/">Home</a> | 
            <a href="/search">Search Nodes</a> | 
            <a href="/search/sysop">Search by Sysop</a> | 
            <a href="/stats">Statistics</a>
        </nav>
        
        <div class="content">
            <h2>Node Information</h2>
            <p><strong>Address:</strong> {{.Address}}</p>
            <p><strong>Active Period:</strong> {{.FirstDate.Format "2006-01-02"}} - {{.LastDate.Format "2006-01-02"}}</p>
            <p><strong>Total Entries:</strong> {{len .History}}</p>
            <p><strong>Changes:</strong> {{len .Changes}}</p>
            
            <h3>Filter Options</h3>
            <form method="get">
                <div class="filter-options">
                    <label><input type="checkbox" name="noflags" value="1" {{if .Filter.IgnoreFlags}}checked{{end}}> Ignore flag changes</label>
                    <label><input type="checkbox" name="nophone" value="1" {{if .Filter.IgnorePhone}}checked{{end}}> Ignore phone changes</label>
                    <label><input type="checkbox" name="nospeed" value="1" {{if .Filter.IgnoreSpeed}}checked{{end}}> Ignore speed changes</label>
                    <label><input type="checkbox" name="nostatus" value="1" {{if .Filter.IgnoreStatus}}checked{{end}}> Ignore status changes</label>
                    <label><input type="checkbox" name="nolocation" value="1" {{if .Filter.IgnoreLocation}}checked{{end}}> Ignore location changes</label>
                    <label><input type="checkbox" name="noname" value="1" {{if .Filter.IgnoreName}}checked{{end}}> Ignore name changes</label>
                    <label><input type="checkbox" name="nosysop" value="1" {{if .Filter.IgnoreSysop}}checked{{end}}> Ignore sysop changes</label>
                </div>
                <button type="submit">Apply Filters</button>
            </form>
            
            <h3>Change History</h3>
            <div class="timeline">
                {{range .Changes}}
                <div class="timeline-entry {{.ChangeType}}">
                    <div class="timeline-marker"></div>
                    <div class="timeline-date">
                        <strong>{{.Date.Format "Jan 2, 2006"}}</strong><br>
                        <small>nodelist.{{printf "%03d" .DayNumber}}</small>
                    </div>
                    <div class="timeline-content">
                        {{if eq .ChangeType "added"}}
                            <h4>✅ Node added to nodelist</h4>
                            {{if .NewNode}}
                            <p><strong>{{.NewNode.SystemName}}</strong> - {{.NewNode.Location}} ({{.NewNode.SysopName}})</p>
                            {{end}}
                        {{else if eq .ChangeType "removed"}}
                            <h4>❌ Node removed from nodelist</h4>
                            <p style="color: var(--text-secondary);">Node was no longer listed in subsequent nodelists</p>
                        {{else if eq .ChangeType "modified"}}
                            <h4>📝 Node information changed</h4>
                            <ul>
                                {{range $field, $change := .Changes}}
                                <li><strong>{{$field}}:</strong> {{$change}}</li>
                                {{end}}
                            </ul>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>
            
            {{if not .Changes}}
                <p>No changes found with current filter settings.</p>
            {{end}}
        </div>
    </div>
</body>
</html>`

	default:
		return "<html><body><h1>Template not found</h1></body></html>"
	}
}

func (s *Server) getCSS() string {
	return `
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap');

:root {
    --primary-color: #2563eb;
    --primary-hover: #1d4ed8;
    --secondary-color: #64748b;
    --success-color: #10b981;
    --warning-color: #f59e0b;
    --error-color: #ef4444;
    --background: #f8fafc;
    --card-bg: #ffffff;
    --text-primary: #1e293b;
    --text-secondary: #64748b;
    --border-color: #e2e8f0;
    --shadow: 0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1);
    --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1);
    --radius: 0.5rem;
    --radius-lg: 0.75rem;
}

* {
    box-sizing: border-box;
}

body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    margin: 0;
    padding: 0;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    min-height: 100vh;
    color: var(--text-primary);
    line-height: 1.6;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 2rem;
    background: var(--card-bg);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    margin-top: 2rem;
    margin-bottom: 2rem;
}

header {
    text-align: center;
    margin-bottom: 3rem;
    padding-bottom: 2rem;
    border-bottom: 2px solid var(--border-color);
}

h1 {
    font-size: 2.5rem;
    font-weight: 700;
    background: linear-gradient(135deg, var(--primary-color), #9333ea);
    background-clip: text;
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    margin: 0 0 0.5rem 0;
}

.subtitle {
    font-size: 1.1rem;
    color: var(--text-secondary);
    font-weight: 400;
}

nav {
    background: var(--card-bg);
    padding: 1rem 0;
    margin-bottom: 2rem;
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
}

.nav-container {
    display: flex;
    justify-content: center;
    flex-wrap: wrap;
    gap: 0.5rem;
}

nav a {
    display: inline-flex;
    align-items: center;
    padding: 0.75rem 1.5rem;
    color: var(--text-secondary);
    text-decoration: none;
    border-radius: var(--radius);
    font-weight: 500;
    transition: all 0.2s ease;
    border: 1px solid transparent;
}

nav a:hover {
    background: var(--primary-color);
    color: white;
    transform: translateY(-1px);
    box-shadow: var(--shadow);
}

nav a.active {
    background: var(--primary-color);
    color: white;
    box-shadow: var(--shadow);
}

.content {
    margin-top: 2rem;
}

.card {
    background: var(--card-bg);
    border-radius: var(--radius-lg);
    padding: 2rem;
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
    margin-bottom: 2rem;
}

.search-form {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1.5rem;
    margin-bottom: 2rem;
}

.form-group {
    display: flex;
    flex-direction: column;
}

.form-group label {
    font-weight: 600;
    margin-bottom: 0.5rem;
    color: var(--text-primary);
    font-size: 0.9rem;
}

.form-group input {
    padding: 0.75rem 1rem;
    border: 2px solid var(--border-color);
    border-radius: var(--radius);
    font-size: 1rem;
    transition: all 0.2s ease;
    background: var(--card-bg);
}

.form-group input:focus {
    outline: none;
    border-color: var(--primary-color);
    box-shadow: 0 0 0 3px rgb(37 99 235 / 0.1);
}

.btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.75rem 1.5rem;
    background: var(--primary-color);
    color: white;
    border: none;
    border-radius: var(--radius);
    font-weight: 600;
    font-size: 1rem;
    cursor: pointer;
    transition: all 0.2s ease;
    text-decoration: none;
    gap: 0.5rem;
}

.btn:hover {
    background: var(--primary-hover);
    transform: translateY(-1px);
    box-shadow: var(--shadow);
}

.btn-secondary {
    background: var(--secondary-color);
}

.btn-secondary:hover {
    background: #475569;
}

.btn-success {
    background: var(--success-color);
}

.btn-success:hover {
    background: #059669;
}

.table-container {
    background: var(--card-bg);
    border-radius: var(--radius-lg);
    overflow: hidden;
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
    margin: 2rem 0;
}

table {
    width: 100%;
    border-collapse: collapse;
}

table th {
    background: linear-gradient(135deg, #f1f5f9, #e2e8f0);
    padding: 1rem;
    text-align: left;
    font-weight: 600;
    color: var(--text-primary);
    border-bottom: 2px solid var(--border-color);
}

table td {
    padding: 1rem;
    border-bottom: 1px solid var(--border-color);
    vertical-align: top;
}

table tr:hover {
    background: #f8fafc;
}

table a {
    color: var(--primary-color);
    text-decoration: none;
    font-weight: 500;
}

table a:hover {
    text-decoration: underline;
}

.stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 1.5rem;
    margin: 2rem 0;
}

.stat-card {
    background: linear-gradient(135deg, var(--card-bg), #f8fafc);
    padding: 2rem;
    border-radius: var(--radius-lg);
    text-align: center;
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
    transition: all 0.2s ease;
}

.stat-card:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-lg);
}

.stat-card h3 {
    font-size: 2.5rem;
    font-weight: 700;
    margin: 0;
    color: var(--primary-color);
}

.stat-card p {
    margin: 0.5rem 0 0 0;
    color: var(--text-secondary);
    font-weight: 500;
    text-transform: uppercase;
    font-size: 0.85rem;
    letter-spacing: 0.05em;
}

.alert {
    padding: 1rem 1.5rem;
    border-radius: var(--radius);
    margin: 1rem 0;
    border-left: 4px solid;
}

.alert-error {
    background: #fef2f2;
    color: #991b1b;
    border-left-color: var(--error-color);
}

.alert-success {
    background: #f0fdf4;
    color: #166534;
    border-left-color: var(--success-color);
}

.filter-panel {
    background: #f8fafc;
    padding: 1.5rem;
    border-radius: var(--radius);
    border: 1px solid var(--border-color);
    margin: 2rem 0;
}

.filter-options {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin: 1rem 0;
}

.filter-options label {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-weight: 400;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: var(--radius);
    transition: background 0.2s ease;
}

.filter-options label:hover {
    background: white;
}

.filter-options input[type="checkbox"] {
    width: 1.2rem;
    height: 1.2rem;
    accent-color: var(--primary-color);
}

.timeline {
    margin: 2rem 0;
    position: relative;
}

.timeline::before {
    content: '';
    position: absolute;
    left: 2rem;
    top: 0;
    bottom: 0;
    width: 2px;
    background: linear-gradient(to bottom, var(--primary-color), transparent);
}

.timeline-entry {
    display: flex;
    margin-bottom: 2rem;
    position: relative;
    align-items: flex-start;
}

.timeline-marker {
    flex: 0 0 4rem;
    height: 4rem;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
}

.timeline-marker::after {
    content: '';
    width: 1rem;
    height: 1rem;
    border-radius: 50%;
    background: var(--primary-color);
    border: 3px solid white;
    box-shadow: var(--shadow);
    z-index: 2;
}

.timeline-entry.added .timeline-marker::after {
    background: var(--success-color);
}

.timeline-entry.removed .timeline-marker::after {
    background: var(--error-color);
}

.timeline-entry.modified .timeline-marker::after {
    background: var(--warning-color);
}

.timeline-date {
    flex: 0 0 140px;
    text-align: right;
    padding-right: 1.5rem;
    font-size: 0.9rem;
    color: var(--text-secondary);
    font-weight: 500;
}

.timeline-content {
    flex: 1;
    background: var(--card-bg);
    padding: 1.5rem;
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
    margin-left: 1rem;
}

.timeline-content h4 {
    margin: 0 0 1rem 0;
    font-weight: 600;
    color: var(--text-primary);
}

.timeline-content ul {
    margin: 0;
    padding-left: 1.5rem;
}

.timeline-content li {
    margin: 0.5rem 0;
    color: var(--text-secondary);
}

.added .timeline-content {
    border-left: 4px solid var(--success-color);
}

.removed .timeline-content {
    border-left: 4px solid var(--error-color);
}

.modified .timeline-content {
    border-left: 4px solid var(--warning-color);
}

.node-info {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin: 2rem 0;
}

.info-item {
    background: #f8fafc;
    padding: 1rem;
    border-radius: var(--radius);
    border: 1px solid var(--border-color);
}

.info-item strong {
    display: block;
    color: var(--text-primary);
    font-weight: 600;
    margin-bottom: 0.25rem;
}

@media (max-width: 768px) {
    .container {
        margin: 1rem;
        padding: 1rem;
        border-radius: var(--radius);
    }
    
    .nav-container {
        flex-direction: column;
    }
    
    .search-form {
        grid-template-columns: 1fr;
    }
    
    .stats-grid {
        grid-template-columns: 1fr;
    }
    
    .timeline-entry {
        flex-direction: column;
        align-items: flex-start;
    }
    
    .timeline-date {
        flex: none;
        text-align: left;
        padding: 0 0 0.5rem 0;
    }
    
    .timeline-content {
        margin-left: 0;
        width: 100%;
    }
}

.loading {
    display: inline-block;
    width: 1.2rem;
    height: 1.2rem;
    border: 2px solid #f3f3f3;
    border-top: 2px solid var(--primary-color);
    border-radius: 50%;
    animation: spin 1s linear infinite;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}

.badge {
    display: inline-block;
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
    font-weight: 500;
    border-radius: 9999px;
    text-transform: uppercase;
    letter-spacing: 0.025em;
}

.badge-success {
    background: #dcfce7;
    color: #166534;
}

.badge-error {
    background: #fee2e2;
    color: #991b1b;
}

.badge-warning {
    background: #fef3c7;
    color: #92400e;
}

.badge-info {
    background: #dbeafe;
    color: #1e40af;
}
`
}

func (s *Server) getJS() string {
	return `
// Simple JavaScript for form enhancements
document.addEventListener('DOMContentLoaded', function() {
    // Auto-focus first input on search page
    const firstInput = document.querySelector('input[type="number"], input[type="text"]');
    if (firstInput) {
        firstInput.focus();
    }
    
    // Add form validation
    const form = document.querySelector('form');
    if (form) {
        form.addEventListener('submit', function(e) {
            const inputs = form.querySelectorAll('input');
            let hasValue = false;
            
            inputs.forEach(function(input) {
                if (input.value.trim()) {
                    hasValue = true;
                }
            });
            
            if (!hasValue) {
                e.preventDefault();
                alert('Please enter at least one search criteria');
            }
        });
    }
});
`
}