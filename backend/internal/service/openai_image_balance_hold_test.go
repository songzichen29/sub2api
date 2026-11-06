package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAIImageHoldRepoStub struct {
	UsageBillingRepository

	reserveCmd *BatchImageBalanceHoldCommand
	releaseCmd *BatchImageBalanceHoldCommand
	reserveErr error
	releaseErr error
}

func (s *openAIImageHoldRepoStub) ReserveBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	s.reserveCmd = cmd
	if s.reserveErr != nil {
		return nil, s.reserveErr
	}
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (s *openAIImageHoldRepoStub) ReleaseBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	s.releaseCmd = cmd
	if s.releaseErr != nil {
		return nil, s.releaseErr
	}
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func TestReserveOpenAIImageBalance_FreezesMaximumConfiguredTier(t *testing.T) {
	price1K, price2K, price4K := 0.8, 1.0, 1.5
	groupID := int64(64)
	repo := &openAIImageHoldRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(
		&openAIRecordUsageLogRepoStub{},
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	svc.usageBillingRepo = repo

	apiKey := &APIKey{
		ID:      574,
		GroupID: &groupID,
		User:    &User{ID: 392},
		Group: &Group{
			ID:                   groupID,
			RateMultiplier:       1,
			ImageRateIndependent: true,
			ImageRateMultiplier:  1,
			ImagePrice1K:         &price1K,
			ImagePrice2K:         &price2K,
			ImagePrice4K:         &price4K,
		},
	}

	hold, err := svc.ReserveOpenAIImageBalance(context.Background(), apiKey, nil, &OpenAIImagesRequest{
		Model: "gpt-image-2",
		N:     2,
	}, ChannelUsageFields{OriginalModel: "gpt-image-2", ChannelMappedModel: "gpt-image-2"})
	require.NoError(t, err)
	require.NotNil(t, hold)
	require.InDelta(t, 3.0, hold.Amount, 0.00000001)
	require.NotNil(t, repo.reserveCmd)
	require.Equal(t, BatchImageHoldRequestID(hold.ID), repo.reserveCmd.RequestID)
	require.InDelta(t, hold.Amount, repo.reserveCmd.HoldAmount, 0.00000001)
}

func TestReserveOpenAIImageBalance_MapsInsufficientBalance(t *testing.T) {
	price := 1.5
	groupID := int64(64)
	repo := &openAIImageHoldRepoStub{reserveErr: ErrBatchImageInsufficientBalance}
	svc := newOpenAIRecordUsageServiceForTest(
		&openAIRecordUsageLogRepoStub{},
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	svc.usageBillingRepo = repo

	_, err := svc.ReserveOpenAIImageBalance(context.Background(), &APIKey{
		ID:      574,
		GroupID: &groupID,
		User:    &User{ID: 392},
		Group: &Group{
			ID:                   groupID,
			RateMultiplier:       1,
			ImageRateIndependent: true,
			ImageRateMultiplier:  1,
			ImagePrice4K:         &price,
		},
	}, nil, &OpenAIImagesRequest{Model: "gpt-image-2", N: 1}, ChannelUsageFields{OriginalModel: "gpt-image-2"})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestReleaseOpenAIImageBalance_UsesCompetingSettlementIdentity(t *testing.T) {
	repo := &openAIImageHoldRepoStub{}
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	apiKey := &APIKey{ID: 574, User: &User{ID: 392}}
	hold := &OpenAIImageBalanceHold{ID: "hold-1", Amount: 1.5}

	require.NoError(t, svc.ReleaseOpenAIImageBalance(context.Background(), apiKey, hold))
	require.NotNil(t, repo.releaseCmd)
	require.Equal(t, hold.settlementRequestID(), repo.releaseCmd.RequestID)
	require.Equal(t, hold.settlementFingerprint("release"), repo.releaseCmd.RequestFingerprint)

	repo.releaseErr = ErrUsageBillingRequestConflict
	require.NoError(t, svc.ReleaseOpenAIImageBalance(context.Background(), apiKey, hold))

	repo.releaseErr = errors.New("db unavailable")
	require.ErrorIs(t, svc.ReleaseOpenAIImageBalance(context.Background(), apiKey, hold), ErrBillingServiceUnavailable)
}

func TestBuildUsageBillingCommand_UsesFrozenBalanceInsteadOfSecondDeduction(t *testing.T) {
	groupID := int64(64)
	hold := &OpenAIImageBalanceHold{ID: "hold-1", Amount: 1.5}
	cmd := buildUsageBillingCommand("usage-1", &UsageLog{}, &postUsageBillingParams{
		Cost:             &CostBreakdown{ActualCost: 0.8},
		User:             &User{ID: 392},
		APIKey:           &APIKey{ID: 574, GroupID: &groupID},
		Account:          &Account{ID: 1},
		ImageBalanceHold: hold,
	})

	require.Equal(t, hold.ID, cmd.BalanceHoldID)
	require.InDelta(t, hold.Amount, cmd.BalanceHoldAmount, 0.00000001)
	require.Equal(t, hold.settlementRequestID(), cmd.BalanceHoldSettlementID)
	require.Equal(t, hold.settlementFingerprint("capture"), cmd.BalanceHoldSettlementFingerprint)
	require.InDelta(t, 0.8, cmd.BalanceCost, 0.00000001)
}

func TestOpenAIImageBalanceHold_SettlementFingerprintFitsBillingLedger(t *testing.T) {
	hold := &OpenAIImageBalanceHold{ID: "944e1749-4525-4c19-85e6-65df969da1ce", Amount: 0.3}
	for _, outcome := range []string{"capture", "release"} {
		t.Run(outcome, func(t *testing.T) {
			fingerprint := hold.settlementFingerprint(outcome)
			require.Len(t, fingerprint, 64, "usage_billing_dedup.request_fingerprint is VARCHAR(64)")
			expected := sha256.Sum256([]byte("openai-image-hold:" + outcome + ":" + hold.ID + ":0.3000000000"))
			require.Equal(t, hex.EncodeToString(expected[:]), fingerprint)
			require.Equal(t, fingerprint, hold.settlementFingerprint(outcome))
		})
	}
	require.NotEqual(t, hold.settlementFingerprint("capture"), hold.settlementFingerprint("release"))
}

func TestOpenAIGatewayServiceRecordUsage_SettlesImageBalanceHold(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price float64
	}{
		{name: "charged_image", price: 0.18},
		{name: "free_image", price: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(69)
			usageRepo := &openAIRecordUsageLogRepoStub{}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			cache := &openAIImageHoldCacheStub{}
			svc.billingCacheService = &BillingCacheService{cache: cache}
			hold := &OpenAIImageBalanceHold{ID: "944e1749-4525-4c19-85e6-65df969da1ce", Amount: 0.3}
			user := &User{ID: 61}
			apiKey := &APIKey{
				ID:      898,
				User:    user,
				GroupID: &groupID,
				Group: &Group{
					ID:             groupID,
					RateMultiplier: 1,
					ImagePrice2K:   &tc.price,
				},
			}

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID:  "image-hold-regression",
					Model:      "gpt-image-2",
					ImageCount: 1,
					ImageSize:  ImageBillingSize2K,
				},
				APIKey:           apiKey,
				User:             user,
				Account:          &Account{ID: 4481},
				ImageBalanceHold: hold,
			})

			require.NoError(t, err)
			require.Equal(t, 1, billingRepo.calls)
			require.NotNil(t, billingRepo.lastCmd)
			require.Equal(t, hold.ID, billingRepo.lastCmd.BalanceHoldID)
			require.InDelta(t, hold.Amount, billingRepo.lastCmd.BalanceHoldAmount, 1e-8)
			require.Equal(t, hold.settlementRequestID(), billingRepo.lastCmd.BalanceHoldSettlementID)
			require.Equal(t, hold.settlementFingerprint("capture"), billingRepo.lastCmd.BalanceHoldSettlementFingerprint)
			require.InDelta(t, tc.price, billingRepo.lastCmd.BalanceCost, 1e-8)
			require.Zero(t, userRepo.deductCalls, "the hold must be settled by the atomic billing repository")
			require.Equal(t, []int64{user.ID}, cache.invalidatedUsers)
			require.Zero(t, cache.deductCalls, "settling a hold must not deduct the cached available balance again")
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, tc.price, usageRepo.lastLog.ActualCost, 1e-8)
		})
	}
}

type openAIImageHoldCacheStub struct {
	BillingCache
	invalidatedUsers []int64
	deductCalls      int
}

func (s *openAIImageHoldCacheStub) InvalidateUserBalance(_ context.Context, userID int64) error {
	s.invalidatedUsers = append(s.invalidatedUsers, userID)
	return nil
}

func (s *openAIImageHoldCacheStub) DeductUserBalance(_ context.Context, _ int64, _ float64) error {
	s.deductCalls++
	return nil
}
