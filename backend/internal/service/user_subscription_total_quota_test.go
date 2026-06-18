package service

import "testing"

func TestUserSubscriptionTotalQuotaChecks(t *testing.T) {
	limit := 10.0
	sub := &UserSubscription{
		QuotaLimitUSD: &limit,
		QuotaUsedUSD:  4,
	}

	if !sub.HasTotalQuotaLimit() {
		t.Fatal("expected total quota limit")
	}
	if !sub.CheckTotalQuota(6) {
		t.Fatal("expected usage up to the limit to be allowed")
	}
	if sub.CheckTotalQuota(6.01) {
		t.Fatal("expected usage above the limit to be rejected")
	}

	sub.QuotaUsedUSD = 10
	if sub.CheckTotalQuota(0) {
		t.Fatal("expected exhausted subscription to fail zero-cost eligibility check")
	}
}

func TestApplySubscriptionQuotaLifecycle(t *testing.T) {
	firstGrant := 10.0
	sub := &UserSubscription{QuotaUsedUSD: 3}

	applyNewSubscriptionQuota(sub, &AssignSubscriptionInput{
		QuotaLimitSpecified: true,
		QuotaLimitUSD:       &firstGrant,
	})
	if sub.QuotaLimitUSD == nil || *sub.QuotaLimitUSD != firstGrant {
		t.Fatalf("expected new quota %.2f, got %#v", firstGrant, sub.QuotaLimitUSD)
	}
	if sub.QuotaUsedUSD != 0 {
		t.Fatalf("expected used quota reset to 0, got %.2f", sub.QuotaUsedUSD)
	}

	sub.QuotaUsedUSD = 4
	secondGrant := 5.0
	applyExtensionQuota(sub, &AssignSubscriptionInput{
		QuotaLimitSpecified: true,
		QuotaLimitUSD:       &secondGrant,
	})
	if sub.QuotaLimitUSD == nil || *sub.QuotaLimitUSD != 15 {
		t.Fatalf("expected extended quota 15, got %#v", sub.QuotaLimitUSD)
	}
	if sub.QuotaUsedUSD != 4 {
		t.Fatalf("expected extension to preserve used quota, got %.2f", sub.QuotaUsedUSD)
	}

	applyFreshPeriodQuota(sub, &AssignSubscriptionInput{
		QuotaLimitSpecified: true,
		QuotaLimitUSD:       nil,
	})
	if sub.QuotaLimitUSD != nil {
		t.Fatalf("expected fresh unlimited period to clear quota limit, got %#v", sub.QuotaLimitUSD)
	}
	if sub.QuotaUsedUSD != 0 {
		t.Fatalf("expected fresh period to reset used quota, got %.2f", sub.QuotaUsedUSD)
	}
}
