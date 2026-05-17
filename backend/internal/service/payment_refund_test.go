//go:build unit

package service

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateRefundRequestRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ORDER").
		SetOutTradeNo("sub2_refund_legacy_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.Error(t, err)
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(188).
		SetPayAmount(188).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ADMIN-ORDER").
		SetOutTradeNo("sub2_refund_legacy_admin_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-admin-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsDailyLimitResetOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-daily-reset@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-daily-reset-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-daily-reset-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.9).
		SetPayAmount(9.9).
		SetFeeRate(0).
		SetRechargeCode("REFUND-DAILY-RESET-ORDER").
		SetOutTradeNo("sub2_refund_daily_reset_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-daily-reset").
		SetOrderType(payment.OrderTypeDailyLimitReset).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsExpiredSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-expired@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-expired-user").
		Save(ctx)
	require.NoError(t, err)

	groupEntity, err := client.Group.Create().
		SetName("refund-sub-expired-group").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-expired-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-EXPIRED").
		SetOutTradeNo("sub2_refund_sub_expired").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-expired").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-40 * 24 * time.Hour)
	expiresAt := time.Now().Add(-10 * 24 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusExpired).
		SetDailyUsageUsd(0).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_EXPIRED", infraerrors.Reason(err))
}

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SNAPSHOT-MISMATCH-ORDER").
		SetOutTradeNo("sub2_refund_snapshot_mismatch_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-snapshot-mismatch").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	err = svc.gwRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
	})
	require.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestPrepareRefundAutoCalculatesHistoricalSubscriptionRefund(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-historical@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-historical-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 10.0
	groupEntity, err := client.Group.Create().
		SetName("refund-sub-group").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-HIST").
		SetOutTradeNo("sub2_refund_sub_hist").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-hist").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-10*24*time.Hour - 2*time.Hour)
	expiresAt := startsAt.Add(30 * 24 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(4).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, payment.DeductionTypeSubscription, plan.DeductionType)
	require.Equal(t, 30, plan.SubDaysToDeduct)
	require.Positive(t, plan.SubscriptionID)
	require.InDelta(t, 196.00, plan.RefundAmount, 0.0001)
	require.InDelta(t, 196.00, plan.GatewayAmount, 0.0001)
}

func TestPrepareRefundRejectsSubscriptionAutoRefundWithoutDailyLimit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-no-daily@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-no-daily-user").
		Save(ctx)
	require.NoError(t, err)

	groupEntity, err := client.Group.Create().
		SetName("refund-sub-group-no-daily").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-no-daily-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-NO-DAILY").
		SetOutTradeNo("sub2_refund_sub_no_daily").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-no-daily").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-24 * time.Hour)
	expiresAt := startsAt.Add(30 * 24 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(1).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "DAILY_LIMIT_NOT_CONFIGURED", infraerrors.Reason(err))
}

func TestPrepareRefundHistoricalSubscriptionCalculationRoundsToTwoDecimals(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-round@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-round-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 3.0
	groupEntity, err := client.Group.Create().
		SetName("refund-sub-round-group").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-round-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-ROUND").
		SetOutTradeNo("sub2_refund_sub_round").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-round").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(3).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-26 * time.Hour)
	expiresAt := startsAt.Add(3 * 24 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(1).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	expected := math.Round((100.0*(1.0+(2.0/3.0))/3.0)*100) / 100
	require.InDelta(t, expected, plan.RefundAmount, 0.0001)
}

func TestPrepareRefundUsesAdjustedSubscriptionRangeForRefundAndDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-adjusted@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-adjusted-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 10.0
	groupEntity, err := client.Group.Create().
		SetName("refund-sub-adjusted-group").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-adjusted-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-ADJUSTED").
		SetOutTradeNo("sub2_refund_sub_adjusted").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-adjusted").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-5 * 24 * time.Hour)
	expiresAt := time.Now().Add(15 * 24 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(2).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		subscriptionSvc: NewSubscriptionService(nil, nil, nil, client, nil),
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, 15, plan.SubDaysToDeduct)
	require.InDelta(t, 200.00, plan.RefundAmount, 0.0001)
}

func TestPrepareRefundSkipsCurrentDayRefundAfterRenewalDailyWindowStarted(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-sub-renew-window@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-sub-renew-window-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	groupEntity, err := client.Group.Create().
		SetName("refund-sub-renew-window-group").
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-sub-renew-window-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	completedAt := time.Now().Add(-30 * time.Minute)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SUB-RENEW-WINDOW").
		SetOutTradeNo("sub2_refund_sub_renew_window").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-sub-renew-window").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupEntity.ID).
		SetSubscriptionDays(1).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(completedAt).
		SetCompletedAt(completedAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-12 * time.Hour)
	windowStart := completedAt.Add(2 * time.Second)
	expiresAt := time.Now().Add(36 * time.Hour)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(groupEntity.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyWindowStart(windowStart).
		SetDailyUsageUsd(0).
		SetNotes("payment order " + strconv.FormatInt(order.ID, 10)).
		SetSource("payment").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	preview, err := svc.PreviewRefund(ctx, order.ID, 0)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.True(t, preview.CalculatedAutomatically)
	require.InDelta(t, 0.0, preview.RefundAmount, 0.0001)

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "NO_REFUNDABLE_AMOUNT", infraerrors.Reason(err))
}

func TestCapDailyOverdraftSubscriptionRefundUsesPeriodRemaining(t *testing.T) {
	daily := 80.0
	weekly := 560.0
	monthly := 2400.0

	strict := &UserSubscription{
		WeeklyUsageUSD: 500,
		Group: &Group{
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &daily,
			WeeklyLimitUSD:   &weekly,
		},
	}
	require.InDelta(t, 80.0, capDailyOverdraftSubscriptionRefund(80, 100, strict), 0.0001)

	overdraftWeekly := &UserSubscription{
		WeeklyUsageUSD:      500,
		AllowDailyOverdraft: true,
		Group: &Group{
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &daily,
			WeeklyLimitUSD:      &weekly,
			AllowDailyOverdraft: true,
		},
	}
	// weekly remaining ratio=(560-500)/560=10.714%, so a 100 order can refund at most 10.71
	require.InDelta(t, 10.714285, capDailyOverdraftSubscriptionRefund(80, 100, overdraftWeekly), 0.0001)

	overdraftMonthly := &UserSubscription{
		MonthlyUsageUSD:     1200,
		AllowDailyOverdraft: true,
		Group: &Group{
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &daily,
			MonthlyLimitUSD:     &monthly,
			AllowDailyOverdraft: true,
		},
	}
	require.InDelta(t, 50.0, capDailyOverdraftSubscriptionRefund(80, 100, overdraftMonthly), 0.0001)
}
