package web

import (
	"html/template"
)

// loadTemplates loads HTML templates
func (s *Server) loadTemplates() {
	templates := []string{"index", "search", "node", "stats", "sysop_search", "node_history", "api_help"}
	
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
	
	for _, tmpl := range templates {
		s.templates[tmpl] = template.Must(template.New(tmpl).Funcs(funcMap).Parse(s.getTemplate(tmpl)))
	}
}

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
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
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
                                        <td style="color: var(--text-secondary);">
                                            {{if eq $zone 1}}United States and Canada
                                            {{else if eq $zone 2}}Europe, Former Soviet Union, and Israel
                                            {{else if eq $zone 3}}Australasia (includes former Zone 6 nodes)
                                            {{else if eq $zone 4}}Latin America (except Puerto Rico)
                                            {{else if eq $zone 5}}Africa
                                            {{else if eq $zone 6}}Asia (removed July 2007, nodes moved to Zone 3)
                                            {{else}}Unknown{{end}}
                                        </td>
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
                                        <th>Description</th>
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
                                        <th>Description</th>
                                        <th>Node Count</th>
                                        <th>Representation</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .Stats.LargestNets}}
                                    <tr>
                                        <td><strong>{{.Zone}}:{{.Net}}</strong></td>
                                        <td>{{if .Name}}{{.Name}}{{else}}<em>-</em>{{end}}</td>
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
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - {{.Address}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>{{.Title}} - {{.Address}}</h1>
            <p class="subtitle">Historical data and changes for FidoNet node</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/api/help">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <h2>Node Information</h2>
                <p><strong>Address:</strong> {{.Address}}</p>
                <p><strong>Active Period:</strong> {{.FirstDate.Format "2006-01-02"}} - {{if .CurrentlyActive}}now{{else}}{{.LastDate.Format "2006-01-02"}}{{end}}</p>
                <p><strong>Total Entries:</strong> {{len .History}}</p>
                <p><strong>Changes:</strong> {{len .Changes}}</p>
            </div>
            
            <div class="card">
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
                    <label><input type="checkbox" name="noconnectivity" value="1" {{if .Filter.IgnoreConnectivity}}checked{{end}}> Ignore connectivity changes (Binkp/Telnet)</label>
                    <label><input type="checkbox" name="nomodemflags" value="1" {{if .Filter.IgnoreModemFlags}}checked{{end}}> Ignore modem flag changes</label>
                    <label><input type="checkbox" name="nointernetprotocols" value="1" {{if .Filter.IgnoreInternetProtocols}}checked{{end}}> Ignore internet protocol changes</label>
                    <label><input type="checkbox" name="nointernethostnames" value="1" {{if .Filter.IgnoreInternetHostnames}}checked{{end}}> Ignore internet hostname changes</label>
                    <label><input type="checkbox" name="nointernetports" value="1" {{if .Filter.IgnoreInternetPorts}}checked{{end}}> Ignore internet port changes</label>
                    <label><input type="checkbox" name="nointernetemails" value="1" {{if .Filter.IgnoreInternetEmails}}checked{{end}}> Ignore internet email changes</label>
                </div>
                <button type="submit" class="btn">Apply Filters</button>
            </form>
            </div>
            
            <div class="card">
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
                            <div class="node-info">
                                <div class="info-item">
                                    <strong>System Name</strong>
                                    {{if .NewNode.SystemName}}{{.NewNode.SystemName}}{{else}}<em>-</em>{{end}}
                                </div>
                                <div class="info-item">
                                    <strong>Location</strong>
                                    {{if .NewNode.Location}}{{.NewNode.Location}}{{else}}<em>-</em>{{end}}
                                </div>
                                <div class="info-item">
                                    <strong>Sysop</strong>
                                    {{if .NewNode.SysopName}}{{.NewNode.SysopName}}{{else}}<em>-</em>{{end}}
                                </div>
                                <div class="info-item">
                                    <strong>Phone</strong>
                                    {{if .NewNode.Phone}}{{.NewNode.Phone}}{{else}}<em>-</em>{{end}}
                                </div>
                                <div class="info-item">
                                    <strong>Node Type</strong>
                                    {{if eq .NewNode.NodeType "Zone"}}<span class="badge badge-error">Zone</span>
                                    {{else if eq .NewNode.NodeType "Region"}}<span class="badge badge-warning">Region</span>
                                    {{else if eq .NewNode.NodeType "Host"}}<span class="badge badge-info">Host</span>
                                    {{else if eq .NewNode.NodeType "Hub"}}<span class="badge badge-success">Hub</span>
                                    {{else if eq .NewNode.NodeType "Pvt"}}<span class="badge badge-warning">Pvt</span>
                                    {{else if eq .NewNode.NodeType "Down"}}<span class="badge badge-error">Down</span>
                                    {{else if eq .NewNode.NodeType "Hold"}}<span class="badge badge-warning">Hold</span>
                                    {{else}}{{.NewNode.NodeType}}{{end}}
                                </div>
                                <div class="info-item">
                                    <strong>Max Speed</strong>
                                    {{if .NewNode.MaxSpeed}}{{.NewNode.MaxSpeed}}{{else}}<em>-</em>{{end}}
                                </div>
                                {{if .NewNode.Region}}
                                <div class="info-item">
                                    <strong>Region</strong>
                                    {{.NewNode.Region}}
                                </div>
                                {{end}}
                                <div class="info-item">
                                    <strong>Capabilities</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{if .NewNode.IsCM}}<span class="badge badge-success">CM</span>{{end}}
                                        {{if .NewNode.IsMO}}<span class="badge badge-success">MO</span>{{end}}
                                        {{if .NewNode.HasBinkp}}<span class="badge badge-info">Binkp</span>{{end}}
                                        {{if .NewNode.HasTelnet}}<span class="badge badge-info">Telnet</span>{{end}}
                                        {{if .NewNode.IsDown}}<span class="badge badge-error">Down</span>{{end}}
                                        {{if .NewNode.IsHold}}<span class="badge badge-warning">Hold</span>{{end}}
                                        {{if .NewNode.IsPvt}}<span class="badge badge-warning">Private</span>{{end}}
                                        {{if not (or .NewNode.IsCM .NewNode.IsMO .NewNode.HasBinkp .NewNode.HasTelnet .NewNode.IsDown .NewNode.IsHold .NewNode.IsPvt)}}<em>None specified</em>{{end}}
                                    </div>
                                </div>
                                {{if .NewNode.Flags}}
                                <div class="info-item" style="grid-column: span 2;">
                                    <strong>Flags</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.Flags}}<span class="badge badge-info" style="margin-right: 0.25rem; margin-bottom: 0.25rem;">{{.}}</span>{{end}}
                                    </div>
                                </div>
                                {{end}}
                                {{if .NewNode.ModemFlags}}
                                <div class="info-item" style="grid-column: span 2;">
                                    <strong>Modem Flags</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.ModemFlags}}<span class="badge badge-info" style="margin-right: 0.25rem; margin-bottom: 0.25rem;">{{.}}</span>{{end}}
                                    </div>
                                </div>
                                {{end}}
                                {{if .NewNode.InternetProtocols}}
                                <div class="info-item" style="grid-column: span 2;">
                                    <strong>Internet Protocols</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.InternetProtocols}}<span class="badge badge-success" style="margin-right: 0.25rem; margin-bottom: 0.25rem;">{{.}}</span>{{end}}
                                    </div>
                                </div>
                                {{end}}
                                {{if .NewNode.InternetHostnames}}
                                <div class="info-item" style="grid-column: span 2;">
                                    <strong>Internet Hostnames</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.InternetHostnames}}<code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem; font-size: 0.9rem; margin-right: 0.5rem;">{{.}}</code>{{end}}
                                    </div>
                                </div>
                                {{end}}
                                {{if .NewNode.InternetPorts}}
                                <div class="info-item">
                                    <strong>Internet Ports</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.InternetPorts}}<code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem; font-size: 0.9rem; margin-right: 0.5rem;">{{.}}</code>{{end}}
                                    </div>
                                </div>
                                {{end}}
                                {{if .NewNode.InternetEmails}}
                                <div class="info-item" style="grid-column: span 2;">
                                    <strong>Internet Emails</strong>
                                    <div style="margin-top: 0.5rem;">
                                        {{range .NewNode.InternetEmails}}<code style="background: #f1f5f9; padding: 0.2rem 0.4rem; border-radius: 0.25rem; font-size: 0.9rem; margin-right: 0.5rem;">{{.}}</code>{{end}}
                                    </div>
                                </div>
                                {{end}}
                            </div>
                            {{end}}
                        {{else if eq .ChangeType "removed"}}
                            <h4>❌ Node removed from nodelist</h4>
                            <p style="color: var(--text-secondary);">Node was no longer listed in subsequent nodelists</p>
                        {{else if eq .ChangeType "modified"}}
                            <h4>📝 Node information changed</h4>
                            <div class="change-list">
                                {{range $field, $change := .Changes}}
                                <div class="change-item">
                                    <strong>
                                        {{if eq $field "binkp"}}🌐 Binkp Support
                                        {{else if eq $field "telnet"}}📡 Telnet Support
                                        {{else if eq $field "modem_flags"}}📞 Modem Flags
                                        {{else if eq $field "internet_protocols"}}🌐 Internet Protocols
                                        {{else if eq $field "internet_hostnames"}}🏠 Internet Hostnames
                                        {{else if eq $field "internet_ports"}}🔌 Internet Ports
                                        {{else if eq $field "internet_emails"}}📧 Internet Emails
                                        {{else if eq $field "status"}}📊 Status
                                        {{else if eq $field "name"}}💻 System Name
                                        {{else if eq $field "location"}}🌍 Location
                                        {{else if eq $field "sysop"}}👤 Sysop
                                        {{else if eq $field "phone"}}📞 Phone
                                        {{else if eq $field "speed"}}⚡ Speed
                                        {{else if eq $field "flags"}}🏷️ Flags
                                        {{else}}{{$field}}{{end}}:
                                    </strong>
                                    <span class="change-value">{{$change}}</span>
                                </div>
                                {{end}}
                            </div>
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
    </div>
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
            <p class="subtitle">REST API endpoints for programmatic access to nodelist data</p>
        </header>
        
        <nav>
            <div class="nav-container">
                <a href="/">🏠 Home</a>
                <a href="/search">🔍 Search Nodes</a>
                <a href="/search/sysop">👤 Search Sysops</a>
                <a href="/stats">📊 Statistics</a>
                <a href="/api/help" class="active">📖 API Help</a>
            </div>
        </nav>
        
        <div class="content">
            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🚀 Getting Started</h2>
                <p style="color: var(--text-secondary); margin-bottom: 2rem;">
                    The NodelistDB API provides RESTful access to FidoNet node data with JSON responses. 
                    All endpoints support standard HTTP methods and return consistent JSON structures.
                </p>
                
                <div class="alert alert-success">
                    <strong>Base URL:</strong> <code>http://localhost:8080/api/</code>
                </div>
            </div>

            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🔍 Search Endpoints</h2>
                
                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Search Nodes</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Search for nodes using various criteria. All parameters are optional.</p>
                
                <div style="background: #f1f5f9; padding: 1rem; border-radius: var(--radius); margin-bottom: 1rem;">
                    <strong>Query Parameters:</strong><br>
                    <code>zone</code> - Zone number (e.g., 1, 2, 3)<br>
                    <code>net</code> - Net number<br>
                    <code>node</code> - Node number<br>
                    <code>system_name</code> - System name (partial match)<br>
                    <code>location</code> - Location (partial match)<br>
                    <code>node_type</code> - Node type (Hub, Host, Zone, etc.)<br>
                    <code>is_active</code> - true/false for active nodes<br>
                    <code>is_cm</code> - true/false for CM nodes<br>
                    <code>date_from</code> - Start date (YYYY-MM-DD)<br>
                    <code>date_to</code> - End date (YYYY-MM-DD)<br>
                    <code>limit</code> - Results limit (default: 100, max: 200)<br>
                    <code>offset</code> - Results offset for pagination
                </div>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes?zone=2&limit=10&is_cm=true</code>
                </div>

                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Search by Sysop</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes/search/sysop</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Find all nodes operated by a specific sysop across all time periods.</p>
                
                <div style="background: #f1f5f9; padding: 1rem; border-radius: var(--radius); margin-bottom: 1rem;">
                    <strong>Query Parameters:</strong><br>
                    <code>name</code> - Sysop name (required)<br>
                    <code>limit</code> - Results limit (default: 50, max: 200)
                </div>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes/search/sysop?name=John_Doe&limit=25</code>
                </div>
            </div>

            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🎯 Node Specific Endpoints</h2>
                
                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Get Node Information</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes/{zone}/{net}/{node}</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Retrieve information for a specific node, including current and historical versions.</p>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes/2/5001/100</code>
                </div>

                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Get Node History</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes/{zone}/{net}/{node}/history</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Get complete historical records for a specific node.</p>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes/2/5001/100/history</code>
                </div>

                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Get Node Changes</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes/{zone}/{net}/{node}/changes</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Get detected changes and timeline events for a node. Supports filtering options.</p>
                
                <div style="background: #f1f5f9; padding: 1rem; border-radius: var(--radius); margin-bottom: 1rem;">
                    <strong>Filter Parameters:</strong><br>
                    <code>noflags=1</code> - Ignore flag changes<br>
                    <code>nophone=1</code> - Ignore phone changes<br>
                    <code>nospeed=1</code> - Ignore speed changes<br>
                    <code>nostatus=1</code> - Ignore status changes<br>
                    <code>nolocation=1</code> - Ignore location changes<br>
                    <code>noname=1</code> - Ignore name changes<br>
                    <code>nosysop=1</code> - Ignore sysop changes<br>
                    <code>noconnectivity=1</code> - Ignore connectivity changes<br>
                    <code>nointernetprotocols=1</code> - Ignore internet protocol changes<br>
                    <code>nointernethostnames=1</code> - Ignore hostname changes<br>
                    <code>nointernetports=1</code> - Ignore port changes<br>
                    <code>nointernetemails=1</code> - Ignore email changes<br>
                    <code>nomodemflags=1</code> - Ignore modem flag changes
                </div>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes/2/5001/100/changes?noflags=1&nophone=1</code>
                </div>

                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Get Node Timeline</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/nodes/{zone}/{net}/{node}/timeline</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Get timeline data optimized for visualization, including activity periods and gaps.</p>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/nodes/2/5001/100/timeline</code>
                </div>
            </div>

            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">📊 Statistics Endpoints</h2>
                
                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Network Statistics</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/stats</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Get comprehensive network statistics including node counts, zone distribution, and capabilities.</p>
                
                <div style="background: #f1f5f9; padding: 1rem; border-radius: var(--radius); margin-bottom: 1rem;">
                    <strong>Query Parameters:</strong><br>
                    <code>date</code> - Specific date for statistics (YYYY-MM-DD, defaults to today)
                </div>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example:</strong><br>
                    <code>GET /api/stats?date=2023-12-01</code>
                </div>
            </div>

            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">🛠️ System Endpoints</h2>
                
                <h3 style="color: var(--text-primary); margin-top: 2rem; margin-bottom: 1rem;">Health Check</h3>
                <div style="background: #f8fafc; padding: 1.5rem; border-radius: var(--radius); border-left: 4px solid var(--primary-color); margin-bottom: 1rem;">
                    <code style="color: var(--primary-color); font-weight: 600;">GET /api/health</code>
                </div>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">Check API service status and uptime.</p>
                
                <div style="background: #f8fafc; padding: 1rem; border-radius: var(--radius); margin-bottom: 2rem;">
                    <strong>Example Response:</strong><br>
                    <code>{"status": "ok", "time": "2023-12-01T12:00:00Z"}</code>
                </div>
            </div>

            <div class="card">
                <h2 style="color: var(--text-primary); margin-bottom: 1.5rem;">💡 Usage Examples</h2>
                
                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">Using curl</h3>
                <div style="background: #1e293b; color: #e2e8f0; padding: 1.5rem; border-radius: var(--radius); margin-bottom: 1rem; font-family: monospace; overflow-x: auto;">
