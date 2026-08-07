package usage_provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSub2APIProviderFetchWalletBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Errorf("path = %q, want /v1/usage", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-sub2" {
			t.Errorf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode":      "unrestricted",
			"isValid":   true,
			"planName":  "钱包余额",
			"remaining": 26.31452308,
			"balance":   26.31452308,
			"unit":      "USD",
		})
	}))
	defer srv.Close()

	provider := NewSub2APIProviderWithClient(testHTTPClient())
	got, err := provider.Fetch(context.Background(), Config{
		Provider:    ProviderSub2API,
		BaseURL:     srv.URL,
		AccessToken: "sk-sub2",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.Remaining != 26.31452308 || got.PlanName != "钱包余额" || got.Unit != "USD" {
		t.Fatalf("Fetch() = %#v", got)
	}
	if got.TotalKnown {
		t.Fatal("wallet response must not claim a known historical total")
	}
}

func TestSub2APIProviderFetchKeyQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode":    "quota_limited",
			"isValid": true,
			"quota": map[string]any{
				"limit":     100.0,
				"used":      35.0,
				"remaining": 65.0,
				"unit":      "USD",
			},
		})
	}))
	defer srv.Close()

	provider := NewSub2APIProviderWithClient(testHTTPClient())
	got, err := provider.Fetch(context.Background(), Config{
		Provider:    ProviderSub2API,
		BaseURL:     srv.URL + "/v1",
		AccessToken: "sk-sub2",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.Remaining != 65 || got.Used != 35 || got.Total != 100 || !got.TotalKnown {
		t.Fatalf("Fetch() = %#v", got)
	}
	if got.Utilization != 0.35 {
		t.Fatalf("utilization = %v, want 0.35", got.Utilization)
	}
}

func TestSub2APIProviderRejectsInvalidKeyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isValid": false,
			"status":  "disabled",
		})
	}))
	defer srv.Close()

	provider := NewSub2APIProviderWithClient(testHTTPClient())
	_, err := provider.Fetch(context.Background(), Config{
		Provider:    ProviderSub2API,
		BaseURL:     srv.URL,
		AccessToken: "sk-sub2",
	})
	if err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("Fetch() error = %v, want invalid key error", err)
	}
}

func TestBuildSub2APIUsageURL(t *testing.T) {
	tests := map[string]string{
		"https://example.com":                     "https://example.com/v1/usage",
		"https://example.com/v1":                  "https://example.com/v1/usage",
		"https://example.com/openai/v1/":          "https://example.com/openai/v1/usage",
		"https://example.com/v1/usage":            "https://example.com/v1/usage",
		"https://example.com/v1?region=us#ignore": "https://example.com/v1/usage?region=us",
	}
	for input, want := range tests {
		if got := buildSub2APIUsageURL(input); got != want {
			t.Errorf("buildSub2APIUsageURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSub2APIProviderLive(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("SUB2API_TEST_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("SUB2API_TEST_API_KEY"))
	if baseURL == "" || apiKey == "" {
		t.Skip("SUB2API_TEST_BASE_URL and SUB2API_TEST_API_KEY are required")
	}

	got, err := NewSub2APIProvider().Fetch(context.Background(), Config{
		Provider:    ProviderSub2API,
		BaseURL:     baseURL,
		AccessToken: apiKey,
	})
	if err != nil {
		t.Fatalf("live Fetch() error = %v", err)
	}
	if got.Remaining < 0 || strings.TrimSpace(got.Unit) == "" {
		t.Fatalf("live Fetch() returned invalid quota: %#v", got)
	}
	t.Logf("live quota: plan=%q remaining=%.8f unit=%s total_known=%t", got.PlanName, got.Remaining, got.Unit, got.TotalKnown)
}
