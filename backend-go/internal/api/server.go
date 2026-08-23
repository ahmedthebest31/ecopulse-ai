// Package api wires the HTTP surface of the EcoPulse AI backend: telemetry
// serving, tariff calculation, analytics aggregation, and the Gemini-backed
// executive report endpoint. All handlers emit JSON and the router applies
// CORS headers so the React frontend can call the API directly.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"backend-go/internal/ai_report"
	"backend-go/internal/analytics"
	"backend-go/internal/tariff"
)

// Config holds the runtime settings injected from the environment.
type Config struct {
	TelemetryPath     string
	SpikeThresholdPct float64
	USDPerEGP         float64
	GeminiAPIKey      string
	GeminiModel       string
	AllowedOrigins    []string
	Logger            *log.Logger
}

// TelemetryFile mirrors the top-level structure of the generated dataset.
type TelemetryFile struct {
	SchemaVersion         string  `json:"schema_version"`
	IntervalMinutes       int     `json:"interval_minutes"`
	Hours                 int     `json:"hours"`
	Timezone              string  `json:"timezone"`
	UTCOffset             string  `json:"utc_offset"`
	SpikeThresholdPercent float64 `json:"spike_threshold_percent"`
	CarbonFactorKgPerKWh  float64 `json:"carbon_factor_kg_per_kwh"`
	PeakWindow            struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"peak_window"`
	Records []analytics.TelemetryRecord `json:"records"`
}

// Server holds the loaded telemetry data and the configured clients.
type Server struct {
	cfg    Config
	file   TelemetryFile
	gemini *ai_report.Client

	// timestamps mirrors file.Records index-by-index with pre-parsed
	// timestamps, so time-filtered requests do not re-parse RFC3339 strings
	// on every call. timestampValid marks records whose timestamp failed to
	// parse; they are excluded from time-filtered results only.
	timestamps     []time.Time
	timestampValid []bool

	// analytics cache: the dataset is immutable after load, so the default
	// aggregation is computed once and reused across requests.
	analyticsMu     sync.Mutex
	cachedAnalytics *analytics.Result
}

// NewServer creates a Server from the given configuration.
func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stdout, "ecopulse ", log.LstdFlags|log.LUTC)
	}
	return &Server{
		cfg:    cfg,
		gemini: ai_report.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel).WithLogger(cfg.Logger),
	}
}

// LoadTelemetry reads and validates the telemetry JSON dataset.
func (s *Server) LoadTelemetry() error {
	raw, err := os.ReadFile(s.cfg.TelemetryPath)
	if err != nil {
		return fmt.Errorf("reading telemetry file %q: %w", s.cfg.TelemetryPath, err)
	}
	if err := json.Unmarshal(raw, &s.file); err != nil {
		return fmt.Errorf("parsing telemetry file %q: %w", s.cfg.TelemetryPath, err)
	}
	if len(s.file.Records) == 0 {
		return fmt.Errorf("telemetry file %q contains no records", s.cfg.TelemetryPath)
	}
	if s.cfg.SpikeThresholdPct <= 0 {
		if s.file.SpikeThresholdPercent > 0 {
			s.cfg.SpikeThresholdPct = s.file.SpikeThresholdPercent
		} else {
			s.cfg.SpikeThresholdPct = 30
		}
	}
	s.timestamps = make([]time.Time, len(s.file.Records))
	s.timestampValid = make([]bool, len(s.file.Records))
	var unparsable int
	for i, record := range s.file.Records {
		ts, err := time.Parse(time.RFC3339, record.Timestamp)
		if err != nil {
			unparsable++
			continue
		}
		s.timestamps[i] = ts
		s.timestampValid[i] = true
	}
	if unparsable > 0 {
		s.cfg.Logger.Printf("warning: %d telemetry records have unparseable timestamps and are excluded from time-filtered queries", unparsable)
	}
	s.cachedAnalytics = nil
	s.cfg.Logger.Printf("loaded %d telemetry records from %s", len(s.file.Records), s.cfg.TelemetryPath)
	return nil
}

// analyticsSnapshot returns the default aggregation over the loaded dataset,
// computing it on first use and reusing it for every later request.
func (s *Server) analyticsSnapshot() analytics.Result {
	s.analyticsMu.Lock()
	defer s.analyticsMu.Unlock()
	if s.cachedAnalytics == nil {
		result := analytics.Analyze(s.file.Records, analytics.Config{
			SpikeThresholdPercent: s.cfg.SpikeThresholdPct,
		})
		s.cachedAnalytics = &result
	}
	return *s.cachedAnalytics
}

// maxBodyBytes caps JSON request bodies so one request cannot force the
// server to buffer an arbitrarily large payload.
const maxBodyBytes = 64 << 10 // 64 KiB

// writeJSON encodes payload as the JSON response body and logs any encoding
// failure. Headers are already committed when Encode runs, so the error can
// only be reported to the server log, never silently discarded.
func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.cfg.Logger.Printf("failed to encode JSON response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSONBody decodes a size-capped JSON request body into dst. On any
// failure it logs the underlying cause and writes a generic 400 response so
// internal error detail is never echoed to clients.
func (s *Server) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := decoder.Decode(dst); err != nil {
		s.cfg.Logger.Printf("invalid JSON request on %s: %v", r.URL.Path, err)
		s.writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return err
	}
	if decoder.More() {
		s.writeError(w, http.StatusBadRequest, "invalid JSON request body: unexpected trailing data")
		return fmt.Errorf("trailing data after JSON value")
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "ecopulse-ai-backend",
	})
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	facilityID := query.Get("facility_id")
	startParam := query.Get("start")
	endParam := query.Get("end")
	limitParam := query.Get("limit")

	var start, end *time.Time
	if startParam != "" {
		parsed, err := time.Parse(time.RFC3339, startParam)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "start must be an RFC3339 timestamp, e.g. 2026-08-03T18:00:00+03:00")
			return
		}
		start = &parsed
	}
	if endParam != "" {
		parsed, err := time.Parse(time.RFC3339, endParam)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "end must be an RFC3339 timestamp, e.g. 2026-08-03T22:00:00+03:00")
			return
		}
		end = &parsed
	}
	if start != nil && end != nil && end.Before(*start) {
		s.writeError(w, http.StatusBadRequest, "end must not be earlier than start")
		return
	}

	limit := 0
	if limitParam != "" {
		parsedLimit, err := strconv.Atoi(limitParam)
		if err != nil || parsedLimit < 0 {
			s.writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = parsedLimit
	}

	capacity := len(s.file.Records)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	selected := make([]analytics.TelemetryRecord, 0, capacity)
	timeFiltered := start != nil || end != nil
	for i, record := range s.file.Records {
		if facilityID != "" && record.FacilityID != facilityID {
			continue
		}
		if timeFiltered {
			if !s.timestampValid[i] {
				continue
			}
			ts := s.timestamps[i]
			if start != nil && ts.Before(*start) {
				continue
			}
			if end != nil && ts.After(*end) {
				continue
			}
		}
		selected = append(selected, record)
		if limit > 0 && len(selected) >= limit {
			break
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"total_records": len(s.file.Records),
		"count":         len(selected),
		"filters": map[string]string{
			"facility_id": facilityID,
			"start":       startParam,
			"end":         endParam,
		},
		"records": selected,
	})
}

func (s *Server) handleTariffCalculate(w http.ResponseWriter, r *http.Request) {
	var req tariff.Request
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		return
	}
	if req.USDPerEGP <= 0 && s.cfg.USDPerEGP > 0 {
		req.USDPerEGP = s.cfg.USDPerEGP
	}
	breakdown, err := tariff.Calculate(req)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, breakdown)
}

func (s *Server) handleAnalyticsSpikes(w http.ResponseWriter, _ *http.Request) {
	result := s.analyticsSnapshot()
	s.writeJSON(w, http.StatusOK, result)
}

type reportRequest struct {
	Locale  string             `json:"locale"`
	Metrics *ai_report.Metrics `json:"metrics,omitempty"`
}

func (s *Server) buildReportMetrics() ai_report.Metrics {
	result := s.analyticsSnapshot()
	return ai_report.Metrics{
		TotalEnergyKWh:     result.TotalEnergyKWh,
		TotalCarbonKG:      result.TotalCarbonKG,
		CriticalSpikeCount: len(result.CriticalSpikes),
		MaintenanceCount:   len(result.MaintenanceAlerts),
		PeakEnergyKWh:      result.PeakMetrics.TotalEnergyKWh,
		MaxDemandKW:        result.PeakMetrics.MaxDemandKW,
		FacilityCount:      result.FacilityCount,
	}
}

// handleConfigGeminiStatus reports whether a GEMINI_API_KEY is set in the
// server environment, so the frontend can choose between the system default
// key and a user-supplied custom key. The value is a plain boolean; the key
// itself is never exposed.
func (s *Server) handleConfigGeminiStatus(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]bool{
		"has_valid_env_key": s.cfg.GeminiAPIKey != "",
	})
}

func (s *Server) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		return
	}
	metrics := req.Metrics
	if metrics == nil {
		computed := s.buildReportMetrics()
		metrics = &computed
	}
	client := s.gemini
	if customKey := r.Header.Get("x-goog-api-key"); customKey != "" {
		client = client.WithAPIKey(customKey)
	}
	result := client.GenerateSummary(r.Context(), req.Locale, *metrics)
	s.writeJSON(w, http.StatusOK, result)
}
