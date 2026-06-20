package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type CouponRepository interface {
	Create(ctx context.Context, coupon *Coupon) error
	GetByID(ctx context.Context, id int64) (*Coupon, error)
	GetByCode(ctx context.Context, code string) (*Coupon, error)
	GetByCodeForUpdate(ctx context.Context, code string) (*Coupon, error)
	Update(ctx context.Context, coupon *Coupon) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Coupon, *pagination.PaginationResult, error)
	CreateUsage(ctx context.Context, usage *CouponUsage) error
	GetUsageByCouponAndUser(ctx context.Context, couponID, userID int64) ([]CouponUsage, error)
	GetUsageByOrder(ctx context.Context, orderID int64) (*CouponUsage, error)
	ListUsagesByCoupon(ctx context.Context, couponID int64, params pagination.PaginationParams) ([]CouponUsage, *pagination.PaginationResult, error)
	IncrementUsedCount(ctx context.Context, id int64) error
	DecrementUsedCount(ctx context.Context, id int64) error
	MarkUsageRefunded(ctx context.Context, usageID int64) error
}
