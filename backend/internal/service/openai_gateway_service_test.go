package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 编译期接口断言
var _ AccountRepository = (*stubOpenAIAccountRepo)(nil)
var _ GatewayCache = (*stubGatewayCache)(nil)

func TestOpenAIStreamDataStartsFirstToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		data      string
		eventType string
		want      bool
	}{
		{name: "empty", data: "", eventType: "", want: false},
		{name: "done_marker", data: "[DONE]", eventType: "", want: false},
		{name: "preamble_created", data: `{"type":"response.created"}`, eventType: "response.created", want: false},
		{name: "preamble_in_progress", data: `{"type":"response.in_progress"}`, eventType: "response.in_progress", want: false},
		{name: "output_item_added", data: `{"type":"response.output_item.added","item":{"type":"message"}}`, eventType: "response.output_item.added", want: true},
		{name: "output_item_done", data: `{"type":"response.output_item.done"}`, eventType: "response.output_item.done", want: true},
		{name: "terminal_completed", data: `{"type":"response.completed"}`, eventType: "response.completed", want: false},
		{name: "terminal_done", data: `{"type":"response.done"}`, eventType: "response.done", want: false},
		{name: "terminal_failed", data: `{"type":"response.failed"}`, eventType: "response.failed", want: false},
		{name: "error_event", data: `{"type":"error","error":{"message":"boom"}}`, eventType: "error", want: false},
		{name: "text_delta", data: `{"type":"response.output_text.delta","delta":"h"}`, eventType: "response.output_text.delta", want: true},
		{name: "empty_text_delta", data: `{"type":"response.output_text.delta","delta":""}`, eventType: "response.output_text.delta", want: true},
		{name: "whitespace_text_delta", data: `{"type":"response.output_text.delta","delta":"   "}`, eventType: "response.output_text.delta", want: true},
		{name: "audio_delta", data: `{"type":"response.output_audio.delta","delta":"abc"}`, eventType: "response.output_audio.delta", want: true},
		{name: "function_arguments_delta", data: `{"type":"response.function_call_arguments.delta","delta":"{}"}`, eventType: "response.function_call_arguments.delta", want: true},
		{name: "reasoning_summary_delta", data: `{"type":"response.reasoning_summary_text.delta","delta":"think"}`, eventType: "response.reasoning_summary_text.delta", want: true},
		{name: "unknown_delta", data: `{"type":"response.custom.delta","delta":"x"}`, eventType: "response.custom.delta", want: true},
		{name: "unknown_non_delta", data: `{"type":"response.custom","value":"x"}`, eventType: "response.custom", want: true},
		{name: "output_text_done", data: `{"type":"response.output_text.done","text":"h"}`, eventType: "response.output_text.done", want: true},
		{name: "output_audio_done", data: `{"type":"response.output_audio.done"}`, eventType: "response.output_audio.done", want: true},
		{name: "output_text_annotation_added", data: `{"type":"response.output_text.annotation.added"}`, eventType: "response.output_text.annotation.added", want: true},
		{name: "event_named_delta_without_type", data: `{"delta":"h"}`, eventType: "response.output_text.delta", want: true},
		{name: "event_named_done_without_type", data: `{"text":"h"}`, eventType: "response.output_text.done", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, openAIStreamDataStartsFirstToken(tc.data, tc.eventType))
		})
	}
}

func TestOpenAIStreamDebugTimingEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enabledValues := []string{"1", "true", "TRUE", "on", "enabled", "yes", " yes "}
	for _, value := range enabledValues {
		t.Run("enabled_"+strings.TrimSpace(value), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.Header.Set(openAIStreamDebugTimingHeader, value)
			require.True(t, openAIStreamDebugTimingEnabled(c))
		})
	}

	disabledValues := []string{"", "0", "false", "off", "disabled", "no", "random"}
	for _, value := range disabledValues {
		t.Run("disabled_"+strings.TrimSpace(value), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.Header.Set(openAIStreamDebugTimingHeader, value)
			require.False(t, openAIStreamDebugTimingEnabled(c))
		})
	}

	require.False(t, openAIStreamDebugTimingEnabled(nil))
}

func TestOpenAIChatChunkStartsFirstToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "empty", payload: "", want: false},
		{name: "usage_only", payload: `{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`, want: false},
		{name: "role_only", payload: `{"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`, want: false},
		{name: "empty_delta_finish", payload: `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, want: false},
		{name: "content", payload: `{"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`, want: true},
		{name: "empty_content_present", payload: `{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}`, want: true},
		{name: "reasoning_content", payload: `{"choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`, want: true},
		{name: "tool_calls", payload: `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":null}]}`, want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, openAIChatChunkStartsFirstToken(tc.payload))
		})
	}
}

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
}

type snapshotUpdateAccountRepo struct {
	stubOpenAIAccountRepo
	updateExtraCalls chan map[string]any
}

func (r *snapshotUpdateAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if r.updateExtraCalls != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCalls <- copied
	}
	return nil
}

func (r stubOpenAIAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (r stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r stubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

type groupAwareStubOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
}

func (r groupAwareStubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform && openAIStickyAccountMatchesGroup(&acc, &groupID) {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r groupAwareStubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform && openAIStickyAccountMatchesGroup(&acc, nil) {
			result = append(result, acc)
		}
	}
	return result, nil
}

type stubConcurrencyCache struct {
	ConcurrencyCache
	loadBatchErr    error
	loadMap         map[int64]*AccountLoadInfo
	acquireResults  map[int64]bool
	waitCounts      map[int64]int
	skipDefaultLoad bool
}

type cancelReadCloser struct{}

func (c cancelReadCloser) Read(p []byte) (int, error) { return 0, context.Canceled }
func (c cancelReadCloser) Close() error               { return nil }

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (r errReadCloser) Close() error             { return nil }

type failingGinWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
}

