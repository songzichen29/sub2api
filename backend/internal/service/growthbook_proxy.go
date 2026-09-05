package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
)

// growthBookClientKey is the GrowthBook client key real Claude Code uses for
// external (non-Anthropic) users. Source: claude-code constants/keys.ts
// (getGrowthBookClientKey, USER_TYPE !== 'ant' branch).
const growthBookClientKey = "sdk-zAZezfDKGoZuXXKe" //nolint:unused // retained for OAuth mimic deployments

// growthBookEvalURL is the remoteEval endpoint CC POSTs to (apiHost + /api/eval/{clientKey}).
const growthBookEvalURL = "https://api.anthropic.com/api/eval/" + growthBookClientKey //nolint:unused // retained for OAuth mimic deployments

const claudeBootstrapURL = "https://api.anthropic.com/api/claude_cli/bootstrap"

// growthbookExposure is one experiment assignment parsed from the remoteEval
// response, to be reported as a GrowthbookExperimentEvent.
type growthbookExposure struct {
	ExperimentID string
	VariationID  int
}

// ProxyGrowthBookFeature proxies Claude Code feature flag reads through a
// selected Anthropic OAuth account. The cache is intentionally best-effort:
// failures fall through to upstream so the client can still progress.
func (s *GatewayService) ProxyGrowthBookFeature(ctx context.Context, groupID *int64, featureKey string) (int, string, []byte, error) {
	featureKey = strings.Trim(strings.TrimSpace(featureKey), "/")
	if featureKey == "" {
		return http.StatusBadRequest, "application/json", []byte(`{"error":"missing feature key"}`), nil
	}
	return s.proxyAnthropicOAuthGET(ctx, groupID, "growthbook-feature:"+featureKey, "https://api.anthropic.com/api/features/"+url.PathEscape(featureKey))
}

// ProxyBootstrap proxies Claude CLI bootstrap config through a selected
// Anthropic OAuth account.
func (s *GatewayService) ProxyBootstrap(ctx context.Context, groupID *int64) (int, string, []byte, error) {
	return s.proxyAnthropicOAuthGET(ctx, groupID, "claude-bootstrap", claudeBootstrapURL)
}

func (s *GatewayService) proxyAnthropicOAuthGET(ctx context.Context, groupID *int64, cachePrefix, upstreamURL string) (int, string, []byte, error) {
	account, err := s.SelectAccountForModel(ctx, groupID, "", "")
	if err != nil {
		return 0, "", nil, err
	}
	if account == nil {
		return 0, "", nil, ErrNoAvailableAccounts
	}
	cacheKey := fmt.Sprintf("%s:%d", cachePrefix, account.ID)
	if s.proxyCache != nil {
		if cached, _ := s.proxyCache.Get(ctx, cacheKey); strings.TrimSpace(cached) != "" {
			return http.StatusOK, "application/json", []byte(cached), nil
		}
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return 0, "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", claude.BetaOAuth)
	req.Header.Set("User-Agent", "claude-cli/"+claude.CLICurrentVersion+" (external, cli)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return 0, "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && s.proxyCache != nil {
		_ = s.proxyCache.Set(ctx, cacheKey, string(body), 10*time.Minute)
	}
	return resp.StatusCode, contentType, body, nil
}

// ensureGrowthBookExperiments does a one-shot (per account, 6h-cached) remoteEval
// against api.anthropic.com and reports any experiment assignments as
// GrowthbookExperimentEvent telemetry, so the account is not left in a
// "zero-experiment-participation" state (a detection signal vs real CC, which
// fetches experiments at startup and logs exposures). Fire-and-forget; safe to
// call on every OAuth mimic request (the Redis marker dedupes within TTL).
func (s *GatewayService) ensureGrowthBookExperiments(ctx context.Context, account *Account, token, deviceID, sessionID string) { //nolint:unused // retained for OAuth mimic deployments
	if s.proxyCache == nil || account == nil || token == "" || deviceID == "" {
		return
	}
	if !account.IsGrowthBookProxyEnabled() {
		return
	}
	cacheKey := fmt.Sprintf("growthbook:%d", account.ID)
	if done, _ := s.proxyCache.Get(ctx, cacheKey); done != "" {
		return
	}

	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		exposures, err := s.fetchGrowthBookEval(bg, account, token, deviceID, sessionID)
		if err != nil {
			// Don't cache failures; retry on a later request.
			return
		}
		// Mark fetched for 6h regardless of whether experiments were assigned —
		// real CC fetches once per session and reports whatever it received.
		_ = s.proxyCache.Set(bg, cacheKey, "1", 6*time.Hour)
		if len(exposures) == 0 || s.telemetryHook == nil {
			return
		}
		s.telemetryHook.EnqueueGrowthbookExperiments(account.ID, token, deviceID, sessionID, account.GetExtraString("account_uuid"), exposures)
	}()
}

// fetchGrowthBookEval POSTs the remoteEval request (attributes body, OAuth
// bearer, uTLS+h2 upstream) and parses experiment assignments from the response.
func (s *GatewayService) fetchGrowthBookEval(ctx context.Context, account *Account, token, deviceID, sessionID string) ([]growthbookExposure, error) { //nolint:unused // retained for OAuth mimic deployments
	attributes := map[string]any{
		"id":        deviceID,
		"deviceID":  deviceID,
		"sessionId": sessionID,
		"platform":  "linux",
	}
	body, _ := json.Marshal(map[string]any{"attributes": attributes})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, growthBookEvalURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", claude.BetaOAuth)
	req.Header.Set("User-Agent", "claude-cli/"+claude.CLICurrentVersion+" (external, cli)")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return nil, fmt.Errorf("growthbook eval: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("growthbook eval status %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseGrowthBookExperiments(respBody), nil
}

// parseGrowthBookExperiments extracts experiment assignments from a remoteEval
// response. Mirrors claude-code processRemoteEvalPayload: features with
// source=="experiment", an experiment.key, and an experimentResult.variationId.
// Robust to missing fields (returns only well-formed exposures).
func parseGrowthBookExperiments(body []byte) []growthbookExposure { //nolint:unused // retained for OAuth mimic deployments
	var out []growthbookExposure
	features := gjson.GetBytes(body, "features")
	if !features.IsObject() {
		return out
	}
	features.ForEach(func(_, f gjson.Result) bool {
		if f.Get("source").String() != "experiment" {
			return true
		}
		expKey := f.Get("experiment.key").String()
		if expKey == "" {
			return true
		}
		if !f.Get("experimentResult").Exists() {
			return true
		}
		out = append(out, growthbookExposure{
			ExperimentID: expKey,
			VariationID:  int(f.Get("experimentResult.variationId").Int()),
		})
		return true
	})
	return out
}
