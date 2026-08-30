package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/usage_provider"
)

// usageQueryExtraKey account.extra 中第三方用量查询配置的 key
const usageQueryExtraKey = "usage_query"

// usageQueryConfig 是 account.extra.usage_query 的内部表示。
//
// access_token 在 DB 中以 AES-GCM 密文形式存储；通过 SecretEncryptor 解密后
// 才能用于 HTTP 请求。BaseURL / UserID 不加密，便于直接展示给管理员。
type usageQueryConfig struct {
	Enabled            bool
	Provider           usage_provider.ProviderType
	BaseURL            string
	EncryptedAccessKey string // 仍为密文，需要解密后才能使用
	UserID             string
}

// extractUsageQueryConfig 从 account.Extra 中解析出第三方用量查询配置。
// 当 enabled 为 false 或字段缺失时返回 ok=false，调用方据此跳过外部查询。
func extractUsageQueryConfig(account *Account) (usageQueryConfig, bool) {
	if account == nil || account.Extra == nil {
		return usageQueryConfig{}, false
	}
	raw, ok := account.Extra[usageQueryExtraKey]
	if !ok {
		return usageQueryConfig{}, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return usageQueryConfig{}, false
	}
	cfg := usageQueryConfig{}
	if v, ok := m["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if !cfg.Enabled {
		return usageQueryConfig{}, false
	}
	if v, ok := m["provider"].(string); ok {
		cfg.Provider = usage_provider.ProviderType(strings.TrimSpace(v))
	}
	if v, ok := m["base_url"].(string); ok {
		cfg.BaseURL = strings.TrimSpace(v)
	}
	if v, ok := m["access_token"].(string); ok {
		cfg.EncryptedAccessKey = strings.TrimSpace(v)
	}
	if v, ok := m["user_id"].(string); ok {
		cfg.UserID = strings.TrimSpace(v)
	}
	return cfg, true
}

// resolveProviderConfig 把内部 usageQueryConfig 转成 Provider 期望的 Config。
// sub2api 复用账号本身的 base_url/api_key；其它 Provider 继续使用独立加密凭据。
func (s *AccountUsageService) resolveProviderConfig(account *Account, cfg usageQueryConfig) (usage_provider.Config, error) {
	provider := cfg.Provider
	if provider == "" {
		provider = usage_provider.ProviderNewAPI
	}
	if provider == usage_provider.ProviderSub2API {
		if account == nil {
			return usage_provider.Config{}, fmt.Errorf("account is required")
		}
		providerCfg := usage_provider.Config{
			Provider:    provider,
			BaseURL:     strings.TrimSpace(account.GetCredential("base_url")),
			AccessToken: strings.TrimSpace(account.GetCredential("api_key")),
		}
		if err := providerCfg.Validate(); err != nil {
			return usage_provider.Config{}, err
		}
		return providerCfg, nil
	}

	if s.secretEncryptor == nil {
		return usage_provider.Config{}, fmt.Errorf("secret encryptor not configured")
	}
	if cfg.EncryptedAccessKey == "" {
		return usage_provider.Config{}, fmt.Errorf("access_token is required")
	}
	plaintext, err := s.secretEncryptor.Decrypt(cfg.EncryptedAccessKey)
	if err != nil {
		return usage_provider.Config{}, fmt.Errorf("decrypt access_token failed: %w", err)
	}
	pc := usage_provider.Config{
		Provider:    provider,
		BaseURL:     cfg.BaseURL,
		AccessToken: plaintext,
		UserID:      cfg.UserID,
	}
	if err := pc.Validate(); err != nil {
		return usage_provider.Config{}, err
	}
	return pc, nil
}