# Search for nodes in Zone 2<br>
curl "http://localhost:8080/api/nodes?zone=2&limit=5"<br><br>

# Get specific node information<br>
curl "http://localhost:8080/api/nodes/2/5001/100"<br><br>

# Search by sysop name<br>
curl "http://localhost:8080/api/nodes/search/sysop?name=John_Doe"<br><br>

# Get network statistics<br>
curl "http://localhost:8080/api/stats"
                </div>

                <h3 style="color: var(--text-primary); margin-bottom: 1rem;">Response Format</h3>
                <p style="color: var(--text-secondary); margin-bottom: 1rem;">All API responses use JSON format with consistent structure:</p>
                <div style="background: #1e293b; color: #e2e8f0; padding: 1.5rem; border-radius: var(--radius); margin-bottom: 2rem; font-family: monospace; overflow-x: auto;">
{<br>
&nbsp;&nbsp;"nodes": [...],<br>
&nbsp;&nbsp;"count": 42,<br>
&nbsp;&nbsp;"filter": {...}<br>
}
                </div>
            </div>

            <div class="alert alert-success">
                <strong>💡 Pro Tip:</strong> Use the <code>limit</code> and <code>offset</code> parameters for efficient pagination when dealing with large result sets. The API performs well with limits up to 200 records per request.
            </div>
        </div>
    </div>
</body>
</html>`

	default:
		return "<html><body><h1>Template not found</h1></body></html>"
	}
}


