package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestTelemetryBuildPayloadMatchesClaudeCodeSampleShape(t *testing.T) {
	svc := NewTelemetryService(TelemetryConfig{Enabled: true})
	ev := svc.buildPayload(TelemetryEvent{
		EventName:   "tengu_api_query",
		AccountID:   42,
		DeviceID:    "727009cf65d616c86958250e3bb82cb3d895c243bb57cbf782bf744e47746378",
		SessionID:   "ca6a940b-53f1-4ce4-8b6f-a6dd15e3be7f",
		Model:       "claude-opus-4-6",
		AccountUUID: "acct_123",
		Extra: map[string]interface{}{
			"stream": true,
		},
		Timestamp: time.Date(2026, 6, 30, 15, 51, 15, 582*1e6, time.UTC),
	})

	b, err := json.Marshal(ev)
	require.NoError(t, err)
	t.Logf("telemetry sample: %s", string(b))

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &top))
	require.JSONEq(t, `"ClaudeCodeInternalEvent"`, string(top["event_type"]))
	require.NotContains(t, top, "core")
	require.Contains(t, top, "event_data")

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["event_data"], &data))
	require.NotContains(t, data, "core")
	require.NotContains(t, data, "additional")
	require.Contains(t, data, "process")
	require.Contains(t, data, "additional_metadata")

	var eventName, ts, betas, processB64, additionalB64, deviceID, sessionID string
	require.NoError(t, json.Unmarshal(data["event_name"], &eventName))
	require.NoError(t, json.Unmarshal(data["client_timestamp"], &ts))
	require.NoError(t, json.Unmarshal(data["betas"], &betas))
	require.NoError(t, json.Unmarshal(data["process"], &processB64))
	require.NoError(t, json.Unmarshal(data["additional_metadata"], &additionalB64))
	require.NoError(t, json.Unmarshal(data["device_id"], &deviceID))
	require.NoError(t, json.Unmarshal(data["session_id"], &sessionID))

	require.Equal(t, "tengu_api_query", eventName)
	require.Equal(t, "2026-06-30T15:51:15.582Z", ts)
	require.Equal(t, "727009cf65d616c86958250e3bb82cb3d895c243bb57cbf782bf744e47746378", deviceID)
	require.Equal(t, "ca6a940b-53f1-4ce4-8b6f-a6dd15e3be7f", sessionID)
	require.Equal(t, "claude-code-20250219,context-1m-2025-08-07,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07", betas)

	var env telemetryEnv
	require.NoError(t, json.Unmarshal(data["env"], &env))
	require.Equal(t, "linux", env.Platform)
	require.Equal(t, "linux", env.PlatformRaw)
	require.Equal(t, "x64", env.Arch)
	require.Equal(t, "v26.3.0", env.NodeVersion)
	require.True(t, env.IsRunningWithBun)
	require.Equal(t, claude.CLICurrentVersion, env.Version)
	require.Equal(t, claude.CLICurrentVersion, env.VersionBase)
	require.Equal(t, claude.CLIBuildTime, env.BuildTime)
	require.Equal(t, "unknown-linux", env.DeploymentEnvironment)
	require.Equal(t, "gnome-terminal", env.Terminal)
	require.Equal(t, "bash", env.Shell)

	processJSON, err := base64.StdEncoding.DecodeString(processB64)
	require.NoError(t, err)
	var process map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(processJSON, &process))
	for _, key := range []string{"uptime", "rss", "heapTotal", "heapUsed", "external", "arrayBuffers", "constrainedMemory", "cpuUsage"} {
		require.Contains(t, process, key)
	}

	additionalJSON, err := base64.StdEncoding.DecodeString(additionalB64)
	require.NoError(t, err)
	var additional map[string]interface{}
	require.NoError(t, json.Unmarshal(additionalJSON, &additional))
	require.Equal(t, "fullscreen", additional["renderer_mode"])
	require.Equal(t, true, additional["stream"])
}

func TestTelemetryEndpointMatchesClaudeCode(t *testing.T) {
	require.Equal(t, "/api/event_logging/v2/batch", telemetryEventEndpoint)
}

func TestGroupTelemetryEventsByAccountPreservesAccountScopedBatches(t *testing.T) {
	events := []telemetryWireEvent{
		{AccountID: 2, EventType: "ClaudeCodeInternalEvent"},
		{AccountID: 1, EventType: "ClaudeCodeInternalEvent"},
		{AccountID: 2, EventType: "ClaudeCodeInternalEvent"},
		{AccountID: 0, EventType: "ClaudeCodeInternalEvent"},
		{AccountID: 1, EventType: "ClaudeCodeInternalEvent"},
	}

	groups := groupTelemetryEventsByAccount(events)
	require.Len(t, groups, 3)
	require.Equal(t, int64(2), groups[0].accountID)
	require.Len(t, groups[0].events, 2)
	require.Equal(t, int64(1), groups[1].accountID)
	require.Len(t, groups[1].events, 2)
	require.Equal(t, int64(0), groups[2].accountID)
	require.Len(t, groups[2].events, 1)
}
