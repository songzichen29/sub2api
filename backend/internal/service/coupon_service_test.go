package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type memoryCouponRepo struct {
	coupon *Coupon
	byCode map[string]*Coupon
	usages []CouponUsage
}

func (r *memoryCouponRepo) Create(context.Context, *Coupon) error           { return nil }
func (r *memoryCouponRepo) GetByID(context.Context, int64) (*Coupon, error) { return r.coupon, nil }
func (r *memoryCouponRepo) GetByCode(_ context.Context, code string) (*Coupon, error) {
	if r.byCode != nil {
		coupon, ok := r.byCode[code]
		if !ok || coupon == nil {
			return nil, ErrCouponInvalid
		}
		return coupon, nil
	}
	if r.coupon == nil {
		return nil, ErrCouponInvalid
	}
	return r.coupon, nil
}
func (r *memoryCouponRepo) GetByCodeForUpdate(ctx context.Context, code string) (*Coupon, error) {
	return r.GetByCode(ctx, code)
}
func (r *memoryCouponRepo) Update(context.Context, *Coupon) error { return nil }
func (r *memoryCouponRepo) Delete(context.Context, int64) error   { return nil }
func (r *memoryCouponRepo) List(context.Context, pagination.PaginationParams, string, string) ([]Coupon, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *memoryCouponRepo) CreateUsage(_ context.Context, usage *CouponUsage) error {
	usage.ID = int64(len(r.usages) + 1)
	r.usages = append(r.usages, *usage)
	return nil
}
func (r *memoryCouponRepo) GetUsageByCouponAndUser(_ context.Context, couponID, userID int64) ([]CouponUsage, error) {
	var out []CouponUsage
	for _, usage := range r.usages {
		if usage.CouponID == couponID && usage.UserID == userID {
			out = append(out, usage)
		}
	}
	return out, nil
}
func (r *memoryCouponRepo) GetUsageByOrder(_ context.Context, orderID int64) (*CouponUsage, error) {
	for i := range r.usages {
		if r.usages[i].OrderID == orderID {
			return &r.usages[i], nil
		}
	}
	return nil, nil
}
func (r *memoryCouponRepo) ListUsagesByCoupon(context.Context, int64, pagination.PaginationParams) ([]CouponUsage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *memoryCouponRepo) IncrementUsedCount(_ context.Context, id int64) error {
	if r.coupon == nil && r.byCode != nil {
		for _, coupon := range r.byCode {
			if coupon != nil && coupon.ID == id {
				coupon.UsedCount++
				return nil
			}
		}
	}
	r.coupon.UsedCount++
	return nil
}
func (r *memoryCouponRepo) DecrementUsedCount(_ context.Context, id int64) error {
	if r.coupon == nil && r.byCode != nil {
		for _, coupon := range r.byCode {
			if coupon != nil && coupon.ID == id {
				coupon.UsedCount--
				return nil
			}
		}
	}
	r.coupon.UsedCount--
	return nil
}
func (r *memoryCouponRepo) MarkUsageRefunded(_ context.Context, usageID int64) error {
	for i := range r.usages {
		if r.usages[i].ID == usageID {
			r.usages[i].Status = CouponUsageStatusRefunded
		}
	}
	return nil
}

func activeTestCoupon() *Coupon {
	return &Coupon{
		ID:           1,
		Code:         "SAVE10",
		Type:         CouponTypeFixed,
		Value:        10,
		Scope:        CouponScopeAll,
		Status:       CouponStatusActive,
		PerUserLimit: 1,
	}
}

func TestCouponServiceValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("valid fixed", func(t *testing.T) {
		repo := &memoryCouponRepo{coupon: activeTestCoupon()}
		svc := NewCouponService(repo, nil)
		got, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 100, OrderType: payment.OrderTypeBalance})
		require.NoError(t, err)
		require.Equal(t, 10.0, got.DiscountAmount)
	})

	t.Run("percent with max discount", func(t *testing.T) {
		c := activeTestCoupon()
		c.Type = CouponTypePercent
		c.Value = 0.20
		c.MaxDiscount = 30
		repo := &memoryCouponRepo{coupon: c}
		svc := NewCouponService(repo, nil)
		got, err := svc.Validate(ctx, CouponValidationRequest{Code: "OFF20", UserID: 1, Amount: 200, OrderType: payment.OrderTypeBalance})
		require.NoError(t, err)
		require.Equal(t, 30.0, got.DiscountAmount)
	})

	t.Run("face value capped by amount", func(t *testing.T) {
		repo := &memoryCouponRepo{coupon: activeTestCoupon()}
		svc := NewCouponService(repo, nil)
		got, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 5, OrderType: payment.OrderTypeBalance})
		require.NoError(t, err)
		require.Equal(t, 5.0, got.DiscountAmount)
	})

	t.Run("expired", func(t *testing.T) {
		c := activeTestCoupon()
		past := time.Now().Add(-time.Hour)
		c.ExpiresAt = &past
		svc := NewCouponService(&memoryCouponRepo{coupon: c}, nil)
		_, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 100, OrderType: payment.OrderTypeBalance})
		require.ErrorIs(t, err, ErrCouponExpired)
	})

	t.Run("not started", func(t *testing.T) {
		c := activeTestCoupon()
		future := time.Now().Add(time.Hour)
		c.StartsAt = &future
		svc := NewCouponService(&memoryCouponRepo{coupon: c}, nil)
		_, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 100, OrderType: payment.OrderTypeBalance})
		require.ErrorIs(t, err, ErrCouponNotStarted)
	})

	t.Run("scope mismatch", func(t *testing.T) {
		c := activeTestCoupon()
		c.Scope = CouponScopeSubscription
		svc := NewCouponService(&memoryCouponRepo{coupon: c}, nil)
		_, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 100, OrderType: payment.OrderTypeBalance})
		require.ErrorIs(t, err, ErrCouponScopeMismatch)
	})

	t.Run("user limit", func(t *testing.T) {
		c := activeTestCoupon()
		repo := &memoryCouponRepo{coupon: c, usages: []CouponUsage{{CouponID: 1, UserID: 1, Status: CouponUsageStatusUsed}}}
		svc := NewCouponService(repo, nil)
		_, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 100, OrderType: payment.OrderTypeBalance})
		require.ErrorIs(t, err, ErrCouponUserLimitReached)
	})

	t.Run("minimum amount", func(t *testing.T) {
		c := activeTestCoupon()
		c.MinAmount = 100
		svc := NewCouponService(&memoryCouponRepo{coupon: c}, nil)
		_, err := svc.Validate(ctx, CouponValidationRequest{Code: "SAVE10", UserID: 1, Amount: 80, OrderType: payment.OrderTypeBalance})
		require.Error(t, err)
		require.Contains(t, err.Error(), "COUPON_MIN_AMOUNT_NOT_MET")
	})
}