func (w *failingGinWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

type slowGinWriter struct {
	gin.ResponseWriter
	delay time.Duration
}

func (w *slowGinWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	return w.ResponseWriter.Write(p)
}

func (c stubConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	if c.acquireResults != nil {
		if result, ok := c.acquireResults[accountID]; ok {
			return result, nil
		}
	}
	return true, nil
}

func (c stubConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
}

func (c stubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if c.loadBatchErr != nil {
		return nil, c.loadBatchErr
	}
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	if c.skipDefaultLoad && c.loadMap != nil {
		for _, acc := range accounts {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
			}
		}
		return out, nil
	}
	for _, acc := range accounts {
		if c.loadMap != nil {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
				continue
			}
		}
		out[acc.ID] = &AccountLoadInfo{AccountID: acc.ID, LoadRate: 0}
	}
	return out, nil
}

func TestOpenAIGatewayService_GenerateSessionHash_Priority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{}

	bodyWithKey := []byte(`{"prompt_cache_key":"ses_aaa"}`)

	// 1) session_id header wins
	c.Request.Header.Set("session_id", "sess-123")
	c.Request.Header.Set("conversation_id", "conv-456")
	h1 := svc.GenerateSessionHash(c, bodyWithKey)
	if h1 == "" {
		t.Fatalf("expected non-empty hash")
	}

	// 2) conversation_id used when session_id absent
	c.Request.Header.Del("session_id")
	h2 := svc.GenerateSessionHash(c, bodyWithKey)
	if h2 == "" {
		t.Fatalf("expected non-empty hash")
	}
	if h1 == h2 {
		t.Fatalf("expected different hashes for different keys")
	}

	// 3) prompt_cache_key used when both headers absent
	c.Request.Header.Del("conversation_id")
	h3 := svc.GenerateSessionHash(c, bodyWithKey)
	if h3 == "" {
		t.Fatalf("expected non-empty hash")
	}
	if h2 == h3 {
		t.Fatalf("expected different hashes for different keys")
	}

	// 4) empty when no signals
	h4 := svc.GenerateSessionHash(c, []byte(`{}`))
	if h4 != "" {
		t.Fatalf("expected empty hash when no signals")
	}
}

func TestOpenAIGatewayService_GenerateSessionHash_UsesXXHash64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-fixed-value")
	svc := &OpenAIGatewayService{}

	got := svc.GenerateSessionHash(c, nil)
	want := fmt.Sprintf("%016x", xxhash.Sum64String("sess-fixed-value"))
	require.Equal(t, want, got)
}

func TestOpenAIGatewayService_GenerateSessionHash_AttachesLegacyHashToContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-legacy-check")
	svc := &OpenAIGatewayService{}

	sessionHash := svc.GenerateSessionHash(c, nil)
	require.NotEmpty(t, sessionHash)
	require.NotNil(t, c.Request)
	require.NotNil(t, c.Request.Context())
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))
}

func TestOpenAIGatewayService_GenerateExplicitSessionHash_SkipsContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"}`)

	t.Run("stateless image body stays unstuck", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

		require.Empty(t, svc.GenerateExplicitSessionHash(c, body))
		require.Empty(t, openAILegacySessionHashFromContext(c.Request.Context()))
	})

	t.Run("prompt_cache_key is explicit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

		got := svc.GenerateExplicitSessionHash(c, []byte(`{"model":"gpt-image-2","prompt_cache_key":"image-session"}`))
		require.Equal(t, fmt.Sprintf("%016x", xxhash.Sum64String("image-session")), got)
		require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))
	})

	t.Run("header overrides body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("session_id", "header-session")

		got := svc.GenerateExplicitSessionHash(c, []byte(`{"prompt_cache_key":"body-session"}`))
		require.Equal(t, fmt.Sprintf("%016x", xxhash.Sum64String("header-session")), got)
	})
}

func TestOpenAIGatewayService_GenerateSessionHashWithFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{}
	seed := "openai_ws_ingress:9:100:200"

	got := svc.GenerateSessionHashWithFallback(c, []byte(`{}`), seed)
	want := fmt.Sprintf("%016x", xxhash.Sum64String(seed))
	require.Equal(t, want, got)
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))

	empty := svc.GenerateSessionHashWithFallback(c, []byte(`{}`), "   ")
	require.Equal(t, "", empty)
}

func TestOpenAIGatewayService_GenerateSessionHash_ContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"}]}`)

	hash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, hash, "content-based fallback should produce a hash")

	hash2 := svc.GenerateSessionHash(c, body)
	require.Equal(t, hash, hash2, "same content should produce same hash")

	bodyExtended := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi!"},{"role":"user","content":"How are you?"}]}`)
	hashExtended := svc.GenerateSessionHash(c, bodyExtended)
	require.Equal(t, hash, hashExtended, "hash should be stable across later turns")

	bodyDifferent := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Different question"}]}`)
	hashDifferent := svc.GenerateSessionHash(c, bodyDifferent)
	require.NotEqual(t, hash, hashDifferent, "different content should produce different hash")
}

func TestOpenAIGatewayService_GenerateSessionHash_ExplicitSignalWinsOverContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hello"}]}`)

	contentHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, contentHash)

	c.Request.Header.Set("session_id", "explicit-session")
	explicitHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, explicitHash)
	require.NotEqual(t, contentHash, explicitHash, "explicit session_id should override content fallback")
}

func TestOpenAIGatewayService_GenerateSessionHash_EmptyBodyStillEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{}
	require.Empty(t, svc.GenerateSessionHash(c, []byte(`{}`)))
	require.Empty(t, svc.GenerateSessionHash(c, nil))
}

