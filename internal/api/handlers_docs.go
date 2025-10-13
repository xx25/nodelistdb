package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nodelistdb/internal/flags"
)

// FlagsDocumentationHandler returns flag descriptions and categories.
// GET /api/flags
func (s *Server) FlagsDocumentationHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Get flag filter from query parameters
	category := r.URL.Query().Get("category")
	specificFlag := r.URL.Query().Get("flag")

	flagDescriptions := flags.GetFlagDescriptions()

	// If a specific flag is requested, check if it's a T-flag that needs dynamic generation
	if specificFlag != "" && len(specificFlag) == 3 && specificFlag[0] == 'T' {
		if _, exists := flagDescriptions[specificFlag]; !exists {
			// Try to generate T-flag description dynamically
			if info, ok := flags.GetTFlagInfo(specificFlag); ok {
				flagDescriptions[specificFlag] = info
			}
		}
	}

	// Filter by category if specified
	if category != "" {
		filteredFlags := make(map[string]flags.FlagInfo)
		for flag, info := range flagDescriptions {
			if info.Category == category {
				filteredFlags[flag] = info
			}
		}
		flagDescriptions = filteredFlags
	}

	// Group flags by category
	categories := make(map[string][]map[string]interface{})
	for flag, info := range flagDescriptions {
		if categories[info.Category] == nil {
			categories[info.Category] = []map[string]interface{}{}
		}

		flagData := map[string]interface{}{
			"flag":        flag,
			"has_value":   info.HasValue,
			"description": info.Description,
		}
		categories[info.Category] = append(categories[info.Category], flagData)
	}

	response := map[string]interface{}{
		"flags":      flagDescriptions,
		"categories": categories,
		"count":      len(flagDescriptions),
		"filter": map[string]interface{}{
			"category": category,
		},
	}

	WriteJSONSuccess(w, response)
}

// OpenAPISpecHandler serves the OpenAPI specification.
func (s *Server) OpenAPISpecHandler(w http.ResponseWriter, r *http.Request) {
	// Set appropriate headers
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle CORS preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Serve the embedded specification
	w.WriteHeader(http.StatusOK)
	w.Write(OpenAPISpec)
}

// SwaggerUIHandler serves the Swagger UI interface.
func (s *Server) SwaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	// Get the base URL for the API spec
	scheme := "http"

	// Check for HTTPS in multiple ways to handle reverse proxies
	if r.TLS != nil {
		scheme = "https"
	} else if r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	} else if r.Header.Get("X-Forwarded-Ssl") == "on" {
		scheme = "https"
	} else if r.Header.Get("X-Url-Scheme") == "https" {
		scheme = "https"
	} else if r.Header.Get("Forwarded") != "" {
		// Parse RFC 7239 Forwarded header
		forwarded := r.Header.Get("Forwarded")
		if strings.Contains(strings.ToLower(forwarded), "proto=https") {
			scheme = "https"
		}
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	specURL := fmt.Sprintf("%s://%s/api/openapi.yaml", scheme, host)

	// Serve a simple Swagger UI HTML page
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>NodelistDB API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui.css" />
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *, *:before, *:after {
            box-sizing: inherit;
        }
        body {
            margin:0;
            background: #fafafa;
        }
        .swagger-ui .topbar {
            background-color: #2c3e50;
        }
        .swagger-ui .topbar .download-url-wrapper .download-url-button {
            background-color: #34495e;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui-standalone-preset.js"></script>
    <script>
    window.onload = function() {
        const ui = SwaggerUIBundle({
            url: '%s',
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "StandaloneLayout",
            validatorUrl: null,
            docExpansion: "list",
            operationsSorter: "alpha",
            tagsSorter: "alpha",
            tryItOutEnabled: true,
            filter: true,
            supportedSubmitMethods: ["get", "post", "put", "delete", "patch"],
            onComplete: function() {
                console.log("NodelistDB API documentation loaded");
            }
        });
    };
    </script>
</body>
</html>`, specURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}
