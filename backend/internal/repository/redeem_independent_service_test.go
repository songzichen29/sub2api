package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type independentRedeemCache struct {
	service.BillingCache
	client      *dbent.Client
	mu          sync.Mutex
	invalidated []int64
	published   []string
}

func (c *independentRedeemCache) InvalidateSubscriptionCache(ctx context.Context, userID, subscriptionID int64) error {
	// 从原 client 读取，确保事务已经提交，而不是通过事务连接看见未提交数据。
	sub, err := c.client.UserSubscription.Get(mixins.SkipSoftDelete(ctx), subscriptionID)
	if err != nil {
		return err
	}
	if sub.UserID != userID {
		return errors.New("invalidated another user's subscription")
	}
	if sub.RedeemCodeID != nil {
		unused, err := c.client.RedeemCode.Query().Where(redeemcode.IDEQ(*sub.RedeemCodeID), redeemcode.StatusEQ(service.StatusUnused)).Exist(ctx)
		if err != nil {
			return err
		}
		if unused {
			return errors.New("invalidated before redemption committed")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, subscriptionID)
	return nil
}

func (c *independentRedeemCache) PublishSubscriptionCacheInvalidation(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, key)
	return nil
}

func (c *independentRedeemCache) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	panic("unexpected subscription cache subscriber")
}

type independentRedeemFixture struct {
	client        *dbent.Client
	user          *service.User
	group         *dbent.Group
	codeRepo      service.RedeemCodeRepository
	subRepo       service.UserSubscriptionRepository
	subscriptions *service.SubscriptionService
	redeemer      *service.RedeemService
	keys          *service.APIKeyService
	cache         *independentRedeemCache
}

func newIndependentRedeemFixture(t *testing.T) *independentRedeemFixture {
	t.Helper()
	keyRepo, client := newAPIKeyRepoSQLite(t)
	db, ok := keyRepo.sql.(*sql.DB)
	require.True(t, ok)
	db.SetMaxOpenConns(1)
	_, err := client.ExecContext(context.Background(), "CREATE TABLE qqc_code_claims (redeem_code_id INTEGER NOT NULL, user_id INTEGER NOT NULL)")
	require.NoError(t, err)
	user := mustCreateAPIKeyRepoUser(t, context.Background(), client, "independent-redeem@test.com")
	group := client.Group.Create().SetName("day card").SetSubscriptionType(service.SubscriptionTypeSubscription).SetDailyLimitUsd(2000).SaveX(context.Background())
	userRepo := NewUserRepository(client, db)
	groupRepo := NewGroupRepository(client, db)
	subRepo := NewUserSubscriptionRepository(client)
	codeRepo := NewRedeemCodeRepository(client)
	cache := &independentRedeemCache{client: client}
	billing := service.NewBillingCacheService(cache, userRepo, subRepo, keyRepo, nil, nil, &config.Config{})
	t.Cleanup(billing.Stop)
	subscriptions := service.NewSubscriptionService(groupRepo, subRepo, billing, client, nil)
	t.Cleanup(subscriptions.Stop)
	return &independentRedeemFixture{
		client: client, user: user, group: group, codeRepo: codeRepo, subRepo: subRepo,
		subscriptions: subscriptions, cache: cache,
		redeemer: service.NewRedeemService(codeRepo, userRepo, subscriptions, nil, billing, client, nil),
		keys:     service.NewAPIKeyService(keyRepo, userRepo, groupRepo, subRepo, nil, nil, &config.Config{}),
	}
}

func (f *independentRedeemFixture) code(t *testing.T, name string, days int) *service.RedeemCode {
	t.Helper()
	code := &service.RedeemCode{Code: name, Type: service.RedeemTypeSubscription, Status: service.StatusUnused, GroupID: &f.group.ID, ValidityDays: days}
	require.NoError(t, f.codeRepo.Create(context.Background(), code))
	return code
}

