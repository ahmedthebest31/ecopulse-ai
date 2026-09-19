package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"backend-go/internal/iot"
)

func TestTelemetryIOTEndpointValid(t *testing.T) {
	s := NewServer(Config{})
	body := `{"device_id":"esp32-factory-001","timestamp":"2026-08-03T18:30:00Z","amperage":15,"voltage":380,"kilowatts":9.9,"accumulated_kwh":123.4}`
	req := httptest.NewRequest(http.MethodPost, "/api/telemetry/iot", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp iot.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if !resp.IsPeakHour {
		t.Fatal("18:30 must be inside the default 18:00-22:00 peak window")
	}
	if resp.CurrentTier != 1 {
		t.Fatalf("expected current_tier 1 on first reading, got %d", resp.CurrentTier)
	}
	if resp.AlertLevel != iot.AlertNormal || resp.LoadShedRecommended {
		t.Fatalf("expected NORMAL, no shed; got %q/%v", resp.AlertLevel, resp.LoadShedRecommended)
	}
}

func TestTelemetryIOTEndpointValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "empty device id", body: `{"device_id":"","timestamp":"2026-08-03T18:30:00Z"}`},
		{name: "invalid timestamp", body: `{"device_id":"d1","timestamp":"2026-08-03 18:30:00"}`},
		{name: "negative kilowatts", body: `{"device_id":"d1","timestamp":"2026-08-03T18:30:00Z","kilowatts":-5}`},
		{name: "malformed json", body: `{device_id: d1}`},
		{name: "trailing data", body: `{"device_id":"d1","timestamp":"2026-08-03T18:30:00Z"} extra`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(Config{})
			req := httptest.NewRequest(http.MethodPost, "/api/telemetry/iot", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "error") {
				t.Fatalf("expected an error field, got: %s", rec.Body.String())
			}
		})
	}
}

func TestTelemetryIOTDevicesEndpoint(t *testing.T) {
	s := NewServer(Config{})
	body := `{"device_id":"esp32-factory-001","timestamp":"2026-08-03T12:00:00Z","amperage":15,"voltage":380,"kilowatts":9.9}`
	req := httptest.NewRequest(http.MethodPost, "/api/telemetry/iot", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed submit failed: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/telemetry/iot/devices", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload struct {
		Count   int              `json:"count"`
		Devices []iot.DeviceInfo `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if payload.Count != 1 || len(payload.Devices) != 1 {
		t.Fatalf("expected 1 device, got count=%d len=%d", payload.Count, len(payload.Devices))
	}
	if payload.Devices[0].DeviceID != "esp32-factory-001" {
		t.Fatalf("expected the seeded device id, got %q", payload.Devices[0].DeviceID)
	}
}

func TestLoadTelemetryAppliesPeakWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	payload := `{
  "schema_version":"1",
  "interval_minutes":5,
  "hours":1,
  "timezone":"Africa/Cairo",
  "utc_offset":"+03:00",
  "spike_threshold_percent":30,
  "carbon_factor_kg_per_kwh":0.438,
  "peak_window":{"start":"20:00","end":"23:00"},
  "records":[{"timestamp":"2026-08-03T20:00:00+03:00","facility_id":"f1","power_kw":10,"energy_kwh":1}]
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	s := NewServer(Config{TelemetryPath: path})
	if err := s.LoadTelemetry(); err != nil {
		t.Fatalf("LoadTelemetry failed: %v", err)
	}
	resp, err := s.iot.Submit(iot.Reading{
		DeviceID: "d1", Timestamp: "2026-08-03T20:00:00+03:00",
		Amperage: 15, Voltage: 380, Kilowatts: 9.9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsPeakHour {
		t.Fatalf("expected peak hour for the loaded dataset window, got %v", resp.IsPeakHour)
	}
}
