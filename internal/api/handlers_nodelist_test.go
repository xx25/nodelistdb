package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupNodelistArchive builds a two-network archive - fidonet at the root,
// fsxnet one level down - and points NODELIST_PATH at it. The two networks
// carry different day numbers so a response cannot be attributed to the wrong
// one by accident.
func setupNodelistArchive(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	for dir, name := range map[string]string{
		filepath.Join(root, "2025"):           "nodelist.300.gz",
		filepath.Join(root, "2026"):           "nodelist.100.gz",
		filepath.Join(root, "fsxnet", "2026"): "fsxnet.098.gz",
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

func latestNodelist(t *testing.T, query string) (int, map[string]interface{}) {
	t.Helper()
	s := &Server{}
	rec := httptest.NewRecorder()
	s.LatestNodelistAPIHandler(rec, httptest.NewRequest("GET", "/api/nodelist/latest"+query, nil))

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, body
}

// TestLatestNodelistIsPerNetwork covers the bug this endpoint carried since
// the multi-network rollout: its own private scanner only ever looked at
// <root>/<year>/ for files named nodelist.*, so every network got FidoNet's
// answer.
func TestLatestNodelistIsPerNetwork(t *testing.T) {
	setupNodelistArchive(t)

	for _, tc := range []struct {
		name     string
		query    string
		wantFile string
		wantNet  string
		wantURL  string
	}{
		{"default is fidonet", "", "nodelist.100", "fidonet", "/download/nodelist/2026/nodelist.100"},
		{"explicit fidonet", "?domain=fidonet", "nodelist.100", "fidonet", "/download/nodelist/2026/nodelist.100"},
		{"fsxnet", "?domain=fsxnet", "fsxnet.098", "fsxnet", "/download/nodelist/fsxnet/2026/fsxnet.098"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := latestNodelist(t, tc.query)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %v", code, body)
			}
			if got := body["filename"]; got != tc.wantFile {
				t.Errorf("filename = %v, want %v", got, tc.wantFile)
			}
			if got := body["network"]; got != tc.wantNet {
				t.Errorf("network = %v, want %v", got, tc.wantNet)
			}
			if got := body["download_url"]; got != tc.wantURL {
				t.Errorf("download_url = %v, want %v", got, tc.wantURL)
			}
			if got := body["year"]; got != "2026" {
				t.Errorf("year = %v, want 2026", got)
			}
		})
	}
}

func TestLatestNodelistUnknownNetwork(t *testing.T) {
	setupNodelistArchive(t)

	if code, _ := latestNodelist(t, "?domain=nosuchnet"); code != http.StatusNotFound {
		t.Errorf("unknown network: status = %d, want 404", code)
	}
	// A name that is not a legal path segment must be refused before it
	// reaches the filesystem, not merely fail to match.
	if code, _ := latestNodelist(t, "?domain=../../etc"); code != http.StatusBadRequest {
		t.Errorf("path traversal: status = %d, want 400", code)
	}
}