func (c stubConcurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if c.waitCounts != nil {
		if count, ok := c.waitCounts[accountID]; ok {
			return count, nil
		}
	}
	return 0, nil
}

type stubGatewayCache struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
}

func (c *stubGatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.sessionBindings[sessionHash]; ok {
		return id, nil
	}
	return 0, errors.New("not found")
}

func (c *stubGatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if c.sessionBindings == nil {
		c.sessionBindings = make(map[string]int64)
	}
	c.sessionBindings[sessionHash] = accountID
	return nil
}

func (c *stubGatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
}

func (c *stubGatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if c.sessionBindings == nil {
		return nil
	}
	if c.deletedSessions == nil {
		c.deletedSessions = make(map[string]int)
	}
	c.deletedSessions[sessionHash]++
	delete(c.sessionBindings, sessionHash)
	return nil
}

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulable(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
	}
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{rateLimited, available}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
	}
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulableWhenNoConcurrencyService(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
	}
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
	}

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{rateLimited, available}},
		// concurrencyService is nil, forcing the non-load-batch selection path.
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
	}
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-1"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2, got %+v", acc)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyOutsideGroupClearsSession(t *testing.T) {
	sessionHash := "session-outside-group"
	groupID := int64(1001)
	repo := groupAwareStubOpenAIAccountRepo{
		stubOpenAIAccountRepo{
			accounts: []Account{
				{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
				{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, AccountGroups: []AccountGroup{{GroupID: groupID}}},
			},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2, got %+v", acc)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-2"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %+v", selection)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountWithLoadAwareness_StickyOutsideGroupClearsSession(t *testing.T) {
	sessionHash := "session-load-outside-group"
	groupID := int64(1002)
	repo := groupAwareStubOpenAIAccountRepo{
		stubOpenAIAccountRepo{
			accounts: []Account{
				{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1},
				{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, AccountGroups: []AccountGroup{{GroupID: groupID}}},
			},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %+v", selection)
	}
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountForModelWithExclusions_NoModelSupport(t *testing.T) {
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{"gpt-3.5-turbo": "gpt-3.5-turbo"}},
			},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for unsupported model")
	}
	if acc != nil {
		t.Fatalf("expected nil account for unsupported model")
	}
	if !strings.Contains(err.Error(), "supporting model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorFallback(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr: errors.New("load batch failed"),
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "fallback", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection")
	}
	if selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %d", selection.Account.ID)
	}
	if cache.sessionBindings["openai:fallback"] != 2 {
		t.Fatalf("expected sticky session updated")
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAISelectAccountWithLoadAwareness_NoSlotFallbackWait(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan fallback")
	}
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_SetsStickyBinding(t *testing.T) {
	sessionHash := "bind"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 1 {
		t.Fatalf("expected account 1")
	}
	if cache.sessionBindings["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session binding")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_StickyWaitPlan(t *testing.T) {
	sessionHash := "sticky-wait"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false},
		waitCounts:     map[int64]int{1: 0},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected sticky wait plan")
	}
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_PrefersLowerLoad(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 80},
			2: {AccountID: 2, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "load", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
	if cache.sessionBindings["openai:load"] != 2 {
		t.Fatalf("expected sticky session updated")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyExcludedFallback(t *testing.T) {
	sessionHash := "excluded"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	excluded := map[int64]struct{}{1: {}}
	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", excluded)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_StickyNonOpenAI(t *testing.T) {
	sessionHash := "non-openai"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1},
	}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_NoAccounts(t *testing.T) {
	repo := stubOpenAIAccountRepo{accounts: []Account{}}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
	if err == nil {
		t.Fatalf("expected error for no accounts")
	}
	if acc != nil {
		t.Fatalf("expected nil account")
	}
	if !strings.Contains(err.Error(), "no available OpenAI accounts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAISelectAccountWithLoadAwareness_NoCandidates(t *testing.T) {
	groupID := int64(1)
	resetAt := time.Now().Add(1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, RateLimitResetAt: &resetAt},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for no candidates")
	}
	if selection != nil {
		t.Fatalf("expected nil selection")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_AllFullWaitPlan(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 100},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorNoAcquire(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr:   errors.New("load batch failed"),
		acquireResults: map[int64]bool{1: false},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_MissingLoadInfo(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 50},
		},
		skipDefaultLoad: true,
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountForModelWithExclusions_LeastRecentlyUsed(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &newTime},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &oldTime},
		},
	}
	cache := &stubGatewayCache{}

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
	}
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAISelectAccountWithLoadAwareness_PreferNeverUsed(t *testing.T) {
	groupID := int64(1)
	lastUsed := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &lastUsed},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		},
	}
	cache := &stubGatewayCache{}
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10},
			2: {AccountID: 2, LoadRate: 10},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
	}
}

func TestOpenAIStreamingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 1,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, start, "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "stream_timeout") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingContextCanceledReturnsIncompleteErrorWithoutInjectingErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       cancelReadCloser{},
		Header:     http.Header{},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	if err == nil || !strings.Contains(err.Error(), "stream usage incomplete") {
		t.Fatalf("expected incomplete stream error, got %v", err)
	}
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "stream_read_error") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingReadErrorBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       errReadCloser{err: io.ErrUnexpectedEOF},
		Header:     http.Header{"X-Request-Id": []string{"rid-disconnect"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"An error occurred while processing your request."}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-failed"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "An error occurred while processing your request")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingResponseFailedBeforeOutputCapacityErrorReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-capacity-failed"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Selected model is at capacity")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPreambleForceReleaseByAge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-preamble-age"}},
	}

	type streamResult struct {
		result *openaiStreamingResult
		err    error
	}
	done := make(chan streamResult, 1)
	go func() {
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "response.created")
	}, 3*time.Second, 25*time.Millisecond, "response.created should be force-released before a token/terminal event")

	_, err = fmt.Fprint(pw, `data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":0}}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Nil(t, got.result.firstTokenMs, "response.created must not be recorded as first token")
		require.NotNil(t, got.result.usage)
		require.Equal(t, 1, got.result.usage.InputTokens)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not finish")
	}
}

func TestOpenAIStreamingDebugTimingCommentDoesNotCountAsFirstToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set(openAIStreamDebugTimingHeader, "1")

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-debug-timing"}},
	}

	type streamResult struct {
		result *openaiStreamingResult
		err    error
	}
	done := make(chan streamResult, 1)
	start := time.Now()
	go func() {
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, start, "model", "model")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		body := rec.Body.String()
		return strings.Contains(body, "sub2api_upstream_first_event_ms=") &&
			strings.Contains(body, "response.created") &&
			!strings.Contains(body, "response.output_text.delta")
	}, 3*time.Second, 25*time.Millisecond, "debug timing comment should be visible before first token")

	time.Sleep(75 * time.Millisecond)
	_, err = fmt.Fprint(pw, `data: {"type":"response.output_text.delta","delta":"你"}`+"\n\n")
	require.NoError(t, err)
	_, err = fmt.Fprint(pw, `data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.NotNil(t, got.result.firstTokenMs, "real output delta should be recorded as first token")
		require.GreaterOrEqual(t, *got.result.firstTokenMs, 50, "debug comment / response.created must not be recorded as first token")
		require.NotNil(t, got.result.upstreamFirstEventMs)
		require.LessOrEqual(t, *got.result.upstreamFirstEventMs, *got.result.firstTokenMs)
		require.NotNil(t, got.result.usage)
		require.Equal(t, 1, got.result.usage.InputTokens)
		require.Equal(t, 1, got.result.usage.OutputTokens)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not finish")
	}

	gotUpstreamFirstEvent, ok := c.Get(OpsOpenAIUpstreamFirstEventMsKey)
	require.True(t, ok)
	require.IsType(t, int64(0), gotUpstreamFirstEvent)
	require.Contains(t, rec.Body.String(), ": sub2api_upstream_first_event_ms=")
	require.Contains(t, rec.Body.String(), "response.output_text.delta")
}

func TestOpenAIStreamingFirstTokenMsRecordedBeforeDownstreamWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &slowGinWriter{ResponseWriter: c.Writer, delay: 300 * time.Millisecond}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"你"}`,
			"",
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-first-token-before-write"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs)
	require.Less(t, *result.firstTokenMs, 200, "firstTokenMs should not include slow downstream write/flush latency")
	require.Contains(t, rec.Body.String(), "response.output_text.delta")
}

func TestOpenAIStreamingTerminalEventDoesNotRecordFirstTokenMs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-done-not-first-token"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.firstTokenMs, "response.completed must not be treated as first token")
	require.Contains(t, rec.Body.String(), "response.completed")
}

func TestOpenAIStreamingOutputTextDoneRecordsForkLikeFirstTokenMs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			`data: {"type":"response.output_text.done","text":"完整文本"}`,
			"",
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-output-text-done-first-token"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs, "fork-like firstTokenMs should record first non-preamble output/progress event")
	require.Contains(t, rec.Body.String(), "response.output_text.done")
}

func TestOpenAIStreamingEventNamedDeltaRecordsFirstTokenMs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.output_text.delta`,
			`data: {"delta":"你"}`,
			"",
			`event: response.completed`,
			`data: {"response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-event-named-delta-first-token"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs, "event: response.output_text.delta should supply type for data-only payload")
	require.Contains(t, rec.Body.String(), `data: {"delta":"你"}`)
}

func TestOpenAIStreamingDebugTimingResponseFailedAfterCommentDoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set(openAIStreamDebugTimingHeader, "1")

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-debug-timing-failed-after-comment"}},
	}

	type streamResult struct {
		result *openaiStreamingResult
		err    error
	}
	done := make(chan streamResult, 1)
	go func() {
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "sub2api_upstream_first_event_ms=")
	}, 3*time.Second, 25*time.Millisecond, "debug timing comment should commit the downstream stream")

	_, err = fmt.Fprint(pw, `data: {"type":"response.failed","error":{"message":"upstream processing failed"}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.Error(t, got.err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(got.err, &failoverErr), "after debug timing comment is released, stream cannot cleanly fail over")
		require.Contains(t, got.err.Error(), "upstream response failed")
		require.NotNil(t, got.result)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not finish")
	}
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), "upstream processing failed")
}

