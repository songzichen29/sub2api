package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// GrokGatewayService 处理 Grok 平台请求,通过 type=upstream 账号透传到 grok2api 网关。
//
// 设计要点:
//   - grok2api 暴露 Anthropic 兼容的 /v1/messages 接口,sub2api 直接转发请求体并双 header 鉴权
//   - SSO Cookie 池、xAI 协议翻译、模型路由全部由 grok2api 处理
//   - sub2api 仅作为统一入口,不感知 grok 平台的认证细节
//   - handleUpstreamError 仅做账号级 429 限流(grok2api 内部已处理 SSO 池限流)
type GrokGatewayService struct {
	accountRepo      AccountRepository
	rateLimitService *RateLimitService
	httpUpstream     HTTPUpstream
	settingService   *SettingService
}

func NewGrokGatewayService(
	accountRepo AccountRepository,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
) *GrokGatewayService {
	return &GrokGatewayService{
		accountRepo:      accountRepo,
		rateLimitService: rateLimitService,
		httpUpstream:     httpUpstream,
		settingService:   settingService,
	}
}

// Forward 是 grok 平台请求的统一入口。
// 仅支持 type=upstream 账号(透传到 grok2api 网关),其他类型直接拒绝。
func (s *GrokGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	if account.Type != AccountTypeUpstream {
		return nil, fmt.Errorf("grok platform only supports type=upstream account, got type=%s", account.Type)
	}
	return s.ForwardUpstream(ctx, c, account, body)
}

// ForwardUpstream 使用 base_url + /v1/messages + 双 header 鉴权透传上游 Claude 请求。
// 实现完全镜像 antigravity_gateway_service.go:4207 ForwardUpstream,因为 grok2api
// 暴露的 Anthropic 兼容接口和上游协议形态完全一致。
func (s *GrokGatewayService) ForwardUpstream(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()
	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("upstream account missing base_url or api_key")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return nil, fmt.Errorf("parse claude request: %w", err)
	}
	if strings.TrimSpace(claudeReq.Model) == "" {
		return nil, fmt.Errorf("missing model")
	}
	originalModel := claudeReq.Model

	upstreamURL := baseURL + "/v1/messages"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-api-key", apiKey) // Claude API 兼容

	if v := c.GetHeader("anthropic-version"); v != "" {
		req.Header.Set("anthropic-version", v)
	}
	if v := c.GetHeader("anthropic-beta"); v != "" {
		req.Header.Set("anthropic-beta", v)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		logger.LegacyPrintf("service.grok_gateway", "%s upstream request failed: %v", prefix, err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

		if resp.StatusCode == http.StatusTooManyRequests {
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
		}

		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		c.Status(resp.StatusCode)
		_, _ = c.Writer.Write(respBody)

		return &ForwardResult{
			Model: originalModel,
		}, nil
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool

	if claudeReq.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		streamRes := s.streamUpstreamResponse(c, resp, startTime)
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
		clientDisconnect = streamRes.clientDisconnect
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		usage = s.extractClaudeUsage(respBody)

		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(respBody)
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.grok_gateway", "%s status=success duration_ms=%d", prefix, duration.Milliseconds())

	return &ForwardResult{
		Model:            originalModel,
		Stream:           claudeReq.Stream,
		Duration:         duration,
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
		Usage: ClaudeUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
		},
	}, nil
}

