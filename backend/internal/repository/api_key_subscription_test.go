package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryIndependentSubscriptionBinding(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "subscription-key@test.com")
	group, err := client.Group.Create().SetName("subscription-key-group").SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	first, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).
		SetStartsAt(now).SetExpiresAt(now.AddDate(0, 0, 30)).SetPurchaseOrderID(101).Save(ctx)
	require.NoError(t, err)
	second, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).
		SetStartsAt(now).SetExpiresAt(now.AddDate(0, 0, 30)).SetPurchaseOrderID(102).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).
		SetStartsAt(now).SetExpiresAt(now.AddDate(0, 0, 30)).SetPurchaseOrderID(101).Save(ctx)
	require.True(t, dbent.IsConstraintError(err), "同笔订单不能发放两份订阅")

	key := &service.APIKey{UserID: user.ID, GroupID: &group.ID, SubscriptionID: &first.ID,
		Key: "sk-independent-subscription", Name: "subscription", Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key))
	stored, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &first.ID, stored.SubscriptionID)
	auth, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, &first.ID, auth.SubscriptionID)

	key.SubscriptionID = &second.ID
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{SubscriptionID: true}))
	// 旧快照仅修改名称，不得覆盖并发切换后的绑定。
	stored.Name = "renamed"
	require.NoError(t, repo.Update(ctx, stored, service.APIKeyUpdateFields{Name: true}))
	stored, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &second.ID, stored.SubscriptionID)
	require.Equal(t, &group.ID, stored.GroupID)

	_, err = repo.ClearGroupIDByGroupID(ctx, group.ID)
	require.NoError(t, err)
	stored, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, stored.SubscriptionID)
	require.Nil(t, stored.GroupID)
}

func TestAPIKeyRepositoryMoveGroupRequiresUnambiguousSubscription(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "subscription-move@test.com")
	source, err := client.Group.Create().SetName("subscription-move-source").Save(ctx)
	require.NoError(t, err)
	target, err := client.Group.Create().SetName("subscription-move-target").SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	key := &service.APIKey{UserID: user.ID, GroupID: &source.ID,
		Key: "sk-subscription-move", Name: "move", Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key))
	now := time.Now()
	first, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(target.ID).
		SetStartsAt(now).SetExpiresAt(now.AddDate(0, 0, 30)).Save(ctx)
	require.NoError(t, err)
	second, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(target.ID).
		SetStartsAt(now).SetExpiresAt(now.AddDate(0, 0, 30)).SetPurchaseOrderID(201).Save(ctx)
	require.NoError(t, err)
	_, err = repo.UpdateGroupIDByUserAndGroup(ctx, user.ID, source.ID, target.ID)
	require.ErrorContains(t, err, "多份订阅")
	stored, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &source.ID, stored.GroupID)
	require.Nil(t, stored.SubscriptionID)

	_, err = client.UserSubscription.UpdateOneID(second.ID).SetStatus(service.SubscriptionStatusExpired).Save(ctx)
	require.NoError(t, err)
	count, err := repo.UpdateGroupIDByUserAndGroup(ctx, user.ID, source.ID, target.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	stored, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &target.ID, stored.GroupID)
	require.Equal(t, &first.ID, stored.SubscriptionID)

	_, err = repo.UpdateGroupIDByUserAndGroup(ctx, user.ID, target.ID, source.ID)
	require.NoError(t, err)
	stored, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &source.ID, stored.GroupID)
	require.Nil(t, stored.SubscriptionID)
}