func TestOpenAIStreamingResponseFailedAfterPreambleForceReleaseDoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-preamble-failed-after-release"}},
	}

	type streamResult struct {
		result *openaiStreamingResult
		err    error
	}
	done := make(chan streamResult, 1)
	go func() {
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "response.created")
	}, 3*time.Second, 25*time.Millisecond, "preamble should be visible before response.failed arrives")

	_, err = fmt.Fprint(pw, `data: {"type":"response.failed","error":{"message":"upstream processing failed"}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.Error(t, got.err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(got.err, &failoverErr), "after preamble is released, stream cannot cleanly fail over")
		require.Contains(t, got.err.Error(), "upstream response failed")
		require.NotNil(t, got.result)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not finish")
	}
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), "upstream processing failed")
}

func TestOpenAIStreamingPreambleOnlyMissingTerminalReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-missing-terminal"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPreambleKeepaliveUsesDownstreamIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   1,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		for i := 0; i < 6; i++ {
			time.Sleep(250 * time.Millisecond)
			_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		}
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), ":\n\n")
	require.Contains(t, rec.Body.String(), "response.completed")
}

func TestOpenAIStreamingPolicyResponseFailedBeforeOutputPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"type":"safety_error","message":"This request has been flagged for potentially high-risk cyber activity."}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-policy-failed"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), "high-risk cyber activity")
}

func TestOpenAIStreamingClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0}

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil || result.usage == nil {
		t.Fatalf("expected usage result")
	}
	if result.usage.InputTokens != 3 || result.usage.OutputTokens != 5 || result.usage.CacheReadInputTokens != 1 {
		t.Fatalf("unexpected usage: %+v", *result.usage)
	}
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "write_failed") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
	}
}

func TestOpenAIStreamingMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"},\"output_index\":0}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
	}
}

func TestOpenAIStreamingPassthroughMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"},\"output_index\":0}\n\n"))
	}()

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
	}
}

func TestOpenAIStreamingPassthroughResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"upstream processing failed"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-passthrough-failed"}},
	}

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "", "")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "upstream processing failed")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingPassthroughPreambleForceReleaseByAge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-passthrough-preamble-age"}},
	}

	type streamResult struct {
		result *openaiStreamingResultPassthrough
		err    error
	}
	done := make(chan streamResult, 1)
	go func() {
		result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "", "")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "response.created")
	}, 3*time.Second, 25*time.Millisecond, "passthrough preamble should be force-released before a token/terminal event")

	_, err = fmt.Fprint(pw, `data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":0}}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Nil(t, got.result.firstTokenMs, "response.created must not be recorded as first token")
		require.NotNil(t, got.result.usage)
		require.Equal(t, 2, got.result.usage.InputTokens)
	case <-time.After(time.Second):
		t.Fatal("passthrough stream handler did not finish")
	}
}

func TestOpenAIStreamingPassthroughDebugTimingCommentDoesNotCountAsFirstToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set(openAIStreamDebugTimingHeader, "yes")

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{"X-Request-Id": []string{"rid-passthrough-debug-timing"}},
	}

	type streamResult struct {
		result *openaiStreamingResultPassthrough
		err    error
	}
	done := make(chan streamResult, 1)
	start := time.Now()
	go func() {
		result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, start, "", "")
		done <- streamResult{result: result, err: err}
	}()

	_, err := fmt.Fprint(pw, `data: {"type":"response.created","response":{"id":"resp_1"}}`+"\n\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		body := rec.Body.String()
		return strings.Contains(body, "sub2api_upstream_first_event_ms=") &&
			strings.Contains(body, "response.created") &&
			!strings.Contains(body, "response.output_text.delta")
	}, 3*time.Second, 25*time.Millisecond, "passthrough debug timing comment should be visible before first token")

	time.Sleep(75 * time.Millisecond)
	_, err = fmt.Fprint(pw, `data: {"type":"response.output_text.delta","delta":"你"}`+"\n\n")
	require.NoError(t, err)
	_, err = fmt.Fprint(pw, `data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1}}}`+"\n\n")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.NotNil(t, got.result.firstTokenMs)
		require.GreaterOrEqual(t, *got.result.firstTokenMs, 50, "debug comment / response.created must not be recorded as first token")
		require.NotNil(t, got.result.upstreamFirstEventMs)
		require.LessOrEqual(t, *got.result.upstreamFirstEventMs, *got.result.firstTokenMs)
		require.NotNil(t, got.result.usage)
		require.Equal(t, 2, got.result.usage.InputTokens)
		require.Equal(t, 1, got.result.usage.OutputTokens)
	case <-time.After(time.Second):
		t.Fatal("passthrough stream handler did not finish")
	}

	gotUpstreamFirstEvent, ok := c.Get(OpsOpenAIUpstreamFirstEventMsKey)
	require.True(t, ok)
	require.IsType(t, int64(0), gotUpstreamFirstEvent)
	require.Contains(t, rec.Body.String(), ": sub2api_upstream_first_event_ms=")
	require.Contains(t, rec.Body.String(), "response.output_text.delta")
}

func TestOpenAIStreamingPassthroughEventNamedDeltaRecordsFirstTokenMs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.output_text.delta`,
			`data: {"delta":"你"}`,
			"",
			`event: response.completed`,
			`data: {"response":{"usage":{"input_tokens":2,"output_tokens":1}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-passthrough-event-named-delta-first-token"}},
	}

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"}, time.Now(), "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs, "event: response.output_text.delta should supply type for passthrough data-only payload")
	require.Contains(t, rec.Body.String(), `data: {"delta":"你"}`)
}

func TestOpenAIStreamingPassthroughResponseDoneWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.done\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.firstTokenMs, "terminal response.done must not be recorded as first token")
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
}

func TestOpenAIStreamingPassthroughResponseIncompleteWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "", "")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
}

func TestOpenAIStreamingTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               64 * 1024,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		// 写入超过 MaxLineSize 的单行数据，触发 ErrTooLong
		payload := "data: " + strings.Repeat("a", 128*1024) + "\n"
		_, _ = pw.Write([]byte(payload))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2}, time.Now(), "model", "model")
	_ = pr.Close()

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "response_too_large") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
	}
}

func TestOpenAINonStreamingContentTypePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/vnd.test+json"}},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{}, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
	}

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.test+json") {
		t.Fatalf("expected Content-Type passthrough, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestOpenAINonStreamingContentTypeDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{}, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
	}

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected default Content-Type, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestOpenAIStreamingHeadersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: false},
		},
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Cache-Control": []string{"upstream"},
			"X-Request-Id":  []string{"req-123"},
			"Content-Type":  []string{"application/custom"},
		},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{}}\n\n"))
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("handleStreamingResponse error: %v", err)
	}

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control override, got %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type override, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id passthrough, got %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestOpenAIStreamingReuseScannerBufferAndStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":3}}}}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "model", "model")
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 1, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
	require.Equal(t, 3, result.usage.CacheReadInputTokens)
}

func TestOpenAIInvalidBaseURLWhenAllowlistDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "://invalid-url"},
	}

	_, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte("{}"), "token", false, "", false)
	if err == nil {
		t.Fatalf("expected error for invalid base_url when allowlist disabled")
	}
}

func TestOpenAIValidateUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	if _, err := svc.validateUpstreamBaseURL("http://not-https.example.com"); err == nil {
		t.Fatalf("expected http to be rejected when allow_insecure_http is false")
	}
	normalized, err := svc.validateUpstreamBaseURL("https://example.com")
	if err != nil {
		t.Fatalf("expected https to be allowed when allowlist disabled, got %v", err)
	}
	if normalized != "https://example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
	}
}

func TestOpenAIValidateUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	normalized, err := svc.validateUpstreamBaseURL("http://not-https.example.com")
	if err != nil {
		t.Fatalf("expected http allowed when allow_insecure_http is true, got %v", err)
	}
	if normalized != "http://not-https.example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
	}
}

func TestOpenAIValidateUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"},
			},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg}

	if _, err := svc.validateUpstreamBaseURL("https://example.com"); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
	}
	if _, err := svc.validateUpstreamBaseURL("https://evil.com"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
	}
}

func TestOpenAIUpdateCodexUsageSnapshotFromHeaders(t *testing.T) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 1)}
	svc := &OpenAIGatewayService{accountRepo: repo}
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-secondary-used-percent", "34")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-reset-after-seconds", "86400")

	svc.UpdateCodexUsageSnapshotFromHeaders(context.Background(), 123, headers)

	select {
	case updates := <-repo.updateExtraCalls:
		require.Equal(t, 12.0, updates["codex_5h_used_percent"])
		require.Equal(t, 34.0, updates["codex_7d_used_percent"])
		require.Equal(t, 600, updates["codex_5h_reset_after_seconds"])
		require.Equal(t, 86400, updates["codex_7d_reset_after_seconds"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected UpdateExtra to be called")
	}
}

func TestOpenAIResponsesRequestPathSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "exact v1 responses", path: "/v1/responses", want: ""},
		{name: "compact v1 responses", path: "/v1/responses/compact", want: "/compact"},
		{name: "compact alias responses", path: "/responses/compact/", want: "/compact"},
		{name: "nested suffix", path: "/openai/v1/responses/compact/detail", want: "/compact/detail"},
		{name: "unrelated path", path: "/v1/chat/completions", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			require.Equal(t, tt.want, openAIResponsesRequestPathSuffix(c))
		})
	}
}

