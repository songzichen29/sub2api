//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestPSStartOfDayUTC_UsesConfiguredTimezoneBoundary(t *testing.T) {
	require.NoError(t, timezone.Init("America/Los_Angeles"))

	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)

	ts := time.Date(2026, 5, 10, 15, 4, 5, 0, loc)
	got := psStartOfDayUTC(ts)

	require.Equal(t, loc.String(), got.Location().String())
	require.Equal(t, time.Date(2026, 5, 10, 0, 0, 0, 0, loc), got)
}

func TestBuildAffiliateRebateAppliedAuditDetail_UsesRebateBaseAmount(t *testing.T) {
	detail := buildAffiliateRebateAppliedAuditDetail("balance", 103, 10.3)
	require.Equal(t, 103.0, detail["rebateBaseAmount"])
	require.Equal(t, "balance", detail["orderType"])
	require.Equal(t, 10.3, detail["rebateAmount"])
}

func TestBuildAffiliateRebateSkippedAuditDetail_UsesRebateBaseAmount(t *testing.T) {
	detail := buildAffiliateRebateSkippedAuditDetail("subscription", 88.8, "affiliate_disabled")
	require.Equal(t, 88.8, detail["rebateBaseAmount"])
	require.Equal(t, "subscription", detail["orderType"])
	require.Equal(t, "affiliate_disabled", detail["reason"])
}
