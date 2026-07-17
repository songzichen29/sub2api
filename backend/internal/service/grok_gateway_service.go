package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
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

	responsesSessionMu     sync.Mutex
	responsesContinuations map[string]grokResponsesContinuation
}

const grokResponsesContinuationTTL = 30 * time.Minute
const grokResponsesContinuationMaxEntries = 2048

type grokResponsesContinuation struct {
	Messages  []apicompat.ChatMessage
	ExpiresAt time.Time
}

type GrokResponsesClientError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *GrokResponsesClientError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
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
// grok2api 的 /v1/messages（Anthropic 协议）对部分新模型返回 "Invalid request"，
// 所以统一走 /v1/chat/completions 协议转换路径，确保所有模型兼容。
func (s *GrokGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	if account.Type != AccountTypeUpstream {
		return nil, fmt.Errorf("grok platform only supports type=upstream account, got type=%s", account.Type)
	}
	return s.ForwardAsCC(ctx, c, account, body)
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

	upstreamURL := buildOpenAIEndpointURL(baseURL, "/v1/messages")

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

		if resp.StatusCode == http.StatusTooManyRequests && s.accountRepo != nil {
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

// ForwardAsCC is a compatibility path that converts Anthropic Messages format
// to Chat Completions format, sends to grok2api /v1/chat/completions, and
// converts the CC response back to Anthropic format. This is needed because
// grok2api's /v1/messages (Anthropic protocol) endpoint does not support some
// newer models (returns "Invalid request").
func (s *GrokGatewayService) ForwardAsCC(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()
	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("upstream account missing base_url or api_key")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 1. Parse Anthropic request.
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse claude request: %w", err)
	}
	originalModel := strings.TrimSpace(anthropicReq.Model)
	if originalModel == "" {
		return nil, fmt.Errorf("missing model")
	}
	clientStream := anthropicReq.Stream

	// 2. Apply model mapping.
	mappedModel := originalModel
	if resolved, matched := account.ResolveMappedModel(originalModel); matched {
		mappedModel = resolved
	}

	// 3. Convert Anthropic -> Responses -> Chat Completions.
	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("convert anthropic to responses: %w", err)
	}
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}
	chatReq.Model = mappedModel
	chatReq.Stream = clientStream
	if chatReq.MaxTokens == nil && chatReq.MaxCompletionTokens != nil {
		chatReq.MaxTokens = chatReq.MaxCompletionTokens
	}
	if anthropicReq.OutputConfig == nil || strings.TrimSpace(anthropicReq.OutputConfig.Effort) == "" {
		chatReq.ReasoningEffort = ""
	}
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	} else {
		chatReq.Stream = false
	}

	ccBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	if !clientStream {
		var ok bool
		ccBody, ok = setJSONValueBytes(ccBody, "stream", false)
		if !ok {
			return nil, fmt.Errorf("set stream false")
		}
	}

	logger.L().Debug("grok forward_as_cc: anthropic->chat_completions conversion",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
	)

	// 4. Send to /v1/chat/completions.
	upstreamURL := buildOpenAIChatCompletionsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(ccBody))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if clientStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		logger.LegacyPrintf("service.grok_gateway", "%s forward_as_cc upstream failed: %v", prefix, err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 5. Handle error response.
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if resp.StatusCode == http.StatusTooManyRequests {
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
		}
		if grokShouldFailover(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: grokShouldRetryOnSameAccount(account, resp.StatusCode),
			}
		}
		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		c.Status(resp.StatusCode)
		_, _ = c.Writer.Write(respBody)
		return &ForwardResult{Model: originalModel, UpstreamModel: mappedModel}, nil
	}

	// 6. Handle success response.
	var usage ClaudeUsage
	var firstTokenMs *int

	if clientStream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		responsesState := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
		anthropicState := apicompat.NewResponsesEventToAnthropicState()
		anthropicState.Model = originalModel

		writeAnthropicEvents := func(events []apicompat.AnthropicStreamEvent) {
			for _, evt := range events {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
				if err != nil {
					logger.L().Warn("grok forward_as_cc: marshal anthropic stream event failed", zap.Error(err))
					continue
				}
				fmt.Fprint(c.Writer, sse)
			}
		}
		writeResponsesEvents := func(events []apicompat.ResponsesStreamEvent) {
			for i := range events {
				writeAnthropicEvents(apicompat.ResponsesEventToAnthropicEvents(&events[i], anthropicState))
			}
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)
		currentEvent := ""
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				currentEvent = ""
				continue
			}
			if eventName, ok := extractOpenAISSEEventLine(line); ok {
				currentEvent = strings.TrimSpace(eventName)
				continue
			}

			payload, ok := extractOpenAISSEDataLine(line)
			if !ok {
				continue
			}
			payload = strings.TrimSpace(payload)
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				break
			}
			if streamErr := detectGrokSSEError(currentEvent, payload); streamErr != nil {
				return nil, s.grokSSEErrorAsFailover(ctx, c, account, resp.Header, streamErr)
			}

			// usage is captured for billing, but usage-only chunks should not
			// create empty output blocks in Anthropic conversion.
			_ = s.extractCCUsage([]byte(payload), &usage)
			if isOpenAIChatUsageOnlyStreamChunk(payload) {
				continue
			}

			var chunk apicompat.ChatCompletionsChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				logger.L().Warn("grok forward_as_cc: parse chat completions stream chunk failed", zap.Error(err))
				continue
			}
			if firstTokenMs == nil && !isOpenAIChatUsageOnlyStreamChunk(payload) && chatChunkStartsResponsesOutput(&chunk) {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			writeResponsesEvents(apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, responsesState))
			c.Writer.Flush()
		}
		if err := scanner.Err(); err != nil {
			logger.L().Warn("grok forward_as_cc: stream read error", zap.Error(err))
		}

		writeResponsesEvents(apicompat.FinalizeChatCompletionsResponsesStream(responsesState))
		writeAnthropicEvents(apicompat.FinalizeResponsesAnthropicStream(anthropicState))
		c.Writer.Flush()
	} else {
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		var ccResp *apicompat.ChatCompletionsResponse
		if grokLooksLikeSSE(resp.Header.Get("Content-Type"), respBody) {
			var sawUsage bool
			ccResp, usage, sawUsage, err = s.collectGrokChatCompletionsSSE(ctx, c, account, resp.Header, bytes.NewReader(respBody), originalModel)
			if err != nil {
				return nil, err
			}
			_ = sawUsage
		} else {
			s.extractCCUsage(respBody, &usage)

			var parsed apicompat.ChatCompletionsResponse
			if err := json.Unmarshal(respBody, &parsed); err != nil {
				return nil, fmt.Errorf("parse upstream CC response: %w", err)
			}
			ccResp = &parsed
		}
		responsesResp := apicompat.ChatCompletionsResponseToResponses(ccResp, originalModel, nil, false, nil)
		anthropicResp := apicompat.ResponsesToAnthropic(responsesResp, originalModel)
		anthropicBytes, err := json.Marshal(anthropicResp)
		if err != nil {
			return nil, fmt.Errorf("marshal anthropic response: %w", err)
		}
		c.Header("Content-Type", "application/json")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(anthropicBytes)
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.grok_gateway", "%s forward_as_cc status=success model=%s duration_ms=%d", prefix, originalModel, duration.Milliseconds())

	return &ForwardResult{
		Model:            originalModel,
		UpstreamModel:    mappedModel,
		Stream:           clientStream,
		Duration:         duration,
		FirstTokenMs:     firstTokenMs,
		Usage:            usage,
		ClientDisconnect: false,
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildOpenAIChatCompletionsURL(baseURL), bytes.NewReader(body))
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
	if clientStream {
		var ok bool
		body, ok = setJSONValueBytes(body, "stream_options.include_usage", true)
		if !ok {
			return nil, fmt.Errorf("enable stream usage")
		}
	} else {
		var ok bool
		body, ok = setJSONValueBytes(body, "stream", false)
		if !ok {
			return nil, fmt.Errorf("set stream false")
		}
	}

	logger.L().Debug("grok forward_as_chat_completions: direct passthrough",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
	)

	// 3. Build upstream request -> /v1/chat/completions
	upstreamURL := buildOpenAIChatCompletionsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if clientStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

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

		if resp.StatusCode == http.StatusTooManyRequests && s.accountRepo != nil {
			prefix := logPrefix(getSessionID(c), account.Name)
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
		}

		if grokShouldFailover(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: grokShouldRetryOnSameAccount(account, resp.StatusCode),
			}
		}

		writeGatewayCCError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 6. Handle success response
	var usage ClaudeUsage
	var firstTokenMs *int
	sawUsage := false

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
		currentEvent := ""

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				currentEvent = ""
				fmt.Fprint(c.Writer, "\n")
				c.Writer.Flush()
				continue
			}
			if eventName, ok := extractOpenAISSEEventLine(line); ok {
				currentEvent = strings.TrimSpace(eventName)
				if strings.EqualFold(currentEvent, "error") {
					continue
				}
				fmt.Fprintf(c.Writer, "%s\n", line)
				c.Writer.Flush()
				continue
			}

			if payload, ok := extractOpenAISSEDataLine(line); ok {
				payload = strings.TrimSpace(payload)
				if payload != "" && payload != "[DONE]" {
					if streamErr := detectGrokSSEError(currentEvent, payload); streamErr != nil {
						return nil, s.grokSSEErrorAsFailover(ctx, c, account, resp.Header, streamErr)
					}
					if firstChunk {
						firstChunk = false
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
					}

					// Extract usage from streaming chunks that contain it.
					if s.extractCCUsage([]byte(payload), &usage) {
						sawUsage = true
					}
				}
			}

			fmt.Fprintf(c.Writer, "%s\n", line)
			c.Writer.Flush()
		}
		if err := scanner.Err(); err != nil {
			logger.L().Warn("grok forward_as_chat_completions: stream read error", zap.Error(err))
		}
	} else {
		// Non-streaming: passthrough JSON, extract usage
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		wasSSE := grokLooksLikeSSE(resp.Header.Get("Content-Type"), respBody)
		if wasSSE {
			ccResp, sseUsage, sseSawUsage, err := s.collectGrokChatCompletionsSSE(ctx, c, account, resp.Header, bytes.NewReader(respBody), originalModel)
			if err != nil {
				return nil, err
			}
			respBody, err = json.Marshal(ccResp)
			if err != nil {
				return nil, fmt.Errorf("marshal collected SSE chat response: %w", err)
			}
			usage = sseUsage
			sawUsage = sseSawUsage
		} else {
			sawUsage = s.extractCCUsage(respBody, &usage)
		}

		if wasSSE {
			c.Header("Content-Type", "application/json")
		} else {
			c.Header("Content-Type", resp.Header.Get("Content-Type"))
		}
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(respBody)
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.grok_gateway", "forward_as_cc status=success model=%s duration_ms=%d", originalModel, duration.Milliseconds())
	logGrokZeroUsageIfNeeded(account, originalModel, mappedModel, clientStream, false, sawUsage, usage)

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

// extractCCUsage extracts token usage from OpenAI Chat Completions style
// responses and Grok-compatible variants. It returns true when an upstream
// usage object is present. Zero values never overwrite previously extracted
// non-zero values because some streaming chunks may carry partial/empty usage.
func (s *GrokGatewayService) extractCCUsage(data []byte, usage *ClaudeUsage) bool {
	if usage == nil {
		return false
	}

	root := gjson.ParseBytes(data)
	sawUsage := false
	for _, path := range []string{"usage", "response.usage"} {
		node := root.Get(path)
		if !node.Exists() || !node.IsObject() {
			continue
		}
		sawUsage = true
		applyGrokCCUsageNode(node, usage)
	}
	return sawUsage
}

func applyGrokCCUsageNode(node gjson.Result, usage *ClaudeUsage) {
	if v := firstPositiveGrokUsageInt(node, "prompt_tokens_details.cached_tokens", "input_tokens_details.cached_tokens"); v > 0 {
		usage.CacheReadInputTokens = v
	}
	if v := firstPositiveGrokUsageInt(node, "prompt_tokens", "input_tokens"); v > 0 {
		// OpenAI-compatible usage reports cached_tokens as a subset of the total
		// prompt/input tokens. Internal billing expects the categories to be disjoint.
		usage.InputTokens = max(v-usage.CacheReadInputTokens, 0)
	}
	if v := firstPositiveGrokUsageInt(node, "completion_tokens", "output_tokens"); v > 0 {
		usage.OutputTokens = v
	}
}

func firstPositiveGrokUsageInt(node gjson.Result, paths ...string) int {
	for _, path := range paths {
		result := node.Get(path)
		if result.Exists() {
			if v := int(result.Int()); v > 0 {
				return v
			}
		}
	}
	return 0
}

type grokSSEError struct {
	StatusCode int
	Message    string
	Body       []byte
}

func detectGrokSSEError(eventName, payload string) *grokSSEError {
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "[DONE]" {
		return nil
	}

	isErrorEvent := strings.EqualFold(strings.TrimSpace(eventName), "error")
	hasErrorObject := gjson.Get(payload, "error").Exists()
	isTypedError := strings.EqualFold(strings.TrimSpace(gjson.Get(payload, "type").String()), "error")
	if !isErrorEvent && !hasErrorObject && !isTypedError {
		return nil
	}

	body := []byte(payload)
	message := strings.TrimSpace(extractUpstreamErrorMessage(body))
	if message == "" {
		message = strings.TrimSpace(gjson.Get(payload, "error").String())
	}
	if message == "" {
		message = "upstream stream error"
	}

	return &grokSSEError{
		StatusCode: inferGrokSSEErrorStatus(payload, message),
		Message:    sanitizeUpstreamErrorMessage(message),
		Body:       body,
	}
}

func inferGrokSSEErrorStatus(payload, message string) int {
	for _, path := range []string{"error.status", "error.status_code", "error.code", "status", "status_code", "code"} {
		raw := strings.TrimSpace(gjson.Get(payload, path).String())
		if raw == "" {
			continue
		}
		if code, err := strconv.Atoi(raw); err == nil && code >= 400 && code <= 599 {
			return code
		}
	}

	lower := strings.ToLower(message + " " + payload)
	switch {
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(lower, "529") || strings.Contains(lower, "overloaded"):
		return 529
	case strings.Contains(lower, "503"):
		return http.StatusServiceUnavailable
	case strings.Contains(lower, "502"):
		return http.StatusBadGateway
	case strings.Contains(lower, "500"):
		return http.StatusInternalServerError
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return http.StatusForbidden
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return http.StatusUnauthorized
	case strings.Contains(lower, "400"):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func (s *GrokGatewayService) grokSSEErrorAsFailover(ctx context.Context, c *gin.Context, account *Account, header http.Header, streamErr *grokSSEError) error {
	if streamErr == nil {
		return nil
	}
	statusCode := streamErr.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadGateway
	}
	if statusCode == http.StatusTooManyRequests && s != nil && account != nil && s.accountRepo != nil {
		prefix := logPrefix(getSessionID(c), account.Name)
		s.handleUpstreamError(ctx, prefix, account, statusCode, header, streamErr.Body)
	}
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           streamErr.Body,
		RetryableOnSameAccount: grokShouldRetryOnSameAccount(account, statusCode),
	}
}

func grokLooksLikeSSE(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("data:")) || bytes.HasPrefix(trimmed, []byte("event:"))
}

