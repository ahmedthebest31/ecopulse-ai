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
}

// NewServer creates a Server from the given configuration.
func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stdout, "ecopulse ", log.LstdFlags|log.LUTC)
	}
	return &Server{
		cfg:    cfg,
		gemini: ai_report.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel),
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
	s.cfg.Logger.Printf("loaded %d telemetry records from %s", len(s.file.Records), s.cfg.TelemetryPath)
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
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
			writeError(w, http.StatusBadRequest, "start must be an RFC3339 timestamp, e.g. 2026-08-03T18:00:00+03:00")
			return
		}
		start = &parsed
	}
	if endParam != "" {
		parsed, err := time.Parse(time.RFC3339, endParam)
		if err != nil {
			writeError(w, http.StatusBadRequest, "end must be an RFC3339 timestamp, e.g. 2026-08-03T22:00:00+03:00")
			return
		}
		end = &parsed
	}

	limit := 0
	if limitParam != "" {
		parsedLimit, err := strconv.Atoi(limitParam)
		if err != nil || parsedLimit < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = parsedLimit
	}

	selected := make([]analytics.TelemetryRecord, 0, len(s.file.Records))
	for _, record := range s.file.Records {
		if facilityID != "" && record.FacilityID != facilityID {
			continue
		}
		if start != nil || end != nil {
			ts, err := time.Parse(time.RFC3339, record.Timestamp)
			if err != nil {
				continue
			}
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

	writeJSON(w, http.StatusOK, map[string]any{
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.USDPerEGP <= 0 && s.cfg.USDPerEGP > 0 {
		req.USDPerEGP = s.cfg.USDPerEGP
	}
	breakdown, err := tariff.Calculate(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, breakdown)
}

func (s *Server) handleAnalyticsSpikes(w http.ResponseWriter, _ *http.Request) {
	result := analytics.Analyze(s.file.Records, analytics.Config{
		SpikeThresholdPercent: s.cfg.SpikeThresholdPct,
	})
	writeJSON(w, http.StatusOK, result)
}

type reportRequest struct {
	Locale  string             `json:"locale"`
	Metrics *ai_report.Metrics `json:"metrics,omitempty"`
}

func (s *Server) buildReportMetrics() ai_report.Metrics {
	result := analytics.Analyze(s.file.Records, analytics.Config{
		SpikeThresholdPercent: s.cfg.SpikeThresholdPct,
	})
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

func (s *Server) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	metrics := req.Metrics
	if metrics == nil {
		computed := s.buildReportMetrics()
		metrics = &computed
	}
	result := s.gemini.GenerateSummary(r.Context(), req.Locale, *metrics)
	writeJSON(w, http.StatusOK, result)
}
