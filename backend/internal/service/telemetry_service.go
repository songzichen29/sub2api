package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const (
	// telemetryBatchInterval is the default interval between batch sends.
	telemetryBatchInterval = 10 * time.Second

	// telemetryMaxBatchSize is max events per POST.
	telemetryMaxBatchSize = 200

	// telemetryEventEndpoint is the 1P event logging endpoint.
	telemetryEventEndpoint = "/api/event_logging/batch"

	// telemetryAPITimeout for POST requests.
	telemetryAPITimeout = 10 * time.Second

	// telemetryMaxQueueSize is max pending events.
	telemetryMaxQueueSize = 8192

	// telemetryMaxRetries for failed sends.
	telemetryMaxRetries = 8

	// telemetryBackoffBase in ms.
	telemetryBackoffBase = 500

	// telemetryBackoffMax in ms.
	telemetryBackoffMax = 30000
)

// TelemetryEvent represents a single internal telemetry event before formatting.
type TelemetryEvent struct {
	EventName   string                 `json:"event_name"`
	AccountID   int64                  `json:"-"` // for routing
	DeviceID    string                 `json:"-"` // from identity service
	SessionID   string                 `json:"-"`
	Model       string                 `json:"-"`
	AccountUUID string                 `json:"-"`
	OrgUUID     string                 `json:"-"`
	UserType    string                 `json:"-"`
	Extra       map[string]interface{} `json:"-"`
	Timestamp   time.Time              `json:"-"`
}

// TelemetryConfig holds configuration for the telemetry service.
type TelemetryConfig struct {
	BaseURL         string
	Enabled         bool
	Token           string // OAuth access token for auth
	TokenType       string // "oauth" or "apikey"
	FlushIntervalMS int
	MaxBatchSize    int
	MaxRetries      int
}

// TelemetryService manages periodic batching and sending of telemetry events.
type TelemetryService struct {
	cfg      TelemetryConfig
	eventCh  chan TelemetryEvent
	done     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	batch    []telemetryWireEvent
	attempts int
	// Per-account session tracking
	sessionsMu sync.RWMutex
	sessions   map[int64]*TelemetrySession
	// Process metrics simulator state
	processStart time.Time
	baseMemory   int64
}

// TelemetrySession tracks state for a single account's "Claude Code session".
type TelemetrySession struct {
	AccountID    int64
	SessionID    string
	DeviceID     string
	Model        string
	StartedAt    time.Time
	APIRequests  int64
	FeaturesSent bool
	ExitSent     bool
}

// telemetryWireEvent is the JSON shape sent to the endpoint.
type telemetryWireEvent struct {
	EventType string           `json:"event_type"`
	EventData telemetryPayload `json:"event_data"`
}

