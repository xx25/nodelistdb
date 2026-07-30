package ftp

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// newLegacyTestFs builds the same filesystem stack the FTP mounts use over a
// temp archive in the canonical layout: fidonet/2026/nodelist.191 plus one
// other network.
func newLegacyTestFs(t *testing.T) afero.Fs {
	t.Helper()
	root := t.TempDir()

	for dir, name := range map[string]string{
		filepath.Join(root, "fidonet", "2026"): "nodelist.191",
		filepath.Join(root, "fsxnet", "2026"):  "fsxnet.191",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	base := afero.NewBasePathFs(afero.NewOsFs(), root)
	return legacyYearFs{afero.NewReadOnlyFs(base)}
}

func TestFidonetAlternatePath(t *testing.T) {
	cases := []struct {
		name string
		want string
		ok   bool
	}{
		// Legacy spelling -> canonical.
		{"/2026/nodelist.191", "/fidonet/2026/nodelist.191", true},
		{"2026/nodelist.191", "/fidonet/2026/nodelist.191", true},
		{"/2026", "/fidonet/2026", true},
		{"/1986", "/fidonet/1986", true},
		// Canonical -> legacy, for years a move has not reached yet.
		{"/fidonet/2026/nodelist.191", "/2026/nodelist.191", true},
		{"/fidonet/1986", "/1986", true},
		{"/FIDONET/2026", "/2026", true},
		// Not year paths: no rewrite in either direction.
		{"/fsxnet/2026/fsxnet.191", "", false},
		{"/fidonet", "", false},
		// The pointlist tree is /fidonet/<source>/<year> and must be left alone.
		{"/fidonet/z2/2025/z2pnt.206.gz", "", false},
		{"/pointlists", "", false},
		{"/", "", false},
		{"/202/x", "", false},
		{"/20266/x", "", false},
		{"/20a6/x", "", false},
	}

	for _, tc := range cases {
		got, ok := fidonetAlternatePath(tc.name)
		if ok != tc.ok {
			t.Errorf("fidonetAlternatePath(%q) ok = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("fidonetAlternatePath(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLegacyYearFsHalfMovedArchive covers the state a migration passes through:
// some year directories moved under fidonet/, some not. Every year must answer
// to both spellings throughout, or the archive appears to lose half its years
// for the duration of the move.
func TestLegacyYearFsHalfMovedArchive(t *testing.T) {
	root := t.TempDir()
	// 2026 has been moved; 1986 has not.
	for dir, name := range map[string]string{
		filepath.Join(root, "fidonet", "2026"): "nodelist.191",
		filepath.Join(root, "1986"):            "nodelist.281",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs := legacyYearFs{afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), root))}

	for _, p := range []string{
		"/fidonet/2026/nodelist.191", // moved, canonical
		"/2026/nodelist.191",         // moved, legacy
		"/1986/nodelist.281",         // unmoved, legacy
		"/fidonet/1986/nodelist.281", // unmoved, canonical
	} {
		f, err := fs.Open(p)
		if err != nil {
			t.Errorf("Open(%q): %v", p, err)
			continue
		}
		f.Close()
		if _, err := fs.Stat(p); err != nil {
			t.Errorf("Stat(%q): %v", p, err)
		}
	}
}

// TestLegacyYearFsServesBothSpellings verifies a file archived under fidonet/
// is readable through the network-less path published before the move.
func TestLegacyYearFsServesBothSpellings(t *testing.T) {
	fs := newLegacyTestFs(t)

	for _, p := range []string{
		"/fidonet/2026/nodelist.191",
		"/2026/nodelist.191",
	} {
		f, err := fs.Open(p)
		if err != nil {
			t.Fatalf("Open(%q): %v", p, err)
		}
		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatalf("reading %q: %v", p, err)
		}
		if string(content) != "nodelist.191" {
			t.Errorf("%q: content = %q", p, content)
		}

		if _, err := fs.Stat(p); err != nil {
			t.Errorf("Stat(%q): %v", p, err)
		}
	}
}

// TestLegacyYearFsDirectoryListing verifies the legacy paths stay reachable
// without being advertised: the archive root lists the network directories
// only, not a second copy of every year.
func TestLegacyYearFsDirectoryListing(t *testing.T) {
	fs := newLegacyTestFs(t)

	root, err := fs.Open("/")
	if err != nil {
		t.Fatalf("Open(/): %v", err)
	}
	defer root.Close()

	names, err := root.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}

	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	if !seen["fidonet"] || !seen["fsxnet"] {
		t.Errorf("root listing = %v, want the network directories", names)
	}
	if seen["2026"] {
		t.Errorf("root listing = %v, want no legacy year directory", names)
	}
}

// TestLegacyYearFsMissesStayMisses verifies the fallback does not turn a real
// 404 into something else, and that it cannot reach outside the mount.
func TestLegacyYearFsMissesStayMisses(t *testing.T) {
	fs := newLegacyTestFs(t)

	for _, p := range []string{
		"/2026/nodelist.999",      // year exists under fidonet/, file does not
		"/1900/nodelist.001",      // no such year
		"/fidonet/2026/nope",      // qualified miss
		"/2026/../../etc/passwd",  // traversal through the rewritten path
		"/fsxnet/2026/nodelist.1", // wrong network's file
	} {
		if _, err := fs.Open(p); err == nil {
			t.Errorf("Open(%q) succeeded, want an error", p)
		}
		if _, err := fs.Stat(p); err == nil {
			t.Errorf("Stat(%q) succeeded, want an error", p)
		}
	}
}
