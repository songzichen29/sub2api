package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

func TestIndependentSubscriptionPurchaseKeepsOldRightsAndRetriesOneGrant(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createIndependentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, time.Now())
	group, err := client.Group.Create().SetName("independent").SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	repo := &subscriptionEntRepo{client: client}
	svc := &SubscriptionService{userSubRepo: repo, groupRepo: &independentSubscriptionGroupRepo{group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription, Status: StatusActive}}}
	ps := &PaymentService{entClient: client, subscriptionSvc: svc}
	quota := 2000.0
	old := &UserSubscription{UserID: order.UserID, GroupID: group.ID, StartsAt: time.Now().Add(-18 * 24 * time.Hour), ExpiresAt: time.Now().Add(12 * 24 * time.Hour), Status: SubscriptionStatusActive, Source: "payment", QuotaLimitUSD: &quota, QuotaUsedUSD: 1500, MonthlyUsageUSD: 1500}
	require.NoError(t, repo.Create(ctx, old))
	order.ProviderSnapshot = map[string]any{"subscription_assignment_mode": "independent"}
	input := &AssignSubscriptionInput{UserID: order.UserID, GroupID: group.ID, ValidityDays: 30, Source: "payment", QuotaLimitSpecified: true, QuotaLimitUSD: &quota}
	first, err := ps.ensurePaymentSubscriptionAssigned(ctx, order, input)
	require.NoError(t, err)
	require.NotEqual(t, old.ID, first.ID)
	require.Equal(t, &order.ID, first.PurchaseOrderID)
	require.Equal(t, 2000.0, *first.QuotaLimitUSD)
	require.Zero(t, first.QuotaUsedUSD)
	require.WithinDuration(t, time.Now().Add(30*24*time.Hour), first.ExpiresAt, 3*time.Second)
	retry, err := ps.ensurePaymentSubscriptionAssigned(ctx, order, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, retry.ID)
	require.Equal(t, 2, client.UserSubscription.Query().CountX(ctx))
	preserved, err := repo.GetByID(ctx, old.ID)
	require.NoError(t, err)
	require.WithinDuration(t, old.ExpiresAt, preserved.ExpiresAt, time.Millisecond)
	require.Equal(t, 1500.0, preserved.QuotaUsedUSD)
	require.Equal(t, 1500.0, preserved.MonthlyUsageUSD)
	// 升级前已下单的续期仍只操作原聚合记录，不能续到新购买的独立卡上。
	oldOrder := createIndependentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, time.Now())
	oldOrder = client.PaymentOrder.UpdateOneID(oldOrder.ID).SetUserID(order.UserID).SaveX(ctx)
	legacy, err := ps.ensurePaymentSubscriptionAssigned(ctx, oldOrder, &AssignSubscriptionInput{UserID: order.UserID, GroupID: group.ID, ValidityDays: 30, Source: "payment"})
	require.NoError(t, err)
	require.Equal(t, old.ID, legacy.ID)
	require.WithinDuration(t, old.ExpiresAt.Add(30*24*time.Hour), legacy.ExpiresAt, time.Millisecond)
	independent, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ExpiresAt, independent.ExpiresAt)
}

func TestIndependentSubscriptionSelectionIsolatesExpiryAndQuota(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := client.User.Create().SetEmail("selection@example.com").SetPasswordHash("hash").SaveX(ctx)
	group := client.Group.Create().SetName("selection").SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	repo := &subscriptionEntRepo{client: client}
	svc := &SubscriptionService{userSubRepo: repo}
	first := &UserSubscription{UserID: user.ID, GroupID: group.ID, StartsAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), Status: SubscriptionStatusActive}
	second := *first
	second.MonthlyUsageUSD = 1900
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, &second))
	a, err := svc.GetActiveSubscription(ctx, user.ID, group.ID, first.ID)
	require.NoError(t, err)
	b, err := svc.GetActiveSubscription(ctx, user.ID, group.ID, second.ID)
	require.NoError(t, err)
	require.Zero(t, a.MonthlyUsageUSD)
	require.Equal(t, 1900.0, b.MonthlyUsageUSD)
	require.NotEqual(t, subCacheKey(user.ID, group.ID, first.ID), subCacheKey(user.ID, group.ID, second.ID))
	_, err = svc.GetActiveSubscription(ctx, user.ID+1, group.ID, first.ID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	client.UserSubscription.UpdateOneID(first.ID).SetExpiresAt(time.Now().Add(-time.Minute)).ExecX(ctx)
	_, err = svc.GetActiveSubscription(ctx, user.ID, group.ID, first.ID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	_, err = svc.GetActiveSubscription(ctx, user.ID, group.ID, second.ID)
	require.NoError(t, err)
}

type independentSubscriptionGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *independentSubscriptionGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	return r.group, nil
}

