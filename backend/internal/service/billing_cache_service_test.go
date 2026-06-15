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

func (b *billingCacheWorkerStub) GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
}

func (b *billingCacheWorkerStub) SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	return nil
}

func (b *billingCacheWorkerStub) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	return nil
}

func (b *billingCacheWorkerStub) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error {
	return nil
}

func (b *billingCacheWorkerStub) PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]UserPlatformQuotaKey, error) {
	return nil, nil
}

func (b *billingCacheWorkerStub) ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []UserPlatformQuotaKey) error {
	return nil
}

func (b *billingCacheWorkerStub) BatchGetUserPlatformQuotaCache(ctx context.Context, keys []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	return make([]*UserPlatformQuotaCacheEntry, len(keys)), nil
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
	data             *SubscriptionCacheData
	lastSetData      *SubscriptionCacheData
	invalidateCalled int
}

func (b *billingCacheSubscriptionStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	return b.data, nil
}

func (b *billingCacheSubscriptionStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	b.lastSetData = data
	return b.billingCacheWorkerStub.SetSubscriptionCache(ctx, userID, groupID, data)
}

func (b *billingCacheSubscriptionStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	b.invalidateCalled++
	return nil
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

func TestBillingCacheServiceCheckSubscriptionEligibility_WeekMonthOverdraftUsesActualUsage(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-4 * 24 * time.Hour)
	dailyStart := startsAt.Add(4 * 24 * time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:              SubscriptionStatusActive,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(30 * 24 * time.Hour),
		ValidityUnit:        "month",
		DailyWindowStart:    &dailyStart,
		DailyUsage:          20,
		WeeklyUsage:         120,
		AllowDailyOverdraft: true,
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	overdraftGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, nil)
	require.NoError(t, err)

	cache.data.WeeklyUsage = 2400
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

func TestBillingCacheServiceCheckSubscriptionEligibility_BackfillsOverdraftFlagFromSubscription(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:      SubscriptionStatusActive,
		StartsAt:    time.Time{},
		ExpiresAt:   startsAt.Add(5 * 24 * time.Hour),
		DailyUsage:  80,
		WeeklyUsage: 120,
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	overdraftGroup := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	subscription := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(5 * 24 * time.Hour), AllowDailyOverdraft: true}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, overdraftGroup, subscription)
	require.NoError(t, err)
}

func TestBillingCacheServiceCheckSubscriptionEligibility_BackfillsOverdraftFlagWithExistingStartsAt(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:      SubscriptionStatusActive,
		StartsAt:    startsAt,
		ExpiresAt:   startsAt.Add(5 * 24 * time.Hour),
		DailyUsage:  80,
		WeeklyUsage: 120,
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

func TestBillingCacheServiceCheckSubscriptionEligibility_DisabledDayOverdraftRepaysDebt(t *testing.T) {
	daily := 40.0
	startsAt := time.Now().Add(-29 * 24 * time.Hour)
	dailyStart := startsAt.Add(29 * 24 * time.Hour)
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:              SubscriptionStatusActive,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(30 * 24 * time.Hour),
		ValidityUnit:        "day",
		DailyWindowStart:    &dailyStart,
		DailyUsage:          0,
		WeeklyUsage:         1160 + 15,
		AllowDailyOverdraft: false,
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)

	group := &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, AllowDailyOverdraft: true}
	err := svc.checkSubscriptionEligibility(context.Background(), 1, group, nil)
	require.NoError(t, err)

	cache.data.DailyUsage = 25
	cache.data.WeeklyUsage = 1160 + 15 + 25
	err = svc.checkSubscriptionEligibility(context.Background(), 1, group, nil)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)
}

func TestBillingCacheServiceCheckSubscriptionEligibility_RechecksStaleDailyLimitCache(t *testing.T) {
	daily := 80.0
	now := time.Now()
	startsAt := now.Add(-7 * time.Hour)
	dailyStart := startsAt
	cache := &billingCacheSubscriptionStub{data: &SubscriptionCacheData{
		Status:           SubscriptionStatusActive,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(5 * 24 * time.Hour),
		DailyWindowStart: &dailyStart,
		DailyUsage:       daily,
		WeeklyUsage:      500,
	}}
	repo := &billingCacheSubscriptionRepoStub{sub: &UserSubscription{
		Status:           SubscriptionStatusActive,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(5 * 24 * time.Hour),
		DailyWindowStart: &dailyStart,
		DailyUsageUSD:    66.9359596,
		WeeklyUsageUSD:   481.56250122,
	}}
	svc := NewBillingCacheService(cache, nil, repo, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)
	group := &Group{ID: 26, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily}

	err := svc.checkSubscriptionEligibility(context.Background(), 110, group, nil)

	require.NoError(t, err)
	require.Equal(t, 1, cache.invalidateCalled)
	require.NotNil(t, cache.lastSetData)
	require.Equal(t, repo.sub.DailyUsageUSD, cache.lastSetData.DailyUsage)
	require.Equal(t, repo.sub.WeeklyUsageUSD, cache.lastSetData.WeeklyUsage)
}
