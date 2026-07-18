package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type redeemSubscriptionRepoStub struct {
	sub         *UserSubscription
	statusCalls int
	expiryCalls int
	notesCalls  int
	lastStatus  string
	lastExpiry  time.Time
	lastNotes   string
}

func (r *redeemSubscriptionRepoStub) clone() *UserSubscription {
	if r.sub == nil {
		return nil
	}
	cp := *r.sub
	return &cp
}

func (r *redeemSubscriptionRepoStub) Create(context.Context, *UserSubscription) error {
	panic("unexpected Create call")
}
func (r *redeemSubscriptionRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.clone(), nil
}
func (r *redeemSubscriptionRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	return r.clone(), nil
}
func (r *redeemSubscriptionRepoStub) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	return r.clone(), nil
}
func (r *redeemSubscriptionRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetActiveByUserIDAndGroupID call")
}
func (r *redeemSubscriptionRepoStub) Update(context.Context, *UserSubscription) error {
	panic("unexpected Update call")
}
func (r *redeemSubscriptionRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (r *redeemSubscriptionRepoStub) Restore(context.Context, int64, string) (*UserSubscription, error) {
	panic("unexpected Restore call")
}
func (r *redeemSubscriptionRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID call")
}
func (r *redeemSubscriptionRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}
func (r *redeemSubscriptionRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (r *redeemSubscriptionRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (r *redeemSubscriptionRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}
func (r *redeemSubscriptionRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsActiveByUserIDAndGroupID call")
}
func (r *redeemSubscriptionRepoStub) ExtendExpiry(_ context.Context, _ int64, newExpiresAt time.Time) error {
	r.expiryCalls++
	r.lastExpiry = newExpiresAt
	if r.sub != nil {
		r.sub.ExpiresAt = newExpiresAt
	}
	return nil
}
func (r *redeemSubscriptionRepoStub) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.statusCalls++
	r.lastStatus = status
	if r.sub != nil {
		r.sub.Status = status
	}
	return nil
}
func (r *redeemSubscriptionRepoStub) UpdateNotes(_ context.Context, _ int64, notes string) error {
	r.notesCalls++
	r.lastNotes = notes
	if r.sub != nil {
		r.sub.Notes = notes
	}
	return nil
}
func (r *redeemSubscriptionRepoStub) UpdateDailyOverdraft(context.Context, int64, bool) error {
	panic("unexpected UpdateDailyOverdraft call")
}
func (r *redeemSubscriptionRepoStub) ActivateWindows(context.Context, int64, time.Time, time.Time, time.Time) error {
	panic("unexpected ActivateWindows call")
}
func (r *redeemSubscriptionRepoStub) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	panic("unexpected ResetUsageWindows call")
}
func (r *redeemSubscriptionRepoStub) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}
func (r *redeemSubscriptionRepoStub) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}
func (r *redeemSubscriptionRepoStub) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}
func (r *redeemSubscriptionRepoStub) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}
func (r *redeemSubscriptionRepoStub) GetLatestUsedAtBySubscriptionIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (r *redeemSubscriptionRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

type redeemBillingCacheStub struct {
	invalidateCalls int
	publishCalls    int
}

func (c *redeemBillingCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	panic("unexpected GetUserBalance call")
}
func (c *redeemBillingCacheStub) SetUserBalance(context.Context, int64, float64) error {
	panic("unexpected SetUserBalance call")
}
func (c *redeemBillingCacheStub) DeductUserBalance(context.Context, int64, float64) error {
	panic("unexpected DeductUserBalance call")
}
func (c *redeemBillingCacheStub) InvalidateUserBalance(context.Context, int64) error {
	panic("unexpected InvalidateUserBalance call")
}
func (c *redeemBillingCacheStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	panic("unexpected GetSubscriptionCache call")
}
func (c *redeemBillingCacheStub) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	panic("unexpected SetSubscriptionCache call")
}
func (c *redeemBillingCacheStub) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	panic("unexpected UpdateSubscriptionUsage call")
}
func (c *redeemBillingCacheStub) InvalidateSubscriptionCache(_ context.Context, _, _ int64) error {
	c.invalidateCalls++
	return nil
}
func (c *redeemBillingCacheStub) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	panic("unexpected GetAPIKeyRateLimit call")
}
func (c *redeemBillingCacheStub) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	panic("unexpected SetAPIKeyRateLimit call")
}
func (c *redeemBillingCacheStub) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected UpdateAPIKeyRateLimitUsage call")
}
func (c *redeemBillingCacheStub) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	panic("unexpected InvalidateAPIKeyRateLimit call")
}
func (c *redeemBillingCacheStub) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	panic("unexpected GetUserPlatformQuotaCache call")
}
func (c *redeemBillingCacheStub) SetUserPlatformQuotaCache(context.Context, int64, string, *UserPlatformQuotaCacheEntry, time.Duration) error {
	panic("unexpected SetUserPlatformQuotaCache call")
}
func (c *redeemBillingCacheStub) DeleteUserPlatformQuotaCache(context.Context, int64, string) error {
	panic("unexpected DeleteUserPlatformQuotaCache call")
}
func (c *redeemBillingCacheStub) IncrUserPlatformQuotaUsageCache(context.Context, int64, string, float64, time.Duration, bool) error {
	panic("unexpected IncrUserPlatformQuotaUsageCache call")
}
func (c *redeemBillingCacheStub) PopDirtyUserPlatformQuotaKeys(context.Context, int) ([]UserPlatformQuotaKey, error) {
	panic("unexpected PopDirtyUserPlatformQuotaKeys call")
}
func (c *redeemBillingCacheStub) ReaddDirtyUserPlatformQuotaKeys(context.Context, []UserPlatformQuotaKey) error {
	panic("unexpected ReaddDirtyUserPlatformQuotaKeys call")
}
func (c *redeemBillingCacheStub) BatchGetUserPlatformQuotaCache(context.Context, []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	panic("unexpected BatchGetUserPlatformQuotaCache call")
}
func (c *redeemBillingCacheStub) PublishSubscriptionCacheInvalidation(_ context.Context, _ string) error {
	c.publishCalls++
	return nil
}
func (c *redeemBillingCacheStub) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	panic("unexpected SubscribeSubscriptionCacheInvalidation call")
}

func TestRedeemService_ReduceOrCancelSubscriptionInvalidatesSharedSubscriptionCaches(t *testing.T) {
	ctx := context.Background()
	sub := &UserSubscription{
		ID:        1,
		UserID:    11,
		GroupID:   22,
		StartsAt:  time.Now().Add(-10 * 24 * time.Hour),
		ExpiresAt: time.Now().Add(10 * 24 * time.Hour),
		Status:    SubscriptionStatusActive,
		Notes:     "old-note",
	}
	repo := &redeemSubscriptionRepoStub{sub: sub}
	cache := &redeemBillingCacheStub{}
	subSvc := &SubscriptionService{
		userSubRepo: repo,
		billingCacheService: &BillingCacheService{
			cache: cache,
		},
	}
	svc := &RedeemService{subscriptionService: subSvc}

	require.NoError(t, svc.reduceOrCancelSubscription(ctx, sub.UserID, sub.GroupID, 1, "CODE-123"))
	require.Equal(t, 1, repo.expiryCalls)
	require.Equal(t, 0, repo.statusCalls)
	require.Equal(t, 1, repo.notesCalls)
	require.Equal(t, 1, cache.invalidateCalls)
	require.Equal(t, 1, cache.publishCalls)
	require.Contains(t, repo.lastNotes, "CODE-123")
}
