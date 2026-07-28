//go:build unit

package service

import (
	"context"
	"testing"
	"time"
)

// TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier locks in the fix
// that subscription-mode billing honours the group (and any user-specific) rate
// multiplier — i.e. cmd.SubscriptionCost tracks ActualCost (= TotalCost *
// RateMultiplier), not raw TotalCost.
func TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	subID := int64(42)

	tests := []struct {
		name            string
		totalCost       float64
		actualCost      float64
		isSubscription  bool
		wantSub         float64
		wantBalance     float64
		wantAPIKeyQuota float64
		wantAllowOver   bool
	}{
		{
			name:            "subscription with 2x multiplier consumes 2x quota",
			totalCost:       1.0,
			actualCost:      2.0,
			isSubscription:  true,
			wantSub:         2.0,
			wantBalance:     0,
			wantAPIKeyQuota: 0,
			wantAllowOver:   true,
		},
		{
			name:            "subscription with 0.5x multiplier consumes 0.5x quota",
			totalCost:       1.0,
			actualCost:      0.5,
			isSubscription:  true,
			wantSub:         0.5,
			wantBalance:     0,
			wantAPIKeyQuota: 0,
			wantAllowOver:   true,
		},
		{
			name:            "free subscription (multiplier 0) consumes no quota",
			totalCost:       1.0,
			actualCost:      0,
			isSubscription:  true,
			wantSub:         0,
			wantBalance:     0,
			wantAPIKeyQuota: 0,
		},
		{
			name:            "balance billing keeps using ActualCost and key quota (regression)",
			totalCost:       1.0,
			actualCost:      2.0,
			isSubscription:  false,
			wantSub:         0,
			wantBalance:     2.0,
			wantAPIKeyQuota: 2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &postUsageBillingParams{
				Cost:               &CostBreakdown{TotalCost: tt.totalCost, ActualCost: tt.actualCost},
				User:               &User{ID: 1},
				APIKey:             &APIKey{ID: 2, GroupID: &groupID, Quota: 100},
				Account:            &Account{ID: 3},
				Subscription:       &UserSubscription{ID: subID},
				IsSubscriptionBill: tt.isSubscription,
				APIKeyService:      &openAIRecordUsageAPIKeyQuotaStub{},
			}

			cmd := buildUsageBillingCommand("req-1", nil, p)
			if cmd == nil {
				t.Fatal("buildUsageBillingCommand returned nil")
			}
			if cmd.SubscriptionCost != tt.wantSub {
				t.Errorf("SubscriptionCost = %v, want %v", cmd.SubscriptionCost, tt.wantSub)
			}
			if cmd.BalanceCost != tt.wantBalance {
				t.Errorf("BalanceCost = %v, want %v", cmd.BalanceCost, tt.wantBalance)
			}
			if cmd.APIKeyQuotaCost != tt.wantAPIKeyQuota {
				t.Errorf("APIKeyQuotaCost = %v, want %v", cmd.APIKeyQuotaCost, tt.wantAPIKeyQuota)
			}
			if cmd.AllowSubscriptionOverLimit != tt.wantAllowOver {
				t.Errorf("AllowSubscriptionOverLimit = %v, want %v", cmd.AllowSubscriptionOverLimit, tt.wantAllowOver)
			}
		})
	}
}

func TestMarkDailyOverdraftPoolExhausted(t *testing.T) {
	daily := 10.0
	startsAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	subscription := &UserSubscription{
		ID:                  42,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		Status:              SubscriptionStatusActive,
		QuotaUsedUSD:        50,
		AllowDailyOverdraft: true,
	}
	repo := &dailyOverdraftBillingSubRepoStub{subscription: subscription}
	result := &UsageBillingApplyResult{Applied: true}

	markDailyOverdraftPoolExhausted(context.Background(), &postUsageBillingParams{
		Cost:               &CostBreakdown{ActualCost: 0.01},
		APIKey:             &APIKey{Group: group},
		Subscription:       &UserSubscription{ID: subscription.ID},
		IsSubscriptionBill: true,
	}, &billingDeps{userSubRepo: repo}, result)

	if !result.SubscriptionQuotaExhausted {
		t.Fatal("expected total-pool exhaustion to be reported")
	}
	if repo.status != SubscriptionStatusQuotaExhausted {
		t.Fatalf("status = %q, want %q", repo.status, SubscriptionStatusQuotaExhausted)
	}
}

type dailyOverdraftBillingSubRepoStub struct {
	userSubRepoNoop
	subscription *UserSubscription
	status       string
}

func (r *dailyOverdraftBillingSubRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.subscription, nil
}

func (r *dailyOverdraftBillingSubRepoStub) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.status = status
	r.subscription.Status = status
	return nil
}
