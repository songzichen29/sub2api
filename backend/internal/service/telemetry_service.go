package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/google/uuid"
)

const (
	// telemetryBatchInterval is the default interval between batch sends.
	telemetryBatchInterval = 10 * time.Second

	// telemetryMaxBatchSize is max events per POST.
	telemetryMaxBatchSize = 200

	// telemetryEventEndpoint is the 1P event logging endpoint used by Claude Code v2.1.197.
	telemetryEventEndpoint = "/api/event_logging/v2/batch"

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
	EventName   string         `json:"event_name"`
	AccountID   int64          `json:"-"` // for routing
	DeviceID    string         `json:"-"` // from identity service metadata.user_id.device_id
	SessionID   string         `json:"-"`
	Model       string         `json:"-"`
	AccountUUID string         `json:"-"`
	OrgUUID     string         `json:"-"`
	UserType    string         `json:"-"`
	Extra       map[string]any `json:"-"`
	Timestamp   time.Time      `json:"-"`
	// Token is an optional per-request fallback OAuth bearer; normal gateway
	// operation resolves auth by AccountID via TokenProvider/AccountRepo.
	Token string `json:"-"`
}

// TelemetryConfig holds configuration for the telemetry service.
type TelemetryConfig struct {
	BaseURL         string
	Enabled         bool
	Token           string // optional fallback OAuth access token for auth
	TokenType       string // "oauth" or "apikey"
	FlushIntervalMS int
	MaxBatchSize    int
	MaxRetries      int
	HTTPUpstream    HTTPUpstream
	TokenProvider   *ClaudeTokenProvider
	AccountRepo     AccountRepository
	TLSProfile      *tlsfingerprint.Profile
}

// TelemetryService manages periodic batching and sending of telemetry events.
type TelemetryService struct {
	cfg      TelemetryConfig
	eventCh  chan TelemetryEvent
	wireCh   chan telemetryWireEvent
	done     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopOnce sync.Once
	batch    []telemetryWireEvent
	// Per-account session tracking
	sessionsMu sync.RWMutex
	sessions   map[int64]*TelemetrySession
	// Process metrics simulator state
	processStart time.Time
}

// TelemetrySession tracks state for a single account's "Claude Code session".
type TelemetrySession struct {
	AccountID    int64
	SessionID    string
	DeviceID     string
	Model        string
	AccountUUID  string
	StartedAt    time.Time
	APIRequests  int64
	FeaturesSent bool
	ExitSent     bool
}

// telemetryWireEvent is the JSON shape sent to the endpoint.
type telemetryWireEvent struct {
	AccountID int64  `json:"-"`
	Token     string `json:"-"`
	EventType string `json:"event_type"`
	// EventData is the serialized event_data payload. For ClaudeCodeInternalEvent
	// it is a marshaled telemetryPayload; for GrowthbookExperimentEvent it is a
	// marshaled growthbookExperimentEventData.
	EventData json.RawMessage `json:"event_data"`
}

type telemetryPayload struct {
	EventName          string         `json:"event_name,omitempty"`
	EventID            string         `json:"event_id"`
	ClientTimestamp    string         `json:"client_timestamp"`
	DeviceID           string         `json:"device_id,omitempty"`
	SessionID          string         `json:"session_id,omitempty"`
	Model              string         `json:"model,omitempty"`
	UserType           string         `json:"user_type,omitempty"`
	Betas              string         `json:"betas,omitempty"`
	Entrypoint         string         `json:"entrypoint,omitempty"`
	IsInteractive      bool           `json:"is_interactive"`
	ClientType         string         `json:"client_type,omitempty"`
	Env                *telemetryEnv  `json:"env,omitempty"`
	Process            string         `json:"process,omitempty"` // base64(processMetricsJSON)
	Auth               *telemetryAuth `json:"auth,omitempty"`
	AdditionalMetadata string         `json:"additional_metadata,omitempty"` // base64(json)
	Email              string         `json:"email,omitempty"`
}