func TestIndependentRedeemThreeDayCardsKeepLegacyAndBindSeparately(t *testing.T) {
	f := newIndependentRedeemFixture(t)
	ctx := context.Background()
	old := f.client.UserSubscription.Create().SetUserID(f.user.ID).SetGroupID(f.group.ID).
		SetStartsAt(time.Now().AddDate(0, 0, -3)).SetExpiresAt(time.Now().AddDate(0, 0, 3)).
		SetQuotaLimitUsd(2000).SetQuotaUsedUsd(380).SetDailyUsageUsd(380).SaveX(ctx)
	var ids []int64
	for i := 1; i <= 3; i++ {
		code := f.code(t, fmt.Sprintf("one-day-%d", i), 1)
		used, err := f.redeemer.Redeem(ctx, f.user.ID, code.Code)
		require.NoError(t, err)
		require.Equal(t, service.StatusUsed, used.Status)
		sub := f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(code.ID)).OnlyX(ctx)
		require.NotContains(t, ids, sub.ID)
		ids = append(ids, sub.ID)
		require.Nil(t, sub.PurchaseOrderID)
		require.Equal(t, domain.SubscriptionSourceRedeem, sub.Source)
		require.WithinDuration(t, time.Now().AddDate(0, 0, 1), sub.ExpiresAt, 3*time.Second)
		require.WithinDuration(t, sub.StartsAt.AddDate(0, 0, 1), sub.ExpiresAt, time.Millisecond)
		require.Zero(t, sub.DailyUsageUsd)
		key, err := f.keys.Create(ctx, f.user.ID, service.CreateAPIKeyRequest{Name: code.Code, GroupID: &f.group.ID, SubscriptionID: &sub.ID})
		require.NoError(t, err)
		require.Equal(t, &sub.ID, key.SubscriptionID)
		bound, err := f.subscriptions.GetActiveSubscription(ctx, f.user.ID, f.group.ID, *key.SubscriptionID)
		require.NoError(t, err)
		require.Equal(t, sub.ID, bound.ID)
		_, err = f.redeemer.Redeem(ctx, f.user.ID, code.Code)
		require.ErrorIs(t, err, service.ErrRedeemCodeUsed)
	}
	active, err := f.subscriptions.ListActiveUserSubscriptions(ctx, f.user.ID)
	require.NoError(t, err)
	var redeemedIDs []int64
	for _, sub := range active {
		if sub.RedeemCodeID != nil {
			redeemedIDs = append(redeemedIDs, sub.ID)
		}
	}
	require.ElementsMatch(t, ids, redeemedIDs)
	require.Equal(t, 4, f.client.UserSubscription.Query().CountX(ctx))
	preserved := f.client.UserSubscription.GetX(ctx, old.ID)
	require.Equal(t, old.ExpiresAt, preserved.ExpiresAt)
	require.Equal(t, old.StartsAt, preserved.StartsAt)
	require.Equal(t, old.QuotaLimitUsd, preserved.QuotaLimitUsd)
	require.Equal(t, old.QuotaUsedUsd, preserved.QuotaUsedUsd)
	require.Equal(t, old.DailyUsageUsd, preserved.DailyUsageUsd)
	require.Equal(t, ids, f.cache.invalidated)
	for _, id := range ids {
		require.Contains(t, f.cache.published, fmt.Sprintf("sub:instance:%d:%d", f.user.ID, id))
	}
	require.Contains(t, f.cache.published, fmt.Sprintf("sub:%d:%d", f.user.ID, f.group.ID))
}

func TestIndependentRedeemZeroDaysAndRollback(t *testing.T) {
	f := newIndependentRedeemFixture(t)
	ctx := context.Background()
	code := f.code(t, "default-thirty-days", 0)
	_, err := f.redeemer.Redeem(ctx, f.user.ID, code.Code)
	require.NoError(t, err)
	sub := f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(code.ID)).OnlyX(ctx)
	require.WithinDuration(t, sub.StartsAt.AddDate(0, 0, 30), sub.ExpiresAt, time.Millisecond)
	failed := f.code(t, "failed-create", 1)
	_, err = f.client.ExecContext(ctx, "CREATE TRIGGER fail_subscription_insert BEFORE INSERT ON user_subscriptions BEGIN SELECT RAISE(ABORT, 'test subscription write failure'); END")
	require.NoError(t, err)
	_, err = f.redeemer.Redeem(ctx, f.user.ID, failed.Code)
	require.ErrorContains(t, err, "test subscription write failure")
	stored := f.client.RedeemCode.GetX(ctx, failed.ID)
	require.Equal(t, service.StatusUnused, stored.Status)
	require.Nil(t, stored.UsedBy)
	require.False(t, f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(failed.ID)).ExistX(ctx))
	require.Equal(t, []int64{sub.ID}, f.cache.invalidated)
	_, err = f.client.ExecContext(ctx, "DROP TRIGGER fail_subscription_insert")
	require.NoError(t, err)
	_, err = f.redeemer.Redeem(ctx, f.user.ID, failed.Code)
	require.NoError(t, err)
	require.Equal(t, 1, f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(failed.ID)).CountX(ctx))
}

