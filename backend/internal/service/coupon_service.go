package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/shopspring/decimal"
)

var (
	ErrCouponInvalid          = infraerrors.BadRequest("COUPON_INVALID", "invalid coupon")
	ErrCouponExpired          = infraerrors.BadRequest("COUPON_EXPIRED", "coupon has expired")
	ErrCouponDisabled         = infraerrors.BadRequest("COUPON_DISABLED", "coupon is disabled")
	ErrCouponNotStarted       = infraerrors.BadRequest("COUPON_NOT_STARTED", "coupon has not started")
	ErrCouponExhausted        = infraerrors.BadRequest("COUPON_EXHAUSTED", "coupon has reached maximum uses")
	ErrCouponUserLimitReached = infraerrors.BadRequest("COUPON_USER_LIMIT_REACHED", "coupon user limit reached")
	ErrCouponScopeMismatch    = infraerrors.BadRequest("COUPON_SCOPE_MISMATCH", "coupon scope does not match order type")
	ErrCouponNotApplicable    = infraerrors.BadRequest("COUPON_NOT_APPLICABLE", "coupon is not applicable to this order")
)

type CouponService struct {
	repo      CouponRepository
	entClient *dbent.Client
}

func NewCouponService(repo CouponRepository, entClient *dbent.Client) *CouponService {
	return &CouponService{repo: repo, entClient: entClient}
}

func (s *CouponService) Validate(ctx context.Context, req CouponValidationRequest) (*CouponValidationResult, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return &CouponValidationResult{}, nil
	}
	coupon, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := s.validateCoupon(ctx, coupon, req.UserID, req.Amount, req.OrderType); err != nil {
		return nil, err
	}
	return &CouponValidationResult{Coupon: coupon, DiscountAmount: calculateCouponDiscount(coupon, req.Amount)}, nil
}

func (s *CouponService) Apply(ctx context.Context, req CouponValidationRequest, orderID int64) (*CouponValidationResult, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return &CouponValidationResult{}, nil
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("coupon service missing ent client")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin coupon transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	result, err := s.ApplyInTx(txCtx, req, orderID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit coupon transaction: %w", err)
	}
	return result, nil
}

func (s *CouponService) ApplyInTx(ctx context.Context, req CouponValidationRequest, orderID int64) (*CouponValidationResult, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return &CouponValidationResult{}, nil
	}
	coupon, err := s.repo.GetByCodeForUpdate(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := s.validateCoupon(ctx, coupon, req.UserID, req.Amount, req.OrderType); err != nil {
		return nil, err
	}
	discount := calculateCouponDiscount(coupon, req.Amount)
	if err := s.repo.CreateUsage(ctx, &CouponUsage{
		CouponID:       coupon.ID,
		UserID:         req.UserID,
		OrderID:        orderID,
		DiscountAmount: discount,
		UsedAt:         time.Now(),
		Status:         CouponUsageStatusUsed,
	}); err != nil {
		return nil, fmt.Errorf("create coupon usage: %w", err)
	}
	if err := s.repo.IncrementUsedCount(ctx, coupon.ID); err != nil {
		return nil, fmt.Errorf("increment coupon used count: %w", err)
	}
	return &CouponValidationResult{Coupon: coupon, DiscountAmount: discount}, nil
}

func (s *CouponService) Refund(ctx context.Context, orderID int64) error {
	usage, err := s.repo.GetUsageByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if usage == nil || usage.Status == CouponUsageStatusRefunded {
		return nil
	}
	if s.entClient == nil {
		return fmt.Errorf("coupon service missing ent client")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin coupon refund transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := s.repo.DecrementUsedCount(txCtx, usage.CouponID); err != nil {
		return err
	}
	if err := s.repo.MarkUsageRefunded(txCtx, usage.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit coupon refund transaction: %w", err)
	}
	return nil
}

func (s *CouponService) validateCoupon(ctx context.Context, coupon *Coupon, userID int64, amount float64, orderType string) error {
	if coupon == nil {
		return ErrCouponInvalid
	}
	now := time.Now()
	if coupon.Status == CouponStatusDisabled || coupon.Status == CouponStatusArchived {
		return ErrCouponDisabled
	}
	if coupon.Status != CouponStatusActive {
		return ErrCouponInvalid
	}
	if coupon.StartsAt != nil && now.Before(*coupon.StartsAt) {
		return ErrCouponNotStarted
	}
	if coupon.ExpiresAt != nil && now.After(*coupon.ExpiresAt) {
		return ErrCouponExpired
	}
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		return ErrCouponExhausted
	}
	if err := validateCouponScope(coupon.Scope, orderType); err != nil {
		return err
	}
	if amount+1e-9 < coupon.MinAmount {
		return infraerrors.BadRequest("COUPON_MIN_AMOUNT_NOT_MET", "coupon minimum amount not met").
			WithMetadata(map[string]string{
				"min_amount":     fmt.Sprintf("%.2f", coupon.MinAmount),
				"current_amount": fmt.Sprintf("%.2f", amount),
			})
	}
	if coupon.PerUserLimit > 0 {
		usages, err := s.repo.GetUsageByCouponAndUser(ctx, coupon.ID, userID)
		if err != nil {
			return fmt.Errorf("check coupon user usage: %w", err)
		}
		count := 0
		for _, usage := range usages {
			if usage.Status != CouponUsageStatusRefunded {
				count++
			}
		}
		if count >= coupon.PerUserLimit {
			return ErrCouponUserLimitReached
		}
	}
	return nil
}

