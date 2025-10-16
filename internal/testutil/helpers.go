package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

// TempDir creates a temporary directory for testing and returns cleanup function
func TempDir(t *testing.T, prefix string) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return dir, cleanup
}

// TempFile creates a temporary file with content for testing
func TempFile(t *testing.T, dir, pattern, content string) string {
	t.Helper()

	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer f.Close()

	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
	}

	return f.Name()
}

// LoadFixture loads a test fixture file from testdata directory
// Walks up the directory tree to find testdata/ regardless of nesting level
func LoadFixture(t *testing.T, path string) []byte {
	t.Helper()

	// Start from current directory and walk up to find testdata/
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Try up to 10 levels up (should be more than enough)
	dir := cwd
	for i := 0; i < 10; i++ {
		testdataPath := filepath.Join(dir, "testdata", path)
		if data, err := os.ReadFile(testdataPath); err == nil {
			return data
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	t.Fatalf("Failed to load fixture %s from testdata/ (searched up from %s)", path, cwd)
	return nil
}

// LoadFixtureString loads a test fixture as string
func LoadFixtureString(t *testing.T, path string) string {
	t.Helper()
	return string(LoadFixture(t, path))
}

// AssertNoError is a helper to check error is nil
func AssertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

// AssertError is a helper to check error is not nil
func AssertError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but got nil", msg)
	}
}

// AssertEqual is a generic equality assertion
func AssertEqual[T comparable](t *testing.T, expected, actual T, msg string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s: expected %v, got %v", msg, expected, actual)
	}
}

// AssertNotEqual is a generic inequality assertion
func AssertNotEqual[T comparable](t *testing.T, notExpected, actual T, msg string) {
	t.Helper()
	if notExpected == actual {
		t.Errorf("%s: expected values to be different, but both are %v", msg, actual)
	}
}

// AssertTrue checks if condition is true
func AssertTrue(t *testing.T, condition bool, msg string) {
	t.Helper()
	if !condition {
		t.Errorf("%s: expected true, got false", msg)
	}
}

// AssertFalse checks if condition is false
func AssertFalse(t *testing.T, condition bool, msg string) {
	t.Helper()
	if condition {
		t.Errorf("%s: expected false, got true", msg)
	}
}

// AssertNil checks if value is nil
func AssertNil(t *testing.T, value interface{}, msg string) {
	t.Helper()
	if value != nil {
		t.Errorf("%s: expected nil, got %v", msg, value)
	}
}

// AssertNotNil checks if value is not nil
func AssertNotNil(t *testing.T, value interface{}, msg string) {
	t.Helper()
	if value == nil {
		t.Errorf("%s: expected non-nil value", msg)
	}
}

// TimeEqual compares two times with tolerance
func TimeEqual(t *testing.T, expected, actual time.Time, tolerance time.Duration, msg string) {
	t.Helper()
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("%s: times differ by %v (tolerance: %v)\n  expected: %s\n  actual:   %s",
			msg, diff, tolerance, expected.Format(time.RFC3339), actual.Format(time.RFC3339))
	}
}

// NodeEqual compares two nodes for equality
func NodeEqual(t *testing.T, expected, actual database.Node, msg string) {
	t.Helper()
	if expected.Zone != actual.Zone ||
		expected.Net != actual.Net ||
		expected.Node != actual.Node ||
		expected.SystemName != actual.SystemName ||
		expected.SysopName != actual.SysopName {
		t.Errorf("%s: nodes not equal\n  expected: %d:%d/%d %s (%s)\n  actual:   %d:%d/%d %s (%s)",
			msg,
			expected.Zone, expected.Net, expected.Node, expected.SystemName, expected.SysopName,
			actual.Zone, actual.Net, actual.Node, actual.SystemName, actual.SysopName)
	}
}

// SkipIfShort skips test if running in short mode
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping in short mode: %s", reason)
	}
}

// SkipIfNoDocker skips test if Docker is not available
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	// Check if docker is available
	if os.Getenv("SKIP_DOCKER_TESTS") != "" {
		t.Skip("Docker tests disabled via SKIP_DOCKER_TESTS environment variable")
	}
}

// SkipIfNoClickHouse skips test if ClickHouse is not available
func SkipIfNoClickHouse(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_CLICKHOUSE_TESTS") != "" {
		t.Skip("ClickHouse tests disabled via SKIP_CLICKHOUSE_TESTS environment variable")
	}
}

// WithTimeout runs a test function with timeout using context
// This is safer than goroutine-based timeout as it doesn't interfere with t.Fatal
func WithTimeout(t *testing.T, timeout time.Duration, fn func(ctx context.Context)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run function with context
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed successfully
	case <-ctx.Done():
		t.Fatalf("Test timed out after %v", timeout)
	}
}