type telemetryPayload struct {
	EventName          string          `json:"event_name,omitempty"`
	EventID            string          `json:"event_id"`
	ClientTimestamp    string          `json:"client_timestamp"`
	DeviceID           string          `json:"device_id,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	Model              string          `json:"model,omitempty"`
	UserType           string          `json:"user_type,omitempty"`
	Betas              string          `json:"betas,omitempty"`
	Entrypoint         string          `json:"entrypoint,omitempty"`
	IsInteractive      bool            `json:"is_interactive"`
	ClientType         string          `json:"client_type,omitempty"`
	Env                *telemetryEnv   `json:"env,omitempty"`
	Process            string          `json:"process,omitempty"` // base64(processMetricsJSON)
	Auth               *telemetryAuth  `json:"auth,omitempty"`
	AdditionalMetadata string          `json:"additional_metadata,omitempty"` // base64(json)
	Core               telemetryCore   `json:"core,omitempty"`
	Email              string          `json:"email,omitempty"`
}

type telemetryCore struct {
	SessionID   string `json:"session_id"`
	Model       string `json:"model"`
	UserType    string `json:"user_type"`
	Betas       string `json:"betas,omitempty"`
	IsInteractive bool   `json:"is_interactive"`
	ClientType  string `json:"client_type"`
}

type telemetryEnv struct {
	Platform              string   `json:"platform"`
	PlatformRaw           string   `json:"platform_raw"`
	Arch                  string   `json:"arch"`
	NodeVersion           string   `json:"node_version"`
	Terminal              string   `json:"terminal"`
	PackageManagers       string   `json:"package_managers"`
	Runtimes              string   `json:"runtimes"`
	IsRunningWithBun      bool     `json:"is_running_with_bun"`
	IsCi                  bool     `json:"is_ci"`
	IsClaubbit            bool     `json:"is_claubbit"`
	IsGithubAction        bool     `json:"is_github_action"`
	IsClaudeCodeAction    bool     `json:"is_claude_code_action"`
	IsClaudeAiAuth        bool     `json:"is_claude_ai_auth"`
	IsClaudeCodeRemote    bool     `json:"is_claude_code_remote"`
	IsConductor           bool     `json:"is_conductor"`
	IsLocalAgentMode      bool     `json:"is_local_agent_mode"`
	Version               string   `json:"version"`
	VersionBase           string   `json:"version_base,omitempty"`
	BuildTime             string   `json:"build_time"`
	DeploymentEnvironment string   `json:"deployment_environment"`
	Shell                 string   `json:"shell,omitempty"`
	Vcs                   string   `json:"vcs,omitempty"`
	LinuxDistroID         string   `json:"linux_distro_id,omitempty"`
	LinuxDistroVersion    string   `json:"linux_distro_version,omitempty"`
	LinuxKernel           string   `json:"linux_kernel,omitempty"`
}

type telemetryAuth struct {
	AccountUUID      string `json:"account_uuid,omitempty"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
}

// processMetricsJSON is the shape marshal'd then base64'd into the Process field.
type processMetricsJSON struct {
	Uptime           float64       `json:"uptime"`
	RSS              int64         `json:"rss"`
	HeapTotal        int64         `json:"heapTotal"`
	HeapUsed         int64         `json:"heapUsed"`
	External         int64         `json:"external"`
	ArrayBuffers     int64         `json:"arrayBuffers"`
	ConstrainedMemory int64        `json:"constrainedMemory"`
	CPUUsage         cpuUsage      `json:"cpuUsage"`
	CPUPercent       float64       `json:"cpuPercent"`
}

type cpuUsage struct {
	User   int64 `json:"user"`
	System int64 `json:"system"`
}

// NewTelemetryService creates a telemetry service.
func NewTelemetryService(cfg TelemetryConfig) *TelemetryService {
	if !cfg.Enabled {
		return &TelemetryService{cfg: cfg}
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = telemetryMaxBatchSize
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = telemetryMaxRetries
	}
	s := &TelemetryService{
		cfg:          cfg,
		eventCh:      make(chan TelemetryEvent, telemetryMaxQueueSize),
		done:         make(chan struct{}),
		sessions:     make(map[int64]*TelemetrySession),
		processStart: time.Now(),
		baseMemory:   int64(100 + rand.Intn(80)) << 20, // 100-180 MB base
	}
	return s
}

// Start begins the background batching and sending loop.
func (s *TelemetryService) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		return
	}
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop gracefully shuts down the telemetry service.
func (s *TelemetryService) Stop() {
	if !s.cfg.Enabled {
		return
	}
	close(s.done)
	s.wg.Wait()
}

// Send enqueues a telemetry event (non-blocking, drops on full queue).
func (s *TelemetryService) Send(ev TelemetryEvent) {
	if !s.cfg.Enabled {
		return
	}
	select {
	case s.eventCh <- ev:
	default:
		// Drop on full queue to avoid blocking the gateway.
	}
}