// streamUpstreamResponse 透传上游 SSE 流并提取 Claude usage。
// 复用同包的 antigravityStreamResult / newAntigravityClientWriter / handleStreamReadError。
func (s *GrokGatewayService) streamUpstreamResponse(c *gin.Context, resp *http.Response, startTime time.Time) *antigravityStreamResult {
	usage := &ClaudeUsage{}
	var firstTokenMs *int

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}()
	defer close(done)

	streamInterval := time.Duration(0)
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	keepaliveInterval := time.Duration(0)
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.settingService.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	lastDataAt := time.Now()

	flusher, _ := c.Writer.(http.Flusher)
	cw := newAntigravityClientWriter(c.Writer, flusher, "grok upstream")

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: cw.Disconnected()}
			}
			if ev.err != nil {
				if disconnect, handled := handleStreamReadError(ev.err, cw.Disconnected(), "grok upstream"); handled {
					return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: disconnect}
				}
				logger.LegacyPrintf("service.grok_gateway", "Stream read error (grok upstream): %v", ev.err)
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs}
			}

			lastDataAt = time.Now()

			line := ev.line

			if firstTokenMs == nil && len(line) > 0 {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}

			s.extractSSEUsage(line, usage)

			cw.Fprintf("%s\n", line)

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if cw.Disconnected() {
				logger.LegacyPrintf("service.grok_gateway", "Upstream timeout after client disconnect (grok upstream), returning collected usage")
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}
			}
			logger.LegacyPrintf("service.grok_gateway", "Stream data interval timeout (grok upstream)")
			return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs}

		case <-keepaliveCh:
			if cw.Disconnected() {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			if !cw.Fprintf("event: ping\ndata: {\"type\": \"ping\"}\n\n") {
				logger.LegacyPrintf("service.grok_gateway", "Client disconnected during keepalive ping (grok upstream), continuing to drain upstream for billing")
				continue
			}
		}
	}
}

// extractSSEUsage 从 SSE data 行中提取 Claude usage(用于流式透传场景)。
// 镜像 antigravity_gateway_service.go:4471 的实现,逻辑完全相同。
func (s *GrokGatewayService) extractSSEUsage(line string, usage *ClaudeUsage) {
	if !strings.HasPrefix(line, "data: ") {
		return
	}
	dataStr := strings.TrimPrefix(line, "data: ")
	var event map[string]any
	if json.Unmarshal([]byte(dataStr), &event) != nil {
		return
	}
	u, ok := event["usage"].(map[string]any)
	if !ok {
		return
	}
	if v, ok := u["input_tokens"].(float64); ok && int(v) > 0 {
		usage.InputTokens = int(v)
	}
	if v, ok := u["output_tokens"].(float64); ok && int(v) > 0 {
		usage.OutputTokens = int(v)
	}
	if v, ok := u["cache_read_input_tokens"].(float64); ok && int(v) > 0 {
		usage.CacheReadInputTokens = int(v)
	}
	if v, ok := u["cache_creation_input_tokens"].(float64); ok && int(v) > 0 {
		usage.CacheCreationInputTokens = int(v)
	}
	if cc, ok := u["cache_creation"].(map[string]any); ok {
		if v, ok := cc["ephemeral_5m_input_tokens"].(float64); ok {
			usage.CacheCreation5mTokens = int(v)
		}
		if v, ok := cc["ephemeral_1h_input_tokens"].(float64); ok {
			usage.CacheCreation1hTokens = int(v)
		}
	}
}

// extractClaudeUsage 从非流式 Claude 响应提取 usage。
// 镜像 antigravity_gateway_service.go:4508 的实现,逻辑完全相同。
func (s *GrokGatewayService) extractClaudeUsage(body []byte) *ClaudeUsage {
	usage := &ClaudeUsage{}
	var resp map[string]any
	if json.Unmarshal(body, &resp) != nil {
		return usage
	}
	if u, ok := resp["usage"].(map[string]any); ok {
		if v, ok := u["input_tokens"].(float64); ok {
			usage.InputTokens = int(v)
		}
		if v, ok := u["output_tokens"].(float64); ok {
			usage.OutputTokens = int(v)
		}
		if v, ok := u["cache_read_input_tokens"].(float64); ok {
			usage.CacheReadInputTokens = int(v)
		}
		if v, ok := u["cache_creation_input_tokens"].(float64); ok {
			usage.CacheCreationInputTokens = int(v)
		}
		if cc, ok := u["cache_creation"].(map[string]any); ok {
			if v, ok := cc["ephemeral_5m_input_tokens"].(float64); ok {
				usage.CacheCreation5mTokens = int(v)
			}
			if v, ok := cc["ephemeral_1h_input_tokens"].(float64); ok {
				usage.CacheCreation1hTokens = int(v)
			}
		}
	}
	return usage
}

