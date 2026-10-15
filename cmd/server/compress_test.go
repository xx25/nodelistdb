package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWrapHandlerCompressesPages pins the thing compression used to miss: a
// web page, not an API response. Before wrapHandler, gzip lived on the API
// router alone and every HTML page and stylesheet went out uncompressed.
func TestWrapHandlerCompressesPages(t *testing.T) {
	body := strings.Repeat("<p>nodelist</p>\n", 200)
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/tarball", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write([]byte{0x1f, 0x8b, 0x08})
	})
	h := wrapHandler(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(plain) != body {
		t.Fatalf("decompressed body differs (%d bytes vs %d)", len(plain), len(body))
	}
	if rec.Body.Len() >= len(body) {
		t.Fatalf("compressed body (%d) is not smaller than plain (%d)", rec.Body.Len(), len(body))
	}

	// A client that does not accept gzip gets the plain body.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page", nil))
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding without Accept-Encoding = %q, want none", got)
	}

	// Already-compressed downloads are passed through untouched.
	req = httptest.NewRequest(http.MethodGet, "/tarball", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("tarball Content-Encoding = %q, want none", got)
	}
}
