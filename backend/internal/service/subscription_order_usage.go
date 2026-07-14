package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
)

const SubscriptionOrderUsageAttributionEstimated = "estimated_by_time_window"

type SubscriptionOrderUsageResponse struct {
	SubscriptionID      int64                        `json:"subscription_id"`
	UserID              int64                        `json:"user_id"`
	GroupID             int64                        `json:"group_id"`
	Attribution         string                       `json:"attribution"`
	Orders              []SubscriptionOrderUsageItem `json:"orders"`
	TotalQuotaUSD       *float64                     `json:"total_quota_usd,omitempty"`
	TotalUsedActualCost float64                      `json:"total_used_actual_cost"`
	TotalRemainingUSD   *float64                     `json:"total_remaining_usd,omitempty"`
	GeneratedAt         time.Time                    `json:"generated_at"`
}

type SubscriptionOrderUsageItem struct {
	OrderID           int64      `json:"order_id"`
	OrderStatus       string     `json:"order_status"`
	OrderType         string     `json:"order_type"`
	RenewalMode       string     `json:"renewal_mode"`
	UserEmail         string     `json:"user_email"`
	PlanID            *int64     `json:"plan_id,omitempty"`
	PaidAt            time.Time  `json:"paid_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	WindowStart       time.Time  `json:"window_start"`
	WindowEnd         time.Time  `json:"window_end"`
	SubscriptionDays  int        `json:"subscription_days"`
	ValidityUnit      string     `json:"validity_unit,omitempty"`
	QuotaUSD          *float64   `json:"quota_usd,omitempty"`
	UsedActualCostUSD float64    `json:"used_actual_cost_usd"`
	UsedBaseCostUSD   float64    `json:"used_base_cost_usd"`
	RemainingUSD      *float64   `json:"remaining_usd,omitempty"`
	RequestCount      int        `json:"request_count"`
	InputTokens       int        `json:"input_tokens"`
	OutputTokens      int        `json:"output_tokens"`
	FirstUsageAt      *time.Time `json:"first_usage_at,omitempty"`
	LastUsageAt       *time.Time `json:"last_usage_at,omitempty"`
	WindowKind        string     `json:"window_kind"`
	Attribution       string     `json:"attribution"`
}

// GetSubscriptionOrderUsage returns an order-level view by reconstructing paid
// subscription windows and aggregating usage_logs inside each window. Existing
// usage_logs do not store order_id, so attribution is intentionally marked as
// estimated_by_time_window.
func (s *SubscriptionService) GetSubscriptionOrderUsage(ctx context.Context, subscriptionID int64) (*SubscriptionOrderUsageResponse, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("subscription order usage requires ent client")
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	group := sub.Group
	if group == nil && s.groupRepo != nil {
		if g, getErr := s.groupRepo.GetByID(ctx, sub.GroupID); getErr == nil {
			group = g
		}
	}

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.SubscriptionIDEQ(subscriptionID),
			paymentorder.OrderTypeEQ("subscription"),
			paymentorder.PaidAtNotNil(),
			paymentorder.StatusNotIn(
				OrderStatusPending,
				OrderStatusExpired,
				OrderStatusCancelled,
				OrderStatusFailed,
				OrderStatusRefunded,
			),
		).
		Order(paymentorder.ByPaidAt(), paymentorder.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}

	resp := &SubscriptionOrderUsageResponse{
		SubscriptionID: subscriptionID,
		UserID:         sub.UserID,
		GroupID:        sub.GroupID,
		Attribution:    SubscriptionOrderUsageAttributionEstimated,
		Orders:         make([]SubscriptionOrderUsageItem, 0, len(orders)),
		GeneratedAt:    time.Now(),
	}

	var previousWindowEnd time.Time
	for _, order := range orders {
		if order.PaidAt == nil {
			continue
		}
		days := subscriptionOrderDays(order)
		windowStart, kind := subscriptionOrderWindowStart(sub, order, previousWindowEnd)
		windowEnd := subscriptionOrderWindowEnd(sub, order, windowStart, days)
		if windowEnd.After(MaxExpiresAt) {
			windowEnd = MaxExpiresAt
		}
		previousWindowEnd = windowEnd

		usage, err := s.aggregateSubscriptionOrderWindowUsage(ctx, sub.UserID, subscriptionID, windowStart, windowEnd)
		if err != nil {
			return nil, err
		}

		quota := subscriptionOrderQuotaUSD(order, group, sub, days)
		var remaining *float64
		if quota != nil {
			v := math.Max(*quota-usage.UsedActualCostUSD, 0)
			remaining = &v
			if resp.TotalQuotaUSD == nil {
				total := 0.0
				resp.TotalQuotaUSD = &total
			}
			*resp.TotalQuotaUSD += *quota
		}
		resp.TotalUsedActualCost += usage.UsedActualCostUSD

		resp.Orders = append(resp.Orders, SubscriptionOrderUsageItem{
			OrderID:           order.ID,
			OrderStatus:       order.Status,
			OrderType:         order.OrderType,
			RenewalMode:       paymentOrderRenewalModeFromSnapshot(order.ProviderSnapshot),
			UserEmail:         order.UserEmail,
			PlanID:            order.PlanID,
			PaidAt:            *order.PaidAt,
			CompletedAt:       order.CompletedAt,
			WindowStart:       windowStart,
			WindowEnd:         windowEnd,
			SubscriptionDays:  days,
			ValidityUnit:      subscriptionOrderValidityUnit(order, sub),
			QuotaUSD:          quota,
			UsedActualCostUSD: usage.UsedActualCostUSD,
			UsedBaseCostUSD:   usage.UsedBaseCostUSD,
			RemainingUSD:      remaining,
			RequestCount:      usage.RequestCount,
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			FirstUsageAt:      usage.FirstUsageAt,
			LastUsageAt:       usage.LastUsageAt,
			WindowKind:        kind,
			Attribution:       SubscriptionOrderUsageAttributionEstimated,
		})
	}

	if resp.TotalQuotaUSD != nil {
		v := math.Max(*resp.TotalQuotaUSD-resp.TotalUsedActualCost, 0)
		resp.TotalRemainingUSD = &v
	}
	return resp, nil
}

type subscriptionOrderWindowUsage struct {
	RequestCount      int
	UsedActualCostUSD float64
	UsedBaseCostUSD   float64
	InputTokens       int
	OutputTokens      int
	FirstUsageAt      *time.Time
	LastUsageAt       *time.Time
}

func (s *SubscriptionService) aggregateSubscriptionOrderWindowUsage(ctx context.Context, userID, subscriptionID int64, start, end time.Time) (*subscriptionOrderWindowUsage, error) {
	out := &subscriptionOrderWindowUsage{}
	count, err := s.entClient.UsageLog.Query().
		Where(
			usagelog.UserIDEQ(userID),
			usagelog.SubscriptionIDEQ(subscriptionID),
			usagelog.CreatedAtGTE(start),
			usagelog.CreatedAtLT(end),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	out.RequestCount = count
	if count == 0 {
		return out, nil
	}

	type row struct {
		UsedActualCostUSD float64   `json:"used_actual_cost_usd"`
		UsedBaseCostUSD   float64   `json:"used_base_cost_usd"`
		InputTokens       int       `json:"input_tokens"`
		OutputTokens      int       `json:"output_tokens"`
		FirstUsageAt      time.Time `json:"first_usage_at"`
		LastUsageAt       time.Time `json:"last_usage_at"`
	}
	var rows []row
	err = s.entClient.UsageLog.Query().
		Where(
			usagelog.UserIDEQ(userID),
			usagelog.SubscriptionIDEQ(subscriptionID),
			usagelog.CreatedAtGTE(start),
			usagelog.CreatedAtLT(end),
		).
		Aggregate(
			dbent.As(dbent.Sum(usagelog.FieldActualCost), "used_actual_cost_usd"),
			dbent.As(dbent.Sum(usagelog.FieldTotalCost), "used_base_cost_usd"),
			dbent.As(dbent.Sum(usagelog.FieldInputTokens), "input_tokens"),
			dbent.As(dbent.Sum(usagelog.FieldOutputTokens), "output_tokens"),
			dbent.As(dbent.Min(usagelog.FieldCreatedAt), "first_usage_at"),
			dbent.As(dbent.Max(usagelog.FieldCreatedAt), "last_usage_at"),
		).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return out, nil
	}
	out.UsedActualCostUSD = rows[0].UsedActualCostUSD
	out.UsedBaseCostUSD = rows[0].UsedBaseCostUSD
	out.InputTokens = rows[0].InputTokens
	out.OutputTokens = rows[0].OutputTokens
	out.FirstUsageAt = &rows[0].FirstUsageAt
	out.LastUsageAt = &rows[0].LastUsageAt
	return out, nil
}

func subscriptionOrderWindowStart(sub *UserSubscription, order *dbent.PaymentOrder, previousWindowEnd time.Time) (time.Time, string) {
	start := *order.PaidAt
	mode := paymentOrderRenewalModeFromSnapshot(order.ProviderSnapshot)
	if mode == "restart" || timeClose(start, sub.StartsAt, 2*time.Minute) {
		return start, "restart"
	}
	if previousWindowEnd.After(start) {
		return previousWindowEnd, "renewal"
	}
	return start, "new"
}

func subscriptionOrderWindowEnd(sub *UserSubscription, order *dbent.PaymentOrder, start time.Time, days int) time.Time {
	if order.SubscriptionPlanExpiresAt != nil && order.SubscriptionPlanExpiresAt.After(start) {
		return *order.SubscriptionPlanExpiresAt
	}
	if days <= 0 {
		days = 1
	}
	duration := time.Duration(days) * 24 * time.Hour
	if sub != nil && sub.SkipWeekends {
		return addWeekendSkippedDuration(start, duration)
	}
	return start.AddDate(0, 0, days)
}

func subscriptionOrderDays(order *dbent.PaymentOrder) int {
	if order == nil || order.SubscriptionDays == nil || *order.SubscriptionDays <= 0 {
		return 1
	}
	return *order.SubscriptionDays
}

func subscriptionOrderQuotaUSD(order *dbent.PaymentOrder, group *Group, sub *UserSubscription, days int) *float64 {
	if order != nil && order.SubscriptionQuotaUsd != nil && *order.SubscriptionQuotaUsd > 0 {
		v := *order.SubscriptionQuotaUsd
		return &v
	}
	if group != nil && group.DailyLimitUSD != nil && *group.DailyLimitUSD > 0 && sub != nil && sub.AllowDailyOverdraft {
		v := *group.DailyLimitUSD * float64(days)
		return &v
	}
	return nil
}

func subscriptionOrderValidityUnit(order *dbent.PaymentOrder, sub *UserSubscription) string {
	if order != nil && order.SubscriptionValidityUnit != nil && *order.SubscriptionValidityUnit != "" {
		return normalizeSubscriptionValidityUnit(*order.SubscriptionValidityUnit)
	}
	if sub != nil {
		return normalizeSubscriptionValidityUnit(sub.ValidityUnit)
	}
	return "day"
}

func paymentOrderRenewalModeFromSnapshot(snapshot map[string]any) string {
	if snapshot == nil {
		return ""
	}
	value, _ := snapshot["subscription_renewal_mode"].(string)
	return strings.TrimSpace(strings.ToLower(value))
}

func timeClose(a, b time.Time, tolerance time.Duration) bool {
	if a.After(b) {
		return a.Sub(b) <= tolerance
	}
	return b.Sub(a) <= tolerance
}
