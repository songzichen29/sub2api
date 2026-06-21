package service

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	DiscountRuleTypeRate   = "rate"
	DiscountRuleTypeReduce = "reduce"
)

type ThresholdDiscountResult struct {
	BaseAmount     float64       `json:"base_amount"`
	DiscountAmount float64       `json:"discount_amount"`
	AfterDiscount  float64       `json:"after_discount"`
	AppliedRule    *DiscountRule `json:"applied_rule,omitempty"`
}

type DiscountService struct{}

func NewDiscountService() *DiscountService {
	return &DiscountService{}
}

func (s *DiscountService) ApplyThresholdDiscount(amount float64, rules []DiscountRule) ThresholdDiscountResult {
	base := decimal.NewFromFloat(amount)
	if amount <= 0 || len(rules) == 0 {
		return ThresholdDiscountResult{BaseAmount: roundMoney(amount), AfterDiscount: roundMoney(amount)}
	}
	normalized, err := normalizeDiscountRules(rules)
	if err != nil {
		return ThresholdDiscountResult{BaseAmount: roundMoney(amount), AfterDiscount: roundMoney(amount)}
	}
	var applied *DiscountRule
	for i := range normalized {
		rule := normalized[i]
		if !rule.Enabled {
			continue
		}
		if amount+1e-9 >= rule.Threshold {
			applied = &normalized[i]
		}
	}
	if applied == nil {
		return ThresholdDiscountResult{BaseAmount: roundMoney(amount), AfterDiscount: roundMoney(amount)}
	}
	discount := decimal.Zero
	switch applied.Type {
	case DiscountRuleTypeRate:
		rate := decimal.NewFromFloat(applied.Value)
		discount = base.Mul(decimal.NewFromInt(1).Sub(rate))
	case DiscountRuleTypeReduce:
		discount = decimal.NewFromFloat(applied.Value)
	}
	if discount.LessThan(decimal.Zero) {
		discount = decimal.Zero
	}
	if discount.GreaterThan(base) {
		discount = base
	}
	after := base.Sub(discount)
	return ThresholdDiscountResult{
		BaseAmount:     roundMoney(amount),
		DiscountAmount: roundMoneyDecimal(discount),
		AfterDiscount:  roundMoneyDecimal(after),
		AppliedRule:    applied,
	}
}

func parseDiscountRules(raw string) []DiscountRule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var rules []DiscountRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil
	}
	normalized, err := normalizeDiscountRules(rules)
	if err != nil {
		return nil
	}
	return normalized
}

func normalizeDiscountRules(rules []DiscountRule) ([]DiscountRule, error) {
	if rules == nil {
		return nil, nil
	}
	normalized := make([]DiscountRule, 0, len(rules))
	seen := map[string]struct{}{}
	for _, rule := range rules {
		rule.Type = strings.TrimSpace(strings.ToLower(rule.Type))
		rule.Label = strings.TrimSpace(rule.Label)
		if math.IsNaN(rule.Threshold) || math.IsInf(rule.Threshold, 0) || rule.Threshold <= 0 {
			return nil, infraerrors.BadRequest("INVALID_DISCOUNT_RULES", "discount rule threshold must be greater than 0")
		}
		key := decimal.NewFromFloat(rule.Threshold).Round(2).StringFixed(2)
		if _, ok := seen[key]; ok {
			return nil, infraerrors.BadRequest("INVALID_DISCOUNT_RULES", "discount rule threshold must be unique")
		}
		seen[key] = struct{}{}
		if rule.Type != DiscountRuleTypeRate && rule.Type != DiscountRuleTypeReduce {
			return nil, infraerrors.BadRequest("INVALID_DISCOUNT_RULES", "discount rule type must be rate or reduce")
		}
		if math.IsNaN(rule.Value) || math.IsInf(rule.Value, 0) || rule.Value <= 0 {
			return nil, infraerrors.BadRequest("INVALID_DISCOUNT_RULES", "discount rule value must be greater than 0")
		}
		if rule.Type == DiscountRuleTypeRate && rule.Value >= 1 {
			return nil, infraerrors.BadRequest("INVALID_DISCOUNT_RULES", "discount rule rate value must be between 0 and 1")
		}
		normalized = append(normalized, DiscountRule{
			Threshold: roundMoney(rule.Threshold),
			Type:      rule.Type,
			Value:     rule.Value,
			Label:     rule.Label,
			Enabled:   rule.Enabled,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Threshold < normalized[j].Threshold
	})
	return normalized, nil
}

func formatDiscountRules(rules []DiscountRule) string {
	if len(rules) == 0 {
		return ""
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseQuickAmounts(raw string) []float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var amounts []float64
	if err := json.Unmarshal([]byte(raw), &amounts); err != nil {
		return nil
	}
	normalized, err := normalizeQuickAmounts(amounts)
	if err != nil {
		return nil
	}
	return normalized
}

func normalizeQuickAmounts(amounts []float64) ([]float64, error) {
	if amounts == nil {
		return nil, nil
	}
	normalized := make([]float64, 0, len(amounts))
	seen := map[string]struct{}{}
	for _, amount := range amounts {
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			return nil, infraerrors.BadRequest("INVALID_PAYMENT_QUICK_AMOUNTS", "quick amount must be greater than 0")
		}
		rounded := roundMoney(amount)
		if math.Abs(amount-rounded) > 1e-9 {
			return nil, infraerrors.BadRequest("INVALID_PAYMENT_QUICK_AMOUNTS", "quick amount allows at most 2 decimal places")
		}
		key := decimal.NewFromFloat(rounded).StringFixed(2)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, rounded)
	}
	sort.Float64s(normalized)
	return normalized, nil
}

func formatQuickAmounts(amounts []float64) string {
	if len(amounts) == 0 {
		return ""
	}
	data, err := json.Marshal(amounts)
	if err != nil {
		return ""
	}
	return string(data)
}

func roundMoney(v float64) float64 {
	return roundMoneyDecimal(decimal.NewFromFloat(v))
}

func roundMoneyDecimal(v decimal.Decimal) float64 {
	f, _ := v.Round(2).Float64()
	return f
}
