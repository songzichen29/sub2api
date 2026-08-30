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
