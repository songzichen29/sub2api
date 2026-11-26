package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestIndependentRedeemLegacyRenewalAndReductionKeepTheirTarget(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createIndependentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, time.Now())
	group := client.Group.Create().SetName("redeem-legacy-target").SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	code := client.RedeemCode.Create().SetCode("redeem-before-legacy-renewal").SetType(RedeemTypeSubscription).SetGroupID(group.ID).SaveX(ctx)
	repo := &subscriptionEntRepo{client: client}
	subscriptions := NewSubscriptionService(&independentSubscriptionGroupRepo{group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription}}, repo, nil, client, nil)
	legacy := &UserSubscription{UserID: order.UserID, GroupID: group.ID, StartsAt: time.Now().AddDate(0, 0, -2), ExpiresAt: time.Now().AddDate(0, 0, 3), Status: SubscriptionStatusActive, Source: domain.SubscriptionSourcePayment, QuotaUsedUSD: 380}
	require.NoError(t, repo.Create(ctx, legacy))
	independent := &UserSubscription{UserID: order.UserID, GroupID: group.ID, RedeemCodeID: &code.ID, StartsAt: time.Now(), ExpiresAt: time.Now().AddDate(0, 0, 1), Status: SubscriptionStatusActive, Source: domain.SubscriptionSourceRedeem}
	require.NoError(t, repo.Create(ctx, independent))
	client.RedeemCode.DeleteOneID(code.ID).ExecX(ctx)
	// 升级前的支付订单仍续原合并份，删除兑换码不会使其独立份成为旧续期目标。
	payment := &PaymentService{entClient: client, subscriptionSvc: subscriptions}
	renewed, err := payment.ensurePaymentSubscriptionAssigned(ctx, order, &AssignSubscriptionInput{UserID: order.UserID, GroupID: group.ID, ValidityDays: 2, Source: domain.SubscriptionSourcePayment})
	require.NoError(t, err)
	require.Equal(t, legacy.ID, renewed.ID)
	require.WithinDuration(t, legacy.ExpiresAt.AddDate(0, 0, 2), renewed.ExpiresAt, time.Millisecond)
	// 负数码沿用同一 legacy 目标，保留剩余小时，不触碰独立兑换份。
	redeemer := &RedeemService{subscriptionService: subscriptions}
	id, err := redeemer.reduceOrCancelSubscription(ctx, order.UserID, group.ID, 1, "minus-one-day")
	require.NoError(t, err)
	require.Equal(t, legacy.ID, id)
	afterReduction, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.WithinDuration(t, legacy.ExpiresAt.AddDate(0, 0, 1), afterReduction.ExpiresAt, time.Millisecond)
	untouched, err := repo.GetByID(ctx, independent.ID)
	require.NoError(t, err)
	require.Equal(t, independent.ExpiresAt, untouched.ExpiresAt)
	require.WithinDuration(t, independent.StartsAt, untouched.StartsAt, time.Millisecond)
	require.Equal(t, &code.ID, untouched.RedeemCodeID)
}

func TestIndependentRedeemExcludedFromLegacyRefundAfterCodeDeletion(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createIndependentSubscriptionOrder(t, ctx, client, OrderStatusCompleted, time.Now())
	group := client.Group.Create().SetName("redeem-refund-target").SetPlatform(PlatformOpenAI).SaveX(ctx)
	order.SubscriptionGroupID = &group.ID
	code := client.RedeemCode.Create().SetCode("deleted-refund-source").SetType(RedeemTypeSubscription).SetGroupID(group.ID).SaveX(ctx)
	// 即使来源文本曾被改成 payment，独立标记仍必须阻止历史订单猜中该实例。
	sub := client.UserSubscription.Create().SetUserID(order.UserID).SetGroupID(group.ID).SetRedeemCodeID(code.ID).
		SetStartsAt(time.Now()).SetExpiresAt(time.Now().AddDate(0, 0, 1)).SetSource(domain.SubscriptionSourcePayment).SaveX(ctx)
	client.RedeemCode.DeleteOneID(code.ID).ExecX(ctx)
	require.Equal(t, &code.ID, client.UserSubscription.GetX(ctx, sub.ID).RedeemCodeID)
	payment := &PaymentService{entClient: client, subscriptionSvc: &SubscriptionService{userSubRepo: &subscriptionEntRepo{client: client}}}
	_, err := payment.resolveRefundSubscription(ctx, order)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}
