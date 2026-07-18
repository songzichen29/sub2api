//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-" + uuid.NewString(),
		Name:   "billing",
		Quota:  1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     1.25,
		APIKeyRateLimitCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, result1.Applied)
	require.True(t, result1.APIKeyQuotaExhausted)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = ?", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = ?", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 1.25, quotaUsed, 0.000001)

	var usage5h float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT usage_5h FROM api_keys WHERE id = ?", apiKey.ID).Scan(&usage5h))
	require.InDelta(t, 1.25, usage5h, 0.000001)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE id = ?", apiKey.ID).Scan(&status))
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	var dedupCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = ? AND api_key_id = ?", requestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-sub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "usage-billing-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-usage-billing-sub-" + uuid.NewString(),
		Name:    "billing-sub",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var dailyUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = ?", subscription.ID).Scan(&dailyUsage))
	require.InDelta(t, 2.5, dailyUsage, 0.000001)
}

func TestUsageBillingRepositoryApply_RequestFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-conflict-" + uuid.NewString(),
		Name:   "billing-conflict",
	})

	requestID := uuid.NewString()
	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 2.50,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
}

func TestUsageBillingRepositoryApply_UpdatesAccountQuota(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-account-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-account-" + uuid.NewString(),
		Name:   "billing-account",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-quota-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 100.0,
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 3.5,
	})
	require.NoError(t, err)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE(JSON_EXTRACT(extra, '$.quota_used'), 0) FROM accounts WHERE id = ?", account.ID).Scan(&quotaUsed))
	require.InDelta(t, 3.5, quotaUsed, 0.000001)
}

func TestUsageBillingRepositoryApply_EnqueuesSchedulerOutboxOnQuotaCrossing(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	newFixture := func(t *testing.T, extra map[string]any) (int64, int64) {
		t.Helper()
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-outbox-user-%d-%s@example.com", time.Now().UnixNano(), uuid.NewString()),
			PasswordHash: "hash",
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-usage-billing-outbox-" + uuid.NewString(),
			Name:   "billing-outbox",
		})
		account := mustCreateAccount(t, client, &service.Account{
			Name:  "usage-billing-outbox-" + uuid.NewString(),
			Type:  service.AccountTypeAPIKey,
			Extra: extra,
		})
		return apiKey.ID, account.ID
	}

	outboxCountFor := func(t *testing.T, accountID int64) int {
		t.Helper()
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = ? AND account_id = ?",
			service.SchedulerOutboxEventAccountChanged, accountID,
		).Scan(&count))
		return count
	}

	t.Run("daily_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_daily_limit": 10.0,
		})
		// 第一次低于日限额：不应入队 outbox
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 4,
		})
		require.NoError(t, err)
		require.Equal(t, 0, outboxCountFor(t, accountID), "below limit should not enqueue")

		// 第二次跨越日限额：应入队一次 outbox
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 8,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "crossing daily limit should enqueue once")

		// 再次递增（已超）：不应重复入队
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 2,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "subsequent increments beyond limit should not re-enqueue")
	})

	t.Run("weekly_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_weekly_limit": 10.0,
		})
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 15, // 单次即跨越
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "single-shot crossing weekly limit should enqueue once")
	})
}

