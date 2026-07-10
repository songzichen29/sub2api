package service

import (
	"context"
	"strings"
)

// CheckChannelPricingRestriction exposes the channel-pricing model guard to
// handler-level prechecks while keeping the existing scheduler path unchanged.
func (s *GatewayService) CheckChannelPricingRestriction(ctx context.Context, groupID *int64, requestedModel string) bool {
	return s.checkChannelPricingRestriction(ctx, groupID, requestedModel)
}

// PeekAvailableModelsSnapshot returns the cached model-availability snapshot
// for a group/platform when one has already been built on the hot path.
func (s *GatewayService) PeekAvailableModelsSnapshot(groupID *int64, platform string) (availableModelsSnapshot, bool) {
	return peekAvailableModelsSnapshot(s.modelsListCache, groupID, strings.TrimSpace(platform))
}

// CheckChannelPricingRestriction exposes OpenAI channel-pricing model guards
// to the OpenAI HTTP handler precheck.
func (s *OpenAIGatewayService) CheckChannelPricingRestriction(ctx context.Context, groupID *int64, requestedModel string) bool {
	return s.checkChannelPricingRestriction(ctx, groupID, requestedModel)
}

// DiagnoseModelAvailabilityForPlatform implements ModelAvailabilityDiagnoser
// for OpenAI-compatible gateway handlers.
func (s *OpenAIGatewayService) DiagnoseModelAvailabilityForPlatform(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	platform string,
) ModelAvailabilityDiagnosis {
	if s == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	platform = normalizeOpenAICompatiblePlatform(strings.TrimSpace(platform))
	if platform == "" {
		platform = PlatformOpenAI
	}

	accounts, err := s.listSchedulableAccounts(ctx, groupID, platform)
	if err != nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	diag := ModelAvailabilityDiagnosis{}
	for i := range accounts {
		diag.HasAccountsInPool = true
		if accounts[i].IsModelSupported(requestedModel) {
			diag.HasModelSupport = true
			return diag
		}
	}
	return diag
}