func (s *GrokGatewayService) collectGrokChatCompletionsSSE(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	header http.Header,
	reader io.Reader,
	model string,
) (*apicompat.ChatCompletionsResponse, ClaudeUsage, bool, error) {
	var usage ClaudeUsage
	sawUsage := false
	state := apicompat.NewChatCompletionsToResponsesStreamState(model)
	responseID := ""
	created := int64(0)
	upstreamModel := model
	finishReason := ""
	currentEvent := ""

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			currentEvent = ""
			continue
		}
		if eventName, ok := extractOpenAISSEEventLine(line); ok {
			currentEvent = strings.TrimSpace(eventName)
			continue
		}
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		if streamErr := detectGrokSSEError(currentEvent, payload); streamErr != nil {
			return nil, usage, sawUsage, s.grokSSEErrorAsFailover(ctx, c, account, header, streamErr)
		}
		if s.extractCCUsage([]byte(payload), &usage) {
			sawUsage = true
		}
		if isOpenAIChatUsageOnlyStreamChunk(payload) {
			continue
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			logger.L().Warn("grok collect chat completions SSE: parse chunk failed", zap.Error(err))
			continue
		}
		if responseID == "" && strings.TrimSpace(chunk.ID) != "" {
			responseID = strings.TrimSpace(chunk.ID)
		}
		if created == 0 && chunk.Created > 0 {
			created = chunk.Created
		}
		if strings.TrimSpace(chunk.Model) != "" {
			upstreamModel = strings.TrimSpace(chunk.Model)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
				finishReason = strings.TrimSpace(*choice.FinishReason)
			}
		}
		_ = apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state)
	}
	if err := scanner.Err(); err != nil {
		return nil, usage, sawUsage, fmt.Errorf("read upstream SSE response: %w", err)
	}

	if responseID == "" {
		responseID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	if finishReason == "" {
		finishReason = state.FinishReason
	}
	if finishReason == "" {
		finishReason = "stop"
	}

	message, ok := grokAssistantMessageFromResponsesStreamState(state)
	if !ok {
		message = apicompat.ChatMessage{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
		}
	}

	resp := &apicompat.ChatCompletionsResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: created,
		Model:   upstreamModel,
		Choices: []apicompat.ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
	}
	if sawUsage {
		resp.Usage = grokClaudeUsageToChatUsage(usage)
	}
	return resp, usage, sawUsage, nil
}

