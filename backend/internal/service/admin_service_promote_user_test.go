//go:build unit

package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminService_PromoteUserToAdmin_Success(t *testing.T) {
	repo := &userRepoStub{user: &User{
		ID:           7,
		Email:        "user@example.com",
		PasswordHash: "hash",
		Role:         RoleUser,
		Status:       StatusActive,
	}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
	}

	user, err := svc.PromoteUserToAdmin(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, RoleAdmin, user.Role)
	require.Len(t, repo.updated, 1)
	require.Equal(t, RoleAdmin, repo.updated[0].Role)
	require.Equal(t, []int64{7}, invalidator.userIDs)
}

func TestAdminService_PromoteUserToAdmin_IdempotentForAdmin(t *testing.T) {
	repo := &userRepoStub{user: &User{
		ID:     8,
		Email:  "admin@example.com",
		Role:   RoleAdmin,
		Status: StatusActive,
	}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
	}

	user, err := svc.PromoteUserToAdmin(context.Background(), 8)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, RoleAdmin, user.Role)
	require.Empty(t, repo.updated)
	require.Empty(t, invalidator.userIDs)
}

func TestAdminService_PromoteUserToAdmin_RejectsDisabledUser(t *testing.T) {
	repo := &userRepoStub{user: &User{
		ID:     9,
		Email:  "disabled@example.com",
		Role:   RoleUser,
		Status: StatusDisabled,
	}}
	svc := &adminServiceImpl{userRepo: repo}

	user, err := svc.PromoteUserToAdmin(context.Background(), 9)
	require.Nil(t, user)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Empty(t, repo.updated)
}