// getThirdPartyUsage 通过第三方 Provider 拉取额度，带 5 分钟成功 / 1 分钟错误负缓存
// + singleflight，防止短时间内大量请求击穿。
func (s *AccountUsageService) getThirdPartyUsage(ctx context.Context, account *Account, cfg usageQueryConfig, force bool) (*UsageInfo, error) {
	if account == nil {
		return buildThirdPartyUsageInfo(nil, fmt.Errorf("account is required")), nil
	}
	accountID := account.ID
	// 1. 检查缓存
	if cached, ok := s.cache.thirdPartyCache.Load(accountID); ok && !force {
		if entry, ok := cached.(*thirdPartyUsageCache); ok {
			age := time.Since(entry.timestamp)
			if entry.err != nil && age < thirdPartyErrorCacheTTL {
				return buildThirdPartyUsageInfo(nil, entry.err), nil
			}
			if entry.quota != nil && age < thirdPartyCacheTTL {
				s.disableSchedulingForZeroThirdPartyQuota(ctx, account, entry.quota)
				return buildThirdPartyUsageInfo(entry.quota, nil), nil
			}
		}
	}

	flightKey := fmt.Sprintf("third_party_usage:%d", accountID)
	result, fetchErr, _ := s.cache.thirdPartyFlight.Do(flightKey, func() (any, error) {
		// 双检
		if cached, ok := s.cache.thirdPartyCache.Load(accountID); ok && !force {
			if entry, ok := cached.(*thirdPartyUsageCache); ok {
				age := time.Since(entry.timestamp)
				if entry.err != nil && age < thirdPartyErrorCacheTTL {
					return nil, entry.err
				}
				if entry.quota != nil && age < thirdPartyCacheTTL {
					return entry.quota, nil
				}
			}
		}

		providerCfg, err := s.resolveProviderConfig(account, cfg)
		if err != nil {
			s.cache.thirdPartyCache.Store(accountID, &thirdPartyUsageCache{
				err:       err,
				timestamp: time.Now(),
			})
			return nil, err
		}

		factory := s.thirdPartyFactory
		if factory == nil {
			factory = usage_provider.New
		}
		provider, err := factory(providerCfg.Provider)
		if err != nil {
			s.cache.thirdPartyCache.Store(accountID, &thirdPartyUsageCache{
				err:       err,
				timestamp: time.Now(),
			})
			return nil, err
		}

		quota, err := provider.Fetch(ctx, providerCfg)
		if err != nil {
			s.cache.thirdPartyCache.Store(accountID, &thirdPartyUsageCache{
				err:       err,
				timestamp: time.Now(),
			})
			return nil, err
		}
		s.cache.thirdPartyCache.Store(accountID, &thirdPartyUsageCache{
			quota:     quota,
			timestamp: time.Now(),
		})
		return quota, nil
	})

	if fetchErr != nil {
		// 错误降级：仍返回结构化 UsageInfo（带 error 字段），与前端 amber 提示样式一致
		return buildThirdPartyUsageInfo(nil, fetchErr), nil
	}
	quota, _ := result.(*usage_provider.QuotaInfo)
	s.disableSchedulingForZeroThirdPartyQuota(ctx, account, quota)
	return buildThirdPartyUsageInfo(quota, nil), nil
}

// disableSchedulingForZeroThirdPartyQuota applies fail-closed scheduling for
// a valid third-party quota response with no remaining balance. Errors and
// unknown results do not change scheduling state.
func (s *AccountUsageService) disableSchedulingForZeroThirdPartyQuota(ctx context.Context, account *Account, quota *usage_provider.QuotaInfo) {
	if s == nil || s.accountRepo == nil || account == nil || quota == nil ||
		math.IsNaN(quota.Remaining) || math.IsInf(quota.Remaining, 0) || quota.Remaining > 0 {
		return
	}
	if !account.Schedulable {
		return
	}
	if err := s.accountRepo.SetSchedulable(ctx, account.ID, false); err != nil {
		slog.Warn("disable_account_scheduling_zero_third_party_quota_failed", "account_id", account.ID, "error", err)
		return
	}
	account.Schedulable = false
	slog.Warn("account_scheduling_disabled_zero_third_party_quota", "account_id", account.ID, "unit", quota.Unit)
}

// buildThirdPartyUsageInfo 把 Provider 结果包装成 UsageInfo。
func buildThirdPartyUsageInfo(quota *usage_provider.QuotaInfo, err error) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		Source:    "active",
		UpdatedAt: &now,
	}
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.ThirdPartyQuota = quota
	return info
}
