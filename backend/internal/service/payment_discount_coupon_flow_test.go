//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestCreateOrderInTxRecordsDiscountAndCouponUsage(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("discount-coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("discount-coupon-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-discount-coupon").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	repo := &memoryCouponRepo{
		byCode: map[string]*Coupon{
			"SAVE5": {
				ID:           1,
				Code:         "SAVE5",
				Type:         CouponTypeFixed,
				Value:        5,
				Scope:        CouponScopeAll,
				PerUserLimit: 1,
				Status:       CouponStatusActive,
			},
		},
	}

	svc := &PaymentService{
		entClient:     client,
		couponService: NewCouponService(repo, client),
	}

	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      user.ID,
			PaymentType: payment.TypeAlipay,
			OrderType:   payment.OrderTypeBalance,
			CouponCode:  "SAVE5",
			ClientIP:    "127.0.0.1",
			SrcHost:     "app.example.com",
		},
		&User{ID: user.ID, Email: user.Email, Username: user.Username},
		nil,
		&PaymentConfig{MaxPendingOrders: 3, OrderTimeoutMin: 30},
		100,
		85,
		10,
		5,
		2,
		86.70,
		&payment.InstanceSelection{
			InstanceID:     strconv.FormatInt(inst.ID, 10),
			ProviderKey:    payment.TypeAlipay,
			SupportedTypes: "alipay",
			PaymentMode:    "redirect",
			Config:         map[string]string{},
		},
	)
	require.NoError(t, err)
	require.InDelta(t, 100, order.Amount, 0.0001)
	require.InDelta(t, 86.70, order.PayAmount, 0.0001)
	require.InDelta(t, 10, order.DiscountAmount, 0.0001)
	require.Equal(t, "SAVE5", order.CouponCode)
	require.InDelta(t, 5, order.CouponDiscountAmount, 0.0001)
	require.Equal(t, 1, repo.byCode["SAVE5"].UsedCount)
	require.Len(t, repo.usages, 1)
	require.Equal(t, order.ID, repo.usages[0].OrderID)
	require.InDelta(t, 5, repo.usages[0].DiscountAmount, 0.0001)
}

func TestCreateOrderInTxWithoutDiscountsKeepsZeroFields(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("no-discount@example.com").
		SetPasswordHash("hash").
		SetUsername("no-discount-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-no-discount").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance},
		&User{ID: user.ID, Email: user.Email, Username: user.Username},
		nil,
		&PaymentConfig{MaxPendingOrders: 3, OrderTimeoutMin: 30},
		50,
		50,
		0,
		0,
		0,
		50,
		&payment.InstanceSelection{
			InstanceID:  strconv.FormatInt(inst.ID, 10),
			ProviderKey: payment.TypeAlipay,
			Config:      map[string]string{},
		},
	)
	require.NoError(t, err)
	require.Zero(t, order.DiscountAmount)
	require.Empty(t, order.CouponCode)
	require.Zero(t, order.CouponDiscountAmount)
	require.InDelta(t, 50, order.PayAmount, 0.0001)
}

func TestPreviewPriceAppliesThresholdAndCoupon(t *testing.T) {
	ctx := context.Background()
	repo := &memoryCouponRepo{
		byCode: map[string]*Coupon{
			"SAVE5": {
				ID:     1,
				Code:   "SAVE5",
				Type:   CouponTypeFixed,
				Value:  5,
				Scope:  CouponScopeAll,
				Status: CouponStatusActive,
			},
		},
	}
	svc := &PaymentService{
		configService: &PaymentConfigService{settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:      "true",
			SettingDiscountRules:       `[{"threshold":100,"type":"rate","value":0.9,"enabled":true}]`,
			SettingRechargeFeeRate:     "2",
			SettingEnabledPaymentTypes: payment.TypeAlipay,
		}}},
		discountService: NewDiscountService(),
		couponService:   NewCouponService(repo, nil),
	}

	got, err := svc.PreviewPrice(ctx, PreviewPriceRequest{
		UserID:      7,
		Amount:      100,
		OrderType:   payment.OrderTypeBalance,
		CouponCode:  "SAVE5",
		PaymentType: payment.TypeAlipay,
	})
	require.NoError(t, err)
	require.InDelta(t, 100, got.BaseAmount, 0.0001)
	require.InDelta(t, 10, got.ThresholdDiscount, 0.0001)
	require.InDelta(t, 5, got.CouponDiscount, 0.0001)
	require.InDelta(t, 85, got.AfterDiscount, 0.0001)
	require.InDelta(t, 1.70, got.Fee, 0.0001)
	require.InDelta(t, 86.70, got.PayAmount, 0.0001)
	require.NotNil(t, got.AppliedThresholdRule)
	require.NotNil(t, got.CouponInfo)
	require.Equal(t, "SAVE5", got.CouponInfo.Code)
}

