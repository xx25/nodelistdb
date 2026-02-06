package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockHealthChecker struct {
	status *HealthStatus
}

func (m *mockHealthChecker) CheckHealth() *HealthStatus {
	return m.status
}

func TestHealthHandler_WithChecker(t *testing.T) {
	s := &Server{}
	s.SetHealthChecker(&mockHealthChecker{
		status: &HealthStatus{
			Status:    "ok",
			Time:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Uptime:    "1h0m0s",
			UptimeSec: 3600,
			Version: VersionInfo{
				Version:   "1.0.0",
				GitCommit: "abc123",
				BuildTime: "2026-01-01",
			},
			Database: DatabaseHealth{
				Connected:  true,
				ResponseMs: 1,
			},
			Nodes: NodeCountInfo{
				LatestDate: "2026-01-01",
				Count:      4500,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	s.HealthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status ok, got %s", result.Status)
	}
	if !result.Database.Connected {
		t.Error("expected database connected")
	}
	if result.Version.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", result.Version.Version)
	}
	if result.UptimeSec != 3600 {
		t.Errorf("expected uptime 3600, got %f", result.UptimeSec)
	}
	if result.Nodes.Count != 4500 {
		t.Errorf("expected 4500 nodes, got %d", result.Nodes.Count)
	}
}

func TestHealthHandler_Degraded(t *testing.T) {
	s := &Server{}
	s.SetHealthChecker(&mockHealthChecker{
		status: &HealthStatus{
			Status: "degraded",
			Time:   time.Now().UTC(),
			Database: DatabaseHealth{
				Connected:  false,
				ResponseMs: 5001,
				Error:      "connection refused",
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	s.HealthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even for degraded, got %d", w.Code)
	}

	var result HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Status != "degraded" {
		t.Errorf("expected degraded, got %s", result.Status)
	}
	if result.Database.Connected {
		t.Error("expected database disconnected")
	}
	if result.Database.Error == "" {
		t.Error("expected error message")
	}
}

func TestHealthHandler_WithoutChecker_BackwardCompat(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	s.HealthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected ok, got %v", result["status"])
	}
	if _, ok := result["time"]; !ok {
		t.Error("expected time field in backward-compatible response")
	}
}

func TestHealthHandler_OptionalFields(t *testing.T) {
	s := &Server{}
	s.SetHealthChecker(&mockHealthChecker{
		status: &HealthStatus{
			Status: "ok",
			Time:   time.Now().UTC(),
			Cache: &CacheHealth{
				Enabled: true,
				Keys:    1500,
				HitRate: 87.5,
			},
			FTP: &FTPHealth{
				Enabled: true,
				Port:    2121,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	s.HealthHandler(w, req)

	var result HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Cache == nil {
		t.Fatal("expected cache info")
	}
	if result.Cache.Keys != 1500 {
		t.Errorf("expected 1500 cache keys, got %d", result.Cache.Keys)
	}
	if result.FTP == nil {
		t.Fatal("expected FTP info")
	}
	if result.FTP.Port != 2121 {
		t.Errorf("expected FTP port 2121, got %d", result.FTP.Port)
	}
}
