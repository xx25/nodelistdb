package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// assetHashLen is how many hex characters of the content hash an asset URL
// carries. 12 hex digits (48 bits) cannot collide across the handful of files
// in static/ and keep URLs short.
const assetHashLen = 12

// loadAssetHashes hashes every file under static/ once, at startup. The hash
// is what makes a changed stylesheet reach a browser that cached the old one:
// static responses are cached for a week by URL, so the URL has to change
// when the content does. The build version is not usable as that key - it
// falls back to the latest git tag, so two builds between tags would serve
// different files under one URL.
//
// A missing or empty static tree (the render tests build a Server without
// one) leaves the map empty; asset() then returns the bare path.
func (s *Server) loadAssetHashes() {
	s.assetHash = make(map[string]string)
	_ = fs.WalkDir(s.staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := s.staticFS.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		s.assetHash[strings.TrimPrefix(p, "static/")] = hex.EncodeToString(sum[:])[:assetHashLen]
		return nil
	})
}

// AssetURL is the URL templates use for a static file: /static/<name>?v=<hash>.
// Only the query string changes between builds, so the route stays
// /static/ and an old link keeps working (with the old cache semantics).
func (s *Server) AssetURL(name string) string {
	name = strings.TrimPrefix(name, "/")
	if h, ok := s.assetHash[name]; ok {
		return "/static/" + name + "?v=" + h
	}
	return "/static/" + name
}

// StaticHandler serves static files from the embedded filesystem.
//
// A request that carries the current content hash (?v=) is immutable for a
// year: the URL changes with the content, so it can never go stale. Any other
// request - a hand-typed URL, an old page still open in a tab - gets the
// older one-week policy plus an ETag, so that once the week is over the
// browser revalidates with a 304 instead of fetching the whole file again.
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the file path from the URL
	requestPath := strings.TrimPrefix(r.URL.Path, "/static/")

	// Security check: prevent directory traversal
	if strings.Contains(requestPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Build the full file path for embedded filesystem
	filePath := path.Join("static", requestPath)

	// Try to open file from embedded filesystem
	file, err := s.staticFS.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	// Set appropriate content type based on file extension
	switch filepath.Ext(requestPath) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	// The body is gzipped for clients that accept it (cmd/server wraps the
	// mux in chi's compressor), so the cache key must include the encoding
	// on every response, not only the compressed ones chi marks itself.
	w.Header().Add("Vary", "Accept-Encoding")

	hash, hashed := s.assetHash[requestPath]
	if hashed {
		// Weak: the identity and gzip bodies are the same resource and
		// revalidate against the same tag; a strong tag would have to differ
		// per encoding.
		etag := `W/"` + hash + `"`
		w.Header().Set("ETag", etag)
		if r.URL.Query().Get("v") == hash {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=604800") // 1 week
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	} else {
		w.Header().Set("Cache-Control", "public, max-age=604800") // 1 week
	}

	// Serve the file content
	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, "Error serving file", http.StatusInternalServerError)
	}
}

// etagMatches implements If-None-Match for a single weak validator: "*", or
// any listed tag equal to ours, compared weakly (a W/ prefix on either side
// is ignored). Substring matching would accept "*" only by accident and
// would match a tag that merely contains ours.
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	strip := func(t string) string {
		t = strings.TrimSpace(t)
		return strings.TrimPrefix(t, "W/")
	}
	want := strip(etag)
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strip(candidate) == want {
			return true
		}
	}
	return false
}