func validateCouponScope(scope, orderType string) error {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		scope = CouponScopeAll
	}
	switch orderType {
	case payment.OrderTypeDailyLimitReset:
		return ErrCouponNotApplicable
	case payment.OrderTypeBalance:
		if scope == CouponScopeAll || scope == CouponScopeBalance {
			return nil
		}
	case payment.OrderTypeSubscription:
		if scope == CouponScopeAll || scope == CouponScopeSubscription {
			return nil
		}
	default:
		return ErrCouponNotApplicable
	}
	return ErrCouponScopeMismatch
}

func calculateCouponDiscount(coupon *Coupon, amount float64) float64 {
	if coupon == nil || amount <= 0 {
		return 0
	}
	base := decimal.NewFromFloat(amount)
	discount := decimal.Zero
	switch coupon.Type {
	case CouponTypeFixed:
		discount = decimal.NewFromFloat(coupon.Value)
	case CouponTypePercent:
		discount = base.Mul(decimal.NewFromFloat(coupon.Value))
		if coupon.MaxDiscount > 0 {
			maxDiscount := decimal.NewFromFloat(coupon.MaxDiscount)
			if discount.GreaterThan(maxDiscount) {
				discount = maxDiscount
			}
		}
	}
	if discount.LessThan(decimal.Zero) {
		discount = decimal.Zero
	}
	if discount.GreaterThan(base) {
		discount = base
	}
	return roundMoneyDecimal(discount)
}

func normalizeCouponInput(c *Coupon) error {
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	c.Type = strings.TrimSpace(strings.ToLower(c.Type))
	c.Scope = strings.TrimSpace(strings.ToLower(c.Scope))
	c.Status = strings.TrimSpace(strings.ToLower(c.Status))
	if c.Scope == "" {
		c.Scope = CouponScopeAll
	}
	if c.Status == "" {
		c.Status = CouponStatusActive
	}
	if c.Type != CouponTypeFixed && c.Type != CouponTypePercent {
		return infraerrors.BadRequest("INVALID_COUPON_TYPE", "coupon type must be fixed or percent")
	}
	if math.IsNaN(c.Value) || math.IsInf(c.Value, 0) || c.Value <= 0 {
		return infraerrors.BadRequest("INVALID_COUPON_VALUE", "coupon value must be greater than 0")
	}
	if c.Type == CouponTypePercent && c.Value >= 1 {
		return infraerrors.BadRequest("INVALID_COUPON_VALUE", "percent coupon value must be between 0 and 1")
	}
	if c.Scope != CouponScopeAll && c.Scope != CouponScopeBalance && c.Scope != CouponScopeSubscription {
		return infraerrors.BadRequest("INVALID_COUPON_SCOPE", "coupon scope is invalid")
	}
	if c.MaxUses < 0 || c.PerUserLimit < 0 || c.MinAmount < 0 || c.MaxDiscount < 0 {
		return infraerrors.BadRequest("INVALID_COUPON_LIMITS", "coupon limits must not be negative")
	}
	return nil
}

func (s *CouponService) GenerateRandomCode() (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate coupon code: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes)), nil
}

func (s *CouponService) Create(ctx context.Context, input CreateCouponInput) (*Coupon, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		var err error
		code, err = s.GenerateRandomCode()
		if err != nil {
			return nil, err
		}
	}
	coupon := &Coupon{
		Code:         code,
		Type:         input.Type,
		Value:        input.Value,
		MinAmount:    input.MinAmount,
		MaxDiscount:  input.MaxDiscount,
		Scope:        input.Scope,
		MaxUses:      input.MaxUses,
		PerUserLimit: input.PerUserLimit,
		Status:       CouponStatusActive,
		StartsAt:     input.StartsAt,
		ExpiresAt:    input.ExpiresAt,
		Notes:        input.Notes,
	}
	if err := normalizeCouponInput(coupon); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, coupon); err != nil {
		return nil, err
	}
	return coupon, nil
}

func (s *CouponService) GetByID(ctx context.Context, id int64) (*Coupon, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *CouponService) Update(ctx context.Context, id int64, input UpdateCouponInput) (*Coupon, error) {
	coupon, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Code != nil {
		coupon.Code = *input.Code
	}
	if input.Type != nil {
		coupon.Type = *input.Type
	}
	if input.Value != nil {
		coupon.Value = *input.Value
	}
	if input.MinAmount != nil {
		coupon.MinAmount = *input.MinAmount
	}
	if input.MaxDiscount != nil {
		coupon.MaxDiscount = *input.MaxDiscount
	}
	if input.Scope != nil {
		coupon.Scope = *input.Scope
	}
	if input.MaxUses != nil {
		coupon.MaxUses = *input.MaxUses
	}
	if input.PerUserLimit != nil {
		coupon.PerUserLimit = *input.PerUserLimit
	}
	if input.Status != nil {
		coupon.Status = *input.Status
	}
	if input.StartsAt != nil {
		coupon.StartsAt = input.StartsAt
	}
	if input.ExpiresAt != nil {
		coupon.ExpiresAt = input.ExpiresAt
	}
	if input.Notes != nil {
		coupon.Notes = *input.Notes
	}
	if err := normalizeCouponInput(coupon); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, coupon); err != nil {
		return nil, err
	}
	return coupon, nil
}

func (s *CouponService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *CouponService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Coupon, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, status, search)
}

func (s *CouponService) ListUsages(ctx context.Context, couponID int64, params pagination.PaginationParams) ([]CouponUsage, *pagination.PaginationResult, error) {
	return s.repo.ListUsagesByCoupon(ctx, couponID, params)
}
