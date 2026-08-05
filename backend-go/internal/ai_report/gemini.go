// Package ai_report produces executive summary text for the analytics
// metrics. When a Gemini API key is configured it calls the Gemini
// generateContent REST endpoint; otherwise it falls back to deterministic
// templated summaries loaded from embedded locale resources (en/ar).
//
// Localization strings intentionally live in JSON resource files, not in Go
// source, so that Arabic text never appears in code files.
package ai_report

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultModel = "gemini-3.5-flash"
const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Metrics carries the executive numbers used to build the summary.
type Metrics struct {
	TotalEnergyKWh     float64 `json:"total_energy_kwh"`
	TotalCarbonKG      float64 `json:"total_carbon_kg"`
	CriticalSpikeCount int     `json:"critical_spike_count"`
	MaintenanceCount   int     `json:"maintenance_count"`
	PeakEnergyKWh      float64 `json:"peak_energy_kwh"`
	MaxDemandKW        float64 `json:"max_demand_kw"`
	FacilityCount      int     `json:"facility_count"`
}

// Result is the response payload for POST /api/report/summary.
type Result struct {
	Locale  string `json:"locale"`
	Summary string `json:"summary"`
	Source  string `json:"source"` // "gemini" or "fallback"
	Model   string `json:"model,omitempty"`
	Warning string `json:"warning,omitempty"`
}

//go:embed locales/*.json
var localeFS embed.FS

// Client wraps the Gemini REST API access.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// NewClient builds a Gemini client. An empty apiKey disables live generation
// and routes all calls to the templated fallback.
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = defaultModel
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 45 * time.Second},
	}
}

// WithAPIKey returns a copy of the client that uses the given key for its
// next call, letting a request-scoped key from a client header override the
// environment-configured default.
func (c *Client) WithAPIKey(apiKey string) *Client {
	clone := *c
	clone.apiKey = apiKey
	return &clone
}

// GenerateSummary returns a localized executive summary for the metrics. If
// no API key is configured, or the upstream call fails, it returns the
// templated fallback text with Source "fallback".
func (c *Client) GenerateSummary(ctx context.Context, locale string, m Metrics) Result {
	locale = normalizeLocale(locale)
	if c.apiKey == "" {
		return Result{
			Locale:  locale,
			Summary: c.fallbackText(locale, m),
			Source:  "fallback",
		}
	}
	text, err := c.generate(ctx, locale, m)
	if err != nil {
		return Result{
			Locale:  locale,
			Summary: c.fallbackText(locale, m),
			Source:  "fallback",
			Warning: fmt.Sprintf("Gemini request failed: %v", err),
		}
	}
	return Result{Locale: locale, Summary: text, Source: "gemini", Model: c.model}
}

func normalizeLocale(locale string) string {
	if locale == "ar" || locale == "en" {
		return locale
	}
	return "en"
}

func (c *Client) fallbackText(locale string, m Metrics) string {
	raw, err := localeFS.ReadFile("locales/" + locale + ".json")
	if err != nil {
		raw = []byte(`{"body":"Total energy consumption was {total_energy_kwh} kWh with {total_carbon_kg} kg CO2 emissions. {critical_spike_count} critical spikes and {maintenance_count} maintenance alerts were reported."}`)
	}
	var doc struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Sprintf("Total energy consumption was %.2f kWh with %.2f kg CO2 emissions.", m.TotalEnergyKWh, m.TotalCarbonKG)
	}
	replacer := strings.NewReplacer(
		"{total_energy_kwh}", fmt.Sprintf("%.2f", m.TotalEnergyKWh),
		"{total_carbon_kg}", fmt.Sprintf("%.2f", m.TotalCarbonKG),
		"{critical_spike_count}", fmt.Sprintf("%d", m.CriticalSpikeCount),
		"{maintenance_count}", fmt.Sprintf("%d", m.MaintenanceCount),
		"{peak_energy_kwh}", fmt.Sprintf("%.2f", m.PeakEnergyKWh),
		"{max_demand_kw}", fmt.Sprintf("%.2f", m.MaxDemandKW),
		"{facility_count}", fmt.Sprintf("%d", m.FacilityCount),
	)
	return replacer.Replace(doc.Body)
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (c *Client) generate(ctx context.Context, locale string, m Metrics) (string, error) {
	languageName := "English"
	if locale == "ar" {
		languageName = "Arabic"
	}
	metricsJSON, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{
				Text: "You are an executive energy analyst for EcoPulse AI, an energy analytics, " +
					"peak-hours optimization, and carbon footprint tracking platform for Egyptian " +
					"smart infrastructure. Summarize the provided metrics into a concise executive " +
					"brief of at most 120 words. Plain paragraphs only, no markdown, no tables. " +
					"Respond in " + languageName + ".",
			}},
		},
		Contents: []geminiContent{{
			Role: "user",
			Parts: []geminiPart{{
				Text: "Language: " + languageName + ". Metrics (JSON): " + string(metricsJSON),
			}},
		}},
		GenerationConfig: &geminiGenerationConfig{Temperature: 0.4, MaxOutputTokens: 512},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(c.baseURL, "/") + "/models/" + c.model + ":generateContent"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", c.apiKey)

	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API returned status %d: %s", response.StatusCode, truncate(string(body), 300))
	}
	var parsed geminiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("Gemini API returned no candidates")
	}
	text := strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text)
	if text == "" {
		return "", errors.New("Gemini API returned empty text")
	}
	return text, nil
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
