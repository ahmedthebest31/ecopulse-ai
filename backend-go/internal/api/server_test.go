package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend-go/internal/tariff"
)

func TestHandleConfigGeminiStatus(t *testing.T) {
	cases := []struct {
		name   string
		envKey string
		want   bool
	}{
		{name: "env key set", envKey: "secret", want: true},
		{name: "env key empty", envKey: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(Config{GeminiAPIKey: tc.envKey})
			req := httptest.NewRequest(http.MethodGet, "/api/config/gemini-status", nil)
			rec := httptest.NewRecorder()
			s.handleConfigGeminiStatus(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			var body map[string]bool
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON body: %v", err)
			}
			if body["has_valid_env_key"] != tc.want {
				t.Fatalf("expected has_valid_env_key=%v, got %v", tc.want, body["has_valid_env_key"])
			}
		})
	}
}

// postJSON builds a POST request with a JSON body for handler tests.
func postJSON(target, payload string) *http.Request {
	return httptest.NewRequest(http.MethodPost, target, strings.NewReader(payload))
}

// TestTariffOverflowRejected is the regression test for the silent empty-200
// bug: kwh=1e308 used to overflow the total to +Inf, which encoding/json
// refuses to marshal, and the discarded encoder error left the client with
// HTTP 200 and an empty body. It must now be a JSON 400.
func TestTariffOverflowRejected(t *testing.T) {
	s := NewServer(Config{})
	req := postJSON("/api/tariff/calculate", `{"kwh": 1e308, "mode": "flat", "flat_rate_egp": 2.5}`)
	rec := httptest.NewRecorder()
	s.handleTariffCalculate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected a non-empty error body")
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON error body: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected an 'error' field in the response body")
	}
}

func TestTariffCalculateValidStillWorks(t *testing.T) {
	s := NewServer(Config{})
	req := postJSON("/api/tariff/calculate", `{"kwh": 100, "mode": "flat", "flat_rate_egp": 2.5}`)
	rec := httptest.NewRecorder()
	s.handleTariffCalculate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var bd tariff.Breakdown
	if err := json.Unmarshal(rec.Body.Bytes(), &bd); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if abs64(bd.TotalCostEGP-250.0) > 0.001 {
		t.Fatalf("expected total_cost_egp=250, got %v", bd.TotalCostEGP)
	}
}

func TestTelemetryInvertedRangeRejected(t *testing.T) {
	s := NewServer(Config{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/telemetry?start=2026-08-03T20:00:00%2B03:00&end=2026-08-03T18:00:00%2B03:00", nil)
	rec := httptest.NewRecorder()
	s.handleTelemetry(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inverted range, got %d", rec.Code)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	s := NewServer(Config{})
	big := strings.Repeat("x", maxBodyBytes+1024)
	req := postJSON("/api/report/summary", big)
	rec := httptest.NewRecorder()
	s.handleReportSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("expected an error field, got: %s", rec.Body.String())
	}
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
