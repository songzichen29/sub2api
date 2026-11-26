package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedeemReductionUsesLockedSubscription(t *testing.T) {
	now := time.Now()
	stale := UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive, Notes: "old"}
	current := stale
	current.ExpiresAt = now.AddDate(0, 0, 20)
	current.Notes = "renewed"
	repo := &lockingRenewalRepo{stale: stale, current: current}
	svc := &RedeemService{subscriptionService: NewSubscriptionService(nil, repo, nil, nil, nil)}
	id, err := svc.reduceOrCancelSubscription(context.Background(), 11, 13, 1, "minus-one-day")
	require.NoError(t, err)
	require.Equal(t, current.ID, id)
	require.Equal(t, current.ExpiresAt.AddDate(0, 0, -1), repo.current.ExpiresAt)
	require.Contains(t, repo.current.Notes, "renewed")
	require.Equal(t, 1, repo.lockReads)
}

type failingReductionLockRepo struct {
	*lockingRenewalRepo
	err error
}

func (r *failingReductionLockRepo) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	return nil, r.err
}
func TestRedeemReductionLockFailureDoesNotWrite(t *testing.T) {
	sub := UserSubscription{ID: 7, ExpiresAt: time.Now().AddDate(0, 0, 10), Status: SubscriptionStatusActive, Notes: "unchanged"}
	repo := &failingReductionLockRepo{lockingRenewalRepo: &lockingRenewalRepo{stale: sub, current: sub}, err: errors.New("lock failed")}
	svc := &RedeemService{subscriptionService: NewSubscriptionService(nil, repo, nil, nil, nil)}
	_, err := svc.reduceOrCancelSubscription(context.Background(), 11, 13, 1, "deduct")
	require.ErrorIs(t, err, repo.err)
	require.Equal(t, sub, repo.current)
}
