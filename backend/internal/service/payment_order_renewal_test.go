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

func TestValidateSubOrderRestartRequiresActiveSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}

	user := createPaymentRenewalTestUser(t, ctx, client)
	group := createPaymentRenewalTestGroup(t, ctx, client)
	plan := createPaymentRenewalTestPlan(t, ctx, client, group.ID)

	subscriptionSvc := NewSubscriptionService(
		&subscriptionGroupRepoStub{
			group: &Group{
				ID:               group.ID,
				Status:           group.Status,
				SubscriptionType: group.SubscriptionType,
			},
		},
		&subscriptionEntRepo{client: client},
		nil,
		client,
		nil,
	)
	svc := &PaymentService{
		entClient:       client,
		configService:   configSvc,
		groupRepo:       &subscriptionGroupRepoStub{group: &Group{ID: group.ID, Status: group.Status, SubscriptionType: group.SubscriptionType}},
		subscriptionSvc: subscriptionSvc,
	}

	_, err := svc.validateSubOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PlanID:      plan.ID,
		RenewalMode: SubscriptionRenewalModeRestart,
	})
	require.Error(t, err)
	require.Equal(t, "RENEWAL_ACTIVE_SUBSCRIPTION_REQUIRED", infraerrors.Reason(err))

	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(time.Now().Add(-2 * time.Hour)).
		SetExpiresAt(time.Now().Add(-time.Hour)).
		SetStatus(SubscriptionStatusExpired).
		SetSource(domain.SubscriptionSourcePayment).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.validateSubOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PlanID:      plan.ID,
		RenewalMode: SubscriptionRenewalModeRestart,
	})
	require.Error(t, err)
	require.Equal(t, "RENEWAL_ACTIVE_SUBSCRIPTION_REQUIRED", infraerrors.Reason(err))

	_, err = client.UserSubscription.Update().
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	got, err := svc.validateSubOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PlanID:      plan.ID,
		RenewalMode: SubscriptionRenewalModeRestart,
	})
	require.NoError(t, err)
	require.Equal(t, plan.ID, got.ID)
}

func TestValidateSubOrderExplicitExtendRequiresActiveSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user := createPaymentRenewalTestUser(t, ctx, client)
	group := createPaymentRenewalTestGroup(t, ctx, client)
	plan := createPaymentRenewalTestPlan(t, ctx, client, group.ID)

	svc := &PaymentService{
		entClient:     client,
		configService: &PaymentConfigService{entClient: client},
		groupRepo:     &subscriptionGroupRepoStub{group: &Group{ID: group.ID, Status: group.Status, SubscriptionType: group.SubscriptionType}},
	}

	_, err := svc.validateSubOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PlanID:      plan.ID,
		RenewalMode: SubscriptionRenewalModeExtend,
	})
	require.Error(t, err)
	require.Equal(t, "RENEWAL_ACTIVE_SUBSCRIPTION_REQUIRED", infraerrors.Reason(err))

	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(SubscriptionStatusQuotaExhausted).
		SetSource(domain.SubscriptionSourcePayment).
		Save(ctx)
	require.NoError(t, err)

	got, err := svc.validateSubOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PlanID:      plan.ID,
		RenewalMode: SubscriptionRenewalModeExtend,
	})
	require.NoError(t, err)
	require.Equal(t, plan.ID, got.ID)
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