// EnsureSession creates or returns an existing session for this account.
// Also sends tengu_started if this is the first request in the session.
func (s *TelemetryService) EnsureSession(accountID int64, deviceID, accountUUID, model string) *TelemetrySession {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	if ses, ok := s.sessions[accountID]; ok {
		ses.APIRequests++
		return ses
	}

	sessionID := uuid.NewString()
	ses := &TelemetrySession{
		AccountID: accountID,
		SessionID: sessionID,
		DeviceID:  deviceID,
		Model:     model,
		StartedAt: time.Now(),
	}
	s.sessions[accountID] = ses

	// Send tengu_started asynchronously.
	go func() {
		s.Send(TelemetryEvent{
			EventName:   "tengu_started",
			AccountID:   accountID,
			DeviceID:    deviceID,
			SessionID:   sessionID,
			Model:       model,
			AccountUUID: accountUUID,
			Timestamp:   time.Now(),
		})
		// Wait a tiny bit then send feature sequence.
		time.Sleep(50 * time.Millisecond)
		s.sendFeatureSequence(accountID, deviceID, sessionID, model, accountUUID)
	}()

	return ses
}

// MarkExit sends tengu_exit for a session and removes it.
func (s *TelemetryService) MarkExit(accountID int64) {
	s.sessionsMu.Lock()
	ses, ok := s.sessions[accountID]
	if !ok {
		s.sessionsMu.Unlock()
		return
	}
	delete(s.sessions, accountID)
	s.sessionsMu.Unlock()

	sessionDuration := time.Since(ses.StartedAt).Seconds()
	s.Send(TelemetryEvent{
		EventName:   "tengu_exit",
		AccountID:   accountID,
		DeviceID:    ses.DeviceID,
		SessionID:   ses.SessionID,
		Model:       ses.Model,
		AccountUUID: "", // will be filled by buildPayload
		Extra: map[string]interface{}{
			"last_session_duration_sec": math.Round(sessionDuration*10) / 10,
			"last_session_api_requests": ses.APIRequests,
		},
		Timestamp: time.Now(),
	})
}

// OnAPIRequest records that an API request happened (for telemetry events tied to API calls).
func (s *TelemetryService) OnAPIRequest(accountID int64, deviceID, sessionID, model, accountUUID string, success bool, durationMs float64, statusCode int) {
	s.Send(TelemetryEvent{
		EventName:   boolToEvent(success, "tengu_api_success", "tengu_api_error"),
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra: map[string]interface{}{
			"duration_ms": durationMs,
			"status_code": statusCode,
		},
		Timestamp: time.Now(),
	})
}

// sendFeatureSequence sends the canonical Claude Code feature event sequence.
func (s *TelemetryService) sendFeatureSequence(accountID int64, deviceID, sessionID, model, accountUUID string) {
	// Real Claude Code sends these in ~300ms at startup.
	features := []string{
		"skill_load_commands_dir",
		"plugin_load_manifest",
		"plugin_load_settings",
		"plugin_load_all",
		"plugin_load_workflows",
		"plugin_load_hooks",
		"plugin_load_commands",
		"plugin_load_skills",
		"cmd_load",
	}
	for _, f := range features {
		s.Send(TelemetryEvent{
			EventName:   "tengu_feature_ok",
			AccountID:   accountID,
			DeviceID:    deviceID,
			SessionID:   sessionID,
			Model:       model,
			AccountUUID: accountUUID,
			Extra: map[string]interface{}{
				"feature_name": f,
			},
			Timestamp: time.Now(),
		})
		time.Sleep(time.Duration(5+rand.Intn(30)) * time.Millisecond)
	}

	// Also send a few more events that real CLI sends.
	s.Send(TelemetryEvent{
		EventName:   "tengu_shell_set_cwd",
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Timestamp:   time.Now(),
	})
}

// loop is the main background worker.
func (s *TelemetryService) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(telemetryBatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.flush()
			return
		case <-s.done:
			s.flush()
			return
		case ev := <-s.eventCh:
			s.mu.Lock()
			s.batch = append(s.batch, s.buildPayload(ev))
			if len(s.batch) >= s.cfg.MaxBatchSize {
				s.flushLocked()
			}
			s.mu.Unlock()
		case <-ticker.C:
			s.mu.Lock()
			if len(s.batch) > 0 {
				s.flushLocked()
			}
			s.mu.Unlock()
		}
	}
}

func (s *TelemetryService) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.batch) > 0 {
		s.flushLocked()
	}
}

