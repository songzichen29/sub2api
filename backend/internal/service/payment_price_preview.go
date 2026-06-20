package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

func (s *PaymentService) PreviewPrice(ctx context.Context, req PreviewPriceRequest) (*PreviewPriceResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	var plan *dbent.SubscriptionPlan
	baseAmount := req.Amount
	switch req.OrderType {
	case payment.OrderTypeBalance:
		if baseAmount <= 0 {
			return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
		}
	case payment.OrderTypeSubscription:
		if req.PlanID == 0 {
			return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
		}
		plan, err = s.configService.GetPlan(ctx, req.PlanID)
		if err != nil || !plan.ForSale {
			return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
		}
		baseAmount = plan.Price
	case payment.OrderTypeDailyLimitReset:
		if strings.TrimSpace(req.CouponCode) != "" {
			return nil, ErrCouponNotApplicable
		}
		baseAmount = req.Amount
	default:
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "unsupported order type")
	}

	discountSvc := s.discountService
	if discountSvc == nil {
		discountSvc = NewDiscountService()
	}
	threshold := ThresholdDiscountResult{BaseAmount: roundMoney(baseAmount), AfterDiscount: roundMoney(baseAmount)}
	if req.OrderType != payment.OrderTypeDailyLimitReset {
		threshold = discountSvc.ApplyThresholdDiscount(baseAmount, cfg.DiscountRules)
	}
	afterDiscount := threshold.AfterDiscount
	var couponInfo *CouponInfo
	couponDiscount := 0.0
	if strings.TrimSpace(req.CouponCode) != "" {
		if s.couponService == nil {
			return nil, fmt.Errorf("coupon service not configured")
		}
		result, err := s.couponService.Validate(ctx, CouponValidationRequest{
			Code:      req.CouponCode,
			UserID:    req.UserID,
			Amount:    afterDiscount,
			OrderType: req.OrderType,
		})
		if err != nil {
			return nil, err
		}
		couponDiscount = result.DiscountAmount
		afterDiscount = roundMoney(afterDiscount - couponDiscount)
		if result.Coupon != nil {
			couponInfo = &CouponInfo{
				Code:           result.Coupon.Code,
				Type:           result.Coupon.Type,
				Value:          result.Coupon.Value,
				DiscountAmount: couponDiscount,
			}
		}
	}
	feeRate := cfg.RechargeFeeRate
	currency := payment.DefaultPaymentCurrency
	if req.PaymentType != "" && s.configService != nil {
		currency, err = s.configService.ValidateMethodCurrencyConsistency(ctx, req.PaymentType)
		if err != nil {
			return nil, err
		}
	}
	_, payAmount, err := calculateCreateOrderPayAmount(afterDiscount, feeRate, currency)
	if err != nil {
		return nil, err
	}
	fee := decimal.NewFromFloat(payAmount).Sub(decimal.NewFromFloat(afterDiscount))
	return &PreviewPriceResponse{
		BaseAmount:           roundMoney(baseAmount),
		ThresholdDiscount:    threshold.DiscountAmount,
		CouponDiscount:       couponDiscount,
		AfterDiscount:        roundMoney(afterDiscount),
		Fee:                  roundMoneyDecimal(fee),
		PayAmount:            payAmount,
		FeeRate:              feeRate,
		AppliedThresholdRule: threshold.AppliedRule,
		CouponInfo:           couponInfo,
	}, nil
}
