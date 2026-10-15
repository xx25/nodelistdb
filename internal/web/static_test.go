package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAssetURLCarriesContentHash: the stylesheet URL has to change when the
// stylesheet does, because static responses are cached for a week by URL.
// The hash comes from the file content, never from the build version, which
// falls back to the latest git tag and so can be the same across two builds.
func TestAssetURLCarriesContentHash(t *testing.T) {
	s := newTestServer(t, nil)
	url := s.AssetURL("style.css")
	if !strings.HasPrefix(url, "/static/style.css?v=") {
		t.Fatalf("AssetURL(style.css) = %q", url)
	}
	hash := strings.TrimPrefix(url, "/static/style.css?v=")
	if len(hash) != assetHashLen {
		t.Fatalf("hash %q has %d chars, want %d", hash, len(hash), assetHashLen)
	}
	if s.AssetURL("vendor/chart.min.js") == "/static/vendor/chart.min.js" {
		t.Fatal("vendored files must be hashed too")
	}
	if got := s.AssetURL("does-not-exist.css"); got != "/static/does-not-exist.css" {
		t.Fatalf("unknown asset = %q, want the bare path", got)
	}
}

// TestStaticHandlerCachePolicy: a hashed URL is immutable, anything else keeps
// the one-week policy, and both revalidate by ETag.
func TestStaticHandlerCachePolicy(t *testing.T) {
	s := newTestServer(t, nil)
	hash := strings.TrimPrefix(s.AssetURL("style.css"), "/static/style.css?v=")

	get := func(target string, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		s.StaticHandler(rec, req)
		return rec
	}

	rec := get("/static/style.css?v="+hash, "")
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("hashed GET: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("hashed URL Cache-Control = %q, want immutable", cc)
	}
	if et := rec.Header().Get("ETag"); et != `W/"`+hash+`"` {
		t.Fatalf("ETag = %q, want %q", et, `W/"`+hash+`"`)
	}
	if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding on every static response", v)
	}

	rec = get("/static/style.css", "")
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=604800" {
		t.Fatalf("bare URL Cache-Control = %q", cc)
	}

	for _, inm := range []string{`W/"` + hash + `"`, `"` + hash + `"`, `"other", W/"` + hash + `"`, "*"} {
		rec = get("/static/style.css", inm)
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
			t.Fatalf("If-None-Match %s: %d, %d bytes, want 304 and empty", inm, rec.Code, rec.Body.Len())
		}
	}
	for _, inm := range []string{`"` + hash + `-not"`, `"unrelated,*,tag"`, `"x", "*"`, `*, "y"`} {
		rec = get("/static/style.css", inm)
		if rec.Code != http.StatusOK {
			t.Fatalf("If-None-Match %s must not match: %d, want 200", inm, rec.Code)
		}
	}

	rec = get("/static/style.css?v=stale", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stale hash: %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Fatalf("stale hash must not be immutable: %q", cc)
	}
}
