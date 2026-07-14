package service

import (
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestNextRestartOrderPaidAtFindsCurrentSubscriptionRestart(t *testing.T) {
	t.Parallel()

	firstPaidAt := time.Date(2026, 7, 3, 16, 48, 10, 0, time.UTC)
	restartPaidAt := time.Date(2026, 7, 7, 19, 40, 57, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:  restartPaidAt,
		ExpiresAt: restartPaidAt.Add(5 * 24 * time.Hour),
	}
	first := &dbent.PaymentOrder{ID: 415, PaidAt: &firstPaidAt}
	restart := &dbent.PaymentOrder{ID: 443, PaidAt: &restartPaidAt}

	got, ok := nextRestartOrderPaidAt(sub, []*dbent.PaymentOrder{first, restart}, first)

	require.True(t, ok)
	require.Equal(t, restartPaidAt, got)
}

func TestFilterSubscriptionCurrentPeriodOrdersExcludesHistoricalReusedSubscriptionOrders(t *testing.T) {
	t.Parallel()

	historicalPaidAt := time.Date(2026, 7, 3, 16, 48, 10, 0, time.UTC)
	currentPaidAt := time.Date(2026, 7, 7, 19, 40, 57, 0, time.UTC)
	renewalPaidAt := time.Date(2026, 7, 13, 13, 27, 51, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:  currentPaidAt,
		ExpiresAt: time.Date(2026, 7, 21, 19, 40, 57, 0, time.UTC),
	}
	orders := []*dbent.PaymentOrder{
		{ID: 415, PaidAt: &historicalPaidAt},
		{ID: 443, PaidAt: &currentPaidAt},
		{ID: 490, PaidAt: &renewalPaidAt},
	}

	got := filterSubscriptionCurrentPeriodOrders(sub, orders)

	require.Len(t, got, 2)
	require.Equal(t, int64(443), got[0].ID)
	require.Equal(t, int64(490), got[1].ID)
}