func (s *TelemetryService) flushLocked() {
	if len(s.batch) == 0 {
		return
	}
	events := s.batch
	s.batch = nil
	s.mu.Unlock()

	s.sendBatch(events, 0)

	s.mu.Lock()
}

func (s *TelemetryService) sendBatch(events []telemetryWireEvent, attempt int) {
	if len(events) == 0 {
		return
	}

	payload := map[string]interface{}{
		"events": events,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("telemetry: failed to marshal batch", "error", err)
		return
	}

	baseURL := s.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := baseURL + telemetryEventEndpoint

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-code/"+claude.CLICurrentVersion)
	req.Header.Set("x-service-name", "claude-code")

	// Add auth if available.
	if s.cfg.Token != "" {
		if s.cfg.TokenType == "oauth" {
			req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
		} else {
			req.Header.Set("x-api-key", s.cfg.Token)
		}
	}

	client := &http.Client{Timeout: telemetryAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		if attempt < s.cfg.MaxRetries {
			delay := telemetryBackoffBase * (1 << attempt)
			if delay > telemetryBackoffMax {
				delay = telemetryBackoffMax
			}
			time.Sleep(time.Duration(delay) * time.Millisecond)
			s.sendBatch(events, attempt+1)
		}
		return
	}
	defer resp.Body.Close()

	// On 401, retry once without auth.
	if resp.StatusCode == 401 && s.cfg.Token != "" && attempt == 0 {
		s.cfg.Token = "" // strip auth for retry
		s.sendBatch(events, attempt+1)
		return
	}

	if resp.StatusCode >= 400 {
		if attempt < s.cfg.MaxRetries {
			delay := telemetryBackoffBase * (1 << attempt)
			if delay > telemetryBackoffMax {
				delay = telemetryBackoffMax
			}
			time.Sleep(time.Duration(delay) * time.Millisecond)
			s.sendBatch(events, attempt+1)
		}
	}
}

// buildPayload converts an internal TelemetryEvent to the wire format.
func (s *TelemetryService) buildPayload(ev TelemetryEvent) telemetryWireEvent {
	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	// Derive device ID from account if not set.
	deviceID := ev.DeviceID
	if deviceID == "" {
		deviceID = s.accountDeviceID(ev.AccountID)
	}
	sessionID := ev.SessionID
	if sessionID == "" {
		sessionID = s.accountSessionID(ev.AccountID)
	}

	// Build betas for mimicry.
	betas := claude.FullClaudeCodeMimicryBetas()
	betasStr := ""
	for i, b := range betas {
		if i > 0 {
			betasStr += ","
		}
		betasStr += b
	}

	// Generate realistic process metrics.
	processMetrics := s.generateProcessMetrics()

	p := telemetryPayload{
		EventName:       ev.EventName,
		EventID:         uuid.NewString(),
		ClientTimestamp: now.UTC().Format(time.RFC3339Nano),
		DeviceID:        deviceID,
		SessionID:       sessionID,
		Model:           ev.Model,
		UserType:        "external",
		Betas:           betasStr,
		Entrypoint:      "cli",
		IsInteractive:   true,
		ClientType:      "cli",
		Env:             s.buildEnv(),
		Process:         processMetrics,
		Core: telemetryCore{
			SessionID:     sessionID,
			Model:         ev.Model,
			UserType:      "external",
			Betas:         betasStr,
			IsInteractive: true,
			ClientType:    "cli",
		},
	}

	if ev.AccountUUID != "" {
		p.Auth = &telemetryAuth{AccountUUID: ev.AccountUUID}
	}

	// Embed extra data as additional_metadata (base64 JSON).
	if len(ev.Extra) > 0 {
		extraJSON, _ := json.Marshal(ev.Extra)
		p.AdditionalMetadata = base64.StdEncoding.EncodeToString(extraJSON)
	}

	return telemetryWireEvent{
		EventType: "ClaudeCodeInternalEvent",
		EventData: p,
	}
}