func TestIndependentSubscriptionKeySelectionAndAuthSnapshot(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := client.User.Create().SetEmail("key-selection@example.com").SetPasswordHash("hash").SaveX(ctx)
	group := client.Group.Create().SetName("keys-selection").SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	repo := &subscriptionEntRepo{client: client}
	first := &UserSubscription{UserID: user.ID, GroupID: group.ID, StartsAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), Status: SubscriptionStatusActive}
	second := *first
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, &second))
	svc := &APIKeyService{userSubRepo: repo, groupRepo: &independentSubscriptionGroupRepo{group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription}}}
	owner := &User{ID: user.ID}
	_, err := svc.resolveKeySubscription(ctx, owner, &group.ID, nil)
	require.ErrorIs(t, err, ErrSubscriptionSelectionRequired)
	selected, err := svc.resolveKeySubscription(ctx, owner, &group.ID, &first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, *selected)
	selected, err = svc.resolveKeySubscription(ctx, owner, &group.ID, &second.ID)
	require.NoError(t, err)
	require.Equal(t, second.ID, *selected)
	_, err = svc.resolveKeySubscription(ctx, &User{ID: user.ID + 1}, &group.ID, &first.ID)
	require.ErrorIs(t, err, ErrGroupNotAllowed)
	key := &APIKey{ID: 8, UserID: user.ID, User: owner, GroupID: &group.ID, SubscriptionID: &second.ID, Status: StatusActive}
	snapshot := svc.snapshotFromAPIKey(ctx, key)
	restored, used, err := svc.applyAuthCacheEntry("key", &APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, &second.ID, restored.SubscriptionID)
}

type independentSubscriptionCache struct {
	billingCacheWorkerStub
	entries     map[int64]*SubscriptionCacheData
	invalidated []int64
	published   []string
	publishErr  error
}

func (c *independentSubscriptionCache) GetSubscriptionCache(_ context.Context, _ int64, id int64) (*SubscriptionCacheData, error) {
	return c.entries[id], nil
}
func (c *independentSubscriptionCache) UpdateSubscriptionUsage(_ context.Context, _ int64, id int64, cost float64) error {
	c.entries[id].MonthlyUsage += cost
	return nil
}

func TestIndependentSubscriptionBillingCacheUsesSelectedInstance(t *testing.T) {
	now := time.Now()
	limit := 2000.0
	cache := &independentSubscriptionCache{entries: map[int64]*SubscriptionCacheData{
		101: {Status: SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), MonthlyUsage: 2000},
		102: {Status: SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), MonthlyUsage: 20},
	}}
	svc := &BillingCacheService{cache: cache}
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeSubscription, MonthlyLimitUSD: &limit}
	err := svc.checkSubscriptionEligibility(context.Background(), 5, group, &UserSubscription{ID: 101})
	require.ErrorIs(t, err, ErrMonthlyLimitExceeded)
	require.NoError(t, svc.checkSubscriptionEligibility(context.Background(), 5, group, &UserSubscription{ID: 102}))
	require.NoError(t, svc.UpdateSubscriptionUsage(context.Background(), 5, 102, 50))
	require.Equal(t, 2000.0, cache.entries[101].MonthlyUsage)
	require.Equal(t, 70.0, cache.entries[102].MonthlyUsage)
}

func createIndependentSubscriptionOrder(t *testing.T, ctx context.Context, client *dbent.Client, status string, updatedAt time.Time) *dbent.PaymentOrder {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := client.User.Create().SetEmail(suffix + "@example.com").SetPasswordHash("hash").SaveX(ctx)
	return client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName("subscription-user").
		SetAmount(80).
		SetPayAmount(80).
		SetFeeRate(0).
		SetRechargeCode("PAY-" + suffix).
		SetOutTradeNo("order-" + suffix).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-" + suffix).
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(100).
		SetSubscriptionGroupID(7).
		SetSubscriptionDays(30).
		SetStatus(status).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetUpdatedAt(updatedAt).
		SaveX(ctx)
}

func (c *independentSubscriptionCache) InvalidateSubscriptionCache(_ context.Context, _ int64, id int64) error {
	c.invalidated = append(c.invalidated, id)
	return nil
}

