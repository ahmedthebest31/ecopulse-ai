// Command server runs the EcoPulse AI telemetry, tariff, analytics, and
// report REST API. Configuration is provided through environment variables:
//
//	PORT                     HTTP listen port (default 8080)
//	HOST                     listen address (default 127.0.0.1; use 0.0.0.0 for LAN access)
//	TELEMETRY_DATA_PATH      path to telemetry_data.json (default ../data-generator/output/telemetry_data.json)
//	SPIKE_THRESHOLD_PERCENT  anomaly spike threshold (default 30)
//	USD_PER_EGP              EGP per USD conversion for USD cost display (default 48.5)
//	GEMINI_API_KEY           Google Gemini API key; when empty the report endpoint uses fallback text
//	GEMINI_MODEL             Gemini model id (default gemini-3.5-flash)
//	ALLOWED_ORIGINS          comma-separated CORS origins (default "*" wildcard)
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"backend-go/internal/api"
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func main() {
	logger := log.New(os.Stdout, "ecopulse ", log.LstdFlags|log.LUTC)

	cfg := api.Config{
		TelemetryPath:     envOr("TELEMETRY_DATA_PATH", "../data-generator/output/telemetry_data.json"),
		SpikeThresholdPct: envFloat("SPIKE_THRESHOLD_PERCENT", 30),
		USDPerEGP:         envFloat("USD_PER_EGP", 48.5),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:       os.Getenv("GEMINI_MODEL"),
		AllowedOrigins:    envList("ALLOWED_ORIGINS"),
		Logger:            logger,
	}

	server := api.NewServer(cfg)
	if err := server.LoadTelemetry(); err != nil {
		logger.Fatalf("startup failed: %v", err)
	}

	host := envOr("HOST", "127.0.0.1")
	port := envOr("PORT", "8080")
	httpServer := &http.Server{
		Addr:              host + ":" + port,
		Handler:           server.Routes(),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Printf("graceful shutdown failed: %v", err)
		}
	}()

	logger.Printf("EcoPulse AI backend listening on %s:%s", host, port)
	logger.Printf("telemetry source: %s", cfg.TelemetryPath)
	if cfg.GeminiAPIKey == "" {
		logger.Printf("GEMINI_API_KEY not set: /api/report/summary will return fallback text")
	}
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server error: %v", err)
	}
	logger.Printf("server stopped")
}