// handleUpstreamError 简化版上游错误处理。
//
// 与 antigravity 不同,grok 走 upstream 透传:
//   - grok2api 内部已处理 SSO 池的模型级限流和上游错误重试
//   - sub2api 这边只需要在 upstream 网关持续 429 时把整条 upstream 账号短暂下线避免雪崩
//   - 不做模型级限流(没必要,grok2api 透传层无法准确归因到具体 grok 模型)
func (s *GrokGatewayService) handleUpstreamError(
	ctx context.Context, prefix string, account *Account,
	statusCode int, headers http.Header, body []byte,
) {
	if !account.ShouldHandleErrorCode(statusCode) {
		return
	}
	if statusCode == http.StatusTooManyRequests {
		ra := time.Now().Add(5 * time.Minute)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, ra); err != nil {
			logger.LegacyPrintf("service.grok_gateway", "%s status=429 rate_limit_set_failed account=%d err=%v", prefix, account.ID, err)
		} else {
			logger.LegacyPrintf("service.grok_gateway", "%s status=429 rate_limited account=%d reset_at=%v", prefix, account.ID, ra.Format("15:04:05"))
		}
		return
	}
	if s.rateLimitService != nil {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
	}
}

// TestConnection 测试 grok upstream 账号到 grok2api 网关的连通性。
// 用最小 payload 调一次非流式 /v1/chat/completions，使用 Bearer 鉴权。
// 返回响应中提取的首条 text，供管理后台 SSE 测试输出展示。
func (s *GrokGatewayService) TestConnection(ctx context.Context, account *Account, modelID string) (*TestConnectionResult, error) {
	if account.Type != AccountTypeUpstream {
		return nil, fmt.Errorf("grok platform only supports type=upstream account, got type=%s", account.Type)
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("upstream account missing base_url or api_key")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 应用账号 model_mapping（与 Forward 路径行为一致）
	mappedModel := modelID
	if resolved, matched := account.ResolveMappedModel(modelID); matched {
		mappedModel = resolved
	}
	if strings.TrimSpace(mappedModel) == "" {
		return nil, fmt.Errorf("missing model")
	}

	payload := map[string]any{
		"model": mappedModel,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 16,
		"stream":     false,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	text := ""
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(respBody, &parsed) == nil {
		for _, choice := range parsed.Choices {
			if choice.Message.Content != "" {
				text = choice.Message.Content
				break
			}
		}
	}
	return &TestConnectionResult{Text: text, MappedModel: mappedModel}, nil
}

// ListUpstreamModels 调 grok2api 网关的 GET /v1/models 拉当前账号可用模型列表。
// grok2api 暴露的是 OpenAI 兼容格式：{"object":"list","data":[{"id":"...", ...}]}。
// 仅返回模型 ID 字符串数组；调用方负责 fallback 到本地默认映射。
//
// 注意：用独立 http.Client 而非 httpUpstream，因为 grok2api 通常部署在本机（localhost:8000），
// httpUpstream 在 Security.URLAllowlist.Enabled=true 下会校验解析 IP 拒绝 loopback / 私有段。
// 模型列表接口非热点路径，独立客户端 15s 超时足够。
func (s *GrokGatewayService) ListUpstreamModels(ctx context.Context, account *Account) ([]string, error) {
	if account.Type != AccountTypeUpstream {
		return nil, fmt.Errorf("grok platform only supports type=upstream account, got type=%s", account.Type)
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("upstream account missing base_url or api_key")
	}

	return FetchOpenAICompatibleUpstreamModels(ctx, baseURL, apiKey)
}

// FetchGrokUpstreamModels 使用 base_url + api_key 探测 grok2api 的 GET /v1/models。
// 供新增/编辑账号表单在账号未保存时直接调用。
func FetchGrokUpstreamModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	return FetchOpenAICompatibleUpstreamModels(ctx, baseURL, apiKey)
}

// ForwardAsChatCompletions receives an OpenAI Chat Completions request and directly
// passes it through to the grok2api gateway's /v1/chat/completions endpoint.
// grok2api's /v1/messages (Anthropic protocol) endpoint does not support some newer
// models (returns "Invalid request"), so we use OpenAI protocol passthrough to avoid
// protocol conversion issues.
func (s *GrokGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	// 1. Get upstream credentials
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("upstream account missing base_url or api_key")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 2. Model mapping
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		return nil, fmt.Errorf("missing model in request")
	}
	mappedModel := originalModel
	if resolved, matched := account.ResolveMappedModel(originalModel); matched {
		mappedModel = resolved
	}
	if mappedModel != originalModel {
		body = ReplaceModelInBody(body, mappedModel)
	}

	clientStream := gjson.GetBytes(body, "stream").Bool()
	reasoningEffort := extractCCReasoningEffortFromBody(body)

	logger.L().Debug("grok forward_as_chat_completions: direct passthrough",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
	)

	// 3. Build upstream request -> /v1/chat/completions
	upstreamURL := baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 4. Send request
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		logger.LegacyPrintf("service.grok_gateway", "forward_as_cc upstream request failed: %v", err)
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 5. Handle error response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		if resp.StatusCode == http.StatusTooManyRequests {
			prefix := logPrefix(getSessionID(c), account.Name)
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
		}

		if grokShouldFailover(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:   resp.StatusCode,
				ResponseBody: respBody,
			}
		}

		writeGatewayCCError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 6. Handle success response
	var usage ClaudeUsage
	var firstTokenMs *int

	if clientStream {
		// Streaming: passthrough SSE lines, extract usage from last chunk
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)
		firstChunk := true

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			if firstChunk && strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
				firstChunk = false
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}

			// Extract usage from streaming chunks that contain it
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"usage"`) {
				payload := strings.TrimPrefix(line, "data: ")
				if payload != "[DONE]" {
					s.extractCCUsage([]byte(payload), &usage)
				}
			}

			fmt.Fprintf(c.Writer, "%s\n", line)
			c.Writer.Flush()
		}
	} else {
		// Non-streaming: passthrough JSON, extract usage
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		s.extractCCUsage(respBody, &usage)

		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(respBody)
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.grok_gateway", "forward_as_cc status=success model=%s duration_ms=%d", originalModel, duration.Milliseconds())

	return &ForwardResult{
		Model:           originalModel,
		UpstreamModel:   mappedModel,
		Stream:          clientStream,
		Duration:        duration,
		FirstTokenMs:    firstTokenMs,
		Usage:           usage,
		ReasoningEffort: reasoningEffort,
	}, nil
}

// extractCCUsage extracts prompt_tokens and completion_tokens from an OpenAI CC response.
func (s *GrokGatewayService) extractCCUsage(data []byte, usage *ClaudeUsage) {
	var wrapper struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &wrapper) == nil && wrapper.Usage != nil {
		usage.InputTokens = wrapper.Usage.PromptTokens
		usage.OutputTokens = wrapper.Usage.CompletionTokens
	}
}

// grokShouldFailover determines if a Grok upstream HTTP status code should trigger account failover.
func grokShouldFailover(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func buildOpenAIModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	switch {
	case strings.HasSuffix(normalized, "/models"):
		return normalized
	case strings.HasSuffix(normalized, "/responses"):
		return strings.TrimSuffix(normalized, "/responses") + "/models"
	case strings.HasSuffix(normalized, "/chat/completions"):
		return strings.TrimSuffix(normalized, "/chat/completions") + "/models"
	case strings.HasSuffix(normalized, "/v1"):
		return normalized + "/models"
	default:
		return normalized + "/v1/models"
	}
}

// FetchOpenAICompatibleUpstreamModels 拉取 OpenAI-compatible /v1/models 的模型 ID 列表。
func FetchOpenAICompatibleUpstreamModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("base_url and api_key are required")
	}
	baseURL = buildOpenAIModelsURL(baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create list-models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream list-models failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list-models returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse list-models response: %w", err)
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
