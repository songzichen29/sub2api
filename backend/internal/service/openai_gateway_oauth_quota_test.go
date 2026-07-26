//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsOAuthQuotaNearLimit_Disabled(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 0, // 默认关闭
				},
			},
		},
	}
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 95.0,
			"codex_primary_used_percent":   92.0,
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "threshold=0 should always return false")
}

func TestIsOAuthQuotaNearLimit_NonOAuth(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	account := &Account{
		Type:     AccountTypeAPIKey,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 95.0,
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "APIKey accounts should not be checked")
}

func TestIsOAuthQuotaNearLimit_SecondaryExceeded(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	now := time.Now().Format(time.RFC3339)
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 92.0,
			"codex_primary_used_percent":   50.0,
			"codex_usage_updated_at":       now,
		},
	}
	require.True(t, svc.isOAuthQuotaNearLimit(account), "secondary > threshold should trigger skip")
}

func TestIsOAuthQuotaNearLimit_PrimaryExceeded(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	now := time.Now().Format(time.RFC3339)
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 50.0,
			"codex_primary_used_percent":   91.0,
			"codex_usage_updated_at":       now,
		},
	}
	require.True(t, svc.isOAuthQuotaNearLimit(account), "primary > threshold should trigger skip")
}

func TestIsOAuthQuotaNearLimit_BelowThreshold(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	now := time.Now().Format(time.RFC3339)
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 80.0,
			"codex_primary_used_percent":   70.0,
			"codex_usage_updated_at":       now,
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "both below threshold should not skip")
}

func TestIsOAuthQuotaNearLimit_StaleData(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	staleTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 95.0,
			"codex_primary_used_percent":   92.0,
			"codex_usage_updated_at":       staleTime,
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "stale data (>5min) should not trigger skip")
}

func TestIsOAuthQuotaNearLimit_NoData(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra:    map[string]any{},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "no quota data should not trigger skip")
}

func TestIsOAuthQuotaNearLimit_NilAccount(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(nil), "nil account should not panic")
}

func TestIsOAuthQuotaNearLimit_NilSvc(t *testing.T) {
	var svc *OpenAIGatewayService
	account := &Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_secondary_used_percent": 95.0,
		},
	}
	require.False(t, svc.isOAuthQuotaNearLimit(account), "nil service should not panic")
}

func TestResolveFreshSchedulableOpenAIAccount_SkipsOAuthNearLimit(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Scheduling: config.GatewaySchedulingConfig{
					OpenAIOAuthQuotaThreshold: 90,
				},
			},
		},
	}
	now := time.Now().Format(time.RFC3339)
	account := &Account{
		ID:          1,
		Type:        AccountTypeOAuth,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_secondary_used_percent": 95.0,
			"codex_usage_updated_at":       now,
		},
	}
	result := svc.resolveFreshSchedulableOpenAIAccount(context.Background(), account, PlatformOpenAI, "", false, "")
	require.Nil(t, result, "account near quota limit should be skipped")
}
