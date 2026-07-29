package web

import (
	"bytes"
	"net/http"

	"github.com/nodelistdb/internal/logging"
)

// render executes a named template into a buffer and only then writes it.
//
// The buffer is the point. html/template writes as it executes, so a template
// that fails part-way - most often because the payload lacks a field the
// template dereferences - has already sent a 200 and some bytes by the time
// the error comes back. http.Error at that point appends its message to a
// half-written document; the client sees a page that simply stops. The
// statistics page shipped that way on all four of its error paths.
//
// Buffering costs one page of memory and turns the same failure into a clean
// 500 with nothing partial on the wire.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	s.renderStatus(w, name, data, http.StatusOK)
}

// renderStatus is render with an explicit status code, for the pages that need
// to answer something other than 200 while still rendering a full page - a
// query that exceeded its budget answers 503 and says so in its banner.
//
// The status is written after the template has executed successfully, which is
// the whole point of the buffer: a template that fails part-way still gets a
// clean 500 rather than this status followed by half a document.
func (s *Server) renderStatus(w http.ResponseWriter, name string, data any, status int) {
	tmpl, ok := s.templates[name]
	if !ok {
		logging.Errorf("render: template %q not loaded", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		logging.Errorf("render: executing template %q: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	if _, err := buf.WriteTo(w); err != nil {
		// The client went away mid-write; there is nothing left to say to it.
		logging.Errorf("render: writing %q to the client: %v", name, err)
	}
}
