package web

import (
	"net/http"
	"path/filepath"
)

// APIHelpHandler handles the API help page
func (s *Server) APIHelpHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "API Documentation",
	}
	
	s.templates["api_help"].Execute(w, data)
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
    background: var(--background);
    margin: 0;
    color: var(--text-primary);
    line-height: 1.6;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 0 1rem;
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
    font-size: 3rem;
    font-weight: 700;
    background: linear-gradient(135deg, var(--primary-color), #7c3aed);
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
    border-radius: var(--radius-lg);
    padding: 1rem 1.5rem;
    margin: 2rem 0;
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
    padding: 0.75rem 1.25rem;
    text-decoration: none;
    color: var(--text-secondary);
    font-weight: 500;
    border-radius: var(--radius);
    transition: all 0.2s ease;
    border: 1px solid transparent;
}

nav a:hover {
    background: var(--background);
    color: var(--primary-color);
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
    margin-bottom: 0.5rem;
    font-weight: 500;
    color: var(--text-primary);
    font-size: 0.9rem;
}

.form-group input {
    padding: 0.75rem;
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

.form-group select {
    padding: 0.75rem;
    border: 2px solid var(--border-color);
    border-radius: var(--radius);
    font-size: 1rem;
    background: var(--card-bg);
    transition: all 0.2s ease;
    width: 100%;
    cursor: pointer;
}

.form-group select:focus {
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
    font-size: 1rem;
    font-weight: 500;
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
    background: var(--background);
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
    background: var(--card-bg);
    padding: 1.5rem;
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
    font-size: 2rem;
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
    padding: 1rem;
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
    background: var(--card-bg);
    padding: 1.5rem;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border-color);
    margin: 2rem 0;
}

.filter-options {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    margin: 1rem 0;
}

.filter-options label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    background: var(--background);
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
    left: 1rem;
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
    width: 2rem;
    height: 2rem;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
}

.timeline-marker::after {
    content: '';
    width: 12px;
    height: 12px;
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
    position: absolute;
    top: -1.5rem;
    left: 0;
    font-size: 0.8rem;
    color: var(--text-secondary);
    font-weight: 500;
}

.timeline-content {
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

.change-list {
    margin: 1rem 0;
}

.change-item {
    display: flex;
    padding: 0.5rem 0;
    border-bottom: 1px solid #f1f5f9;
}

.change-item:last-child {
    border-bottom: none;
}

.change-value {
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
    background: #f8fafc;
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    margin-left: 0.5rem;
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
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 1rem;
    margin: 2rem 0;
}

.info-item {
    background: var(--card-bg);
    padding: 1rem;
    border-radius: var(--radius);
    border: 1px solid var(--border-color);
}

.info-item strong {
    display: block;
    color: var(--primary-color);
    font-weight: 600;
    margin-bottom: 0.25rem;
}

@media (max-width: 768px) {
    .container {
        padding: 0 0.5rem;
    }
    
    h1 {
        font-size: 2rem;
    }
    
    .search-form {
        grid-template-columns: 1fr;
    }
    
    .stats-grid {
        grid-template-columns: 1fr;
    }
    
    .nav-container {
        flex-direction: column;
    }
    
    nav a {
        justify-content: center;
    }
    
    table {
        font-size: 0.9rem;
    }
    
    .card {
        padding: 1rem;
    }
    
    .timeline-content {
        margin-left: 0.5rem;
        padding: 1rem;
    }
    
    .node-info {
        grid-template-columns: 1fr;
    }
    
    .filter-options {
        flex-direction: column;
        align-items: flex-start;
    }
    
    .filter-options label {
        width: 100%;
    }
}

.loading {
    display: inline-block;
    width: 20px;
    height: 20px;
    border: 3px solid rgba(255,255,255,.3);
    border-radius: 50%;
    border-top-color: #fff;
    animation: spin 1s ease-in-out infinite;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}

.badge {
    display: inline-block;
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
    font-weight: 600;
    border-radius: 0.25rem;
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
/* Analytics Styles */
.analytics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 2rem;
    margin: 2rem 0;
}

.analytics-card {
    background: var(--card-bg);
    padding: 2rem;
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    border: 1px solid var(--border-color);
    transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.analytics-card:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-lg);
}

.analytics-card h3 {
    margin: 0 0 1rem 0;
    font-size: 1.25rem;
    color: var(--primary-color);
    font-weight: 600;
}

.analytics-links {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1rem;
}

.analytics-links .btn {
    text-align: center;
}

.analytics-links form {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
}

.analytics-links input {
    padding: 0.5rem;
    border: 1px solid var(--border-color);
    border-radius: var(--radius);
    font-size: 0.9rem;
}

.btn-disabled {
    background: var(--secondary-color) !important;
    cursor: not-allowed;
    opacity: 0.6;
}

.btn-disabled:hover {
    background: var(--secondary-color) !important;
    transform: none;
}

.progress-bar {
    background: var(--primary-color);
    height: 4px;
    border-radius: 2px;
    min-width: 2px;
    transition: width 0.3s ease;
}

.data-table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 1rem;
}

.data-table th,
.data-table td {
    padding: 0.75rem;
    text-align: left;
    border-bottom: 1px solid var(--border-color);
}

.data-table th {
    background: var(--background);
    font-weight: 600;
    color: var(--text-primary);
}

.data-table tr:hover {
    background: var(--background);
}

.data-table td:last-child {
    width: 200px;
}

details {
    margin-top: 1rem;
}

details summary {
    padding: 0.75rem;
    background: var(--background);
    cursor: pointer;
    border-radius: var(--radius);
    font-weight: 500;
}

details[open] summary {
    margin-bottom: 1rem;
}

pre {
    background: #f8fafc;
    padding: 1rem;
    border-radius: var(--radius);
    overflow-x: auto;
    max-height: 400px;
    overflow-y: auto;
}

/* Responsive adjustments */
@media (max-width: 768px) {
    .analytics-grid {
        grid-template-columns: 1fr;
    }
    
    .analytics-links form {
        flex-direction: column;
        align-items: stretch;
    }
    
    .analytics-links input {
        width: 100%;
    }
    
    .data-table {
        font-size: 0.9rem;
    }
    
    .data-table th,
    .data-table td {
        padding: 0.5rem;
    }
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
    
    // Handle full address input
    const fullAddressInput = document.getElementById('full_address');
    const zoneInput = document.getElementById('zone');
    const netInput = document.getElementById('net');
    const nodeInput = document.getElementById('node');
    
    if (fullAddressInput) {
        fullAddressInput.addEventListener('input', function() {
            const address = this.value.trim();
            // If full address is being typed, clear individual fields
            if (address && zoneInput && netInput && nodeInput) {
                zoneInput.value = '';
                netInput.value = '';
                nodeInput.value = '';
            }
        });
    }
    
    // Clear full address when individual fields are used
    if (zoneInput || netInput || nodeInput) {
        // Handle zone select dropdown
        if (zoneInput) {
            zoneInput.addEventListener('change', function() {
                if (this.value && fullAddressInput) {
                    fullAddressInput.value = '';
                }
            });
        }
        
        // Handle net and node input fields
        [netInput, nodeInput].forEach(function(input) {
            if (input) {
                input.addEventListener('input', function() {
                    if (this.value.trim() && fullAddressInput) {
                        fullAddressInput.value = '';
                    }
                });
            }
        });
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