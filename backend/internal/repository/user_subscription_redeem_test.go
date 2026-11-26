package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionRepositoryIndependentRedeemExcludesLegacyLookups(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "redeem-subscription@test.com")
	group, err := client.Group.Create().SetName("redeem-subscription-group").SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client)
	now := time.Now().Truncate(time.Second)
	firstCodeID, secondCodeID, orderID := int64(101), int64(102), int64(103)
	for _, provenance := range []struct {
		redeemCodeID *int64
		orderID      *int64
	}{
		{redeemCodeID: &firstCodeID},
		{redeemCodeID: &secondCodeID},
		{orderID: &orderID},
	} {
		sub := &service.UserSubscription{
			UserID: user.ID, GroupID: group.ID, RedeemCodeID: provenance.redeemCodeID, PurchaseOrderID: provenance.orderID,
			StartsAt: now, ExpiresAt: now.AddDate(0, 0, 30), Status: service.SubscriptionStatusActive,
		}
		require.NoError(t, repo.Create(ctx, sub))
		require.Equal(t, provenance.redeemCodeID, sub.RedeemCodeID)
		stored, err := repo.GetByID(ctx, sub.ID)
		require.NoError(t, err)
		require.Equal(t, provenance.redeemCodeID, stored.RedeemCodeID)
		require.Equal(t, provenance.orderID, stored.PurchaseOrderID)
	}

	_, err = repo.GetByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	exists, err := repo.ExistsByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.False(t, exists)
	exists, err = repo.ExistsActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.False(t, exists)

	quota := 2000.0
	legacy := &service.UserSubscription{
		UserID: user.ID, GroupID: group.ID, StartsAt: now.AddDate(0, 0, -10), ExpiresAt: now.AddDate(0, 0, 20),
		Status: service.SubscriptionStatusActive, QuotaLimitUSD: &quota, QuotaUsedUSD: 1250,
	}
	require.NoError(t, repo.Create(ctx, legacy))
	stored, err := repo.GetByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.Equal(t, legacy.ID, stored.ID)
	require.Equal(t, legacy.ExpiresAt, stored.ExpiresAt)
	require.Equal(t, &quota, stored.QuotaLimitUSD)
	require.Equal(t, 1250.0, stored.QuotaUsedUSD)
	exists, err = repo.ExistsByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = repo.ExistsActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.True(t, exists)
	all, err := repo.ListActiveByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, all, 4)
}

func TestUserSubscriptionRepositoryRedeemProvenanceSurvivesCodeDeletion(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "redeem-delete@test.com")
	group, err := client.Group.Create().SetName("redeem-delete-group").SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	code, err := client.RedeemCode.Create().SetCode("independent-delete-test").SetType(service.RedeemTypeSubscription).SetGroupID(group.ID).Save(ctx)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client)
	now := time.Now().Truncate(time.Second)
	sub := &service.UserSubscription{
		UserID: user.ID, GroupID: group.ID, RedeemCodeID: &code.ID,
		StartsAt: now, ExpiresAt: now.AddDate(0, 0, 30), Status: service.SubscriptionStatusActive,
	}
	require.NoError(t, repo.Create(ctx, sub))
	require.NoError(t, client.RedeemCode.DeleteOneID(code.ID).Exec(ctx))
	stored, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, &code.ID, stored.RedeemCodeID)
	_, err = repo.GetByUserIDAndGroupID(ctx, user.ID, group.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)

	duplicate := *sub
	duplicate.ID = 0
	require.ErrorIs(t, repo.Create(ctx, &duplicate), service.ErrSubscriptionAlreadyExists)
	require.NoError(t, repo.Delete(ctx, sub.ID))
	stored, err = repo.GetByIDIncludeDeleted(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, &code.ID, stored.RedeemCodeID)
	require.ErrorIs(t, repo.Create(ctx, &duplicate), service.ErrSubscriptionAlreadyExists)
}