type concurrentRedeemReads struct {
	service.RedeemCodeRepository
	ready *sync.WaitGroup
	start <-chan struct{}
}

func (r *concurrentRedeemReads) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	result, err := r.RedeemCodeRepository.GetByCode(ctx, code)
	r.ready.Done()
	<-r.start
	return result, err
}

func TestIndependentRedeemConcurrentSameCodeUsesDatabaseCAS(t *testing.T) {
	f := newIndependentRedeemFixture(t)
	ctx := context.Background()
	code := f.code(t, "same-code-concurrently", 1)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	reader := &concurrentRedeemReads{RedeemCodeRepository: f.codeRepo, ready: &ready, start: start}
	svc := service.NewRedeemService(reader, NewUserRepository(f.client, nil), f.subscriptions, nil, nil, f.client, nil)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := svc.Redeem(ctx, f.user.ID, code.Code); results <- err }()
	}
	ready.Wait()
	close(start)
	var succeeded, alreadyUsed int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, service.ErrRedeemCodeUsed)
		alreadyUsed++
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, alreadyUsed)
	require.Equal(t, 1, f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(code.ID)).CountX(ctx))
}

func TestIndependentRedeemDeletedCodeRemainsIndependentAndNegativeHasNoTarget(t *testing.T) {
	f := newIndependentRedeemFixture(t)
	ctx := context.Background()
	code := f.code(t, "delete-used-code", 1)
	_, err := f.redeemer.Redeem(ctx, f.user.ID, code.Code)
	require.NoError(t, err)
	sub := f.client.UserSubscription.Query().Where(usersubscription.RedeemCodeIDEQ(code.ID)).OnlyX(ctx)
	require.NoError(t, f.codeRepo.Delete(ctx, code.ID))
	stored, err := f.subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, &code.ID, stored.RedeemCodeID)
	available, err := f.keys.GetAvailableGroups(ctx, f.user.ID)
	require.NoError(t, err)
	require.Len(t, available, 1)
	require.Equal(t, f.group.ID, available[0].ID)
	_, err = f.subRepo.GetByUserIDAndGroupID(ctx, f.user.ID, f.group.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	negative := f.code(t, "negative-needs-legacy", -1)
	_, err = f.redeemer.Redeem(ctx, f.user.ID, negative.Code)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	require.Equal(t, service.StatusUnused, f.client.RedeemCode.GetX(ctx, negative.ID).Status)
	require.Equal(t, sub.ExpiresAt, f.client.UserSubscription.GetX(ctx, sub.ID).ExpiresAt)
	// 管理员旧分配入口新建其自己的 legacy 记录，不复用兑换独立卡。
	legacy, err := f.subscriptions.AssignSubscription(ctx, &service.AssignSubscriptionInput{UserID: f.user.ID, GroupID: f.group.ID, ValidityDays: 3})
	require.NoError(t, err)
	require.NotEqual(t, sub.ID, legacy.ID)
	require.Nil(t, legacy.RedeemCodeID)
	require.NoError(t, f.subscriptions.RevokeSubscription(ctx, sub.ID))
	restored, err := f.subscriptions.RestoreSubscription(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, &code.ID, restored.RedeemCodeID)
	require.Equal(t, 1, f.client.RedeemCode.Query().Where(redeemcode.IDEQ(negative.ID)).CountX(ctx))
}
