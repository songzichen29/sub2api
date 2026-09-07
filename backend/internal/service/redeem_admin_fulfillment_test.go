//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminFulfillmentBypassesLimitAndDoesNotApplyRedeemAffiliate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userID := int64(42)
	code := &RedeemCode{
		ID: 102, Code: "ADMIN-SUCCESS", Type: RedeemTypeBalance, Value: 20, Status: StatusUnused,
	}
	redeemRepo := &paymentFulfillmentRedeemRepo{
		paymentOrderLifecycleRedeemRepo: paymentOrderLifecycleRedeemRepo{
			codesByCode: map[string]*RedeemCode{code.Code: code},
		},
	}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: userID}}
	userRepo.updateBalanceFn = func(context.Context, int64, float64) error { return nil }
	cache := &paymentFulfillmentRedeemCacheStub{count: redeemMaxFailedAttempts}
	svc := NewRedeemService(redeemRepo, userRepo, nil, cache, nil, client, nil)

	result, err := svc.RedeemForAdminFulfillment(ctx, userID, code.Code)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, cache.getCalls)
	require.Zero(t, cache.incrementCalls)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
	require.Len(t, redeemRepo.useCalls, 1)
}
