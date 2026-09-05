package service

import (
	"net/http"
	"time"
)

// TelemetryHook provides lightweight integration between gateway and telemetry service.
// It's designed to be called from GatewayService.Forward() and GatewayHandler.Messages().
type TelemetryHook struct {
	svc *TelemetryService
}

// NewTelemetryHook creates a hook that wraps a TelemetryService.
func NewTelemetryHook(svc *TelemetryService) *TelemetryHook {
	if svc == nil || !svc.cfg.Enabled {
		return &TelemetryHook{} // no-op
	}
	return &TelemetryHook{svc: svc}
}

// OnSessionStart should be called when a new session begins for an OAuth account.
// The deviceID/sessionID must come from the final metadata.user_id so telemetry,
// request body, and X-Claude-Code-Session-Id are one coherent identity.
func (h *TelemetryHook) OnSessionStart(accountID int64, deviceID, sessionID, accountUUID, model string) (string, string) {
	if h.svc == nil {
		return "", ""
	}
	ses := h.svc.EnsureSession(accountID, deviceID, sessionID, accountUUID, model)
	return ses.SessionID, ses.DeviceID
}

// OnSessionEnd sends tengu_exit for an account.
func (h *TelemetryHook) OnSessionEnd(accountID int64) {
	if h.svc == nil {
		return
	}
	h.svc.MarkExit(accountID)
}

// OnAPIQuery sends telemetry before an API call.
func (h *TelemetryHook) OnAPIQuery(accountID int64, deviceID, sessionID, model, accountUUID, token string) {
	if h.svc == nil {
		return
	}
	h.svc.Send(TelemetryEvent{
		EventName:   "tengu_api_query",
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra: map[string]any{
			"stream": true,
		},
		Timestamp: time.Now(),
		Token:     token,
	})
}

// OnAPIResponse sends telemetry after an API call.
func (h *TelemetryHook) OnAPIResponse(accountID int64, deviceID, sessionID, model, accountUUID, token string, success bool, durationMs float64, statusCode int, tokenCount int64) {
	if h.svc == nil {
		return
	}
	eventName := "tengu_api_success"
	if !success {
		eventName = "tengu_api_error"
	}
	h.svc.Send(TelemetryEvent{
		EventName:   eventName,
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra: map[string]any{
			"duration_ms":  durationMs,
			"status_code":  statusCode,
			"input_tokens": tokenCount,
		},
		Timestamp: time.Now(),
		Token:     token,
	})
}

// OnToolUse sends telemetry for tool usage.
func (h *TelemetryHook) OnToolUse(accountID int64, deviceID, sessionID, model, accountUUID, toolName string, success bool) {
	if h.svc == nil {
		return
	}
	eventName := "tengu_tool_use_success"
	if !success {
		eventName = "tengu_tool_use_error"
	}
	h.svc.Send(TelemetryEvent{
		EventName:   eventName,
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra: map[string]any{
			"tool_name": toolName,
		},
		Timestamp: time.Now(),
	})
}

// EnsureTelemetryStarted is the main integration point called from GatewayService.Forward().
// Returns sessionID and deviceID for use in subsequent events.
func EnsureTelemetryStarted(h *TelemetryHook, accountID int64, deviceID, sessionID, accountUUID, model string) (string, string) {
	if h == nil {
		return "", ""
	}
	return h.OnSessionStart(accountID, deviceID, sessionID, accountUUID, model)
}

// RecordAPIStart records telemetry before the upstream HTTP call.
func RecordAPIStart(h *TelemetryHook, accountID int64, deviceID, sessionID, model, accountUUID, token string) {
	if h == nil {
		return
	}
	h.OnAPIQuery(accountID, deviceID, sessionID, model, accountUUID, token)
}

// RecordAPIEnd records telemetry after the upstream HTTP call.
func RecordAPIEnd(h *TelemetryHook, accountID int64, deviceID, sessionID, model, accountUUID, token string, success bool, durationMs float64, statusCode int, tokenCount int64) {
	if h == nil {
		return
	}
	h.OnAPIResponse(accountID, deviceID, sessionID, model, accountUUID, token, success, durationMs, statusCode, tokenCount)
}

// statusCodeFromResp extracts the HTTP status code safely.
func statusCodeFromResp(resp *http.Response, err error) int { //nolint:unused // retained for alternate telemetry transports
	if resp != nil {
		return resp.StatusCode
	}
	return 0
}

// EnqueueGrowthbookExperiments reports GrowthBook experiment exposures to the
// 1P event log as GrowthbookExperimentEvent for the account.
func (h *TelemetryHook) EnqueueGrowthbookExperiments(accountID int64, token, deviceID, sessionID, accountUUID string, exposures []growthbookExposure) {
	if h.svc == nil {
		return
	}
	h.svc.EnqueueGrowthbookExperiments(accountID, token, deviceID, sessionID, accountUUID, exposures)
}
