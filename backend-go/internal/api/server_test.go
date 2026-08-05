package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
