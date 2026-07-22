package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// normalizePlanCurrency validates and normalizes the display-only currency label.
// Empty means "no label" and is kept as-is so existing plans stay unchanged.
func normalizePlanCurrency(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	currency, err := payment.NormalizePaymentCurrency(raw)
	if err != nil {
		return "", infraerrors.BadRequest("PLAN_CURRENCY_INVALID", "currency must be a 3-letter ISO currency code")
	}
	return currency, nil
}

// OptionalPlanExpiresAt preserves patch semantics for subscription plan expiry:
// omitted means "do not change"; null or empty string means "clear".
type OptionalPlanExpiresAt struct {
	Set   bool
	Value *time.Time
}

func (f *OptionalPlanExpiresAt) UnmarshalJSON(data []byte) error {
	f.Set = true
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		f.Value = nil
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		f.Value = nil
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fmt.Errorf("invalid expires_at: %w", err)
	}
	f.Value = &t
	return nil
}

// validatePlanRequired checks that all required fields for a plan are provided.
func validatePlanRequired(name string, groupID int64, price float64, validityDays int, validityUnit string, originalPrice *float64) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if groupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if validityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if strings.TrimSpace(validityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if originalPrice != nil && *originalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	return nil
}

func validatePlanQuotaLimit(quota *float64) error {
	if quota != nil && *quota < 0 {
		return infraerrors.BadRequest("PLAN_QUOTA_INVALID", "quota_limit_usd must be >= 0")
	}
	return nil
}

func validatePlanMaxBuyCount(v *int) error {
	if v != nil && *v < 1 {
		return infraerrors.BadRequest("PLAN_MAX_BUY_COUNT_INVALID", "max_buy_count must be >= 1")
	}
	return nil
}

func validatePlanExpiresAt(expiresAt *time.Time, now time.Time) error {
	if expiresAt == nil {
		return nil
	}
	if !expiresAt.After(now) {
		return infraerrors.BadRequest("PLAN_EXPIRES_AT_INVALID", "plan expires_at must be later than now")
	}
	if expiresAt.After(MaxExpiresAt) {
		return infraerrors.BadRequest("PLAN_EXPIRES_AT_INVALID", "plan expires_at exceeds supported maximum (2099-12-31T23:59:59Z)")
	}
	return nil
}

// validatePlanPatch validates only the non-nil fields in a patch update.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	if err := validatePlanQuotaLimit(req.QuotaLimitUSD); err != nil {
		return err
	}
	if req.ExpiresAt.Set {
		if err := validatePlanExpiresAt(req.ExpiresAt.Value, time.Now()); err != nil {
			return err
		}
	}
	if req.MaxBuyCount.Set {
		if err := validatePlanMaxBuyCount(req.MaxBuyCount.Value); err != nil {
			return err
		}
	}
	return nil
}

// --- Plan CRUD ---

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	Platform           string   `json:"platform"`
	Name               string   `json:"name"`
	RateMultiplier     float64  `json:"rate_multiplier"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start"`
	PeakEnd            string   `json:"peak_end"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier"`
	DailyLimitUSD      *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64 `json:"monthly_limit_usd"`
	ModelScopes        []string `json:"supported_model_scopes"`
}

// GetGroupInfoMap returns a map of group_id → PlanGroupInfo for the given plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int64]PlanGroupInfo, len(groups))
	for _, g := range groups {
		m[int64(g.ID)] = PlanGroupInfo{
			Platform:           g.Platform,
			Name:               g.Name,
			RateMultiplier:     g.RateMultiplier,
			PeakRateEnabled:    g.PeakRateEnabled,
			PeakStart:          g.PeakStart,
			PeakEnd:            g.PeakEnd,
			PeakRateMultiplier: g.PeakRateMultiplier,
			DailyLimitUSD:      g.DailyLimitUsd,
			WeeklyLimitUSD:     g.WeeklyLimitUsd,
			MonthlyLimitUSD:    g.MonthlyLimitUsd,
			ModelScopes:        g.SupportedModelScopes,
		}
	}
	return m
}

