package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// archiveLayout selects where a test fixture puts FidoNet's year directories.
//
// Both layouts are live: canonical is <root>/fidonet/<year>, and legacy is the
// <root>/<year> shape FidoNet had before it became an ordinary network here.
// Every download route has to keep working against either one, because a
// deployed binary meets the old layout until the files are moved, and
// unmigrated installs never move them at all.
type archiveLayout int

const (
	layoutCanonical archiveLayout = iota
	layoutLegacy
)

func (l archiveLayout) String() string {
	if l == layoutLegacy {
		return "legacy root layout"
	}
	return "canonical fidonet layout"
}

// setupNodelistArchive creates a temp archive holding one FidoNet and one
// fsxnet nodelist, and points NODELIST_PATH at it.
func setupNodelistArchive(t *testing.T, layout archiveLayout) {
	t.Helper()
	root := t.TempDir()

	fidonetDir := filepath.Join(root, "fidonet", "2026")
	if layout == layoutLegacy {
		fidonetDir = filepath.Join(root, "2026")
	}

	for dir, name := range map[string]string{
		fidonetDir:                            "nodelist.100",
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

// newDownloadServer builds a Server for the download routes, which read the
// archive off disk and never touch storage.
func newDownloadServer(t *testing.T) *Server {
	t.Helper()
	return newTestServer(t, nil)
}

// TestNodelistHandlerScopedByNetworkCookie verifies that the downloads page
// follows the global network switcher: fsxnet selected → fsxnet files are the
// main content, fidonet moves to the other-networks section. Every link the
// page emits names its network, including FidoNet's, under either layout.
func TestNodelistHandlerScopedByNetworkCookie(t *testing.T) {
	for _, layout := range []archiveLayout{layoutCanonical, layoutLegacy} {
		t.Run(layout.String(), func(t *testing.T) {
			setupNodelistArchive(t, layout)
			s := newDownloadServer(t)

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
						"/download/nodelist/fidonet/2026/nodelist.100", // main content
						"/nodelists/fsxnet/2026",                       // other-networks card
					},
					rejectLink: "/download/nodelist/fsxnet/",
				},
				{
					name:   "fsxnet cookie",
					cookie: "fsxnet",
					wantLinks: []string{
						"/download/nodelist/fsxnet/2026/fsxnet.098", // main content
						"/download/year/fsxnet/2026.tar.gz",         // year archive button
						"/nodelists/fidonet/2026",                   // fidonet in other-networks card
					},
					rejectLink: "/download/nodelist/fidonet/",
				},
				{
					name:   "explicit fidonet cookie",
					cookie: "fidonet",
					wantLinks: []string{
						"/download/nodelist/fidonet/2026/nodelist.100",
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
		})
	}
}

// TestNodelistHalfMovedArchive covers the state the one-time migration passes
// through: moving 41 year directories is 41 renames, so for a while some years
// sit under fidonet/ and the rest are still at the archive root. The scanner
// merges both roots, so the collection must read as whole throughout — a
// scanner that picked one root would drop whichever half it did not pick.
func TestNodelistHalfMovedArchive(t *testing.T) {
	root := t.TempDir()
	for dir, name := range map[string]string{
		filepath.Join(root, "fidonet", "2026"): "nodelist.100", // moved
		filepath.Join(root, "2025"):            "nodelist.300", // not yet
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NODELIST_PATH", root)
	s := newDownloadServer(t)

	t.Run("both years listed", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.NodelistHandler(w, nodelistPageRequest("/nodelists", ""))
		body := w.Body.String()
		for _, link := range []string{
			"/download/nodelist/fidonet/2026/nodelist.100",
			"/download/nodelist/fidonet/2025/nodelist.300",
		} {
			if !strings.Contains(body, `"`+link+`"`) {
				t.Errorf("page is missing %q", link)
			}
		}
	})

	t.Run("both years downloadable by either spelling", func(t *testing.T) {
		for _, p := range []string{
			"/download/nodelist/fidonet/2026/nodelist.100",
			"/download/nodelist/2026/nodelist.100",
			"/download/nodelist/fidonet/2025/nodelist.300",
			"/download/nodelist/2025/nodelist.300",
		} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://example.com", nil)
			r.URL.Path = p
			s.NodelistDownloadHandler(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("%q: status = %d, want 200", p, w.Code)
			}
		}
	})

	t.Run("both year archives build", func(t *testing.T) {
		for _, p := range []string{
			"/download/year/fidonet/2026.tar.gz",
			"/download/year/fidonet/2025.tar.gz",
			"/download/year/2025.tar.gz",
		} {
			w := httptest.NewRecorder()
			s.YearArchiveHandler(w, httptest.NewRequest("GET", p, nil))
			if w.Code != http.StatusOK {
				t.Errorf("%q: status = %d, want 200", p, w.Code)
			}
			if w.Body.Len() == 0 {
				t.Errorf("%q: archive body is empty", p)
			}
		}
	})

	t.Run("latest spans both roots", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.LatestNodelistHandler(w, nodelistPageRequest("/download/latest", ""))
		if got := w.Header().Get("Location"); got != "/download/nodelist/fidonet/2026/nodelist.100" {
			t.Errorf("redirect = %q, want the 2026 file", got)
		}
	})
}