func TestIndependentSubscriptionBulkInvalidatesOnlyChangedInstance(t *testing.T) {
	for _, action := range []string{"extend", "reset_quota", "revoke", "restore"} {
		t.Run(action, func(t *testing.T) {
			order := int64(77)
			sub := &UserSubscription{ID: 101, PurchaseOrderID: &order, UserID: 5, GroupID: 7, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}
			if action == "restore" {
				deleted := time.Now()
				sub.DeletedAt = &deleted
			}
			repo := &bulkActionSubscriptionRepo{subscriptions: map[int64]*UserSubscription{101: sub}}
			cache := &independentSubscriptionCache{}
			svc := NewSubscriptionService(nil, repo, &BillingCacheService{cache: cache}, nil, nil)
			defer svc.Stop()
			result, err := svc.BulkSubscriptionAction(context.Background(), &BulkSubscriptionActionInput{SubscriptionIDs: []int64{101}, Action: action, Days: 1, Monthly: true})
			require.NoError(t, err)
			require.Equal(t, 1, result.SuccessCount)
			require.NotEmpty(t, cache.invalidated)
			for _, id := range cache.invalidated {
				require.Equal(t, int64(101), id)
			}
		})
	}
}

func TestIndependentSubscriptionFirstBindInvalidatesLegacyInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := client.User.Create().SetEmail("bind-cache@example.com").SetPasswordHash("hash").SaveX(ctx)
	group := client.Group.Create().SetName("bind-cache").SetPlatform(PlatformOpenAI).SaveX(ctx)
	repo := &subscriptionEntRepo{client: client}
	sub := &UserSubscription{UserID: user.ID, GroupID: group.ID, Status: SubscriptionStatusActive, StartsAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, repo.Create(ctx, sub))
	cache := &independentSubscriptionCache{}
	subscriptions := &SubscriptionService{userSubRepo: repo, billingCacheService: &BillingCacheService{cache: cache}}
	auth := &AuthService{defaultSubAssigner: subscriptions}
	auth.invalidateAssignedSubscriptionCaches(user.ID, []int64{group.ID})
	require.Equal(t, []int64{sub.ID}, cache.invalidated)
}

func (c *independentSubscriptionCache) PublishSubscriptionCacheInvalidation(_ context.Context, key string) error {
	c.published = append(c.published, key)
	return c.publishErr
}
func (c *independentSubscriptionCache) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func TestIndependentSubscriptionInvalidatesBothCacheNamespaces(t *testing.T) {
	cache := &independentSubscriptionCache{}
	svc := &SubscriptionService{billingCacheService: &BillingCacheService{cache: cache}}
	require.NoError(t, svc.invalidateSubscriptionCaches(5, 7, 101))
	require.Equal(t, []int64{101}, cache.invalidated)
	require.Equal(t, []string{subCacheKey(5, 7), subCacheKey(5, 7, 101)}, cache.published)
}

func TestIndependentSubscriptionExcludedFromLegacyRefundMatching(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createIndependentSubscriptionOrder(t, ctx, client, OrderStatusCompleted, time.Now())
	group := client.Group.Create().SetName("legacy-refund").SetPlatform(PlatformOpenAI).SaveX(ctx)
	order.SubscriptionGroupID = &group.ID
	// 原历史订阅已不在，仅剩另一笔新订单的独立份。
	client.UserSubscription.Create().SetUserID(order.UserID).SetGroupID(group.ID).SetPurchaseOrderID(order.ID + 1).SetStartsAt(time.Now()).SetExpiresAt(time.Now().Add(time.Hour)).SetSource("payment").SaveX(ctx)
	svc := &PaymentService{entClient: client, subscriptionSvc: &SubscriptionService{userSubRepo: &subscriptionEntRepo{client: client}}}
	_, err := svc.resolveRefundSubscription(ctx, order)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestIndependentSubscriptionNotificationFailureStillClearsLocalCaches(t *testing.T) {
	l1, err := ristretto.NewCache(&ristretto.Config{NumCounters: 100, MaxCost: 10, BufferItems: 64})
	require.NoError(t, err)
	defer l1.Close()
	for _, key := range []string{subCacheKey(5, 7), subCacheKey(5, 7, 101)} {
		require.True(t, l1.Set(key, &UserSubscription{ID: 101}, 1))
	}
	l1.Wait()
	failure := errors.New("pubsub unavailable")
	cache := &independentSubscriptionCache{publishErr: failure}
	svc := &SubscriptionService{subCacheL1: l1, billingCacheService: &BillingCacheService{cache: cache}}
	require.ErrorIs(t, svc.invalidateSubscriptionCaches(5, 7, 101), failure)
	for _, key := range []string{subCacheKey(5, 7), subCacheKey(5, 7, 101)} {
		_, ok := l1.Get(key)
		require.False(t, ok)
	}
	require.Equal(t, []int64{101}, cache.invalidated)
	require.Len(t, cache.published, 2)
}
