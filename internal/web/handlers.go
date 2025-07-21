package web

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
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
	templates := []string{"index", "search", "node", "stats"}
	
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

// SetupRoutes sets up web routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.IndexHandler)
	mux.HandleFunc("/search", s.SearchHandler)
	mux.HandleFunc("/stats", s.StatsHandler)
	
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
            <a href="/stats">Statistics</a> |
            <a href="/api/health">API Health</a>
        </nav>
        
        <div class="content">
            <p>Welcome to the FidoNet Nodelist Database. This system provides access to historical and current FidoNet node information.</p>
            
            <h2>Quick Actions</h2>
            <ul>
                <li><a href="/search">Search for nodes</a> - Find nodes by zone, net, system name, or location</li>
                <li><a href="/stats">View statistics</a> - See network statistics and trends</li>
                <li><a href="/api/health">API status</a> - Check API health</li>
            </ul>
        </div>
    </div>
</body>
</html>`

	case "search":
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
            <a href="/stats">Statistics</a>
        </nav>
        
        <div class="content">
            <form method="post">
                <div class="form-group">
                    <label for="zone">Zone:</label>
                    <input type="number" id="zone" name="zone" placeholder="e.g. 1">
                </div>
                
                <div class="form-group">
                    <label for="net">Net:</label>
                    <input type="number" id="net" name="net" placeholder="e.g. 234">
                </div>
                
                <div class="form-group">
                    <label for="node">Node:</label>
                    <input type="number" id="node" name="node" placeholder="e.g. 56">
                </div>
                
                <div class="form-group">
                    <label for="system_name">System Name:</label>
                    <input type="text" id="system_name" name="system_name" placeholder="e.g. Example BBS">
                </div>
                
                <div class="form-group">
                    <label for="location">Location:</label>
                    <input type="text" id="location" name="location" placeholder="e.g. New York">
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
                            <th>Type</th>
                            <th>Date</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Nodes}}
                        <tr>
                            <td>{{.Zone}}:{{.Net}}/{{.Node}}</td>
                            <td>{{.SystemName}}</td>
                            <td>{{.Location}}</td>
                            <td>{{.SysopName}}</td>
                            <td>{{.NodeType}}</td>
                            <td>{{.NodelistDate.Format "2006-01-02"}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            {{end}}
        </div>
    </div>
</body>
</html>`

	case "stats":
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
            <a href="/stats">Statistics</a>
        </nav>
        
        <div class="content">
            {{if .Error}}
                <div class="error">Error: {{.Error}}</div>
            {{else if .Stats}}
                <h2>Network Statistics for {{.Stats.Date.Format "2006-01-02"}}</h2>
                
                <div class="stats-grid">
                    <div class="stat-item">
                        <h3>{{.Stats.TotalNodes}}</h3>
                        <p>Total Nodes</p>
                    </div>
                    
                    <div class="stat-item">
                        <h3>{{.Stats.ActiveNodes}}</h3>
                        <p>Active Nodes</p>
                    </div>
                    
                    <div class="stat-item">
                        <h3>{{.Stats.CMNodes}}</h3>
                        <p>CM Nodes</p>
                    </div>
                    
                    <div class="stat-item">
                        <h3>{{.Stats.BinkpNodes}}</h3>
                        <p>Binkp Nodes</p>
                    </div>
                    
                    <div class="stat-item">
                        <h3>{{.Stats.InternetNodes}}</h3>
                        <p>Internet Nodes</p>
                    </div>
                    
                    <div class="stat-item">
                        <h3>{{.Stats.DownNodes}}</h3>
                        <p>Down Nodes</p>
                    </div>
                </div>
                
                {{if .Stats.ZoneDistribution}}
                    <h3>Zone Distribution</h3>
                    <table>
                        <thead>
                            <tr>
                                <th>Zone</th>
                                <th>Node Count</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range $zone, $count := .Stats.ZoneDistribution}}
                            <tr>
                                <td>{{$zone}}</td>
                                <td>{{$count}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                {{end}}
            {{else}}
                <p>No statistics available for this date.</p>
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
body {
    font-family: Arial, sans-serif;
    margin: 0;
    padding: 0;
    background-color: #f5f5f5;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
    background-color: white;
    box-shadow: 0 0 10px rgba(0,0,0,0.1);
}

h1 {
    color: #333;
    border-bottom: 2px solid #007acc;
    padding-bottom: 10px;
}

nav {
    margin: 20px 0;
    padding: 10px 0;
    border-bottom: 1px solid #ddd;
}

nav a {
    color: #007acc;
    text-decoration: none;
    margin-right: 10px;
}

nav a:hover {
    text-decoration: underline;
}

.content {
    margin-top: 20px;
}

.form-group {
    margin-bottom: 15px;
}

.form-group label {
    display: block;
    margin-bottom: 5px;
    font-weight: bold;
}

.form-group input {
    width: 200px;
    padding: 8px;
    border: 1px solid #ddd;
    border-radius: 4px;
}

button {
    background-color: #007acc;
    color: white;
    padding: 10px 20px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
}

button:hover {
    background-color: #005a99;
}

table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 20px;
}

table th, table td {
    border: 1px solid #ddd;
    padding: 8px;
    text-align: left;
}

table th {
    background-color: #f2f2f2;
    font-weight: bold;
}

table tr:nth-child(even) {
    background-color: #f9f9f9;
}

.stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 20px;
    margin: 20px 0;
}

.stat-item {
    background-color: #f8f9fa;
    padding: 20px;
    border-radius: 8px;
    text-align: center;
    border-left: 4px solid #007acc;
}

.stat-item h3 {
    font-size: 2em;
    margin: 0;
    color: #007acc;
}

.stat-item p {
    margin: 10px 0 0 0;
    color: #666;
    font-weight: bold;
}

.error {
    background-color: #ffebee;
    color: #c62828;
    padding: 10px;
    border-radius: 4px;
    margin: 20px 0;
    border-left: 4px solid #c62828;
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