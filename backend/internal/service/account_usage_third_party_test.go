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

type zeroQuotaAccountRepoStub struct {
	AccountRepository
	setSchedulableCalls int
	lastSchedulable     bool
}

func (s *zeroQuotaAccountRepoStub) SetSchedulable(_ context.Context, _ int64, schedulable bool) error {
	s.setSchedulableCalls++
	s.lastSchedulable = schedulable
	return nil
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

func TestThirdPartyUsageDisablesSchedulingWhenBalanceIsZero(t *testing.T) {
	repo := &zeroQuotaAccountRepoStub{}
	svc := &AccountUsageService{
		accountRepo: repo,
		cache:       NewUsageCache(),
		thirdPartyFactory: func(usage_provider.ProviderType) (usage_provider.Provider, error) {
			return zeroQuotaProvider{}, nil
		},
	}
	account := &Account{
		ID:          7,
		Schedulable: true,
		Credentials: map[string]any{"base_url": "https://sub2.example/v1", "api_key": "key"},
	}

	usage, err := svc.getThirdPartyUsage(context.Background(), account, usageQueryConfig{Provider: usage_provider.ProviderSub2API}, false)
	if err != nil || usage == nil || usage.ThirdPartyQuota == nil || usage.ThirdPartyQuota.Remaining != 0 {
		t.Fatalf("zero quota usage = %#v, err=%v", usage, err)
	}
	if repo.setSchedulableCalls != 1 || repo.lastSchedulable {
		t.Fatalf("SetSchedulable calls = %d, last=%t; want one false update", repo.setSchedulableCalls, repo.lastSchedulable)
	}
	if account.Schedulable {
		t.Fatal("account should be marked unschedulable after zero quota")
	}

	_, err = svc.getThirdPartyUsage(context.Background(), account, usageQueryConfig{Provider: usage_provider.ProviderSub2API}, false)
	if err != nil {
		t.Fatalf("cached zero quota error = %v", err)
	}
	if repo.setSchedulableCalls != 1 {
		t.Fatalf("cached zero quota repeated scheduling update: %d calls", repo.setSchedulableCalls)
	}
}

type zeroQuotaProvider struct{}

func (zeroQuotaProvider) Type() usage_provider.ProviderType { return usage_provider.ProviderSub2API }

func (zeroQuotaProvider) Fetch(context.Context, usage_provider.Config) (*usage_provider.QuotaInfo, error) {
	return &usage_provider.QuotaInfo{Remaining: 0, Unit: "USD"}, nil
}
