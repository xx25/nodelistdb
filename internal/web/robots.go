package web

import (
	_ "embed"
	"net/http"
)

//go:embed robots.txt
var robotsTxt []byte

// RobotsHandler serves /robots.txt. It is a route of its own because "/" is a
// catch-all: without one the path answered 200 with the search page, which a
// crawler reads as a robots file that allows everything.
func (s *Server) RobotsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(robotsTxt)
}
