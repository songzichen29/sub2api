//go:build unit

package service

import (
	"encoding/json"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestValidatePlanRequired_AllValid(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_EmptyName(t *testing.T) {
	err := validatePlanRequired("", 1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_WhitespaceName(t *testing.T) {
	err := validatePlanRequired("   ", 1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_ZeroGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", 0, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_NegativeGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", -1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_ZeroPrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 0, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_NegativePrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, -5, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_ZeroValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 0, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_NegativeValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, -7, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_EmptyValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_WhitespaceValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "   ", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_NameValidatedFirst(t *testing.T) {
	err := validatePlanRequired("", 0, 0, 0, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_TrimmedValidName(t *testing.T) {
	err := validatePlanRequired("  Pro  ", 1, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_NegativeOriginalPrice(t *testing.T) {
	neg := -10.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &neg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanRequired_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &zero)
	require.NoError(t, err)
}

func TestValidatePlanRequired_ValidOriginalPrice(t *testing.T) {
	op := 19.99
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &op)
	require.NoError(t, err)
}

// --- validatePlanPatch tests ---

func TestValidatePlanPatch_NegativeOriginalPrice(t *testing.T) {
	neg := -5.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &neg})
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanPatch_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &zero})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ValidOriginalPrice(t *testing.T) {
	op := 29.99
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &op})
	require.NoError(t, err)
}

func TestValidatePlanPatch_NilOriginalPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: nil})
	require.NoError(t, err)
}

// --- validatePlanPatch: other fields ---

func ptrStr(s string) *string     { return &s }
func ptrInt(i int) *int           { return &i }
func ptrInt64(i int64) *int64     { return &i }
func ptrFloat(f float64) *float64 { return &f }

func TestValidatePlanPatch_EmptyName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanPatch_ValidName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("Basic")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroGroupID(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{GroupID: ptrInt64(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanPatch_NegativePrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(-1)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ZeroPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ValidPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(9.99)})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroValidityDays(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityDays: ptrInt(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanPatch_EmptyValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanPatch_ValidValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("days")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_AllNil(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{})
	require.NoError(t, err)
}

func TestOptionalPlanExpiresAt_UnmarshalPatchSemantics(t *testing.T) {
	var omitted UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"Pro"}`), &omitted))
	require.False(t, omitted.ExpiresAt.Set)
	require.Nil(t, omitted.ExpiresAt.Value)

	var clearedByNull UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":null}`), &clearedByNull))
	require.True(t, clearedByNull.ExpiresAt.Set)
	require.Nil(t, clearedByNull.ExpiresAt.Value)

	var clearedByEmpty UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":""}`), &clearedByEmpty))
	require.True(t, clearedByEmpty.ExpiresAt.Set)
	require.Nil(t, clearedByEmpty.ExpiresAt.Value)

	var set UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":"2030-01-02T03:04:05Z"}`), &set))
	require.True(t, set.ExpiresAt.Set)
	require.NotNil(t, set.ExpiresAt.Value)
	require.Equal(t, "2030-01-02T03:04:05Z", set.ExpiresAt.Value.UTC().Format(time.RFC3339))
}

func TestValidatePlanExpiresAt(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	require.NoError(t, validatePlanExpiresAt(nil, now))

	future := now.Add(time.Hour)
	require.NoError(t, validatePlanExpiresAt(&future, now))

	past := now.Add(-time.Second)
	require.Error(t, validatePlanExpiresAt(&past, now))

	tooFar := MaxExpiresAt.Add(time.Second)
	require.Error(t, validatePlanExpiresAt(&tooFar, now))
}

func TestPsPlanSubscriptionDaysUsesFixedExpiry(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(25 * time.Hour)
	require.Equal(t, 2, psPlanSubscriptionDays(&dbent.SubscriptionPlan{
		ValidityDays: 30,
		ValidityUnit: "days",
		ExpiresAt:    &expiresAt,
	}, now))

	expiresAt = now.Add(30 * time.Minute)
	require.Equal(t, 1, psPlanSubscriptionDays(&dbent.SubscriptionPlan{
		ValidityDays: 30,
		ValidityUnit: "days",
		ExpiresAt:    &expiresAt,
	}, now))

	expiresAt = now.Add(-time.Second)
	require.Equal(t, 0, psPlanSubscriptionDays(&dbent.SubscriptionPlan{
		ValidityDays: 30,
		ValidityUnit: "days",
		ExpiresAt:    &expiresAt,
	}, now))
}