func grokClaudeUsageToChatUsage(usage ClaudeUsage) *apicompat.ChatUsage {
	promptTokens := usage.InputTokens + usage.CacheReadInputTokens
	total := promptTokens + usage.OutputTokens
	out := &apicompat.ChatUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      total,
	}
	if usage.CacheReadInputTokens > 0 {
		out.PromptTokensDetails = &apicompat.ChatTokenDetails{
			CachedTokens: usage.CacheReadInputTokens,
		}
	}
	return out
}

func (u ClaudeUsage) totalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens + u.CacheCreation5mTokens + u.CacheCreation1hTokens + u.ImageOutputTokens
}

func (s *GrokGatewayService) grokContinuationScope(c *gin.Context, account *Account) string {
	platform := ""
	if account != nil {
		platform = account.Platform
	}
	apiKeyID := int64(0)
	userID := int64(0)
	groupID := int64(0)
	if apiKey := getAPIKeyFromContext(c); apiKey != nil {
		apiKeyID = apiKey.ID
		userID = apiKey.UserID
		if apiKey.GroupID != nil {
			groupID = *apiKey.GroupID
		}
	}
	return fmt.Sprintf("platform=%s:user=%d:key=%d:group=%d", platform, userID, apiKeyID, groupID)
}

func (s *GrokGatewayService) grokContinuationKey(scope, responseID string) string {
	return scope + ":response=" + strings.TrimSpace(responseID)
}

