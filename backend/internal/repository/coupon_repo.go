package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/coupon"
	"github.com/Wei-Shaw/sub2api/ent/couponusage"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type couponRepository struct {
	client *dbent.Client
}

func NewCouponRepository(client *dbent.Client) service.CouponRepository {
	return &couponRepository{client: client}
}

func (r *couponRepository) Create(ctx context.Context, c *service.Coupon) error {
	client := clientFromContext(ctx, r.client)
	builder := client.Coupon.Create().
		SetCode(c.Code).
		SetType(c.Type).
		SetValue(c.Value).
		SetMinAmount(c.MinAmount).
		SetMaxDiscount(c.MaxDiscount).
		SetScope(c.Scope).
		SetMaxUses(c.MaxUses).
		SetUsedCount(c.UsedCount).
		SetPerUserLimit(c.PerUserLimit).
		SetStatus(c.Status).
		SetNotes(c.Notes)
	if c.StartsAt != nil {
		builder.SetStartsAt(*c.StartsAt)
	}
	if c.ExpiresAt != nil {
		builder.SetExpiresAt(*c.ExpiresAt)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	*c = *couponEntityToService(created)
	return nil
}

func (r *couponRepository) GetByID(ctx context.Context, id int64) (*service.Coupon, error) {
	m, err := r.client.Coupon.Query().Where(coupon.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrCouponInvalid
		}
		return nil, err
	}
	return couponEntityToService(m), nil
}

func (r *couponRepository) GetByCode(ctx context.Context, code string) (*service.Coupon, error) {
	m, err := r.client.Coupon.Query().Where(coupon.CodeEqualFold(strings.TrimSpace(code))).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrCouponInvalid
		}
		return nil, err
	}
	return couponEntityToService(m), nil
}

func (r *couponRepository) GetByCodeForUpdate(ctx context.Context, code string) (*service.Coupon, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.Coupon.Query().
		Where(coupon.CodeEqualFold(strings.TrimSpace(code))).
		ForUpdate().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrCouponInvalid
		}
		return nil, err
	}
	return couponEntityToService(m), nil
}

func (r *couponRepository) Update(ctx context.Context, c *service.Coupon) error {
	client := clientFromContext(ctx, r.client)
	builder := client.Coupon.UpdateOneID(c.ID).
		SetCode(c.Code).
		SetType(c.Type).
		SetValue(c.Value).
		SetMinAmount(c.MinAmount).
		SetMaxDiscount(c.MaxDiscount).
		SetScope(c.Scope).
		SetMaxUses(c.MaxUses).
		SetUsedCount(c.UsedCount).
		SetPerUserLimit(c.PerUserLimit).
		SetStatus(c.Status).
		SetNotes(c.Notes)
	if c.StartsAt != nil {
		builder.SetStartsAt(*c.StartsAt)
	} else {
		builder.ClearStartsAt()
	}
	if c.ExpiresAt != nil {
		builder.SetExpiresAt(*c.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	*c = *couponEntityToService(updated)
	return nil
}

func (r *couponRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.Coupon.UpdateOneID(id).SetStatus(service.CouponStatusArchived).Save(ctx)
	return err
}

func (r *couponRepository) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Coupon, *pagination.PaginationResult, error) {
	q := r.client.Coupon.Query()
	if strings.TrimSpace(status) != "" {
		q = q.Where(coupon.StatusEQ(strings.TrimSpace(status)))
	}
	if strings.TrimSpace(search) != "" {
		q = q.Where(coupon.CodeContainsFold(strings.TrimSpace(search)))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	rows, err := q.Order(dbent.Desc(coupon.FieldCreatedAt)).
		Offset((params.Page - 1) * params.PageSize).
		Limit(params.PageSize).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.Coupon, 0, len(rows))
	for _, row := range rows {
		items = append(items, *couponEntityToService(row))
	}
	return items, newPaginationResult(params.Page, params.PageSize, total), nil
}

func (r *couponRepository) CreateUsage(ctx context.Context, u *service.CouponUsage) error {
	client := clientFromContext(ctx, r.client)
	created, err := client.CouponUsage.Create().
		SetCouponID(u.CouponID).
		SetUserID(u.UserID).
		SetOrderID(u.OrderID).
		SetDiscountAmount(u.DiscountAmount).
		SetUsedAt(u.UsedAt).
		SetStatus(u.Status).
		Save(ctx)
	if err != nil {
		return err
	}
	*u = *couponUsageEntityToService(created)
	return nil
}

func (r *couponRepository) GetUsageByCouponAndUser(ctx context.Context, couponID, userID int64) ([]service.CouponUsage, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.CouponUsage.Query().
		Where(couponusage.CouponIDEQ(couponID), couponusage.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]service.CouponUsage, 0, len(rows))
	for _, row := range rows {
		items = append(items, *couponUsageEntityToService(row))
	}
	return items, nil
}

func (r *couponRepository) GetUsageByOrder(ctx context.Context, orderID int64) (*service.CouponUsage, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.CouponUsage.Query().Where(couponusage.OrderIDEQ(orderID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return couponUsageEntityToService(row), nil
}

func (r *couponRepository) ListUsagesByCoupon(ctx context.Context, couponID int64, params pagination.PaginationParams) ([]service.CouponUsage, *pagination.PaginationResult, error) {
	q := r.client.CouponUsage.Query().Where(couponusage.CouponIDEQ(couponID))
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	rows, err := q.Order(dbent.Desc(couponusage.FieldUsedAt)).
		Offset((params.Page - 1) * params.PageSize).
		Limit(params.PageSize).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.CouponUsage, 0, len(rows))
	for _, row := range rows {
		items = append(items, *couponUsageEntityToService(row))
	}
	return items, newPaginationResult(params.Page, params.PageSize, total), nil
}

func (r *couponRepository) IncrementUsedCount(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.Coupon.UpdateOneID(id).AddUsedCount(1).Save(ctx)
	return err
}

func (r *couponRepository) DecrementUsedCount(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.Coupon.UpdateOneID(id).AddUsedCount(-1).Save(ctx)
	return err
}

func (r *couponRepository) MarkUsageRefunded(ctx context.Context, usageID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.CouponUsage.UpdateOneID(usageID).SetStatus(service.CouponUsageStatusRefunded).Save(ctx)
	return err
}

func couponEntityToService(m *dbent.Coupon) *service.Coupon {
	if m == nil {
		return nil
	}
	return &service.Coupon{
		ID:           m.ID,
		Code:         m.Code,
		Type:         m.Type,
		Value:        m.Value,
		MinAmount:    m.MinAmount,
		MaxDiscount:  m.MaxDiscount,
		Scope:        m.Scope,
		MaxUses:      m.MaxUses,
		UsedCount:    m.UsedCount,
		PerUserLimit: m.PerUserLimit,
		Status:       m.Status,
		StartsAt:     m.StartsAt,
		ExpiresAt:    m.ExpiresAt,
		Notes:        derefCouponString(m.Notes),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func newPaginationResult(page, pageSize int, total int) *pagination.PaginationResult {
	pages := 0
	if pageSize > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	return &pagination.PaginationResult{Total: int64(total), Page: page, PageSize: pageSize, Pages: pages}
}

func derefCouponString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func couponUsageEntityToService(m *dbent.CouponUsage) *service.CouponUsage {
	if m == nil {
		return nil
	}
	return &service.CouponUsage{
		ID:             m.ID,
		CouponID:       m.CouponID,
		UserID:         m.UserID,
		OrderID:        m.OrderID,
		DiscountAmount: m.DiscountAmount,
		UsedAt:         m.UsedAt,
		Status:         m.Status,
	}
}
