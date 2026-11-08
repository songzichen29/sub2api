package service

// One-hop pool fallback, adapted from DeanZFC/sub2api-custom 915cc0ed (LGPL-3.0).
// The authenticated key, billing group and admission limits never change.
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type fallbackGroupLoader func(context.Context, int64) (*Group, error)
type groupFallbackAttemptKey struct{}
type accountFallbackBillingGroupKey struct{}
type accountFallbackRouteKey struct{}
type groupFallbackEvidenceKey struct{}

func preventGroupFallback(ctx context.Context) {
	if failed, ok := ctx.Value(groupFallbackEvidenceKey{}).(*atomic.Bool); ok {
		failed.Store(true)
	}
}

func accountFallbackBillingGroup(ctx context.Context) *Group {
	group, _ := ctx.Value(accountFallbackBillingGroupKey{}).(*Group)
	return group
}

func fallbackPolicyGroupID(ctx context.Context, id *int64) *int64 {
	if group := accountFallbackBillingGroup(ctx); group != nil {
		return &group.ID
	}
	return id
}

func fallbackStickyGroupID(ctx context.Context, id *int64) *int64 {
	if route, ok := ctx.Value(accountFallbackRouteKey{}).(int64); ok && route > 0 {
		return &route
	}
	return id
}

func withAccountFallbackGroup(ctx context.Context, billing, routing *Group) context.Context {
	ctx = context.WithValue(ctx, accountFallbackBillingGroupKey{}, billing)
	// A fallback pool must not weaken either group's account requirements.
	// Copy the snapshot so one request cannot alter shared cached group state.
	pool := *routing
	pool.RequirePrivacySet = pool.RequirePrivacySet || billing.RequirePrivacySet
	pool.RequireOAuthOnly = pool.RequireOAuthOnly || billing.RequireOAuthOnly
	return context.WithValue(ctx, ctxkey.Group, &pool)
}

func fallbackAccountAllowed(ctx context.Context, account *Account) bool {
	original := accountFallbackBillingGroup(ctx)
	if original == nil {
		return true
	}
	if account == nil {
		return false
	}
	pool, _ := ctx.Value(ctxkey.Group).(*Group)
	privacy, oauthOnly := original.RequirePrivacySet, original.RequireOAuthOnly
	if pool != nil {
		privacy = privacy || pool.RequirePrivacySet
		oauthOnly = oauthOnly || pool.RequireOAuthOnly
	}
	return (!privacy || account.IsPrivacySet()) && (!oauthOnly || account.Type != AccountTypeAPIKey)
}

func filterFallbackAccounts(ctx context.Context, accounts []Account) []Account {
	if accountFallbackBillingGroup(ctx) == nil {
		return accounts
	}
	filtered := make([]Account, 0, len(accounts))
	for i := range accounts {
		if fallbackAccountAllowed(ctx, &accounts[i]) {
			filtered = append(filtered, accounts[i])
		}
	}
	return filtered
}

func (s *OpenAIGatewayService) loadFallbackGroup(ctx context.Context, id int64) (*Group, error) {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && group != nil && group.ID == id {
		return group, nil
	}
	if s.schedulerSnapshot == nil {
		return nil, nil
	}
	return s.schedulerSnapshot.GetGroupByIDLite(ctx, id)
}

// Nesting is disabled before entering the normal selector, including its own
// proxy, image-size and legacy retries. Only a confirmed empty eligible pool
// can reach the second attempt; a wait plan/capacity error is returned as-is.
func selectWithGroupFallback[T any](ctx context.Context, groupID *int64, model string, load fallbackGroupLoader,
	selectInGroup func(context.Context, *int64) (T, error)) (T, error) {
	if active, _ := ctx.Value(groupFallbackAttemptKey{}).(bool); active {
		return selectInGroup(ctx, groupID)
	}
	attemptCtx := context.WithValue(ctx, groupFallbackAttemptKey{}, true)
	var uncertain atomic.Bool
	attemptCtx = context.WithValue(attemptCtx, groupFallbackEvidenceKey{}, &uncertain)
	value, err := selectInGroup(attemptCtx, groupID)
	if err == nil || groupID == nil || *groupID <= 0 || ctx.Err() != nil || uncertain.Load() ||
		(!errors.Is(err, ErrNoAvailableAccounts) && !errors.Is(err, ErrNoAvailableCompactAccounts) && !errors.Is(err, ErrBatchImageNoAccountAvailable)) {
		return value, err
	}
	if force, _ := ctx.Value(ctxkey.ForcePlatform).(string); force != "" {
		return value, err
	}
	source, _ := ctx.Value(ctxkey.Group).(*Group)
	if source == nil || source.ID != *groupID {
		if load == nil {
			return value, err
		}
		var loadErr error
		source, loadErr = load(ctx, *groupID)
		if loadErr != nil {
			return value, loadErr
		}
	}
	if source == nil || !source.IsActive() || source.Platform == PlatformComposite ||
		(source.ClaudeCodeOnly && !IsClaudeCodeClient(ctx)) ||
		source.FallbackGroupIDOnUnavailable == nil || *source.FallbackGroupIDOnUnavailable <= 0 || *source.FallbackGroupIDOnUnavailable == source.ID {
		return value, err
	}
	if load == nil {
		return value, fmt.Errorf("group fallback repository is unavailable")
	}
	target, loadErr := load(ctx, *source.FallbackGroupIDOnUnavailable)
	if loadErr != nil {
		return value, loadErr
	}
	if target == nil || !target.IsActive() || target.Platform != source.Platform ||
		(target.ClaudeCodeOnly && !IsClaudeCodeClient(ctx)) {
		return value, err
	}
	value, err = selectInGroup(withAccountFallbackGroup(attemptCtx, source, target), &target.ID)
	if err == nil {
		if selected, ok := any(value).(*AccountSelectionResult); ok && selected != nil {
			selected.routedGroupID = target.ID
		}
		slog.Info("group_unavailable_fallback_selected", "billing_group_id", source.ID, "routed_group_id", target.ID, "model", model)
	}
	return value, err
}
