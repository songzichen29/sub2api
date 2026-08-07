package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/usage_provider"
)

type countingUsageProvider struct {
	calls int
}

func (p *countingUsageProvider) Type() usage_provider.ProviderType {
	return usage_provider.ProviderSub2API
}

func (p *countingUsageProvider) Fetch(context.Context, usage_provider.Config) (*usage_provider.QuotaInfo, error) {
	p.calls++
	return &usage_provider.QuotaInfo{Remaining: float64(p.calls), Unit: "USD"}, nil
}

func TestResolveProviderConfigSub2APIUsesAccountCredentials(t *testing.T) {
	svc := &AccountUsageService{}
	account := &Account{Credentials: map[string]any{
		"base_url": " https://sub2.example/v1 ",
		"api_key":  " sk-current-account ",
	}}

	got, err := svc.resolveProviderConfig(account, usageQueryConfig{
		Provider: usage_provider.ProviderSub2API,
	})
	if err != nil {
		t.Fatalf("resolveProviderConfig() error = %v", err)
	}
	if got.BaseURL != "https://sub2.example/v1" || got.AccessToken != "sk-current-account" {
		t.Fatalf("resolveProviderConfig() = %#v", got)
	}
	if got.UserID != "" {
		t.Fatalf("UserID = %q, want empty", got.UserID)
	}
}

func TestThirdPartyUsageForceBypassesCache(t *testing.T) {
	provider := &countingUsageProvider{}
	svc := &AccountUsageService{
		cache: NewUsageCache(),
		thirdPartyFactory: func(usage_provider.ProviderType) (usage_provider.Provider, error) {
			return provider, nil
		},
	}
	account := &Account{
		ID: 42,
		Credentials: map[string]any{
			"base_url": "https://sub2.example/v1",
			"api_key":  "sk-current-account",
		},
	}
	cfg := usageQueryConfig{Provider: usage_provider.ProviderSub2API}

	first, err := svc.getThirdPartyUsage(context.Background(), account, cfg, false)
	if err != nil || first.ThirdPartyQuota == nil || first.ThirdPartyQuota.Remaining != 1 {
		t.Fatalf("first query = %#v, err=%v", first, err)
	}
	second, err := svc.getThirdPartyUsage(context.Background(), account, cfg, false)
	if err != nil || second.ThirdPartyQuota == nil || second.ThirdPartyQuota.Remaining != 1 {
		t.Fatalf("cached query = %#v, err=%v", second, err)
	}
	forced, err := svc.getThirdPartyUsage(context.Background(), account, cfg, true)
	if err != nil || forced.ThirdPartyQuota == nil || forced.ThirdPartyQuota.Remaining != 2 {
		t.Fatalf("forced query = %#v, err=%v", forced, err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}