func (s *TelemetryService) buildEnv() *telemetryEnv {
	hostname, _ := os.Hostname()
	kernel := detectKernelVersion()
	distroID, distroVer := detectLinuxDistro()

	return &telemetryEnv{
		Platform:              "linux",
		PlatformRaw:           "linux",
		Arch:                  runtime.GOARCH,
		NodeVersion:           "v26.3.0",  // v2.1.196 uses Node v26.3.0 (Bun's bundled Node compat)
		Terminal:              "unknown",
		PackageManagers:       "npm",
		Runtimes:              "node",
		IsRunningWithBun:      true,       // CRITICAL: Bun-compiled binary
		IsCi:                  false,
		IsClaubbit:            false,
		IsGithubAction:        false,
		IsClaudeCodeAction:    false,
		IsClaudeAiAuth:        false,      // OAuth user (not API key)
		IsClaudeCodeRemote:    false,
		IsConductor:           false,
		Version:               claude.CLICurrentVersion,
		VersionBase:           claude.CLICurrentVersion,
		BuildTime:             "2026-06-29T00:53:27Z",
		DeploymentEnvironment: detectDeploymentEnv(hostname),
		Shell:                 "/bin/bash",
		Vcs:                   "git",
		LinuxDistroID:         distroID,
		LinuxDistroVersion:    distroVer,
		LinuxKernel:           kernel,
		IsLocalAgentMode:      false,
	}
}

// generateProcessMetrics returns base64-encoded realistic Bun-style process metrics JSON.
// IMPORTANT: Claude Code v2.1.196 uses Bun (JSC), not Node.js (V8).
// Bun's memory model: heapUsed can EXCEED heapTotal (unlike Node.js).
// Pattern: heapUsed ~108MB > heapTotal ~59MB, rss ~314MB (real data from v2.1.196).
func (s *TelemetryService) generateProcessMetrics() string {
	uptime := time.Since(s.processStart).Seconds()

	// Bun/JSC memory pattern: heapUsed often exceeds heapTotal
	timeFactor := uptime / 3600.0
	baseHeapTotal := int64(50 + rand.Intn(30)) << 20   // 50-80 MB
	baseHeapUsed  := int64(90 + rand.Intn(50)) << 20   // 90-140 MB (> heapTotal)
	baseRSS       := int64(250 + rand.Intn(100)) << 20 // 250-350 MB

	// Small growth over time
	heapTotal := baseHeapTotal + int64(float64(baseHeapTotal)*0.01*timeFactor)
	heapUsed  := baseHeapUsed + int64(float64(baseHeapUsed)*0.015*timeFactor)
	rss       := baseRSS + int64(float64(baseRSS)*0.01*timeFactor) + int64(rand.Intn(20<<20))

	// external and arrayBuffers
	external     := int64(30 + rand.Intn(50)) << 20  // 30-80 MB
	arrayBuffers := int64(2 + rand.Intn(8)) << 20    // 2-10 MB

	// CPU: idle ~3-8%, API calls spike to 20-80%
	cpuBase := float64(runtime.NumCPU()) * 2.0
	cpuPercent := cpuBase + float64(rand.Intn(15)) + 10*math.Sin(uptime/15.0)

	// constrainedMemory = system total memory (typically 128GB+)
	constrainedMemory := getSystemMemory()

	pm := processMetricsJSON{
		Uptime:            math.Round(uptime*100) / 100,
		RSS:               rss,
		HeapTotal:         heapTotal,
		HeapUsed:          heapUsed,    // NOTE: Bun: heapUsed > heapTotal is NORMAL
		External:          external,
		ArrayBuffers:      arrayBuffers,
		ConstrainedMemory: constrainedMemory,
		CPUUsage: cpuUsage{
			User:   int64(uptime * 1e6 * float64(runtime.NumCPU()) * 0.25),
			System: int64(uptime * 1e6 * float64(runtime.NumCPU()) * 0.05),
		},
		CPUPercent: math.Round(cpuPercent*100) / 100,
	}

	b, _ := json.Marshal(pm)
	return base64.StdEncoding.EncodeToString(b)
}

// getSystemMemory returns the system's total memory in bytes.
func getSystemMemory() int64 {
	// Try to read from /proc/meminfo on Linux
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 128 << 30 // fallback 128GB
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb * 1024
				}
			}
		}
	}
	return 128 << 30
}