func cloneGrokChatMessages(messages []apicompat.ChatMessage) []apicompat.ChatMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]apicompat.ChatMessage, len(messages))
	copy(out, messages)
	for i := range out {
		if len(messages[i].Content) > 0 {
			out[i].Content = append(json.RawMessage(nil), messages[i].Content...)
		}
		if len(messages[i].ToolCalls) > 0 {
			out[i].ToolCalls = append([]apicompat.ChatToolCall(nil), messages[i].ToolCalls...)
		}
		if messages[i].FunctionCall != nil {
			fc := *messages[i].FunctionCall
			out[i].FunctionCall = &fc
		}
	}
	return out
}

func grokAssistantMessageFromChatResponse(resp *apicompat.ChatCompletionsResponse) (apicompat.ChatMessage, bool) {
	if resp == nil || len(resp.Choices) == 0 {
		return apicompat.ChatMessage{}, false
	}
	message := resp.Choices[0].Message
	if strings.TrimSpace(message.Role) == "" {
		message.Role = "assistant"
	}
	return message, true
}

func grokAssistantMessageFromResponsesStreamState(state *apicompat.ChatCompletionsToResponsesStreamState) (apicompat.ChatMessage, bool) {
	if state == nil {
		return apicompat.ChatMessage{}, false
	}
	content, _ := json.Marshal(state.Text.String())
	message := apicompat.ChatMessage{
		Role:    "assistant",
		Content: content,
	}
	if state.Reasoning.Len() > 0 {
		message.ReasoningContent = state.Reasoning.String()
	}
	if len(state.ToolCalls) > 0 {
		for i := 0; i < len(state.ToolCalls); i++ {
			toolCall := state.ToolCalls[i]
			if toolCall == nil {
				continue
			}
			copyCall := *toolCall
			if strings.TrimSpace(copyCall.Type) == "" {
				copyCall.Type = "function"
			}
			message.ToolCalls = append(message.ToolCalls, copyCall)
		}
	}
	if state.Text.Len() == 0 && state.Reasoning.Len() == 0 && len(message.ToolCalls) == 0 {
		return apicompat.ChatMessage{}, false
	}
	return message, true
}

