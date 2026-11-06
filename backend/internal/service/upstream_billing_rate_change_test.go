package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingRateChangeSurvivesProbeAndRefresh(t *testing.T) {
	localRate := 9.0 // Local billing must never become the previous upstream rate.
	account := &Account{
		ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Concurrency: 1, RateMultiplier: &localRate,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example"},
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: true},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	now := time.Date(2026, time.November, 7, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	probe := func(rate float64, status int) *UpstreamBillingProbeSnapshot {
		t.Helper()
		now = now.Add(time.Minute)
		body := fmt.Sprintf(`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":%g,"resolved_rate_multiplier":%g,"effective_rate_multiplier":%g,"peak_rate_enabled":false,"observed_at":%q}`, rate, rate, rate, now.Format(time.RFC3339))
		upstream.resp = &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
		snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
		require.NoError(t, err)
		// Model a database JSON round trip, then use the same compact refresh
		// projection the table polls. History must survive both boundaries.
		raw, err := json.Marshal(account.Extra)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &account.Extra))
		items := BuildUpstreamBillingRateSnapshotItems([]Account{*account})
		require.Len(t, items, 1)
		require.Equal(t, snapshot.RateChange, items[0].Snapshot.RateChange)
		return items[0].Snapshot
	}

	require.Nil(t, probe(0.8, 200).RateChange)
	up := probe(1.2, 200)
	require.Equal(t, &UpstreamBillingRateChange{PreviousRateMultiplier: 0.8, CurrentRateMultiplier: 1.2, DetectedAt: now}, up.RateChange)
	require.Equal(t, up.RateChange, probe(1.2, 200).RateChange)
	require.Equal(t, up.RateChange, probe(100, 500).RateChange)
	require.Equal(t, up.RateChange, probe(100, 404).RateChange)
	down := probe(0.000125, 200)
	require.Equal(t, 1.2, down.RateChange.PreviousRateMultiplier)
	require.Equal(t, 0.000125, down.RateChange.CurrentRateMultiplier)
	zero := probe(0, 200)
	require.Equal(t, 0.000125, zero.RateChange.PreviousRateMultiplier)
	require.Zero(t, zero.RateChange.CurrentRateMultiplier)
	require.Equal(t, 9.0, *account.RateMultiplier)
}

func TestUpstreamBillingRateChangeUsesObservedPeakAndRejectsInvalidHistory(t *testing.T) {
	now := time.Date(2026, time.November, 7, 10, 0, 0, 0, time.UTC)
	previous := &UpstreamBillingProbeSnapshot{
		ReceivedAt: probeTimePtr(now.Add(-time.Hour)),
		Data:       map[string]any{"effective_rate_multiplier": 1.2, "resolved_rate_multiplier": 0.8},
	}
	change := upstreamBillingRateChange(previous, map[string]any{"effective_rate_multiplier": 0.8}, now)
	require.Equal(t, 1.2, change.PreviousRateMultiplier)
	require.Equal(t, 0.8, change.CurrentRateMultiplier)
	previous.RateChange = change
	require.Equal(t, change, upstreamBillingRateChange(previous, map[string]any{"effective_rate_multiplier": 1.2 + 1e-12}, now))
	for _, invalid := range []any{nil, -1.0, math.NaN(), math.Inf(1), "bad"} {
		previous.Data["effective_rate_multiplier"] = invalid
		require.Nil(t, upstreamBillingRateChange(previous, map[string]any{"effective_rate_multiplier": 0.8}, now))
	}
	previous.ReceivedAt = nil
	require.Nil(t, upstreamBillingRateChange(previous, map[string]any{"effective_rate_multiplier": 0.8}, now))
}