type telemetryEnv struct {
	Platform              string `json:"platform"`
	NodeVersion           string `json:"node_version"`
	Terminal              string `json:"terminal"`
	PackageManagers       string `json:"package_managers"`
	Runtimes              string `json:"runtimes"`
	IsRunningWithBun      bool   `json:"is_running_with_bun"`
	IsCi                  bool   `json:"is_ci"`
	IsClaubbit            bool   `json:"is_claubbit"`
	IsGithubAction        bool   `json:"is_github_action"`
	IsClaudeCodeAction    bool   `json:"is_claude_code_action"`
	IsClaudeAiAuth        bool   `json:"is_claude_ai_auth"`
	Version               string `json:"version"`
	Arch                  string `json:"arch"`
	IsClaudeCodeRemote    bool   `json:"is_claude_code_remote"`
	DeploymentEnvironment string `json:"deployment_environment"`
	IsConductor           bool   `json:"is_conductor"`
	VersionBase           string `json:"version_base,omitempty"`
	BuildTime             string `json:"build_time"`
	IsLocalAgentMode      bool   `json:"is_local_agent_mode"`
	LinuxDistroID         string `json:"linux_distro_id,omitempty"`
	LinuxDistroVersion    string `json:"linux_distro_version,omitempty"`
	LinuxKernel           string `json:"linux_kernel,omitempty"`
	Vcs                   string `json:"vcs,omitempty"`
	PlatformRaw           string `json:"platform_raw"`
	Shell                 string `json:"shell,omitempty"`
}

