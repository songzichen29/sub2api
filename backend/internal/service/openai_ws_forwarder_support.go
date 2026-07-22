package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) selectAccountByPreviousResponseIDForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) (*AccountSelectionResult, error) {
	if s == nil {
		return nil, nil
	}
	accountID, account, responseID, store := s.resolveAccountByPreviousResponseIDForCapability(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	if accountID <= 0 || account == nil || store == nil {
		return nil, nil
	}

	result, acquireErr := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if acquireErr == nil && result.Acquired {
		logOpenAIWSBindResponseAccountWarn(
			derefGroupID(groupID),
			accountID,
			responseID,
			store.BindResponseAccount(ctx, derefGroupID(groupID), responseID, accountID, s.openAIWSResponseStickyTTL()),
		)
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, nil
	}

	cfg := s.schedulingConfig()
	if s.concurrencyService != nil {
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
			},
		}, nil
	}
	return nil, nil
}

func (s *OpenAIGatewayService) ResolveAccountIDByPreviousResponseIDForScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) int64 {
	accountID, _, _, _ := s.resolveAccountByPreviousResponseIDForCapability(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	return accountID
}

func (s *OpenAIGatewayService) resolveAccountByPreviousResponseIDForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) (int64, *Account, string, OpenAIWSStateStore) {
	if s == nil {
		return 0, nil, "", nil
	}
	responseID := strings.TrimSpace(previousResponseID)
	if responseID == "" {
		return 0, nil, "", nil
	}
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return 0, nil, "", nil
	}

	accountID, err := store.GetResponseAccount(ctx, derefGroupID(groupID), responseID)
	if err != nil || accountID <= 0 {
		return 0, nil, "", nil
	}
	if excludedIDs != nil {
		if _, excluded := excludedIDs[accountID]; excluded {
			return 0, nil, "", nil
		}
	}

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
	}
	if s.getOpenAIWSProtocolResolver().Resolve(account).Transport != OpenAIUpstreamTransportResponsesWebsocketV2 {
		return 0, nil, "", nil
	}
	if shouldClearStickySession(account, requestedModel) || !account.IsOpenAI() || !account.IsSchedulable() {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
	}
	if !parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
	}
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return 0, nil, "", nil
	}
	if !account.SupportsOpenAIEndpointCapability(requiredCapability) {
		return 0, nil, "", nil
	}
	if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
		return 0, nil, "", nil
	}
	if s.schedulerSnapshot != nil && s.accountRepo != nil {
		latest, latestErr := s.accountRepo.GetByID(ctx, account.ID)
		if latestErr != nil || latest == nil {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
		}
		if shouldClearStickySession(latest, requestedModel) || !latest.IsOpenAI() || !latest.IsSchedulable() {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
		}
		if !parentHealthyForShadow(latest, s.parentAccountLookup(ctx)) {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
		}
		if requestedModel != "" && !latest.IsModelSupported(requestedModel) {
			return 0, nil, "", nil
		}
		if !latest.SupportsOpenAIEndpointCapability(requiredCapability) {
			return 0, nil, "", nil
		}
		if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, latest); paused {
			return 0, nil, "", nil
		}
		if s.isOpenAIAccountRequestRuntimeBlocked(latest, requestedModel) {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
		}
		account = latest
	}
	if requireCompact && openAICompactSupportTier(account) == 0 {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
	}
	return accountID, account, responseID, store
}

func normalizeOpenAIWSTerminalEvent(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "response.completed":
		return "response.completed"
	case "response.done":
		return "response.done"
	case "response.failed":
		return "response.failed"
	case "response.incomplete":
		return "response.incomplete"
	case "response.cancelled", "response.canceled":
		return "response.cancelled"
	default:
		return ""
	}
}

func openAIWSPayloadTransientStatus(payload []byte) int {
	if len(payload) == 0 {
		return 0
	}
	status := int(gjson.GetBytes(payload, "response.error.status_code").Int())
	if status == 0 {
		status = int(gjson.GetBytes(payload, "response.error.status").Int())
	}
	if status == 0 {
		status = int(gjson.GetBytes(payload, "error.status_code").Int())
	}
	if status == 0 {
		status = int(gjson.GetBytes(payload, "error.status").Int())
	}
	if shouldCooldownOpenAITransientUpstreamError(status, payload) {
		return status
	}
	if status != 0 {
		return 0
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	switch {
	case code == "server_is_overloaded", code == "slow_down":
		return http.StatusServiceUnavailable
	case strings.Contains(code, "server_error"),
		strings.Contains(code, "internal_error"),
		strings.Contains(code, "upstream_error"),
		strings.Contains(errType, "server_error"),
		strings.Contains(errType, "internal_error"),
		strings.Contains(errType, "upstream_error"):
		return http.StatusInternalServerError
	default:
		return 0
	}
}

func (s *OpenAIGatewayService) handleOpenAIWSTerminalTransientFailure(ctx context.Context, account *Account, canonicalModel string, headers http.Header, payload []byte) string {
	eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
	terminalEvent := normalizeOpenAIWSTerminalEvent(eventType)
	if terminalEvent != "response.failed" {
		return terminalEvent
	}
	status := openAIWSPayloadTransientStatus(payload)
	if status != 0 {
		s.handleOpenAIAccountUpstreamError(ctx, account, status, headers, payload, canonicalModel)
	}
	return terminalEvent
}

func (s *OpenAIGatewayService) handleOpenAIWSErrorEventTransientFailure(ctx context.Context, account *Account, canonicalModel string, headers http.Header, payload []byte) {
	eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
	if eventType != "error" {
		return
	}
	status := openAIWSPayloadTransientStatus(payload)
	if status != 0 {
		s.handleOpenAIAccountUpstreamError(ctx, account, status, headers, payload, canonicalModel)
	}
}

func (s *OpenAIGatewayService) handleOpenAIWSDialTransientFailure(ctx context.Context, account *Account, canonicalModel string, err error) {
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) || dialErr == nil || !shouldCooldownOpenAITransientUpstreamError(dialErr.StatusCode, dialErr.ResponseBody) {
		return
	}
	s.handleOpenAIAccountUpstreamError(ctx, account, dialErr.StatusCode, dialErr.ResponseHeaders, dialErr.ResponseBody, canonicalModel)
}