func (s *GrokGatewayService) prependGrokContinuationMessages(scope string, req *apicompat.ResponsesRequest, chatReq *apicompat.ChatCompletionsRequest) (bool, error) {
	if req == nil || chatReq == nil {
		return false, nil
	}
	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID == "" {
		return false, nil
	}
	if ClassifyOpenAIPreviousResponseIDKind(previousResponseID) != OpenAIPreviousResponseIDKindResponseID {
		return false, &GrokResponsesClientError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request_error",
			Message:    "previous_response_id must be a response.id (resp_*)",
		}
	}

	key := s.grokContinuationKey(scope, previousResponseID)
	now := time.Now()

	s.responsesSessionMu.Lock()
	defer s.responsesSessionMu.Unlock()
	s.pruneExpiredGrokContinuationsLocked(now)

	entry, ok := s.responsesContinuations[key]
	if !ok || now.After(entry.ExpiresAt) {
		if ok {
			delete(s.responsesContinuations, key)
		}
		return false, &GrokResponsesClientError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request_error",
			Message:    "previous_response_id not found or expired for Grok Responses chat fallback",
		}
	}

	history := cloneGrokChatMessages(entry.Messages)
	current := cloneGrokChatMessages(chatReq.Messages)
	chatReq.Messages = append(history, current...)
	return true, nil
}

