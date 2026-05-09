//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type stubSettingRepo struct {
	values map[string]string
}

func (s *stubSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if s == nil || s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *stubSettingRepo) SetValue(context.Context, string, string) error { return nil }
func (s *stubSettingRepo) GetValues(context.Context) (map[string]string, error) {
	if s == nil || s.values == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out, nil
}

func TestResolveRebateRatePercentByKind_PrefersKindSpecificThenGeneral(t *testing.T) {
	t.Parallel()

	svc := &AffiliateService{}
	general := 15.0
	recharge := 25.0
	subscription := 35.0
	summary := &AffiliateSummary{
		AffRebateRatePercent:             &general,
		AffRechargeRebateRatePercent:     &recharge,
		AffSubscriptionRebateRatePercent: &subscription,
	}

	require.InDelta(t, 25.0,
		svc.resolveRebateRatePercentByKind(context.Background(), summary, payment.OrderTypeBalance), 1e-9)
	require.InDelta(t, 35.0,
		svc.resolveRebateRatePercentByKind(context.Background(), summary, payment.OrderTypeSubscription), 1e-9)
}

func TestResolveRebateRatePercentByKind_FallsBackToGeneralRate(t *testing.T) {
	t.Parallel()

	svc := &AffiliateService{}
	general := 18.0
	summary := &AffiliateSummary{AffRebateRatePercent: &general}

	require.InDelta(t, 18.0,
		svc.resolveRebateRatePercentByKind(context.Background(), summary, payment.OrderTypeBalance), 1e-9)
	require.InDelta(t, 18.0,
		svc.resolveRebateRatePercentByKind(context.Background(), summary, payment.OrderTypeSubscription), 1e-9)
}

func TestSettingService_AffiliateKindRateFallsBackToGlobalRate(t *testing.T) {
	t.Parallel()

	svc := &SettingService{
		settingRepo: &stubSettingRepo{
			values: map[string]string{
				SettingKeyAffiliateRebateRate: "35",
			},
		},
	}

	require.InDelta(t, 35.0, svc.GetAffiliateRechargeRebateRatePercent(context.Background()), 1e-9)
	require.InDelta(t, 35.0, svc.GetAffiliateSubscriptionRebateRatePercent(context.Background()), 1e-9)
}

func TestSettingService_AffiliateKindRatePrefersExplicitSetting(t *testing.T) {
	t.Parallel()

	svc := &SettingService{
		settingRepo: &stubSettingRepo{
			values: map[string]string{
				SettingKeyAffiliateRebateRate:              "35",
				SettingKeyAffiliateRechargeRebateRate:      "25",
				SettingKeyAffiliateSubscriptionRebateRate:  "15",
			},
		},
	}

	require.InDelta(t, 25.0, svc.GetAffiliateRechargeRebateRatePercent(context.Background()), 1e-9)
	require.InDelta(t, 15.0, svc.GetAffiliateSubscriptionRebateRatePercent(context.Background()), 1e-9)
}

func TestAffiliateService_IsAffiliateKindEnabled_UsesDefaultsWhenSettingServiceNil(t *testing.T) {
	t.Parallel()

	svc := &AffiliateService{}

	require.Equal(t, AffiliateRechargeEnabledDefault, svc.isAffiliateRechargeEnabled(context.Background()))
	require.Equal(t, AffiliateSubscriptionEnabledDefault, svc.isAffiliateSubscriptionEnabled(context.Background()))
}

// TestIsEnabled_NilSettingServiceReturnsDefault verifies that IsEnabled
// safely handles a nil settingService dependency by returning the default
// (off). This protects callers from nil-pointer crashes in misconfigured
// environments.
func TestIsEnabled_NilSettingServiceReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}
	require.False(t, svc.IsEnabled(context.Background()))
	require.Equal(t, AffiliateEnabledDefault, svc.IsEnabled(context.Background()))
}

// TestValidateExclusiveRate_BoundaryAndInvalid covers the validator used by
// admin-facing rate setters: nil is always valid (clear), in-range values
// are accepted, NaN/Inf and out-of-range values produce a typed BadRequest.
func TestValidateExclusiveRate_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateExclusiveRate(nil))

	for _, v := range []float64{0, 0.01, 50, 99.99, 100} {
		v := v
		require.NoError(t, validateExclusiveRate(&v), "value %v should be valid", v)
	}

	for _, v := range []float64{-0.01, 100.01, -100, 200} {
		v := v
		require.Error(t, validateExclusiveRate(&v), "value %v should be rejected", v)
	}

	nan := math.NaN()
	require.Error(t, validateExclusiveRate(&nan))
	posInf := math.Inf(1)
	require.Error(t, validateExclusiveRate(&posInf))
	negInf := math.Inf(-1)
	require.Error(t, validateExclusiveRate(&negInf))
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
}

func TestIsValidAffiliateCodeFormat(t *testing.T) {
	t.Parallel()

	// 邀请码格式校验同时服务于：
	// 1) 系统自动生成的 12 位随机码（A-Z 去 I/O，2-9 去 0/1）
	// 2) 管理员设置的自定义专属码（如 "VIP2026"、"NEW_USER-1"）
	// 因此校验放宽到 [A-Z0-9_-]{4,32}（要求调用方先 ToUpper）。
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical 12-char", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"valid admin custom short", "VIP1", true},
		{"valid admin custom with hyphen", "NEW-USER", true},
		{"valid admin custom with underscore", "VIP_2026", true},
		{"valid 32-char max", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true},
		// Previously-excluded chars (I/O/0/1) are now allowed since admins may use them.
		{"letter I now allowed", "IBCDEFGHJKLM", true},
		{"letter O now allowed", "OBCDEFGHJKLM", true},
		{"digit 0 now allowed", "0BCDEFGHJKLM", true},
		{"digit 1 now allowed", "1BCDEFGHJKLM", true},
		{"too short (3 chars)", "ABC", false},
		{"too long (33 chars)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // bytes out of charset
		{"ascii punctuation .", "ABCDEFGHJK.M", false},
		{"whitespace", "ABCDEFGHJK M", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isValidAffiliateCodeFormat(tc.in))
		})
	}
}
