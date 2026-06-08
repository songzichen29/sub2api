//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type openAIOAuthErrorAlertSettingRepoStub struct {
	values map[string]string
}

func (s *openAIOAuthErrorAlertSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) Set(context.Context, string, string) error {
	return nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}

func (s *openAIOAuthErrorAlertSettingRepoStub) Delete(context.Context, string) error {
	return nil
}

type openAIOAuthErrorAlertAccountRepoStub struct {
	accounts []Account
}

func (s *openAIOAuthErrorAlertAccountRepoStub) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	result := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account.Platform == platform {
			result = append(result, account)
		}
	}
	return result, nil
}

type openAIOAuthErrorAlertUsageRepoStub struct {
	stats map[int64]*usagestats.AccountStats
}

func (s *openAIOAuthErrorAlertUsageRepoStub) GetAccountWindowStats(_ context.Context, accountID int64, _ time.Time) (*usagestats.AccountStats, error) {
	return s.stats[accountID], nil
}

func TestOpenAIOAuthErrorAlertService_NotifyAccountError(t *testing.T) {
	repo := &openAIOAuthErrorAlertSettingRepoStub{
		values: map[string]string{
			SettingKeyOpsEmailNotificationConfig: `{"alert":{"enabled":true,"recipients":["a@example.com","b@example.com"]},"report":{"enabled":false,"recipients":[]}}`,
			SettingKeyFrontendURL:                "https://example.com/",
		},
	}
	var sentTo []string
	var sentSubjects []string
	var sentBodies []string
	emailSvc := &EmailService{
		sendFunc: func(_ context.Context, to, subject, body string) error {
			sentTo = append(sentTo, to)
			sentSubjects = append(sentSubjects, subject)
			sentBodies = append(sentBodies, body)
			return nil
		},
	}
	accountRepo := &openAIOAuthErrorAlertAccountRepoStub{
		accounts: []Account{
			{ID: 12, Name: "test-openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 22, Name: "other-openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		},
	}
	usageRepo := &openAIOAuthErrorAlertUsageRepoStub{
		stats: map[int64]*usagestats.AccountStats{
			12: {Requests: 10, Tokens: 1000, Cost: 1.23},
			22: {Requests: 20, Tokens: 2000, Cost: 2.34},
		},
	}

	svc := NewOpenAIOAuthErrorAlertService(repo, emailSvc, accountRepo, usageRepo)
	svc.NotifyAccountError(context.Background(), &Account{
		ID:          12,
		Name:        "test-openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"email": "owner@example.com"},
	}, "ratelimit.handleAuthError", "Access forbidden (403) token=secret-token-value-1234567890")

	require.Equal(t, []string{"a@example.com", "b@example.com"}, sentTo)
	require.Len(t, sentSubjects, 2)
	require.Contains(t, sentSubjects[0], "OpenAI OAuth 账号错误")
	require.Len(t, sentBodies, 2)
	require.True(t, strings.Contains(sentBodies[0], "test-openai-oauth"))
	require.True(t, strings.Contains(sentBodies[0], "owner@example.com"))
	require.True(t, strings.Contains(sentBodies[0], "当前可用 OpenAI OAuth 账号"))
	require.True(t, strings.Contains(sentBodies[0], "1 个"))
	require.True(t, strings.Contains(sentBodies[0], "ratelimit.handleAuthError"))
	require.True(t, strings.Contains(sentBodies[0], "https://example.com/admin/accounts"))
	require.True(t, strings.Contains(sentBodies[0], "建议动作"))
	require.True(t, strings.Contains(sentBodies[0], "[已脱敏]"))
	require.False(t, strings.Contains(sentBodies[0], "secret-token-value-1234567890"))
	require.False(t, strings.Contains(sentBodies[0], "同平台账号汇总"))
	require.False(t, strings.Contains(sentBodies[0], "剩余可用账号与使用情况"))
	require.False(t, strings.Contains(sentBodies[0], "other-openai-oauth"))
	require.False(t, strings.Contains(sentBodies[0], "req=20, tokens=2000, cost=2.3400"))
}

func TestOpenAIOAuthErrorAlertService_NotifyAccountError_SkipsNonOpenAIOAuth(t *testing.T) {
	repo := &openAIOAuthErrorAlertSettingRepoStub{
		values: map[string]string{
			SettingKeyOpsEmailNotificationConfig: `{"alert":{"enabled":true,"recipients":["a@example.com"]},"report":{"enabled":false,"recipients":[]}}`,
		},
	}
	var sent int
	emailSvc := &EmailService{
		sendFunc: func(_ context.Context, to, subject, body string) error {
			sent++
			return nil
		},
	}

	svc := NewOpenAIOAuthErrorAlertService(repo, emailSvc, nil, nil)
	svc.NotifyAccountError(context.Background(), &Account{
		ID:       13,
		Name:     "not-openai-oauth",
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}, "trigger", "error")

	require.Equal(t, 0, sent)
}
