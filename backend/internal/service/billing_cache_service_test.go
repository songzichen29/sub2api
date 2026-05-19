package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingCacheWorkerStub struct {
	balanceUpdates      int64
	subscriptionUpdates int64
}

func (b *billingCacheWorkerStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	return nil
}

func (b *billingCacheWorkerStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (b *billingCacheWorkerStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func TestBillingCacheServiceQueueHighLoad(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	start := time.Now()
	for i := 0; i < cacheWriteBufferSize*2; i++ {
		svc.QueueDeductBalance(1, 1)
	}
	require.Less(t, time.Since(start), 2*time.Second)

	svc.QueueUpdateSubscriptionUsage(1, 2, 1.5)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.balanceUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.subscriptionUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingCacheServiceEnqueueAfterStopReturnsFalse(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	svc.Stop()

	enqueued := svc.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: 1,
		amount: 1,
	})
	require.False(t, enqueued)
}

type billingCacheSubscriptionStub struct {
	billingCacheWorkerStub
	data        *SubscriptionCacheData
	lastSetData *SubscriptionCacheData
}

func (b *billingCacheSubscriptionStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return b.data, nil
}

func (b *billingCacheSubscriptionStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	b.lastSetData = data
	return b.billingCacheWorkerStub.SetSubscriptionCache(ctx, userID, groupID, data)
}

type billingCacheSubscriptionRepoStub struct {
	userSubRepoNoop
	sub *UserSubscription
}

func (r *billingCacheSubscriptionRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	if r.sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func TestBillingCacheServiceCheckSubscriptionEligibility_DailyOverdraftUsesPeriodPool(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:              SubscriptionStatusActive,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		DailyUsage:          80,
		WeeklyUsage:         120,
		AllowDailyOverdraft: true,
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	strictGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, strictGroup, nil)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)

	overdraftGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	err = svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, nil)
	require.NoError(t, err)

	cache.data.WeeklyUsage = 400
	err = svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, nil)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)
}

func TestBillingCacheServiceCheckSubscriptionEligibility_DailyOverdraftUsesPassedSubscriptionStartForOldCache(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:              SubscriptionStatusActive,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		DailyUsage:          80,
		WeeklyUsage:         120,
		AllowDailyOverdraft: true,
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	overdraftGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	subscription := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(5 * 24 * time.Hour), AllowDailyOverdraft: true}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, subscription)
	require.NoError(t, err)
}

func TestBillingCacheServiceCheckSubscriptionEligibility_DailyOverdraftBackfillsMissingCacheAnchors(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:              SubscriptionStatusActive,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		DailyUsage:          80,
		WeeklyUsage:         120,
		AllowDailyOverdraft: true,
	}}
	repo := &billingCacheSubscriptionRepoStub{sub: &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		DailyUsageUSD:       80,
		WeeklyUsageUSD:      120,
		AllowDailyOverdraft: true,
	}}
	svc := NewBillingCacheService(cache, nil, repo, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	overdraftGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, nil)
	require.NoError(t, err)
	require.Greater(t, atomic.LoadInt64(&cache.subscriptionUpdates), int64(0))
	require.NotNil(t, cache.lastSetData)
	require.Equal(t, startsAt.Unix(), cache.lastSetData.StartsAt.Unix())
}