// TestNodelistPageNamesFidonetProperly guards the prose on the most-visited
// page in this feature. Naming the default network rather than eliding it made
// every "{{if .Network}}...{{else}}FidoNet{{end}}" fallback unreachable, which
// would have quietly downgraded "FidoNet" to "fidonet" everywhere it is read by
// a human. Link assertions cannot catch that.
func TestNodelistPageNamesFidonetProperly(t *testing.T) {
	setupNodelistArchive(t, layoutCanonical)
	s := newDownloadServer(t)

	w := httptest.NewRecorder()
	s.NodelistHandler(w, nodelistPageRequest("/nodelists", ""))
	body := w.Body.String()

	if !strings.Contains(body, "the FidoNet nodelist collection") {
		t.Error("subtitle does not name FidoNet properly")
	}
	// The URL templates must still use the lowercase path segment.
	if !strings.Contains(body, "/download/nodelist/fidonet/{year}/{filename}") {
		t.Error("the documented download URL is not network-qualified")
	}

	// The year page's title too.
	w = httptest.NewRecorder()
	s.NodelistYearHandler(w, httptest.NewRequest("GET", "/nodelists/fidonet/2026", nil))
	if !strings.Contains(w.Body.String(), "FidoNet 2026") {
		t.Error("year page title does not name FidoNet properly")
	}
}

// TestNodelistHandlerListsFidonetOnce guards the other-networks section against
// listing FidoNet twice: ListNetworks now returns it like any other network, so
// a handler that also looks it up separately would emit it once per source.
func TestNodelistHandlerListsFidonetOnce(t *testing.T) {
	setupNodelistArchive(t, layoutCanonical)
	s := newDownloadServer(t)

	w := httptest.NewRecorder()
	s.NodelistHandler(w, nodelistPageRequest("/nodelists", "fsxnet"))
	if got := strings.Count(w.Body.String(), `"/nodelists/fidonet/2026"`); got != 1 {
		t.Errorf("fidonet browse link appears %d times, want 1", got)
	}
}