func TestPreviewPriceCouponValidationErrors(t *testing.T) {
	ctx := context.Background()
	repo := &memoryCouponRepo{
		byCode: map[string]*Coupon{
			"MIN100": {
				ID:        1,
				Code:      "MIN100",
				Type:      CouponTypeFixed,
				Value:     5,
				MinAmount: 100,
				Scope:     CouponScopeAll,
				Status:    CouponStatusActive,
			},
		},
	}
	svc := &PaymentService{
		configService:   &PaymentConfigService{settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{}}},
		discountService: NewDiscountService(),
		couponService:   NewCouponService(repo, nil),
	}

	_, err := svc.PreviewPrice(ctx, PreviewPriceRequest{UserID: 7, Amount: 50, OrderType: payment.OrderTypeBalance, CouponCode: "NOPE"})
	require.Error(t, err)
	require.Equal(t, "COUPON_INVALID", infraerrors.Reason(err))

	_, err = svc.PreviewPrice(ctx, PreviewPriceRequest{UserID: 7, Amount: 50, OrderType: payment.OrderTypeBalance, CouponCode: "MIN100"})
	require.Error(t, err)
	require.Equal(t, "COUPON_MIN_AMOUNT_NOT_MET", infraerrors.Reason(err))
}

func TestPreviewPriceDailyLimitResetRejectsCouponAndSkipsDiscount(t *testing.T) {
	ctx := context.Background()
	svc := &PaymentService{
		configService: &PaymentConfigService{settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingDiscountRules: `[{"threshold":10,"type":"reduce","value":9,"enabled":true}]`,
		}}},
		discountService: NewDiscountService(),
	}

	got, err := svc.PreviewPrice(ctx, PreviewPriceRequest{UserID: 7, Amount: 20, OrderType: payment.OrderTypeDailyLimitReset})
	require.NoError(t, err)
	require.InDelta(t, 20, got.PayAmount, 0.0001)
	require.Zero(t, got.ThresholdDiscount)

	_, err = svc.PreviewPrice(ctx, PreviewPriceRequest{UserID: 7, Amount: 20, OrderType: payment.OrderTypeDailyLimitReset, CouponCode: "SAVE5"})
	require.Error(t, err)
	require.Equal(t, "COUPON_NOT_APPLICABLE", infraerrors.Reason(err))
}

func TestRefundUsesPayAmountAndCouponRefund(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-coupon-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-coupon").
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
		SetPayAmount(86.70).
		SetFeeRate(2).
		SetDiscountAmount(10).
		SetCouponCode("SAVE5").
		SetCouponDiscountAmount(5).
		SetRechargeCode("REFUND-COUPON").
		SetOutTradeNo("sub2_refund_coupon").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-coupon").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	repo := &memoryCouponRepo{
		byCode: map[string]*Coupon{
			"SAVE5": {ID: 1, Code: "SAVE5", Type: CouponTypeFixed, Value: 5, Scope: CouponScopeAll, UsedCount: 1, Status: CouponStatusActive},
		},
		usages: []CouponUsage{{ID: 1, CouponID: 1, UserID: user.ID, OrderID: order.ID, DiscountAmount: 5, UsedAt: time.Now(), Status: CouponUsageStatusUsed}},
	}
	svc := &PaymentService{
		entClient:     client,
		couponService: NewCouponService(repo, client),
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.InDelta(t, 86.70, plan.RefundAmount, 0.0001)
	require.InDelta(t, 86.70, plan.GatewayAmount, 0.0001)

	result, err = svc.markRefundOk(ctx, plan)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 0, repo.byCode["SAVE5"].UsedCount)
	require.Equal(t, CouponUsageStatusRefunded, repo.usages[0].Status)
}

func TestPrepareRefundWithoutCouponUsesPayAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-no-coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-no-coupon-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-no-coupon").
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
		SetPayAmount(90).
		SetFeeRate(0).
		SetDiscountAmount(10).
		SetRechargeCode("REFUND-NO-COUPON").
		SetOutTradeNo("sub2_refund_no_coupon").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-no-coupon").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.InDelta(t, 90, plan.RefundAmount, 0.0001)
	require.InDelta(t, 90, plan.GatewayAmount, 0.0001)
}
