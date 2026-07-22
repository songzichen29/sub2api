package service

import (
	"context"
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