// TestLatestNodelistHandlerScopedByNetworkCookie verifies /download/latest
// redirects to the selected network's newest file, naming the network even for
// the default (cookie-less, scripted) case.
func TestLatestNodelistHandlerScopedByNetworkCookie(t *testing.T) {
	for _, layout := range []archiveLayout{layoutCanonical, layoutLegacy} {
		t.Run(layout.String(), func(t *testing.T) {
			setupNodelistArchive(t, layout)
			s := newDownloadServer(t)

			cases := []struct {
				cookie string
				want   string
			}{
				{"", "/download/nodelist/fidonet/2026/nodelist.100"},
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
		})
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
	s := newDownloadServer(t)

	// The decoded path the mux would hand us after unescaping %2e%2e etc.
	traversals := []string{
		"/download/nodelist/s/2026/s./../../secrets/creds.yaml",
		"/download/nodelist/2026/../secrets/creds.yaml",
		"/download/nodelist/fsxnet/2026/../../secrets/creds.yaml",
		"/download/nodelist/fidonet/2026/../../secrets/creds.yaml",
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

// TestNodelistRoutesAcceptBothSpellings verifies the network-less URLs that
// were published for FidoNet's whole history still resolve, under either
// on-disk layout, alongside the network-qualified ones now advertised.
func TestNodelistRoutesAcceptBothSpellings(t *testing.T) {
	for _, layout := range []archiveLayout{layoutCanonical, layoutLegacy} {
		t.Run(layout.String(), func(t *testing.T) {
			setupNodelistArchive(t, layout)
			s := newDownloadServer(t)

			t.Run("file download", func(t *testing.T) {
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
			})

			t.Run("year page", func(t *testing.T) {
				for _, p := range []string{"/nodelists/2026", "/nodelists/fidonet/2026"} {
					w := httptest.NewRecorder()
					s.NodelistYearHandler(w, httptest.NewRequest("GET", p, nil))
					if w.Code != http.StatusOK {
						t.Errorf("%q: status = %d, want 200", p, w.Code)
					}
					if !strings.Contains(w.Body.String(), "nodelist.100") {
						t.Errorf("%q: page does not list the year's file", p)
					}
				}
			})

			t.Run("year archive", func(t *testing.T) {
				for _, p := range []string{
					"/download/year/2026.tar.gz",
					"/download/year/fidonet/2026.tar.gz",
				} {
					w := httptest.NewRecorder()
					s.YearArchiveHandler(w, httptest.NewRequest("GET", p, nil))
					if w.Code != http.StatusOK {
						t.Errorf("%q: status = %d, want 200", p, w.Code)
					}
					if w.Body.Len() == 0 {
						t.Errorf("%q: archive body is empty", p)
					}
				}
			})
		})
	}
}

// TestYearArchiveHandlerNetworkPath verifies the network-scoped tar.gz route
// and its download filename, which names the network for every network now.
func TestYearArchiveHandlerNetworkPath(t *testing.T) {
	setupNodelistArchive(t, layoutCanonical)
	s := newDownloadServer(t)

	for _, tc := range []struct{ path, wantName string }{
		{"/download/year/fsxnet/2026.tar.gz", "fsxnet-nodelists-2026.tar.gz"},
		{"/download/year/fidonet/2026.tar.gz", "fidonet-nodelists-2026.tar.gz"},
		// The legacy path serves the same archive under the same new name.
		{"/download/year/2026.tar.gz", "fidonet-nodelists-2026.tar.gz"},
	} {
		w := httptest.NewRecorder()
		s.YearArchiveHandler(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%q: status = %d, want 200", tc.path, w.Code)
		}
		if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `"`+tc.wantName+`"`) {
			t.Errorf("%q: Content-Disposition = %q, want %q", tc.path, cd, tc.wantName)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%q: archive body is empty", tc.path)
		}
	}
}

// TestURLListHandlerNamesEveryNetwork verifies the machine-readable URL list
// emits network-qualified links and no duplicates.
func TestURLListHandlerNamesEveryNetwork(t *testing.T) {
	setupNodelistArchive(t, layoutCanonical)
	s := newDownloadServer(t)

	w := httptest.NewRecorder()
	s.URLListHandler(w, httptest.NewRequest("GET", "http://example.com/download/urls.txt", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		"/download/nodelist/fidonet/2026/nodelist.100",
		"/download/nodelist/fsxnet/2026/fsxnet.098",
	} {
		if got := strings.Count(body, want+"\n"); got != 1 {
			t.Errorf("URL %q appears %d times, want 1\n%s", want, got, body)
		}
	}
}
