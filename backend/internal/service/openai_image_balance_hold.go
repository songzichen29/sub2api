package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
)

// OpenAIImageBalanceHold is an amount atomically frozen before a normal Images
// request reaches the upstream provider. Its ID is private billing state and is
// consumed exactly once by the later usage settlement transaction.
type OpenAIImageBalanceHold struct {
	ID     string
	Amount float64
}

func (h *OpenAIImageBalanceHold) settlementRequestID() string {
	if h == nil {
		return ""
	}
	return BatchImageCaptureRequestID(h.ID)
}

func (h *OpenAIImageBalanceHold) settlementFingerprint(outcome string) string {
	if h == nil {
		return ""
	}
	return "openai-image-hold:" + strings.TrimSpace(outcome) + ":" + strings.TrimSpace(h.ID) + ":" + strconv.FormatFloat(h.Amount, 'f', 10, 64)
}

// ReserveOpenAIImageBalance freezes the maximum known per-image charge for a
// balance-billed OpenAI Images request. The database conditional update is the
// admission decision: a stale cache can no longer admit multiple requests that
// collectively cost more than the user's available balance.
func (s *OpenAIGatewayService) ReserveOpenAIImageBalance(
	ctx context.Context,
	apiKey *APIKey,
	subscription *UserSubscription,
	request *OpenAIImagesRequest,
	usageFields ChannelUsageFields,
) (*OpenAIImageBalanceHold, error) {
	if s == nil || apiKey == nil || apiKey.User == nil || request == nil {
		return nil, nil
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, nil
	}
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() && subscription != nil {
		return nil, nil
	}
	if s.usageBillingRepo == nil || s.billingService == nil {
		return nil, ErrBillingServiceUnavailable.WithCause(errors.New("openai image balance hold dependencies are not configured"))
	}

	holdAmount, supported := s.estimateOpenAIImageBalanceHold(ctx, apiKey, request, usageFields)
	if !supported || holdAmount <= 0 {
		return nil, nil
	}

	hold := &OpenAIImageBalanceHold{
		ID:     uuid.NewString(),
		Amount: holdAmount,
	}
	_, err := s.usageBillingRepo.ReserveBatchImageBalance(ctx, &BatchImageBalanceHoldCommand{
		RequestID:  BatchImageHoldRequestID(hold.ID),
		APIKeyID:   apiKey.ID,
		UserID:     apiKey.User.ID,
		BatchID:    hold.ID,
		HoldAmount: hold.Amount,
	})
	if err != nil {
		if errors.Is(err, ErrBatchImageInsufficientBalance) {
			return nil, ErrInsufficientBalance
		}
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	s.invalidateOpenAIImageBalanceCache(ctx, apiKey.User.ID)
	return hold, nil
}

// ReleaseOpenAIImageBalance returns a hold when no successful image result is
// available to settle. A release is idempotent through usage_billing_dedup.
func (s *OpenAIGatewayService) ReleaseOpenAIImageBalance(ctx context.Context, apiKey *APIKey, hold *OpenAIImageBalanceHold) error {
	if s == nil || apiKey == nil || apiKey.User == nil || hold == nil || hold.Amount <= 0 || strings.TrimSpace(hold.ID) == "" {
		return nil
	}
	if s.usageBillingRepo == nil {
		return ErrBillingServiceUnavailable.WithCause(errors.New("openai image balance hold repository is not configured"))
	}
	_, err := s.usageBillingRepo.ReleaseBatchImageBalance(ctx, &BatchImageBalanceHoldCommand{
		RequestID:          hold.settlementRequestID(),
		RequestFingerprint: hold.settlementFingerprint("release"),
		APIKeyID:           apiKey.ID,
		UserID:             apiKey.User.ID,
		BatchID:            hold.ID,
		HoldAmount:         hold.Amount,
	})
	if err != nil && !errors.Is(err, ErrUsageBillingRequestConflict) {
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	s.invalidateOpenAIImageBalanceCache(ctx, apiKey.User.ID)
	return nil
}

// estimateOpenAIImageBalanceHold returns a conservative upper bound for pricing
// modes that are known before upstream dispatch. Token-priced image channels are
// intentionally excluded because their final output-token count is not bounded
// by this API request; their admission remains on the existing token path.
func (s *OpenAIGatewayService) estimateOpenAIImageBalanceHold(
	ctx context.Context,
	apiKey *APIKey,
	request *OpenAIImagesRequest,
	usageFields ChannelUsageFields,
) (float64, bool) {
	if s == nil || s.billingService == nil || apiKey == nil || apiKey.User == nil || request == nil {
		return 0, false
	}
	if groupMediaPricingLooksIncomplete(apiKey.Group) {
		return 0, false
	}

	models := openAIImageHoldBillingModels(request.Model, usageFields)
	if len(models) == 0 {
		return 0, false
	}
	if usageFields.BillingModelSource == BillingModelSourceUpstream && !apiKeyHasAnyConfiguredImagePrice(apiKey) {
		return 0, false
	}

	baseMultiplier := 1.0
	if s.cfg != nil {
		baseMultiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		resolver := s.userGroupRateResolver
		if resolver == nil {
			resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_image_hold")
		}
		baseMultiplier = resolver.Resolve(ctx, apiKey.User.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}
	_, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, timezone.Now())

	imageCount := request.N
	if imageCount <= 0 {
		imageCount = 1
	}
	maxCost := 0.0
	for _, model := range models {
		if resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey); resolved != nil && resolved.Mode == BillingModeToken {
			// The actual charge is output-token based and cannot be capped by n/size.
			return 0, false
		}
		for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
			cost := s.calculateOpenAIImageCost(ctx, model, apiKey, &OpenAIForwardResult{
				Model:      model,
				ImageCount: imageCount,
				ImageSize:  tier,
			}, imageMultiplier)
			if cost != nil && cost.ActualCost > maxCost {
				maxCost = cost.ActualCost
			}
		}
	}
	return maxCost, maxCost > 0
}

func openAIImageHoldBillingModels(requestedModel string, usageFields ChannelUsageFields) []string {
	models := make([]string, 0, 2)
	appendModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		for _, existing := range models {
			if strings.EqualFold(existing, model) {
				return
			}
		}
		models = append(models, model)
	}
	switch usageFields.BillingModelSource {
	case BillingModelSourceRequested:
		appendModel(usageFields.OriginalModel)
	case BillingModelSourceChannelMapped:
		appendModel(usageFields.ChannelMappedModel)
	default:
		appendModel(requestedModel)
		appendModel(usageFields.ChannelMappedModel)
	}
	appendModel(requestedModel)
	return models
}

func apiKeyHasAnyConfiguredImagePrice(apiKey *APIKey) bool {
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	return apiKey.Group.ImagePrice1K != nil || apiKey.Group.ImagePrice2K != nil || apiKey.Group.ImagePrice4K != nil
}

func (s *OpenAIGatewayService) invalidateOpenAIImageBalanceCache(ctx context.Context, userID int64) {
	if s == nil || s.billingCacheService == nil || userID <= 0 {
		return
	}
	if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
		logger.LegacyPrintf("service.openai_image_hold", "invalidate balance cache failed user=%d: %v", userID, err)
	}
}
