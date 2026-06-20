package service

import "time"

const (
	CouponTypeFixed   = "fixed"
	CouponTypePercent = "percent"

	CouponScopeAll          = "all"
	CouponScopeBalance      = "balance"
	CouponScopeSubscription = "subscription"

	CouponStatusActive   = "active"
	CouponStatusDisabled = "disabled"
	CouponStatusArchived = "archived"

	CouponUsageStatusUsed     = "used"
	CouponUsageStatusRefunded = "refunded"
)

type Coupon struct {
	ID           int64
	Code         string
	Type         string
	Value        float64
	MinAmount    float64
	MaxDiscount  float64
	Scope        string
	MaxUses      int
	UsedCount    int
	PerUserLimit int
	Status       string
	StartsAt     *time.Time
	ExpiresAt    *time.Time
	Notes        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	UsageRecords []CouponUsage
}

type CouponUsage struct {
	ID             int64
	CouponID       int64
	UserID         int64
	OrderID        int64
	DiscountAmount float64
	UsedAt         time.Time
	Status         string
	Coupon         *Coupon
	User           *User
}

type CreateCouponInput struct {
	Code         string
	Type         string
	Value        float64
	MinAmount    float64
	MaxDiscount  float64
	Scope        string
	MaxUses      int
	PerUserLimit int
	StartsAt     *time.Time
	ExpiresAt    *time.Time
	Notes        string
}

type UpdateCouponInput struct {
	Code         *string
	Type         *string
	Value        *float64
	MinAmount    *float64
	MaxDiscount  *float64
	Scope        *string
	MaxUses      *int
	PerUserLimit *int
	Status       *string
	StartsAt     *time.Time
	ExpiresAt    *time.Time
	Notes        *string
}

type CouponValidationRequest struct {
	Code      string
	UserID    int64
	Amount    float64
	OrderType string
}

type CouponValidationResult struct {
	Coupon         *Coupon `json:"coupon,omitempty"`
	DiscountAmount float64 `json:"discount_amount"`
}