func TestDashboardAggregationRepositoryCleanupUsageBillingDedup_BatchDeletesOldRows(t *testing.T) {
	ctx := context.Background()
	repo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	oldRequestID := "dedup-old-" + uuid.NewString()
	newRequestID := "dedup-new-" + uuid.NewString()
	oldCreatedAt := time.Now().UTC().AddDate(0, 0, -400)
	newCreatedAt := time.Now().UTC().Add(-time.Hour)

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint, created_at)
		VALUES (?, 1, ?, ?), (?, 1, ?, ?)
	`,
		oldRequestID, strings.Repeat("a", 64), oldCreatedAt,
		newRequestID, strings.Repeat("b", 64), newCreatedAt,
	)
	require.NoError(t, err)

	require.NoError(t, repo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	var oldCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = ?", oldRequestID).Scan(&oldCount))
	require.Equal(t, 0, oldCount)

	var newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = ?", newRequestID).Scan(&newCount))
	require.Equal(t, 1, newCount)

	var archivedCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup_archive WHERE request_id = ?", oldRequestID).Scan(&archivedCount))
	require.Equal(t, 1, archivedCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesAgainstArchivedKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	aggRepo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-archive-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-archive-" + uuid.NewString(),
		Name:   "billing-archive",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET created_at = ?
		WHERE request_id = ? AND api_key_id = ?
	`, time.Now().UTC().AddDate(0, 0, -400), requestID, apiKey.ID)
	require.NoError(t, err)
	require.NoError(t, aggRepo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = ?", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)
}

