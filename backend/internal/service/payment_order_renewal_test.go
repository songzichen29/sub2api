package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateSubOrderNewPurchasesAreIndependent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createPaymentRenewalTestUser(t, ctx, client)
	group := createPaymentRenewalTestGroup(t, ctx, client)
	plan := createPaymentRenewalTestPlan(t, ctx, client, group.ID)
	svc := &PaymentService{entClient: client, configService: &PaymentConfigService{entClient: client}, groupRepo: &subscriptionGroupRepoStub{group: &Group{ID: group.ID, Status: group.Status, SubscriptionType: group.SubscriptionType}}}
	for _, status := range []string{SubscriptionStatusExpired, SubscriptionStatusActive, SubscriptionStatusQuotaExhausted} {
		_, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(time.Hour)).SetStatus(status).SetSource(domain.SubscriptionSourcePayment).Save(ctx)
		require.NoError(t, err)
		for _, mode := range []string{SubscriptionRenewalModeExtend, SubscriptionRenewalModeRestart} {
			_, err := svc.validateSubOrder(ctx, CreateOrderRequest{UserID: user.ID, PlanID: plan.ID, RenewalMode: mode})
			require.Equal(t, "SUBSCRIPTION_RENEWAL_UNSUPPORTED", infraerrors.Reason(err))
		}
		got, err := svc.validateSubOrder(ctx, CreateOrderRequest{UserID: user.ID, PlanID: plan.ID})
		require.NoError(t, err)
		require.Equal(t, plan.ID, got.ID)
	}
}

func createPaymentRenewalTestUser(t *testing.T, ctx context.Context, client *dbent.Client) *dbent.User {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("payment-renewal@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-renewal-user").
		Save(ctx)
	require.NoError(t, err)
	return user
}

func createPaymentRenewalTestGroup(t *testing.T, ctx context.Context, client *dbent.Client) *dbent.Group {
	t.Helper()
	group, err := client.Group.Create().
		SetName("payment-renewal-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)
	return group
}

func createPaymentRenewalTestPlan(t *testing.T, ctx context.Context, client *dbent.Client, groupID int64) *dbent.SubscriptionPlan {
	t.Helper()
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(groupID).
		SetName("payment-renewal-plan").
		SetPrice(99).
		SetValidityDays(7).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)
	return plan
}
