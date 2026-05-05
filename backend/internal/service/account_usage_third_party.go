package service

import (
	"context"
	"fmt"
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

// resolveProviderConfig 把内部 usageQueryConfig 解密并校验后转成 Provider 期望的 Config。
func (s *AccountUsageService) resolveProviderConfig(cfg usageQueryConfig) (usage_provider.Config, error) {
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
	provider := cfg.Provider
	if provider == "" {
		provider = usage_provider.ProviderNewAPI
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
func (s *AccountUsageService) getThirdPartyUsage(ctx context.Context, accountID int64, cfg usageQueryConfig) (*UsageInfo, error) {
	// 1. 检查缓存
	if cached, ok := s.cache.thirdPartyCache.Load(accountID); ok {
		if entry, ok := cached.(*thirdPartyUsageCache); ok {
			age := time.Since(entry.timestamp)
			if entry.err != nil && age < thirdPartyErrorCacheTTL {
				return buildThirdPartyUsageInfo(nil, entry.err), nil
			}
			if entry.quota != nil && age < thirdPartyCacheTTL {
				return buildThirdPartyUsageInfo(entry.quota, nil), nil
			}
		}
	}

	flightKey := fmt.Sprintf("third_party_usage:%d", accountID)
	result, fetchErr, _ := s.cache.thirdPartyFlight.Do(flightKey, func() (any, error) {
		// 双检
		if cached, ok := s.cache.thirdPartyCache.Load(accountID); ok {
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

		providerCfg, err := s.resolveProviderConfig(cfg)
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
	return buildThirdPartyUsageInfo(quota, nil), nil
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
