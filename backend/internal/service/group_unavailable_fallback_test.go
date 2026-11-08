package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGroupFallbackPreservesAccountPolicyWithoutMutatingSnapshots(t *testing.T) {
	source := &Group{ID: 1, RequireOAuthOnly: true, RequirePrivacySet: true}
	target := &Group{ID: 2}
	ctx := withAccountFallbackGroup(context.Background(), source, target)
	pool := ctx.Value(ctxkey.Group).(*Group)
	require.True(t, pool.RequirePrivacySet)
	require.True(t, pool.RequireOAuthOnly)
	require.False(t, target.RequirePrivacySet)
	require.False(t, target.RequireOAuthOnly)
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff}},
	}
	eligible := filterFallbackAccounts(ctx, accounts)
	require.Len(t, eligible, 1)
	require.EqualValues(t, 3, eligible[0].ID)
	require.Len(t, accounts, 3, "do not rewrite cached account lists")
	require.Len(t, filterFallbackAccounts(context.Background(), accounts), 3, "ordinary selection remains unchanged")
	require.Equal(t, "fallback_account_policy", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, &accounts[0], PlatformOpenAI, "", false, ""))
	require.False(t, (&GatewayService{}).isAccountSchedulableForModelSelection(ctx, &accounts[1], ""))

	// A more restrictive destination remains authoritative as well.
	ctx = withAccountFallbackGroup(context.Background(), &Group{ID: 1}, &Group{ID: 2, RequireOAuthOnly: true})
	require.False(t, fallbackAccountAllowed(ctx, &accounts[0]))
	require.True(t, fallbackAccountAllowed(ctx, &accounts[1]))
}

func TestGroupFallbackDoesNotBypassProductionMetricAdmission(t *testing.T) {
	for _, state := range []string{"paused", "database-error", "allowed"} {
		t.Run(state, func(t *testing.T) {
			targetID := int64(2)
			source := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, FallbackGroupIDOnUnavailable: &targetID}
			ctx := context.WithValue(context.Background(), ctxkey.Group, source)
			cache := &accountMetricGuardCache{acquired: true}
			admission := &accountMetricGuardAdmission{allowed: state == "allowed"}
			if state == "database-error" {
				admission.err = errors.New("metric state unavailable")
			}
			svc := &OpenAIGatewayService{concurrencyService: NewConcurrencyService(cache, admission)}
			visited := 0
			result, err := selectWithGroupFallback(ctx, &source.ID, "", func(context.Context, int64) (*Group, error) {
				t.Fatal("metric admission rejection must not borrow another pool")
				return nil, nil
			}, func(ctx context.Context, _ *int64) (*AccountSelectionResult, error) {
				visited++
				slot, err := svc.tryAcquireAccountSlot(ctx, 7, 2)
				if err != nil {
					return nil, err
				}
				if !slot.Acquired {
					return nil, ErrNoAvailableAccounts
				}
				return &AccountSelectionResult{Account: &Account{ID: 7}, Acquired: true, ReleaseFunc: slot.ReleaseFunc}, nil
			})
			require.Equal(t, 1, visited)
			require.Equal(t, 1, admission.calls)
			if state == "allowed" {
				require.NoError(t, err)
				result.ReleaseFunc()
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
			require.Equal(t, 1, cache.releases)
		})
	}
}

func TestGroupFallbackRejectsInvalidDestination(t *testing.T) {
	for _, state := range []string{"missing", "inactive", "different-platform", "claude-code-only"} {
		t.Run(state, func(t *testing.T) {
			targetID := int64(2)
			source := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, FallbackGroupIDOnUnavailable: &targetID}
			target := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive}
			switch state {
			case "missing":
				target = nil
			case "inactive":
				target.Status = StatusDisabled
			case "different-platform":
				target.Platform = PlatformAnthropic
			case "claude-code-only":
				target.ClaudeCodeOnly = true
			}
			calls := 0
			_, err := selectWithGroupFallback(context.WithValue(context.Background(), ctxkey.Group, source), &source.ID, "", func(context.Context, int64) (*Group, error) {
				return target, nil
			}, func(context.Context, *int64) (*Account, error) {
				calls++
				return nil, ErrNoAvailableAccounts
			})
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Equal(t, 1, calls)
		})
	}
}

func TestGroupFallbackOneHopAndOriginalPolicy(t *testing.T) {
	id2, id3 := int64(2), int64(3)
	source := profitControlTestGroup(1, .2, .1)
	source.RateMultiplier = .5
	source.RequirePrivacySet = true
	source.FallbackGroupIDOnUnavailable = &id2
	target := profitControlTestGroup(2, 0, 0)
	target.ProfitControlEnabled = false
	target.RateMultiplier = 20
	target.FallbackGroupIDOnUnavailable = &id3
	ctx := context.WithValue(context.Background(), ctxkey.Group, source)
	load := func(_ context.Context, id int64) (*Group, error) { require.Equal(t, id2, id); return target, nil }
	var visited []int64
	selection, err := selectWithGroupFallback(ctx, &source.ID, "model", load, func(ctx context.Context, id *int64) (*AccountSelectionResult, error) {
		visited = append(visited, *id)
		if *id == source.ID {
			return nil, ErrNoAvailableAccounts
		}
		require.Same(t, source, accountFallbackBillingGroup(ctx))
		require.Equal(t, source.ID, *fallbackPolicyGroupID(ctx, id))
		svc := &OpenAIGatewayService{}
		require.True(t, svc.loadOpenAIGroupRequiresPrivacySet(ctx, id))
		ctx = svc.withOpenAIProfitControlGate(ctx, id)
		gate := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
		require.Equal(t, source.ID, gate.groupID)
		require.InDelta(t, .35, gate.threshold, 0.000001)
		return selectWithGroupFallback(ctx, id, "model", load, func(ctx context.Context, _ *int64) (*AccountSelectionResult, error) {
			return attachSelectionProfitGate(ctx, &AccountSelectionResult{Account: &Account{ID: 9}}), nil
		})
	})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, visited)
	require.Equal(t, id2, selection.routedGroupID)
	require.Same(t, source, ctx.Value(ctxkey.Group))
	admission := ContextWithSelectionProfitGate(ctx, selection)
	require.Equal(t, id2, *fallbackStickyGroupID(admission, &source.ID))
}

func TestGroupFallbackNeverExpandsUnknownCapacityOrContinuation(t *testing.T) {
	for _, scenario := range []string{"capacity", "database", "cancelled", "force", "continuation", "nested", "disabled", "composite"} {
		t.Run(scenario, func(t *testing.T) {
			id := int64(2)
			group := profitControlTestGroup(1, 0, 0)
			group.FallbackGroupIDOnUnavailable = &id
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if scenario == "force" {
				ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformOpenAI)
			}
			if scenario == "continuation" || scenario == "nested" {
				ctx = context.WithValue(ctx, groupFallbackAttemptKey{}, true)
			}
			if scenario == "disabled" {
				group.FallbackGroupIDOnUnavailable = nil
			}
			if scenario == "composite" {
				group.Platform = PlatformComposite
			}
			calls := 0
			_, _ = selectWithGroupFallback(ctx, &group.ID, "model", func(context.Context, int64) (*Group, error) { t.Fatal("must not load another pool"); return nil, nil }, func(ctx context.Context, _ *int64) (*AccountSelectionResult, error) {
				calls++
				if scenario == "capacity" {
					preventGroupFallback(ctx)
				}
				if scenario == "database" {
					return nil, errors.New("database offline")
				}
				return nil, ErrNoAvailableAccounts
			})
			require.Equal(t, 1, calls)
		})
	}
}