func (s *GrokGatewayService) storeGrokContinuation(scope, responseID string, messages []apicompat.ChatMessage) {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || len(messages) == 0 {
		return
	}

	now := time.Now()
	key := s.grokContinuationKey(scope, responseID)
	entry := grokResponsesContinuation{
		Messages:  cloneGrokChatMessages(messages),
		ExpiresAt: now.Add(grokResponsesContinuationTTL),
	}

	s.responsesSessionMu.Lock()
	defer s.responsesSessionMu.Unlock()
	if s.responsesContinuations == nil {
		s.responsesContinuations = make(map[string]grokResponsesContinuation)
	}
	s.pruneExpiredGrokContinuationsLocked(now)
	if len(s.responsesContinuations) >= grokResponsesContinuationMaxEntries {
		s.evictOldestGrokContinuationLocked()
	}
	s.responsesContinuations[key] = entry
}

func (s *GrokGatewayService) pruneExpiredGrokContinuationsLocked(now time.Time) {
	if len(s.responsesContinuations) == 0 {
		return
	}
	for key, entry := range s.responsesContinuations {
		if now.After(entry.ExpiresAt) {
			delete(s.responsesContinuations, key)
		}
	}
}

func (s *GrokGatewayService) evictOldestGrokContinuationLocked() {
	oldestKey := ""
	var oldestTime time.Time
	for key, entry := range s.responsesContinuations {
		if oldestKey == "" || entry.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ExpiresAt
		}
	}
	if oldestKey != "" {
		delete(s.responsesContinuations, oldestKey)
	}
}