func TestUsageBillingRepositoryApply_SubscriptionDailyOverdraftLimits(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	newFixtureWithUnit := func(t *testing.T, allowOverdraft bool, dailyUsage, weeklyUsage float64, validityUnit string) (int64, int64) {
		t.Helper()
		daily := 80.0
		weekly := 560.0
		startsAt := time.Now().Add(-time.Hour)
		dailyStart := startsAt
		weeklyStart := startsAt
		monthlyStart := startsAt
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-overdraft-%d-%s@example.com", time.Now().UnixNano(), uuid.NewString()),
			PasswordHash: "hash",
		})
		group := mustCreateGroup(t, client, &service.Group{
			Name:                "usage-billing-overdraft-" + uuid.NewString(),
			Platform:            service.PlatformAnthropic,
			RateMultiplier:      1,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			DailyLimitUSD:       &daily,
			WeeklyLimitUSD:      &weekly,
			AllowDailyOverdraft: allowOverdraft,
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID:  user.ID,
			GroupID: &group.ID,
			Key:     "sk-usage-billing-overdraft-" + uuid.NewString(),
			Name:    "billing-overdraft",
		})
		subscription := mustCreateSubscription(t, client, &service.UserSubscription{
			UserID:              user.ID,
			GroupID:             group.ID,
			StartsAt:            startsAt,
			ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
			DailyWindowStart:    &dailyStart,
			WeeklyWindowStart:   &weeklyStart,
			MonthlyWindowStart:  &monthlyStart,
			DailyUsageUSD:       dailyUsage,
			WeeklyUsageUSD:      weeklyUsage,
			ValidityUnit:        validityUnit,
			AllowDailyOverdraft: allowOverdraft,
		})
		return apiKey.ID, subscription.ID
	}
	newFixture := func(t *testing.T, allowOverdraft bool, dailyUsage, weeklyUsage float64) (int64, int64) {
		return newFixtureWithUnit(t, allowOverdraft, dailyUsage, weeklyUsage, "day")
	}

	t.Run("strict daily rejects", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixture(t, false, 79, 100)
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 2,
		})
		require.ErrorIs(t, err, service.ErrUsageBillingSubscriptionLimitExceeded)
	})

	t.Run("overdraft daily uses validity day pool", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixture(t, true, 80, 120)
		result, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 5,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.SubscriptionQuotaExhausted)
		var dailyUsage, weeklyUsage float64
		var status string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd, weekly_usage_usd, status FROM user_subscriptions WHERE id = ?", subscriptionID).Scan(&dailyUsage, &weeklyUsage, &status))
		require.InDelta(t, 85, dailyUsage, 0.000001)
		require.InDelta(t, 125, weeklyUsage, 0.000001)
		require.Equal(t, service.SubscriptionStatusActive, status)
	})

	t.Run("overdraft ignores weekly and monthly status exhaustion", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixture(t, true, 70, 120)
		_, err := integrationDB.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET weekly_usage_usd = 120, monthly_usage_usd = 559
			WHERE id = ?
		`, subscriptionID)
		require.NoError(t, err)

		result, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 1,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.SubscriptionQuotaExhausted)

		var status string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM user_subscriptions WHERE id = ?", subscriptionID).Scan(&status))
		require.Equal(t, service.SubscriptionStatusActive, status)
	})

	t.Run("overdraft rejects once validity day pool already full", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixture(t, true, 80, 400)
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 1,
		})
		require.ErrorIs(t, err, service.ErrUsageBillingSubscriptionLimitExceeded)
	})

	t.Run("day unit counts elapsed daily quota", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixtureWithUnit(t, true, 0, 0, "day")
		startsAt := time.Now().Add(-4 * 24 * time.Hour)
		_, err := integrationDB.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET starts_at = ?, expires_at = ?, daily_window_start = ?, daily_usage_usd = 10, weekly_usage_usd = 10
			WHERE id = ?
		`, startsAt, startsAt.Add(5*24*time.Hour), startsAt.Add(4*24*time.Hour), subscriptionID)
		require.NoError(t, err)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 1,
		})
		require.NoError(t, err)
	})

	t.Run("overdraft counts current daily usage after anchored reset", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixtureWithUnit(t, true, 0, 0, "day")
		startsAt := time.Now().Add(-4 * 24 * time.Hour)
		currentDailyStart := startsAt.Add(4 * 24 * time.Hour)
		_, err := integrationDB.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET starts_at = ?, expires_at = ?, daily_window_start = ?, daily_usage_usd = 70, weekly_usage_usd = 390
			WHERE id = ?
		`, startsAt, startsAt.Add(5*24*time.Hour), currentDailyStart, subscriptionID)
		require.NoError(t, err)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 1,
		})
		require.NoError(t, err)
	})

	t.Run("month unit keeps actual usage accounting", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixtureWithUnit(t, true, 0, 0, "month")
		startsAt := time.Now().Add(-4 * 24 * time.Hour)
		_, err := integrationDB.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET starts_at = ?, expires_at = ?, daily_window_start = ?, daily_usage_usd = 10, weekly_usage_usd = 10
			WHERE id = ?
		`, startsAt, startsAt.Add(5*24*time.Hour), startsAt.Add(4*24*time.Hour), subscriptionID)
		require.NoError(t, err)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 1,
		})
		require.NoError(t, err)
	})

	t.Run("disabled day overdraft repays historical debt", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixtureWithUnit(t, true, 0, 0, "day")
		startsAt := time.Now().Add(-4 * 24 * time.Hour)
		_, err := integrationDB.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET starts_at = ?, expires_at = ?, daily_window_start = ?, daily_usage_usd = 0, weekly_usage_usd = 330, allow_daily_overdraft = FALSE
			WHERE id = ?
		`, startsAt, startsAt.Add(5*24*time.Hour), startsAt.Add(4*24*time.Hour), subscriptionID)
		require.NoError(t, err)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 70,
		})
		require.NoError(t, err)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			SubscriptionID:   &subscriptionID,
			SubscriptionCost: 0.01,
		})
		require.ErrorIs(t, err, service.ErrUsageBillingSubscriptionLimitExceeded)
	})

	t.Run("allow over limit permits final crossing but rejects next", func(t *testing.T) {
		apiKeyID, subscriptionID := newFixture(t, true, 80, 399)
		result, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:                  uuid.NewString(),
			APIKeyID:                   apiKeyID,
			SubscriptionID:             &subscriptionID,
			SubscriptionCost:           1,
			AllowSubscriptionOverLimit: true,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.SubscriptionQuotaExhausted)

		var status string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM user_subscriptions WHERE id = ?", subscriptionID).Scan(&status))
		require.Equal(t, service.SubscriptionStatusQuotaExhausted, status)

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:                  uuid.NewString(),
			APIKeyID:                   apiKeyID,
			SubscriptionID:             &subscriptionID,
			SubscriptionCost:           1,
			AllowSubscriptionOverLimit: true,
		})
		require.ErrorIs(t, err, service.ErrUsageBillingSubscriptionLimitExceeded)
	})
}
