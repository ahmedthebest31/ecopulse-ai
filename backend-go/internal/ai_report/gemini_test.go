package ai_report

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWithAPIKeyUsesCustomKey(t *testing.T) {
	var sentKey string
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		sentKey = req.Header.Get("x-goog-api-key")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"candidates":[{"content":{"parts":[{"text":"custom key used"}]}}]}`,
			)),
		}, nil
	})

	client := NewClient("env-key", defaultModel)
	client.http = &http.Client{Transport: transport}

	scoped := client.WithAPIKey("custom-key")
	result := scoped.GenerateSummary(context.Background(), "en", Metrics{TotalEnergyKWh: 10})

	if result.Source != "gemini" {
		t.Fatalf("expected source gemini, got %s (warning: %s)", result.Source, result.Warning)
	}
	if sentKey != "custom-key" {
		t.Fatalf("expected x-goog-api-key header %q, got %q", "custom-key", sentKey)
	}
	if !strings.Contains(result.Summary, "custom key used") {
		t.Fatalf("expected echoed summary, got %q", result.Summary)
	}
}

func TestWithAPIKeyKeepsOriginalClient(t *testing.T) {
	client := NewClient("env-key", defaultModel)
	scoped := client.WithAPIKey("custom-key")
	if scoped == client {
		t.Fatal("WithAPIKey must return a different client instance")
	}
	if client.apiKey != "env-key" {
		t.Fatalf("original client key changed to %q", client.apiKey)
	}
}