func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]string {
	info := s.GetGroupInfoMap(ctx, plans)
	if len(info) == 0 {
		return nil
	}
	out := make(map[int64]string, len(info))
	for groupID, groupInfo := range info {
		out[groupID] = groupInfo.Platform
	}
	return out
}

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	now := time.Now()
	return s.entClient.SubscriptionPlan.Query().
		Where(
			subscriptionplan.ForSaleEQ(true),
			subscriptionplan.Or(
				subscriptionplan.ExpiresAtIsNil(),
				subscriptionplan.ExpiresAtGT(now),
			),
		).
		Order(subscriptionplan.BySortOrder()).
		All(ctx)
}

func (s *PaymentConfigService) GetPlanSalesCountMap(ctx context.Context, planIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int)
	if len(planIDs) == 0 {
		return result, nil
	}

	type row struct {
		PlanID int64 `json:"plan_id"`
		Count  int   `json:"count"`
	}

	var rows []row
	err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.PlanIDIn(planIDs...),
			paymentorder.OrderTypeEQ("subscription"),
			paymentorder.StatusIn("PAID", "RECHARGING", "COMPLETED", "PARTIALLY_REFUNDED", "REFUNDED"),
		).
		GroupBy(paymentorder.FieldPlanID).
		Aggregate(dbent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.PlanID] = row.Count
	}
	return result, nil
}

// GetUserPlanPurchaseCountMap 返回指定用户在给定套餐 ID 列表中的已购次数（仅计算成功订单）。
func (s *PaymentConfigService) GetUserPlanPurchaseCountMap(ctx context.Context, userID int64, planIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int)
	if userID <= 0 || len(planIDs) == 0 {
		return result, nil
	}

	type row struct {
		PlanID int64 `json:"plan_id"`
		Count  int   `json:"count"`
	}

	var rows []row
	err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.PlanIDIn(planIDs...),
			paymentorder.OrderTypeEQ("subscription"),
			paymentorder.StatusIn("PAID", "RECHARGING", "COMPLETED", "PARTIALLY_REFUNDED"),
		).
		GroupBy(paymentorder.FieldPlanID).
		Aggregate(dbent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.PlanID] = row.Count
	}
	return result, nil
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanRequired(req.Name, req.GroupID, req.Price, req.ValidityDays, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
	}
	currency, err := normalizePlanCurrency(req.Currency)
	if err != nil {
		return nil, err
	}
	if err := validatePlanQuotaLimit(req.QuotaLimitUSD); err != nil {
		return nil, err
	}
	if err := validatePlanExpiresAt(req.ExpiresAt, time.Now()); err != nil {
		return nil, err
	}
	if err := validatePlanMaxBuyCount(req.MaxBuyCount); err != nil {
		return nil, err
	}
	b := s.entClient.SubscriptionPlan.Create().
		SetGroupID(req.GroupID).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetCurrency(currency).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.QuotaLimitUSD != nil {
		if *req.QuotaLimitUSD > 0 {
			b.SetQuotaLimitUsd(*req.QuotaLimitUSD)
		}
	}
	if req.ExpiresAt != nil {
		b.SetExpiresAt(*req.ExpiresAt)
	}
	if req.MaxBuyCount != nil && *req.MaxBuyCount > 0 {
		b.SetMaxBuyCount(*req.MaxBuyCount)
	}
	return b.Save(ctx)
}

// UpdatePlan updates a subscription plan by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update boilerplate
// plus a validation guard for non-nil fields.
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
	}
	u := s.entClient.SubscriptionPlan.UpdateOneID(id)
	if req.GroupID != nil {
		u.SetGroupID(*req.GroupID)
	}
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.Currency != nil {
		currency, err := normalizePlanCurrency(*req.Currency)
		if err != nil {
			return nil, err
		}
		u.SetCurrency(currency)
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	if req.QuotaLimitUSD != nil {
		if *req.QuotaLimitUSD > 0 {
			u.SetQuotaLimitUsd(*req.QuotaLimitUSD)
		} else {
			u.ClearQuotaLimitUsd()
		}
	}
	if req.ExpiresAt.Set {
		if req.ExpiresAt.Value == nil {
			u.ClearExpiresAt()
		} else {
			u.SetExpiresAt(*req.ExpiresAt.Value)
		}
	}
	if req.MaxBuyCount.Set {
		if req.MaxBuyCount.Value == nil {
			u.ClearMaxBuyCount()
		} else {
			u.SetMaxBuyCount(*req.MaxBuyCount.Value)
		}
	}
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	return u.Save(ctx)
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}
