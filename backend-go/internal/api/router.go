package api

import (
	"log"
	"net/http"
	"time"
)

// Routes assembles the HTTP handler with CORS and logging middleware.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/telemetry", s.handleTelemetry)
	mux.HandleFunc("POST /api/tariff/calculate", s.handleTariffCalculate)
	mux.HandleFunc("GET /api/analytics/spikes", s.handleAnalyticsSpikes)
	mux.HandleFunc("GET /api/config/gemini-status", s.handleConfigGeminiStatus)
	mux.HandleFunc("POST /api/report/summary", s.handleReportSummary)
	return s.withCORS(withLogging(s.cfg.Logger, mux))
}

// withCORS answers preflight requests and emits cross-origin headers so the
// React frontend can call the API from a different origin. With no configured
// origins the behavior stays fully permissive ("*" wildcard, matching the
// original contract); when cfg.AllowedOrigins lists specific origins, only
// those are echoed back.
func (s *Server) withCORS(next http.Handler) http.Handler {
	wildcard := len(s.cfg.AllowedOrigins) == 0
	for _, origin := range s.cfg.AllowedOrigins {
		if origin == "*" {
			wildcard = true
		}
	}
	allowed := make(map[string]bool, len(s.cfg.AllowedOrigins))
	for _, origin := range s.cfg.AllowedOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case wildcard:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case allowed[r.Header.Get("Origin")]:
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-goog-api-key")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func withLogging(logger *log.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, recorder.status, time.Since(start).Round(time.Millisecond))
	})
}
