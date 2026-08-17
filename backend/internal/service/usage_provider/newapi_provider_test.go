package usage_provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testHTTPClient 返回一个不做私网 IP 校验的 client，专供本文件单测使用
func testHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func TestNewAPIProvider_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验路径与方法
		if r.URL.Path != "/api/user/self" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		// 校验 header（与用户提供脚本一致）
		if got := r.Header.Get("Authorization"); got != "Bearer my-token" {
			t.Errorf("Authorization header = %q, want Bearer my-token", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "12345" {
			t.Errorf("New-Api-User header = %q, want 12345", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type header = %q, want application/json", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "cc-switch") {
			t.Errorf("User-Agent header = %q, want prefix cc-switch", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"group":      "VIP",
				"quota":      71250000.0, // 142.5 USD * 500000
				"used_quota": 28750000.0, // 57.5 USD * 500000
			},
		})
	}))
	defer srv.Close()

	p := NewNewAPIProviderWithClient(testHTTPClient())
	got, err := p.Fetch(context.Background(), Config{
		Provider:    ProviderNewAPI,
		BaseURL:     srv.URL,
		AccessToken: "my-token",
		UserID:      "12345",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PlanName != "VIP" {
		t.Errorf("plan_name = %q, want VIP", got.PlanName)
	}
	if got.Remaining != 142.5 {
		t.Errorf("remaining = %v, want 142.5", got.Remaining)
	}
	if got.Used != 57.5 {
		t.Errorf("used = %v, want 57.5", got.Used)
	}
	if got.Total != 200 {
		t.Errorf("total = %v, want 200", got.Total)
	}
	if got.Unit != "USD" {
		t.Errorf("unit = %q, want USD", got.Unit)
	}
	if got.Utilization < 0.287 || got.Utilization > 0.288 {
		t.Errorf("utilization = %v, want ~0.2875", got.Utilization)
	}
}

func TestNewAPIProvider_Fetch_BusinessFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "invalid token",
		})
	}))
	defer srv.Close()

	p := NewNewAPIProviderWithClient(testHTTPClient())
	_, err := p.Fetch(context.Background(), Config{
		Provider:    ProviderNewAPI,
		BaseURL:     srv.URL,
		AccessToken: "bad-token",
		UserID:      "12345",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("error = %v, want contain 'invalid token'", err)
	}
}

func TestNewAPIProvider_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := NewNewAPIProviderWithClient(testHTTPClient())
	_, err := p.Fetch(context.Background(), Config{
		Provider:    ProviderNewAPI,
		BaseURL:     srv.URL,
		AccessToken: "expired-token",
		UserID:      "12345",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want contain '401'", err)
	}
}

func TestNewAPIProvider_Fetch_ZeroTotalUtilization(t *testing.T) {
	// 边界：quota+used_quota = 0 时 utilization 应为 0 不报错
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"group":      "",
				"quota":      0,
				"used_quota": 0,
			},
		})
	}))
	defer srv.Close()

	p := NewNewAPIProviderWithClient(testHTTPClient())
	got, err := p.Fetch(context.Background(), Config{
		Provider:    ProviderNewAPI,
		BaseURL:     srv.URL,
		AccessToken: "tok",
		UserID:      "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Utilization != 0 {
		t.Errorf("utilization = %v, want 0", got.Utilization)
	}
	if got.PlanName != "默认套餐" {
		t.Errorf("plan_name = %q, want 默认套餐", got.PlanName)
	}
}

func TestConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid newapi", Config{Provider: ProviderNewAPI, BaseURL: "https://x", AccessToken: "t", UserID: "1"}, false},
		{"valid sub2api without user id", Config{Provider: ProviderSub2API, BaseURL: "https://x", AccessToken: "t"}, false},
		{"missing provider", Config{BaseURL: "https://x", AccessToken: "t", UserID: "1"}, true},
		{"missing base_url", Config{Provider: ProviderNewAPI, AccessToken: "t", UserID: "1"}, true},
		{"missing access_token", Config{Provider: ProviderNewAPI, BaseURL: "https://x", UserID: "1"}, true},
		{"missing user_id", Config{Provider: ProviderNewAPI, BaseURL: "https://x", AccessToken: "t"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func TestNew_FactoryDispatch(t *testing.T) {
	p, err := New(ProviderNewAPI)
	if err != nil {
		t.Fatalf("New(newapi) err = %v", err)
	}
	if p.Type() != ProviderNewAPI {
		t.Errorf("Type() = %q, want newapi", p.Type())
	}
	p, err = New(ProviderSub2API)
	if err != nil {
		t.Fatalf("New(sub2api) err = %v", err)
	}
	if p.Type() != ProviderSub2API {
		t.Errorf("Type() = %q, want sub2api", p.Type())
	}

	if _, err := New("oneapi"); err == nil {
		t.Error("New(oneapi) expected error, got nil")
	}
}
