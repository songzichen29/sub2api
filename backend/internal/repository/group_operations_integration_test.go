//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Run against the isolated MySQL harness: migrations, Ent write/read and the
// explicit authentication projection must agree on both new group fields.
func TestGroupOperationsPersistenceAndAuthProjection(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newGroupRepositoryWithSQL(client, tx)
	target := &service.Group{Name: "operations-target", Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 1}
	require.NoError(t, repo.Create(ctx, target))
	source := &service.Group{
		Name: "operations-source", Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 0.5,
		UserConcurrencyLimit: 2, FallbackGroupIDOnUnavailable: &target.ID,
	}
	require.NoError(t, repo.Create(ctx, source))
	read, err := repo.GetByIDLite(ctx, source.ID)
	require.NoError(t, err)
	require.Equal(t, 2, read.UserConcurrencyLimit)
	require.Equal(t, target.ID, *read.FallbackGroupIDOnUnavailable)

	user := mustCreateUser(t, client, &service.User{Email: "group-operations@example.com", Concurrency: 5})
	keyRepo := newAPIKeyRepositoryWithSQL(client, tx)
	key := &service.APIKey{Key: "sk-group-operations-integration", Name: "operations", UserID: user.ID, GroupID: &source.ID, Status: service.StatusActive}
	require.NoError(t, keyRepo.Create(ctx, key))
	auth, err := keyRepo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, auth.Group)
	require.Equal(t, 2, auth.Group.UserConcurrencyLimit)
	require.Equal(t, target.ID, *auth.Group.FallbackGroupIDOnUnavailable)

	source.UserConcurrencyLimit = 0
	source.FallbackGroupIDOnUnavailable = nil
	require.NoError(t, repo.Update(ctx, source))
	read, err = repo.GetByIDLite(ctx, source.ID)
	require.NoError(t, err)
	require.Zero(t, read.UserConcurrencyLimit)
	require.Nil(t, read.FallbackGroupIDOnUnavailable)
}