type telemetryAuth struct {
	AccountUUID      string `json:"account_uuid,omitempty"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
}

// processMetricsJSON is the shape marshal'd then base64'd into the Process field.
type processMetricsJSON struct {
	Uptime            float64  `json:"uptime"`
	RSS               int64    `json:"rss"`
	HeapTotal         int64    `json:"heapTotal"`
	HeapUsed          int64    `json:"heapUsed"`
	External          int64    `json:"external"`
	ArrayBuffers      int64    `json:"arrayBuffers"`
	ConstrainedMemory int64    `json:"constrainedMemory"`
	CPUUsage          cpuUsage `json:"cpuUsage"`
	CPUPercent        *float64 `json:"cpuPercent,omitempty"`
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
		wireCh:       make(chan telemetryWireEvent, telemetryMaxQueueSize),
		done:         make(chan struct{}),
		sessions:     make(map[int64]*TelemetrySession),
		processStart: time.Now(),
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
	if s == nil || !s.cfg.Enabled {
		return
	}
	s.stopOnce.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
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
func (s *TelemetryService) EnsureSession(accountID int64, deviceID, sessionID, accountUUID, model string) *TelemetrySession {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	if ses, ok := s.sessions[accountID]; ok {
		if deviceID != "" {
			ses.DeviceID = deviceID
		}
		if sessionID != "" {
			ses.SessionID = sessionID
		}
		if model != "" {
			ses.Model = model
		}
		if accountUUID != "" {
			ses.AccountUUID = accountUUID
		}
		ses.APIRequests++
		return ses
	}

	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	ses := &TelemetrySession{
		AccountID:   accountID,
		SessionID:   sessionID,
		DeviceID:    deviceID,
		Model:       model,
		AccountUUID: accountUUID,
		StartedAt:   time.Now(),
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
		AccountUUID: ses.AccountUUID,
		Extra: map[string]any{
			"last_session_duration_sec": math.Round(sessionDuration*10) / 10,
			"last_session_api_requests": ses.APIRequests,
			"renderer_mode":             "fullscreen",
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
		Extra: map[string]any{
			"duration_ms":   durationMs,
			"status_code":   statusCode,
			"renderer_mode": "fullscreen",
		},
		Timestamp: time.Now(),
	})
}

// sendFeatureSequence sends the canonical Claude Code feature event sequence.
func (s *TelemetryService) sendFeatureSequence(accountID int64, deviceID, sessionID, model, accountUUID string) {
	// Real Claude Code sends these around startup. Keep names conservative: they
	// occur frequently in the local telemetry samples and carry only feature_name.
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
			Extra: map[string]any{
				"feature_name":  f,
				"renderer_mode": "fullscreen",
			},
			Timestamp: time.Now(),
		})
		time.Sleep(time.Duration(5+rand.Intn(30)) * time.Millisecond)
	}

	s.Send(TelemetryEvent{
		EventName:   "tengu_shell_set_cwd",
		AccountID:   accountID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra: map[string]any{
			"renderer_mode": "fullscreen",
		},
		Timestamp: time.Now(),
	})
}

// loop is the main background worker.
func (s *TelemetryService) loop(ctx context.Context) {
	defer s.wg.Done()
	interval := telemetryBatchInterval
	if s.cfg.FlushIntervalMS > 0 {
		interval = time.Duration(s.cfg.FlushIntervalMS) * time.Millisecond
	}
	ticker := time.NewTicker(interval)
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
		case ev := <-s.wireCh:
			s.mu.Lock()
			s.batch = append(s.batch, ev)
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

	// 1P telemetry auth is account-scoped. A single global queue can collect
	// events from different OAuth accounts before a flush; never send those under
	// the first account's bearer token. Partition while preserving first-seen
	// account order so every POST uses the matching account token/proxy.
	for _, group := range groupTelemetryEventsByAccount(events) {
		s.sendAccountBatch(group.accountID, group.token, group.events, attempt)
	}
}

type telemetryAccountEventGroup struct {
	accountID int64
	token     string
	events    []telemetryWireEvent
}

func groupTelemetryEventsByAccount(events []telemetryWireEvent) []telemetryAccountEventGroup {
	groups := make([]telemetryAccountEventGroup, 0, 1)
	index := make(map[int64]int)
	for _, ev := range events {
		accountID := ev.AccountID
		if pos, ok := index[accountID]; ok {
			if groups[pos].token == "" && ev.Token != "" {
				groups[pos].token = ev.Token
			}
			groups[pos].events = append(groups[pos].events, ev)
			continue
		}
		index[accountID] = len(groups)
		groups = append(groups, telemetryAccountEventGroup{accountID: accountID, token: ev.Token, events: []telemetryWireEvent{ev}})
	}
	return groups
}

func (s *TelemetryService) sendAccountBatch(accountID int64, token string, events []telemetryWireEvent, attempt int) {
	payload := map[string]any{"events": events}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("telemetry: failed to marshal batch", "error", err)
		return
	}

	if err := s.doTelemetryPOST(context.Background(), body, accountID, token, true); err != nil {
		if attempt < s.cfg.MaxRetries {
			delay := telemetryBackoffBase * (1 << attempt)
			if delay > telemetryBackoffMax {
				delay = telemetryBackoffMax
			}
			time.Sleep(time.Duration(delay) * time.Millisecond)
			s.sendAccountBatch(accountID, token, events, attempt+1)
		}
	}
}

func (s *TelemetryService) doTelemetryPOST(ctx context.Context, body []byte, accountID int64, eventToken string, withAuth bool) error {
	baseURL := s.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := strings.TrimRight(baseURL, "/") + telemetryEventEndpoint

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	applyTelemetryHeaders(req)

	proxyURL, concurrency := s.resolveTelemetryProxy(ctx, accountID)

	if withAuth {
		token, tokenType := s.resolveAuthToken(ctx, accountID, eventToken)
		if token != "" {
			if tokenType == "apikey" {
				setHeaderRaw(req.Header, "x-api-key", token)
			} else {
				setHeaderRaw(req.Header, "Authorization", "Bearer "+token)
				setHeaderRaw(req.Header, "anthropic-beta", claude.BetaOAuth)
			}
		}
	}

	resp, err := s.doHTTPRequest(req, accountID, proxyURL, concurrency)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized && withAuth {
		return s.doTelemetryPOST(ctx, body, accountID, "", false)
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telemetry send failed (status %d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (s *TelemetryService) resolveAuthToken(ctx context.Context, accountID int64, eventToken string) (string, string) {
	if accountID > 0 && s.cfg.AccountRepo != nil {
		if account, err := s.cfg.AccountRepo.GetByID(ctx, accountID); err == nil && account != nil {
			if account.Type == AccountTypeOAuth && s.cfg.TokenProvider != nil {
				if token, err := s.cfg.TokenProvider.GetAccessToken(ctx, account); err == nil && strings.TrimSpace(token) != "" {
					return token, "oauth"
				}
			}
			if account.IsOAuth() {
				if token := strings.TrimSpace(account.GetCredential("access_token")); token != "" {
					return token, "oauth"
				}
			}
			if account.Type == AccountTypeAPIKey {
				if token := strings.TrimSpace(account.GetCredential("api_key")); token != "" {
					return token, "apikey"
				}
			}
			if account.Type == AccountTypeServiceAccount && s.cfg.TokenProvider != nil {
				if token, err := s.cfg.TokenProvider.GetAccessToken(ctx, account); err == nil && strings.TrimSpace(token) != "" {
					return token, "oauth"
				}
			}
		}
	}
	if strings.TrimSpace(eventToken) != "" {
		return strings.TrimSpace(eventToken), "oauth"
	}
	if strings.TrimSpace(s.cfg.Token) == "" {
		return "", ""
	}
	return s.cfg.Token, s.cfg.TokenType
}

func (s *TelemetryService) resolveTelemetryProxy(ctx context.Context, accountID int64) (string, int) {
	if accountID <= 0 || s.cfg.AccountRepo == nil {
		return "", 0
	}
	account, err := s.cfg.AccountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return "", 0
	}
	if account.ProxyID != nil && account.Proxy != nil && account.Proxy.IsActive() {
		return account.Proxy.URL(), account.Concurrency
	}
	return "", account.Concurrency
}

func (s *TelemetryService) doHTTPRequest(req *http.Request, accountID int64, proxyURL string, concurrency int) (*http.Response, error) {
	if s.cfg.HTTPUpstream != nil {
		return s.cfg.HTTPUpstream.DoWithTLS(req, proxyURL, accountID, concurrency, s.telemetryTLSProfile())
	}
	dialer := tlsfingerprint.NewDialer(s.telemetryTLSProfile(), nil)
	client := &http.Client{
		Timeout: telemetryAPITimeout,
		Transport: &http.Transport{
			DialTLSContext:     dialer.DialTLSContext,
			DisableCompression: true,
			ForceAttemptHTTP2:  false,
			MaxIdleConns:       10,
			IdleConnTimeout:    90 * time.Second,
		},
	}
	return client.Do(req)
}

func (s *TelemetryService) telemetryTLSProfile() *tlsfingerprint.Profile {
	if s.cfg.TLSProfile != nil {
		return s.cfg.TLSProfile
	}
	return &tlsfingerprint.Profile{Name: "Built-in Default (Bun/Node.js v26.3.0)"}
}

func applyTelemetryHeaders(req *http.Request) {
	setHeaderRaw(req.Header, "Content-Type", "application/json")
	setHeaderRaw(req.Header, "User-Agent", "claude-code/"+claude.CLICurrentVersion)
	setHeaderRaw(req.Header, "x-service-name", "claude-code")
	setHeaderRaw(req.Header, "Accept-Encoding", "gzip, deflate, br, zstd")
}

// buildPayload converts an internal TelemetryEvent to the wire format.
func (s *TelemetryService) buildPayload(ev TelemetryEvent) telemetryWireEvent {
	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	deviceID := ev.DeviceID
	if deviceID == "" {
		deviceID = s.accountDeviceID(ev.AccountID)
	}
	sessionID := ev.SessionID
	if sessionID == "" {
		sessionID = s.accountSessionID(ev.AccountID)
	}

	acct := s.resolveTelemetryAccountContext(ev, deviceID)
	betasStr := strings.Join(claude.FullClaudeCodeMimicryBetasForModel(ev.Model), ",")

	p := telemetryPayload{
		EventName:       ev.EventName,
		EventID:         uuid.NewString(),
		ClientTimestamp: now.UTC().Format("2006-01-02T15:04:05.000Z"),
		DeviceID:        deviceID,
		SessionID:       sessionID,
		Model:           ev.Model,
		UserType:        defaultString(ev.UserType, "external"),
		Betas:           betasStr,
		Env:             s.buildEnv(acct.isClaudeAIAuth),
		Entrypoint:      "cli",
		IsInteractive:   true,
		ClientType:      "cli",
		Process:         s.generateProcessMetrics(ev.AccountID, deviceID, acct.constrainedMemory),
	}

	if acct.accountUUID != "" || acct.orgUUID != "" {
		p.Auth = &telemetryAuth{AccountUUID: acct.accountUUID, OrganizationUUID: acct.orgUUID}
	}

	additional := make(map[string]any, len(ev.Extra)+1)
	if shouldAttachRendererMode(ev.EventName) {
		additional["renderer_mode"] = "fullscreen"
	}
	for k, v := range ev.Extra {
		if strings.HasPrefix(k, "_PROTO_") {
			continue
		}
		if k == "renderer_mode" && !shouldAttachRendererMode(ev.EventName) {
			continue
		}
		additional[k] = v
	}
	if len(additional) > 0 {
		extraJSON, _ := json.Marshal(additional)
		p.AdditionalMetadata = base64.StdEncoding.EncodeToString(extraJSON)
	}

	raw, _ := json.Marshal(p)
	return telemetryWireEvent{AccountID: ev.AccountID, Token: ev.Token, EventType: "ClaudeCodeInternalEvent", EventData: raw}
}

type telemetryAccountContext struct {
	accountUUID       string
	orgUUID           string
	isClaudeAIAuth    bool
	constrainedMemory int64
}

func (s *TelemetryService) resolveTelemetryAccountContext(ev TelemetryEvent, deviceID string) telemetryAccountContext {
	ctx := telemetryAccountContext{
		accountUUID:       strings.TrimSpace(ev.AccountUUID),
		orgUUID:           strings.TrimSpace(ev.OrgUUID),
		constrainedMemory: stableConstrainedMemory(ev.AccountID, deviceID),
	}
	if ev.AccountID <= 0 || s == nil || s.cfg.AccountRepo == nil {
		return ctx
	}
	account, err := s.cfg.AccountRepo.GetByID(context.Background(), ev.AccountID)
	if err != nil || account == nil {
		return ctx
	}
	ctx.isClaudeAIAuth = account.IsOAuth()
	if ctx.accountUUID == "" {
		ctx.accountUUID = strings.TrimSpace(firstNonEmptyTelemetryString(
			account.GetExtraString("account_uuid"),
			account.GetCredential("account_uuid"),
		))
	}
	if ctx.orgUUID == "" {
		ctx.orgUUID = strings.TrimSpace(firstNonEmptyTelemetryString(
			account.GetExtraString("org_uuid"),
			account.GetExtraString("organization_uuid"),
			account.GetCredential("org_uuid"),
			account.GetCredential("organization_uuid"),
		))
	}
	return ctx
}

func shouldAttachRendererMode(eventName string) bool {
	name := strings.ToLower(eventName)
	for _, marker := range []string{"keybinding", "tip", "status_line", "render"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func stableConstrainedMemory(accountID int64, deviceID string) int64 {
	material := fmt.Sprintf("sub2api-constrained-memory:%d:%s", accountID, deviceID)
	if accountID <= 0 && strings.TrimSpace(deviceID) == "" {
		material = "sub2api-constrained-memory:default"
	}
	sum := sha256.Sum256([]byte(material))
	gb := int64(64 + binary.BigEndian.Uint64(sum[:8])%193) // 64..256 GiB inclusive
	return gb << 30
}

func firstNonEmptyTelemetryString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *TelemetryService) buildEnv(isClaudeAIAuth bool) *telemetryEnv {
	return &telemetryEnv{
		Platform:              "linux",
		NodeVersion:           "v26.3.0",
		Terminal:              "gnome-terminal",
		PackageManagers:       "npm",
		Runtimes:              "node",
		IsRunningWithBun:      true,
		IsCi:                  false,
		IsClaubbit:            false,
		IsGithubAction:        false,
		IsClaudeCodeAction:    false,
		IsClaudeAiAuth:        isClaudeAIAuth,
		Version:               claude.CLICurrentVersion,
		Arch:                  "x64",
		IsClaudeCodeRemote:    false,
		DeploymentEnvironment: "unknown-linux",
		IsConductor:           false,
		VersionBase:           claude.CLICurrentVersion,
		BuildTime:             claude.CLIBuildTime,
		IsLocalAgentMode:      false,
		LinuxDistroID:         "ubuntu",
		LinuxDistroVersion:    "22.04",
		LinuxKernel:           "6.2.0-26-generic",
		Vcs:                   "git",
		PlatformRaw:           "linux",
		Shell:                 "bash",
	}
}

// generateProcessMetrics returns base64-encoded realistic Bun-style process metrics JSON.
// IMPORTANT: Claude Code v2.1.197 uses Bun (JSC), not Node.js (V8).
// Bun's memory model: heapUsed can exceed heapTotal.
func (s *TelemetryService) generateProcessMetrics(accountID int64, deviceID string, constrainedMemory int64) string {
	uptime := time.Since(s.processStart).Seconds()
	if uptime < 0 {
		uptime = 0
	}

	baseHeapTotal := int64(50+rand.Intn(30)) << 20 // 50-80 MB
	baseHeapUsed := int64(95+rand.Intn(65)) << 20  // 95-160 MB (> heapTotal often)
	baseRSS := int64(280+rand.Intn(130)) << 20     // 280-410 MB
	growth := uptime / 7200.0

	heapTotal := baseHeapTotal + int64(float64(baseHeapTotal)*0.01*growth)
	heapUsed := baseHeapUsed + int64(float64(baseHeapUsed)*0.015*growth)
	rss := baseRSS + int64(float64(baseRSS)*0.01*growth) + int64(rand.Intn(24<<20))
	external := int64(45+rand.Intn(75)) << 20
	arrayBuffers := int64(6+rand.Intn(26)) << 20
	if constrainedMemory <= 0 {
		constrainedMemory = stableConstrainedMemory(accountID, deviceID)
	}

	pm := processMetricsJSON{
		Uptime:            math.Round(uptime*1e6) / 1e6,
		RSS:               rss,
		HeapTotal:         heapTotal,
		HeapUsed:          heapUsed,
		External:          external,
		ArrayBuffers:      arrayBuffers,
		ConstrainedMemory: constrainedMemory,
		CPUUsage: cpuUsage{
			User:   int64(uptime*180000) + int64(rand.Intn(250000)),
			System: int64(uptime*45000) + int64(rand.Intn(90000)),
		},
	}
	// Real samples include cpuPercent sparsely (~31%).
	if rand.Intn(100) < 31 {
		cpuPercent := math.Round((2.5+rand.Float64()*22.5)*100) / 100
		pm.CPUPercent = &cpuPercent
	}

	b, _ := json.Marshal(pm)
	return base64.StdEncoding.EncodeToString(b)
}

// accountDeviceID is a last-resort fallback. Gateway integration passes the
// parsed metadata.user_id.device_id so telemetry, header, and body share one value.
func (s *TelemetryService) accountDeviceID(accountID int64) string {
	if accountID <= 0 {
		return ""
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	if ses, ok := s.sessions[accountID]; ok {
		return ses.DeviceID
	}
	return ""
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

func boolToEvent(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func defaultString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

// --- HTTP client helper for the telemetry endpoint ---

// sendTelemetry is a standalone function that can be called without the service.
func sendTelemetry(baseURL, token, tokenType string, events []telemetryWireEvent) error {
	if len(events) == 0 {
		return nil
	}
	s := NewTelemetryService(TelemetryConfig{Enabled: true, BaseURL: baseURL, Token: token, TokenType: tokenType})
	payload := map[string]any{"events": events}
	body, _ := json.Marshal(payload)
	return s.doTelemetryPOST(context.Background(), body, 0, "", true)
}

// BuildTelemetryEvent is a helper to build a single event and send it synchronously.
// This is used for critical events like tengu_started/tengu_exit and tests.
func BuildTelemetryEvent(
	eventName string,
	deviceID, sessionID, model, accountUUID string,
	extra map[string]any,
	config TelemetryConfig,
) error {
	s := NewTelemetryService(TelemetryConfig{Enabled: true})
	event := s.buildPayload(TelemetryEvent{
		EventName:   eventName,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		Model:       model,
		AccountUUID: accountUUID,
		Extra:       extra,
		Timestamp:   time.Now(),
	})

	logger.LegacyPrintf("service.telemetry", "Sending %s event for account (device=%s session=%s)", eventName, safePrefix(deviceID, 16), safePrefix(sessionID, 8))
	return sendTelemetry(config.BaseURL, config.Token, config.TokenType, []telemetryWireEvent{event})
}

// growthbookExperimentEventData is the event_data payload for a
// GrowthbookExperimentEvent (mirrors claude-code GrowthbookExperimentEvent proto:
// event_id, timestamp, experiment_id, variation_id, environment, user_attributes,
// device_id, session_id).
type growthbookExperimentEventData struct {
	EventID        string `json:"event_id"`
	Timestamp      string `json:"timestamp,omitempty"`
	ExperimentID   string `json:"experiment_id,omitempty"`
	VariationID    int    `json:"variation_id"`
	Environment    string `json:"environment,omitempty"`
	UserAttributes string `json:"user_attributes,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

// EnqueueGrowthbookExperiments builds one GrowthbookExperimentEvent wire event
// per exposure and feeds them to the batch loop. Events carry AccountID (and an
// optional current OAuth token fallback) so sendBatch authenticates per account.
func (s *TelemetryService) EnqueueGrowthbookExperiments(accountID int64, token, deviceID, sessionID, accountUUID string, exposures []growthbookExposure) {
	if s == nil || !s.cfg.Enabled || len(exposures) == 0 {
		return
	}
	userAttributes, _ := json.Marshal(map[string]any{
		"id":        deviceID,
		"deviceID":  deviceID,
		"sessionId": sessionID,
		"platform":  "linux",
	})
	for _, ex := range exposures {
		data := growthbookExperimentEventData{
			EventID:        uuid.NewString(),
			Timestamp:      time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			ExperimentID:   ex.ExperimentID,
			VariationID:    ex.VariationID,
			Environment:    "production",
			UserAttributes: string(userAttributes),
			DeviceID:       deviceID,
			SessionID:      sessionID,
		}
		raw, _ := json.Marshal(data)
		ev := telemetryWireEvent{AccountID: accountID, Token: token, EventType: "GrowthbookExperimentEvent", EventData: raw}
		select {
		case s.wireCh <- ev:
		default:
			// Drop on full queue to avoid blocking request handling.
		}
	}
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
