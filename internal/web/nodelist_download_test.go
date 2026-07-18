package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupNodelistArchive creates a temp archive layout with a fidonet year at
// the root and an fsxnet network subdirectory, and points NODELIST_PATH at it.
func setupNodelistArchive(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	for dir, name := range map[string]string{
		filepath.Join(root, "2026"):           "nodelist.100",
		filepath.Join(root, "fsxnet", "2026"): "fsxnet.098",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NODELIST_PATH", root)
}

func nodelistPageRequest(path, cookie string) *http.Request {
	r := httptest.NewRequest("GET", path, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: networkCookieName, Value: cookie})
	}
	return r
}

// TestNodelistHandlerScopedByNetworkCookie verifies that the downloads page
// follows the global network switcher: fsxnet selected → fsxnet files are the
// main content, fidonet moves to the other-networks section.
func TestNodelistHandlerScopedByNetworkCookie(t *testing.T) {
	setupNodelistArchive(t)
	s := New(nil, TemplatesFS, StaticFS)

	cases := []struct {
		name       string
		cookie     string
		wantLinks  []string
		rejectLink string
	}{
		{
			name:   "default fidonet",
			cookie: "",
			wantLinks: []string{
				"/download/nodelist/2026/nodelist.100", // main content
				"/nodelists/fsxnet/2026",               // other-networks card
			},
			rejectLink: "/download/nodelist/fsxnet/",
		},
		{
			name:   "fsxnet cookie",
			cookie: "fsxnet",
			wantLinks: []string{
				"/download/nodelist/fsxnet/2026/fsxnet.098", // main content
				"/download/year/fsxnet/2026.tar.gz",         // year archive button
				"/nodelists/2026",                           // fidonet in other-networks card
			},
			rejectLink: "/download/nodelist/2026/",
		},
		{
			name:   "explicit fidonet cookie",
			cookie: "fidonet",
			wantLinks: []string{
				"/download/nodelist/2026/nodelist.100",
				"/nodelists/fsxnet/2026",
			},
			rejectLink: "/download/nodelist/fsxnet/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.NodelistHandler(w, nodelistPageRequest("/nodelists", tc.cookie))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			body := w.Body.String()
			for _, link := range tc.wantLinks {
				if !strings.Contains(body, `"`+link+`"`) {
					t.Errorf("page is missing link %q", link)
				}
			}
			if strings.Contains(body, tc.rejectLink) {
				t.Errorf("page unexpectedly contains %q", tc.rejectLink)
			}
		})
	}
}

// TestLatestNodelistHandlerScopedByNetworkCookie verifies /download/latest
// redirects to the selected network's newest file (fidonet without a cookie,
// so scripted use is unchanged).
func TestLatestNodelistHandlerScopedByNetworkCookie(t *testing.T) {
	setupNodelistArchive(t)
	s := New(nil, TemplatesFS, StaticFS)

	cases := []struct {
		cookie string
		want   string
	}{
		{"", "/download/nodelist/2026/nodelist.100"},
		{"fsxnet", "/download/nodelist/fsxnet/2026/fsxnet.098"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		s.LatestNodelistHandler(w, nodelistPageRequest("/download/latest", tc.cookie))
		if w.Code != http.StatusFound {
			t.Fatalf("cookie %q: status = %d, want 302", tc.cookie, w.Code)
		}
		if got := w.Header().Get("Location"); got != tc.want {
			t.Errorf("cookie %q: redirect = %q, want %q", tc.cookie, got, tc.want)
		}
	}
}

// TestNodelistDownloadHandlerRejectsTraversal verifies the direct-download
// route can't be tricked into reading files outside the archive tree via an
// embedded (percent-decoded) path in the filename segment.
func TestNodelistDownloadHandlerRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	// A secret sibling that must never be reachable.
	if err := os.MkdirAll(filepath.Join(root, "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secrets", "creds.yaml"), []byte("DB_PASSWORD=hunter2"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODELIST_PATH", root)
	s := New(nil, TemplatesFS, StaticFS)

	// The decoded path the mux would hand us after unescaping %2e%2e etc.
	traversals := []string{
		"/download/nodelist/s/2026/s./../../secrets/creds.yaml",
		"/download/nodelist/2026/../secrets/creds.yaml",
		"/download/nodelist/fsxnet/2026/../../secrets/creds.yaml",
	}
	for _, p := range traversals {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://example.com", nil)
		r.URL.Path = p // set decoded path directly, bypassing URL parsing
		s.NodelistDownloadHandler(w, r)
		if w.Code == http.StatusOK {
			t.Errorf("traversal %q returned 200 (body=%q)", p, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "hunter2") {
			t.Errorf("traversal %q leaked secret content", p)
		}
	}
}

// TestNodelistDownloadHandlerFidonetAlias verifies /download/nodelist/fidonet/...
// serves the same file as the root fidonet path.
func TestNodelistDownloadHandlerFidonetAlias(t *testing.T) {
	setupNodelistArchive(t)
	s := New(nil, TemplatesFS, StaticFS)

	for _, p := range []string{
		"/download/nodelist/2026/nodelist.100",
		"/download/nodelist/fidonet/2026/nodelist.100",
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://example.com", nil)
		r.URL.Path = p
		s.NodelistDownloadHandler(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%q: status = %d, want 200", p, w.Code)
		}
		if w.Body.String() != "test" {
			t.Errorf("%q: body = %q, want file contents", p, w.Body.String())
		}
	}
}

// TestYearArchiveHandlerNetworkPath verifies the network-scoped tar.gz route.
func TestYearArchiveHandlerNetworkPath(t *testing.T) {
	setupNodelistArchive(t)
	s := New(nil, TemplatesFS, StaticFS)

	w := httptest.NewRecorder()
	s.YearArchiveHandler(w, httptest.NewRequest("GET", "/download/year/fsxnet/2026.tar.gz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "fsxnet-nodelists-2026.tar.gz") {
		t.Errorf("Content-Disposition = %q, want fsxnet archive name", cd)
	}
	if w.Body.Len() == 0 {
		t.Error("archive body is empty")
	}

	// The fidonet path shape must be unchanged.
	w = httptest.NewRecorder()
	s.YearArchiveHandler(w, httptest.NewRequest("GET", "/download/year/2026.tar.gz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("fidonet archive status = %d, want 200", w.Code)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `"nodelists-2026.tar.gz"`) {
		t.Errorf("fidonet Content-Disposition = %q, want unchanged name", cd)
	}
}
