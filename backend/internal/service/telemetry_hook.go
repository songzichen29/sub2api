package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// It sends tengu_started and the feature sequence asynchronously.
func (h *TelemetryHook) OnSessionStart(accountID int64, accountUUID, model string) (sessionID, deviceID string) {
	if h.svc == nil {
		return "", ""
	}
	deviceID = deriveDeviceID(accountID)
	ses := h.svc.EnsureSession(accountID, deviceID, accountUUID, model)
	return ses.SessionID, deviceID
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
		Extra: map[string]interface{}{
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
		Extra: map[string]interface{}{
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
		Extra: map[string]interface{}{
			"tool_name": toolName,
		},
		Timestamp: time.Now(),
	})
}

// deriveDeviceID generates a stable device ID for an account.
func deriveDeviceID(accountID int64) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("sub2api-device-seed-2026-%d", accountID)))
	return hex.EncodeToString(hash[:])
}

// EnsureTelemetryStarted is the main integration point called from GatewayService.Forward().
// It should be called at the beginning of Forward, for OAuth accounts with mimicClaudeCode=true.
// Returns sessionID and deviceID for use in subsequent events.
func EnsureTelemetryStarted(h *TelemetryHook, accountID int64, accountUUID, model string) (sessionID, deviceID string) {
	if h == nil {
		return "", ""
	}
	return h.OnSessionStart(accountID, accountUUID, model)
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
func statusCodeFromResp(resp *http.Response, err error) int {
	if resp != nil {
		return resp.StatusCode
	}
	return 0
}

// EnqueueGrowthbookExperiments reports GrowthBook experiment exposures to the
// 1P event log as GrowthbookExperimentEvent for the account.
func (h *TelemetryHook) EnqueueGrowthbookExperiments(token, deviceID, sessionID, accountUUID string, exposures []growthbookExposure) {
	if h.svc == nil {
		return
	}
	h.svc.EnqueueGrowthbookExperiments(token, deviceID, sessionID, accountUUID, exposures)
}