func TestNormalizeOpenAICompactRequestBodyPreservesCurrentCodexPayloadFields(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"compact me"}],"instructions":"compact-test","tools":[{"type":"function","name":"shell"}],"parallel_tool_calls":true,"reasoning":{"effort":"high"},"text":{"verbosity":"low"},"previous_response_id":"resp_123","store":true,"stream":true,"prompt_cache_key":"cache_123"}`)

	normalized, changed, err := normalizeOpenAICompactRequestBody(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(normalized, "model").String())
	require.True(t, gjson.GetBytes(normalized, "tools").Exists())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Equal(t, "high", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, "low", gjson.GetBytes(normalized, "text.verbosity").String())
	require.Equal(t, "resp_123", gjson.GetBytes(normalized, "previous_response_id").String())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "prompt_cache_key").Exists())
}

func TestOpenAIBuildUpstreamRequestOpenAIPassthroughPreservesCompactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{}
	account := &Account{Type: AccountTypeOAuth}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token")
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(req.Context()))
}

func TestOpenAIBuildUpstreamRequestCompactForcesJSONAcceptForOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", true)
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(req.Context()))
}

func TestOpenAIBuildUpstreamRequestOAuthMessagesBridgeUsesSessionOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"anthropic-metadata-session-1","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"<sub2api-claude-code-todo-guard>"}]},{"type":"message","role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("OpenAI-Beta", "responses=experimental")
	c.Request.Header.Set("originator", "codex_cli_rs")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", true, "anthropic-metadata-session-1", false)
	require.NoError(t, err)
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Empty(t, req.Header.Get("Conversation_Id"))
	require.Empty(t, req.Header.Get("OpenAI-Beta"))
	require.Empty(t, req.Header.Get("originator"))
}

func TestOpenAIBuildUpstreamRequestPreservesCompactPathForAPIKeyBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"base_url": "https://example.com/v1"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1/responses/compact", req.URL.String())
}

func TestOpenAIBuildUpstreamRequestOAuthOfficialClientOriginatorCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userAgent      string
		originator     string
		wantOriginator string
	}{
		{name: "desktop originator preserved", originator: "Codex Desktop", wantOriginator: "Codex Desktop"},
		{name: "vscode originator preserved", originator: "codex_vscode", wantOriginator: "codex_vscode"},
		{name: "official ua fallback to codex_cli_rs", userAgent: "Codex Desktop/1.2.3", wantOriginator: "codex_cli_rs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.originator != "" {
				c.Request.Header.Set("originator", tt.originator)
			}

			svc := &OpenAIGatewayService{}
			account := &Account{
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
			}

			isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
			req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", isCodexCLI)
			require.NoError(t, err)
			require.Equal(t, tt.wantOriginator, req.Header.Get("originator"))
		})
	}
}

// ==================== P1-08 修复：model 替换性能优化测试 ====================

// ==================== P1-08 修复：model 替换性能优化测试 =============
func TestReplaceModelInSSELine(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		line     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "顶层 model 字段替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-4o","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-custom-model",
			expected: `data: {"id":"chatcmpl-123","model":"my-custom-model","choices":[]}`,
		},
		{
			name:     "嵌套 response.model 替换",
			line:     `data: {"type":"response","response":{"id":"resp-1","model":"gpt-4o","output":[]}}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"type":"response","response":{"id":"resp-1","model":"my-model","output":[]}}`,
		},
		{
			name:     "model 不匹配时不替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
		},
		{
			name:     "无 model 字段时不替换",
			line:     `data: {"id":"chatcmpl-123","choices":[]}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","choices":[]}`,
		},
		{
			name:     "空 data 行",
			line:     `data: `,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: `,
		},
		{
			name:     "[DONE] 行",
			line:     `data: [DONE]`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: [DONE]`,
		},
		{
			name:     "非 data: 前缀行",
			line:     `event: message`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `event: message`,
		},
		{
			name:     "非法 JSON 不替换",
			line:     `data: {invalid json}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {invalid json}`,
		},
		{
			name:     "无空格 data: 格式",
			line:     `data:{"id":"x","model":"gpt-4o"}`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"x","model":"my-model"}`,
		},
		{
			name:     "model 名含特殊字符",
			line:     `data: {"model":"org/model-v2.1-beta"}`,
			from:     "org/model-v2.1-beta",
			to:       "custom/alias",
			expected: `data: {"model":"custom/alias"}`,
		},
		{
			name:     "空行",
			line:     "",
			from:     "gpt-4o",
			to:       "my-model",
			expected: "",
		},
		{
			name:     "保持其他字段不变",
			line:     `data: {"id":"abc","object":"chat.completion.chunk","model":"gpt-4o","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `data: {"id":"abc","object":"chat.completion.chunk","model":"alias","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		},
		{
			name:     "顶层优先于嵌套：同时存在两个 model",
			line:     `data: {"model":"gpt-4o","response":{"model":"gpt-4o"}}`,
			from:     "gpt-4o",
			to:       "replaced",
			expected: `data: {"model":"replaced","response":{"model":"gpt-4o"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSELine(tt.line, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestReplaceModelInSSEBody(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "多行 SSE body 替换",
			body:     "data: {\"model\":\"gpt-4o\",\"choices\":[]}\n\ndata: {\"model\":\"gpt-4o\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"alias\",\"choices\":[]}\n\ndata: {\"model\":\"alias\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n",
		},
		{
			name:     "无需替换的 body",
			body:     "data: {\"model\":\"gpt-3.5-turbo\"}\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"gpt-3.5-turbo\"}\n\ndata: [DONE]\n",
		},
		{
			name:     "混合 event 和 data 行",
			body:     "event: message\ndata: {\"model\":\"gpt-4o\"}\n\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "event: message\ndata: {\"model\":\"alias\"}\n\n",
		},
		{
			name:     "空 body",
			body:     "",
			from:     "gpt-4o",
			to:       "alias",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSEBody(tt.body, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestReplaceModelInResponseBody(t *testing.T) {
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
	}{
		{
			name:     "替换顶层 model",
			body:     `{"id":"chatcmpl-123","model":"gpt-4o","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"alias","choices":[]}`,
		},
		{
			name:     "model 不匹配不替换",
			body:     `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]}`,
		},
		{
			name:     "无 model 字段不替换",
			body:     `{"id":"chatcmpl-123","choices":[]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","choices":[]}`,
		},
		{
			name:     "非法 JSON 返回原值",
			body:     `not json`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `not json`,
		},
		{
			name:     "空 body 返回原值",
			body:     ``,
			from:     "gpt-4o",
			to:       "alias",
			expected: ``,
		},
		{
			name:     "保持嵌套结构不变",
			body:     `{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"model":"alias","usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInResponseBody([]byte(tt.body), tt.from, tt.to)
			require.Equal(t, tt.expected, string(got))
		})
	}
}

func TestExtractOpenAISSEDataLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantData string
		wantOK   bool
	}{
		{name: "标准格式", line: `data: {"type":"x"}`, wantData: `{"type":"x"}`, wantOK: true},
		{name: "无空格格式", line: `data:{"type":"x"}`, wantData: `{"type":"x"}`, wantOK: true},
		{name: "纯空数据", line: `data:   `, wantData: ``, wantOK: true},
		{name: "非 data 行", line: `event: message`, wantData: ``, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOpenAISSEDataLine(tt.line)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantData, got)
		})
	}
}

func TestParseSSEUsage_SelectiveParsing(t *testing.T) {
	svc := &OpenAIGatewayService{}
	usage := &OpenAIUsage{InputTokens: 9, OutputTokens: 8, CacheReadInputTokens: 7}

	// 非 completed 事件，不应覆盖 usage
	svc.parseSSEUsage(`{"type":"response.in_progress","response":{"usage":{"input_tokens":1,"output_tokens":2}}}`, usage)
	require.Equal(t, 9, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	require.Equal(t, 7, usage.CacheReadInputTokens)

	// completed 事件，应提取 usage
	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":5,"input_tokens_details":{"cached_tokens":2}}}}`, usage)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.CacheReadInputTokens)

	// done 事件同样可能携带最终 usage
	svc.parseSSEUsage(`{"type":"response.done","response":{"usage":{"input_tokens":13,"output_tokens":15,"input_tokens_details":{"cached_tokens":4}}}}`, usage)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 15, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)

	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"prompt_tokens":21,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":6}}}}`, usage)
	require.Equal(t, 21, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	require.Equal(t, 6, usage.CacheReadInputTokens)
}

func TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"id":"resp_1","usage":{"input_tokens":3,"output_tokens":5,"input_tokens_details":{"cached_tokens":2}}}`))
	require.True(t, ok)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.CacheReadInputTokens)

	usage, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.completed","response":{"usage":{"prompt_tokens":13,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":4}}}}`))
	require.True(t, ok)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
}

func TestExtractCodexFinalResponse_SampleReplay(t *testing.T) {
	body := strings.Join([]string{
		`event: message`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3}}}}`,
		`data: [DONE]`,
	}, "\n")

	finalResp, ok := extractCodexFinalResponse(body)
	require.True(t, ok)
	require.Contains(t, string(finalResp), `"id":"resp_1"`)
	require.Contains(t, string(finalResp), `"input_tokens":11`)
}

func TestHandleSSEToJSON_CompletedEventReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_2"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-4o","usage":{"input_tokens":7,"output_tokens":9,"input_tokens_details":{"cached_tokens":1}}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.InputTokens)
	require.Equal(t, 9, usage.OutputTokens)
	require.Equal(t, 1, usage.CacheReadInputTokens)
	// Header 可能由上游 Content-Type 透传；关键是 body 已转换为最终 JSON 响应。
	require.NotContains(t, rec.Body.String(), "event:")
	require.Contains(t, rec.Body.String(), `"id":"resp_2"`)
	require.NotContains(t, rec.Body.String(), "data:")
}

func TestHandleSSEToJSON_ReconstructsImageGenerationOutputItemDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","result":"aGVsbG8=","revised_prompt":"draw a cat","output_format":"png"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_img","model":"gpt-5.4","output":[],"usage":{"input_tokens":7,"output_tokens":9,"output_tokens_details":{"image_tokens":4}}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 4, usage.ImageOutputTokens)
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "aGVsbG8=", gjson.Get(rec.Body.String(), "output.0.result").String())
	require.Equal(t, "draw a cat", gjson.Get(rec.Body.String(), "output.0.revised_prompt").String())
}

func TestHandleSSEToJSON_NoFinalResponseKeepsSSEBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_3"}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, rec.Body.String(), `data: {"type":"response.in_progress"`)
}

func TestHandleSSEToJSON_ResponseFailedReturnsProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","error":{"message":"upstream rejected request"}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestOpenAICompatSSEFrameParserResetsEventTypeAtFrameBoundary(t *testing.T) {
	var parser openAICompatSSEFrameParser

	frame, ok := parser.AddLine("event: response.created")
	require.False(t, ok)
	require.Empty(t, frame)

	frame, ok = parser.AddLine(`data: {"response":{"id":"resp_1"}}`)
	require.False(t, ok)
	require.Empty(t, frame)

	frame, ok = parser.AddLine("")
	require.True(t, ok)
	require.Equal(t, "response.created", frame.EventType)
	require.JSONEq(t, `{"response":{"id":"resp_1"}}`, frame.Data)

	frame, ok = parser.AddLine(`data: {"delta":"ok"}`)
	require.False(t, ok)
	require.Empty(t, frame.EventType)

	frame, ok = parser.AddLine("")
	require.True(t, ok)
	require.Empty(t, frame.EventType)
	require.JSONEq(t, `{"delta":"ok"}`, frame.Data)
}

func TestStreamingPassthroughCyberPolicyMarksAndPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"r1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"flagged for cyber policy"}}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-cyber"}},
	}

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "a"}, time.Now(), "m", "m")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "cyber must NOT failover")
	require.Contains(t, rec.Body.String(), "cyber_policy", "response.failed passed through to client")
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, "flagged for cyber policy", mark.Message)
}

func TestHandleStreamingResponseCyberPolicyMarks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &OpenAIGatewayService{cfg: cfg}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"r1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"code":"cyber_policy","message":"flagged"}}`,
			"",
		}, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid"}},
	}
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "a"}, time.Now(), "m", "m")
	require.Error(t, err)
	var fo *UpstreamFailoverError
	require.False(t, errors.As(err, &fo))
	require.NotNil(t, GetOpsCyberPolicy(c))
}

func TestHandleErrorResponseCyberPolicyPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	cyberBody := `{"error":{"code":"cyber_policy","message":"flagged for cyber policy"}}`
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"rid"}},
		Body:       io.NopCloser(strings.NewReader(cyberBody)),
	}
	_, err := svc.handleErrorResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "a"}, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "passthrough upstream 400, not rewrapped 502")
	require.Contains(t, rec.Body.String(), "cyber_policy", "client sees original cyber body")
	require.NotContains(t, rec.Body.String(), "Upstream request failed", "must not 502-rewrap")
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, http.StatusBadRequest, mark.UpstreamStatus)
}

func TestHandleCompatErrorResponseCyberPolicyEarlyReturn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	cyberBody := `{"error":{"code":"cyber_policy","message":"flagged for cyber policy"}}`
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(cyberBody)),
	}
	var gotStatus int
	var gotType, gotMsg string
	writeError := func(_ *gin.Context, statusCode int, errType, message string) {
		gotStatus, gotType, gotMsg = statusCode, errType, message
	}
	// cyber 命中应早返回(写兼容错误 + 不冷却账号)，而非落到通用 "Upstream request failed"。
	_, err := svc.handleCompatErrorResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "a"}, writeError)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, gotStatus)
	require.Equal(t, "invalid_request_error", gotType)
	require.Contains(t, gotMsg, "flagged for cyber policy")
	require.NotContains(t, gotMsg, "Upstream request failed")
	require.NotNil(t, GetOpsCyberPolicy(c))
}