func logGrokZeroUsageIfNeeded(account *Account, originalModel, mappedModel string, stream bool, hasPreviousResponseID bool, sawUsage bool, usage ClaudeUsage) {
	if usage.totalTokens() != 0 {
		return
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	logger.L().Warn("grok gateway: zero token usage from chat completions upstream",
		zap.Int64("account_id", accountID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("stream", stream),
		zap.Bool("has_previous_response_id", hasPreviousResponseID),
		zap.Bool("saw_upstream_usage", sawUsage),
	)
}

// ForwardAsResponses accepts an OpenAI Responses API request body, converts it
// to Chat Completions format, sends it to grok2api /v1/chat/completions, and
// converts the Chat Completions response back to Responses format.
// grok2api's native /v1/responses endpoint does not support some newer models,
// so we use /v1/chat/completions with bidirectional protocol conversion.
func (s *GrokGatewayService) ForwardAsResponses(
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

	// 2. Parse Responses request
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := responsesReq.Model
	clientStream := responsesReq.Stream
	hasPreviousResponseID := strings.TrimSpace(responsesReq.PreviousResponseID) != ""
	continuationScope := s.grokContinuationScope(c, account)

	// 3. Convert Responses -> Chat Completions
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&responsesReq)
	if err != nil {
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}

	// 4. Model mapping
	mappedModel := originalModel
	if resolved, matched := account.ResolveMappedModel(originalModel); matched {
		mappedModel = resolved
	}
	chatReq.Model = mappedModel
	if chatReq.MaxTokens == nil && chatReq.MaxCompletionTokens != nil {
		chatReq.MaxTokens = chatReq.MaxCompletionTokens
	}
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	} else {
		chatReq.Stream = false
	}
	continuationAttached, err := s.prependGrokContinuationMessages(continuationScope, &responsesReq, chatReq)
	if err != nil {
		var clientErr *GrokResponsesClientError
		if errors.As(err, &clientErr) {
			return nil, clientErr
		}
		return nil, err
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	if !clientStream {
		var ok bool
		chatBody, ok = setJSONValueBytes(chatBody, "stream", false)
		if !ok {
			return nil, fmt.Errorf("set stream false")
		}
	}
	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body)

	logger.L().Debug("grok forward_as_responses: responses->chat_completions conversion",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
		zap.Bool("has_previous_response_id", hasPreviousResponseID),
		zap.Bool("continuation_attached", continuationAttached),
	)

	// 5. Build upstream request -> /v1/chat/completions
	upstreamURL := buildOpenAIChatCompletionsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(chatBody))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 6. Send request
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		logger.LegacyPrintf("service.grok_gateway", "forward_as_responses upstream request failed: %v", err)
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 7. Handle error response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		if resp.StatusCode == http.StatusTooManyRequests && s.accountRepo != nil {
			prefix := logPrefix(getSessionID(c), account.Name)
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
		}

		if grokShouldFailover(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: grokShouldRetryOnSameAccount(account, resp.StatusCode),
			}
		}

		writeResponsesError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 8. Handle success response
	var usage ClaudeUsage
	var firstTokenMs *int
	sawUsage := false

	if clientStream {
		// Streaming: convert CC SSE chunks to Responses SSE events
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)
		state := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
		firstChunk := true
		sawDone := false
		writeResponsesEvents := func(events []apicompat.ResponsesStreamEvent) {
			for _, evt := range events {
				sse, err := apicompat.ResponsesEventToSSE(evt)
				if err != nil {
					logger.L().Warn("grok forward_as_responses: failed to marshal stream event",
						zap.Error(err),
						zap.String("event_type", evt.Type),
					)
					continue
				}
				_, _ = fmt.Fprint(c.Writer, sse)
			}
		}

		for scanner.Scan() {
			payload, ok := extractOpenAISSEDataLine(scanner.Text())
			if !ok {
				continue
			}
			payload = strings.TrimSpace(payload)
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				sawDone = true
				break
			}

			// Extract usage
			if strings.Contains(payload, "\"usage\"") {
				sawUsage = true
				s.extractCCUsage([]byte(payload), &usage)
			}

			var ccChunk apicompat.ChatCompletionsChunk
			if json.Unmarshal([]byte(payload), &ccChunk) == nil {
				if firstChunk && chatChunkStartsResponsesOutput(&ccChunk) {
					firstChunk = false
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				writeResponsesEvents(apicompat.ChatCompletionsChunkToResponsesEvents(&ccChunk, state))
			}
			c.Writer.Flush()
		}
		if err := scanner.Err(); err != nil {
			logger.L().Warn("grok forward_as_responses: stream read error", zap.Error(err))
		}
		writeResponsesEvents(apicompat.FinalizeChatCompletionsResponsesStream(state))
		conversation := cloneGrokChatMessages(chatReq.Messages)
		if assistantMessage, ok := grokAssistantMessageFromResponsesStreamState(state); ok {
			conversation = append(conversation, assistantMessage)
		}
		s.storeGrokContinuation(continuationScope, state.ResponseID, conversation)
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
		if !sawDone {
			logger.L().Debug("grok forward_as_responses: upstream stream ended without done sentinel")
		}
	} else {
		// Non-streaming: read CC response, convert to Responses
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		var ccResp *apicompat.ChatCompletionsResponse
		if grokLooksLikeSSE(resp.Header.Get("Content-Type"), respBody) {
			var sseUsage ClaudeUsage
			var sseSawUsage bool
			ccResp, sseUsage, sseSawUsage, err = s.collectGrokChatCompletionsSSE(ctx, c, account, resp.Header, bytes.NewReader(respBody), originalModel)
			if err != nil {
				return nil, err
			}
			usage = sseUsage
			sawUsage = sseSawUsage
		} else {
			var parsed apicompat.ChatCompletionsResponse
			if err := json.Unmarshal(respBody, &parsed); err != nil {
				return nil, fmt.Errorf("parse upstream CC response: %w", err)
			}
			ccResp = &parsed
			sawUsage = ccResp.Usage != nil
			s.extractCCUsage(respBody, &usage)
		}

		responsesResp := apicompat.ChatCompletionsResponseToResponses(ccResp, originalModel, nil, false, nil)
		conversation := cloneGrokChatMessages(chatReq.Messages)
		if assistantMessage, ok := grokAssistantMessageFromChatResponse(ccResp); ok {
			conversation = append(conversation, assistantMessage)
		}
		s.storeGrokContinuation(continuationScope, responsesResp.ID, conversation)
		if respBytes, err := json.Marshal(responsesResp); err == nil {
			c.Header("Content-Type", "application/json")
			c.Status(http.StatusOK)
			_, _ = c.Writer.Write(respBytes)
		} else {
			c.Header("Content-Type", "application/json")
			c.Status(http.StatusOK)
			_, _ = c.Writer.Write(respBody)
		}
	}

	duration := time.Since(startTime)
	logger.LegacyPrintf("service.grok_gateway", "forward_as_responses status=success model=%s duration_ms=%d", originalModel, duration.Milliseconds())
	logGrokZeroUsageIfNeeded(account, originalModel, mappedModel, clientStream, hasPreviousResponseID, sawUsage, usage)

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

// grokShouldFailover determines if a Grok upstream HTTP status code should trigger account failover.
func grokShouldFailover(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func grokShouldRetryOnSameAccount(account *Account, statusCode int) bool {
	if account == nil || !account.IsPoolMode() {
		return false
	}
	return isPoolModeRetryableStatus(statusCode) ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == 529
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
	default:
		return buildOpenAIEndpointURL(normalized, "/v1/models")
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