// accountDeviceID derives a stable device ID from account ID.
func (s *TelemetryService) accountDeviceID(accountID int64) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("sub2api-device-%d", accountID)))
	return hex.EncodeToString(hash[:])
}

// accountSessionID returns the current session ID for an account.
func (s *TelemetryService) accountSessionID(accountID int64) string {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	if ses, ok := s.sessions[accountID]; ok {
		return ses.SessionID
	}
	return uuid.NewString()
}

// detectDeploymentEnv returns a deployment environment string.
func detectDeploymentEnv(hostname string) string {
	if hostname == "" {
		return "unknown-linux"
	}
	return fmt.Sprintf("unknown-%s", hostname)
}

// detectKernelVersion reads the kernel version from /proc/version.
func detectKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "6.2.0"
	}
	fields := bytes.Fields(data)
	if len(fields) >= 3 {
		return string(fields[2])
	}
	return "6.2.0"
}

// detectLinuxDistro reads /etc/os-release for distro info.
func detectLinuxDistro() (id, version string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "ubuntu", "22.04"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}
	if id == "" {
		id = "ubuntu"
	}
	if version == "" {
		version = "22.04"
	}
	return id, version
}

func boolToEvent(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

// --- HTTP client helper for the telemetry endpoint ---

// sendTelemetry is a standalone function that can be called without the service.
func sendTelemetry(baseURL, token, tokenType string, events []telemetryWireEvent) error {
	if len(events) == 0 {
		return nil
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := baseURL + telemetryEventEndpoint

	payload := map[string]interface{}{"events": events}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-code/"+claude.CLICurrentVersion)
	req.Header.Set("x-service-name", "claude-code")

	if token != "" {
		if tokenType == "oauth" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			req.Header.Set("x-api-key", token)
		}
	}

	client := &http.Client{Timeout: telemetryAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// On 401, retry once without auth.
	if resp.StatusCode == 401 && token != "" {
		req.Header.Del("Authorization")
		req.Header.Del("x-api-key")
		resp2, err2 := client.Do(req)
		if err2 != nil {
			return err2
		}
		defer resp2.Body.Close()
		if resp2.StatusCode >= 400 {
			body, _ := io.ReadAll(resp2.Body)
			return fmt.Errorf("telemetry send failed (status %d): %s", resp2.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telemetry send failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// BuildTelemetryEvent is a helper to build a single event and send it synchronously.
// This is used for critical events like tengu_started/tengu_exit.
func BuildTelemetryEvent(
	eventName string,
	deviceID, sessionID, model, accountUUID string,
	extra map[string]interface{},
	config TelemetryConfig,
) error {
	betas := claude.FullClaudeCodeMimicryBetas()
	betasStr := ""
	for i, b := range betas {
		if i > 0 {
			betasStr += ","
		}
		betasStr += b
	}

	p := telemetryPayload{
		EventName:       eventName,
		EventID:         uuid.NewString(),
		ClientTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
		DeviceID:        deviceID,
		SessionID:       sessionID,
		Model:           model,
		UserType:        "external",
		Betas:           betasStr,
		Entrypoint:      "cli",
		IsInteractive:   true,
		ClientType:      "cli",
		Core: telemetryCore{
			SessionID:     sessionID,
			Model:         model,
			UserType:      "external",
			Betas:         betasStr,
			IsInteractive: true,
			ClientType:    "cli",
		},
	}

	if accountUUID != "" {
		p.Auth = &telemetryAuth{AccountUUID: accountUUID}
	}
	if len(extra) > 0 {
		b, _ := json.Marshal(extra)
		p.AdditionalMetadata = base64.StdEncoding.EncodeToString(b)
	}

	event := telemetryWireEvent{
		EventType: "ClaudeCodeInternalEvent",
		EventData: p,
	}

	logger.LegacyPrintf("service.telemetry", "Sending %s event for account (device=%s session=%s)", eventName, deviceID[:16]+"...", sessionID[:8]+"...")
	return sendTelemetry(config.BaseURL, config.Token, config.TokenType, []telemetryWireEvent{event})
}
