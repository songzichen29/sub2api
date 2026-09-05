package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	openaiPkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	reasoningReplayCacheTTL          = 1 * time.Hour
	reasoningReplayCacheMaxEntries   = 10240
	reasoningReplayCacheEvictBatch   = 128
	reasoningReplayCacheCleanupEvery = 5 * time.Minute
)

type reasoningReplayCacheEntry struct {
	Items     []map[string]any
	Timestamp time.Time
}

var (
	reasoningReplayMu      sync.Mutex
	reasoningReplayEntries = make(map[string]reasoningReplayCacheEntry)
	reasoningReplayOnce    sync.Once
)

func startReasoningReplayCleanup() {
	go func() {
		ticker := time.NewTicker(reasoningReplayCacheCleanupEvery)
		defer ticker.Stop()
		for now := range ticker.C {
			purgeExpiredReasoningReplayEntries(now)
		}
	}()
}

// reasoningReplayCacheKey 生成缓存 key，由 model + sessionKey 组成。
func reasoningReplayCacheKey(model, sessionKey string) string {
	model = strings.TrimSpace(model)
	sessionKey = strings.TrimSpace(sessionKey)
	if model == "" || sessionKey == "" {
		return ""
	}
	return strings.Join([]string{"reasoning-replay", model, sessionKey}, "\x00")
}

// CacheReasoningReplayItems 成功响应后缓存 reasoning 和 function_call items。
func CacheReasoningReplayItems(model, sessionKey string, items []map[string]any) bool {
	key := reasoningReplayCacheKey(model, sessionKey)
	if key == "" {
		return false
	}
	normalized := normalizeReasoningReplayItems(items)
	if len(normalized) == 0 {
		return false
	}

	reasoningReplayOnce.Do(startReasoningReplayCleanup)
	now := time.Now()
	reasoningReplayMu.Lock()
	defer reasoningReplayMu.Unlock()
	reasoningReplayEntries[key] = reasoningReplayCacheEntry{
		Items:     normalized,
		Timestamp: now,
	}
	if len(reasoningReplayEntries) > reasoningReplayCacheMaxEntries {
		evictOldestReasoningReplayEntries(reasoningReplayCacheEvictBatch)
	}
	return true
}

// GetReasoningReplayItems 读取缓存的 replay items，刷新 TTL。
func GetReasoningReplayItems(model, sessionKey string) ([]map[string]any, bool) {
	key := reasoningReplayCacheKey(model, sessionKey)
	if key == "" {
		return nil, false
	}

	reasoningReplayOnce.Do(startReasoningReplayCleanup)
	now := time.Now()
	reasoningReplayMu.Lock()
	defer reasoningReplayMu.Unlock()
	entry, ok := reasoningReplayEntries[key]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.Timestamp) > reasoningReplayCacheTTL {
		delete(reasoningReplayEntries, key)
		return nil, false
	}
	entry.Timestamp = now
	reasoningReplayEntries[key] = entry
	return cloneReasoningReplayItems(entry.Items), true
}

// DeleteReasoningReplayItems 删除指定 session 的缓存（如上游返回签名错误时调用）。
func DeleteReasoningReplayItems(model, sessionKey string) {
	key := reasoningReplayCacheKey(model, sessionKey)
	if key == "" {
		return
	}
	reasoningReplayMu.Lock()
	delete(reasoningReplayEntries, key)
	reasoningReplayMu.Unlock()
}

// ClearAllReasoningReplayCache 清空所有缓存（测试用）。
func ClearAllReasoningReplayCache() {
	reasoningReplayMu.Lock()
	reasoningReplayEntries = make(map[string]reasoningReplayCacheEntry)
	reasoningReplayMu.Unlock()
}

// normalizeReasoningReplayItems 校验并规范化 output items，只保留
// reasoning（需签名合法）和 function_call（需 call_id/name/arguments 完整）。
func normalizeReasoningReplayItems(items []map[string]any) []map[string]any {
	var normalized []map[string]any
	for _, item := range items {
		if n, ok := normalizeReasoningReplayItem(item); ok {
			normalized = append(normalized, n)
		}
	}
	return normalized
}

func normalizeReasoningReplayItem(item map[string]any) (map[string]any, bool) {
	itemType, _ := item["type"].(string)
	switch strings.TrimSpace(itemType) {
	case "reasoning":
		return normalizeReasoningReplayReasoningItem(item)
	case "function_call":
		return normalizeReasoningReplayFunctionCallItem(item)
	default:
		return nil, false
	}
}

func normalizeReasoningReplayReasoningItem(item map[string]any) (map[string]any, bool) {
	ec, ok := item["encrypted_content"].(string)
	if !ok {
		return nil, false
	}
	if ec != strings.TrimSpace(ec) {
		return nil, false
	}
	if !openaiPkg.IsValidReasoningEncryptedContent(ec) {
		return nil, false
	}
	return map[string]any{
		"type":              "reasoning",
		"encrypted_content": ec,
		"summary":           []any{},
		"content":           nil,
	}, true
}

func normalizeReasoningReplayFunctionCallItem(item map[string]any) (map[string]any, bool) {
	callID, _ := item["call_id"].(string)
	name, _ := item["name"].(string)
	arguments, hasArgs := item["arguments"].(string)
	if strings.TrimSpace(callID) == "" || strings.TrimSpace(name) == "" || !hasArgs {
		return nil, false
	}
	return map[string]any{
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}, true
}

// extractReasoningReplayItemsFromOutput 从 response.output 数组中提取可缓存的 items。
func extractReasoningReplayItemsFromOutput(output []any) []map[string]any { //nolint:unused // retained for alternate Responses payload paths
	var items []map[string]any
	for _, v := range output {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := m["type"].(string)
		switch strings.TrimSpace(itemType) {
		case "reasoning", "function_call":
			items = append(items, m)
		}
	}
	return items
}

// filterReasoningReplayItemsForInput 过滤掉 input 中已存在的 items，避免重复。
func filterReasoningReplayItemsForInput(cachedItems []map[string]any, input []any) []map[string]any {
	if len(cachedItems) == 0 {
		return nil
	}
	existingCallIDs := make(map[string]bool)
	hasReasoningInput := false
	for _, v := range input {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := m["type"].(string)
		switch strings.TrimSpace(itemType) {
		case "reasoning":
			hasReasoningInput = true
		case "function_call":
			if callID, _ := m["call_id"].(string); callID != "" {
				existingCallIDs[callID] = true
			}
		}
	}

	var filtered []map[string]any
	for _, item := range cachedItems {
		itemType, _ := item["type"].(string)
		switch strings.TrimSpace(itemType) {
		case "reasoning":
			if hasReasoningInput {
				continue
			}
			filtered = append(filtered, item)
			hasReasoningInput = true
		case "function_call":
			callID, _ := item["call_id"].(string)
			if existingCallIDs[callID] {
				continue
			}
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// insertReasoningReplayItemsIntoInput 将缓存 items 插入 input 数组的正确位置。
// function_call 插在第一个 function_call_output 之前，reasoning 插在 function_call 之前。
func insertReasoningReplayItemsIntoInput(input []any, replayItems []map[string]any) []any {
	if len(replayItems) == 0 {
		return input
	}

	insertPos := findReasoningReplayInsertPosition(input)

	var reasoning []any
	var functionCalls []any
	for _, item := range replayItems {
		itemType, _ := item["type"].(string)
		if itemType == "reasoning" {
			reasoning = append(reasoning, item)
		} else {
			functionCalls = append(functionCalls, item)
		}
	}

	// 插入顺序：reasoning 在前，function_call 在后（紧贴 function_call_output）
	toInsert := append(reasoning, functionCalls...)
	result := make([]any, 0, len(input)+len(toInsert))
	result = append(result, input[:insertPos]...)
	result = append(result, toInsert...)
	result = append(result, input[insertPos:]...)
	return result
}

// findReasoningReplayInsertPosition 找到第一个 function_call_output 的位置作为插入点。
func findReasoningReplayInsertPosition(input []any) int {
	for i, v := range input {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := m["type"].(string)
		if strings.TrimSpace(itemType) == "function_call_output" {
			return i
		}
	}
	return len(input)
}

// resolveReasoningReplaySessionKey 从请求上下文中构建 session key。
// 优先使用 promptCacheKey，其次使用 session_id 和 conversation_id。
func resolveReasoningReplaySessionKey(promptCacheKey, sessionID, conversationID string) string {
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	sessionID = strings.TrimSpace(sessionID)
	conversationID = strings.TrimSpace(conversationID)
	if promptCacheKey != "" {
		return fmt.Sprintf("pck:%s", promptCacheKey)
	}
	if sessionID != "" {
		return fmt.Sprintf("sid:%s", sessionID)
	}
	if conversationID != "" {
		return fmt.Sprintf("cid:%s", conversationID)
	}
	return ""
}

func cloneReasoningReplayItems(items []map[string]any) []map[string]any {
	cloned := make([]map[string]any, len(items))
	for i, item := range items {
		c := make(map[string]any, len(item))
		for k, v := range item {
			c[k] = v
		}
		cloned[i] = c
	}
	return cloned
}

func evictOldestReasoningReplayEntries(count int) {
	if count <= 0 || len(reasoningReplayEntries) == 0 {
		return
	}
	type candidate struct {
		key       string
		timestamp time.Time
	}
	candidates := make([]candidate, 0, len(reasoningReplayEntries))
	for key, entry := range reasoningReplayEntries {
		candidates = append(candidates, candidate{key: key, timestamp: entry.Timestamp})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].timestamp.Before(candidates[j].timestamp)
	})
	if count > len(candidates) {
		count = len(candidates)
	}
	for i := 0; i < count; i++ {
		delete(reasoningReplayEntries, candidates[i].key)
	}
}

// reasoningReplayCacheContext 存放 replay cache 写入所需的上下文信息，
// 通过 gin.Context 传递给 handleStreamingResponse / forwardOpenAIWSV2。
type reasoningReplayCacheContext struct { //nolint:unused // retained for alternate Responses payload paths
	Model      string
	SessionKey string
}

const reasoningReplayCacheCtxKey = "__reasoning_replay_cache_ctx" //nolint:unused // retained for alternate Responses payload paths

// setReasoningReplayCacheContext 在 gin.Context 中设置 replay cache 写入上下文。
func setReasoningReplayCacheContext(c *gin.Context, model, sessionKey string) { //nolint:unused // retained for alternate Responses payload paths
	if c == nil || model == "" || sessionKey == "" {
		return
	}
	c.Set(reasoningReplayCacheCtxKey, &reasoningReplayCacheContext{
		Model:      model,
		SessionKey: sessionKey,
	})
}

// getReasoningReplayCacheContext 从 gin.Context 中获取 replay cache 写入上下文。
func getReasoningReplayCacheContext(c *gin.Context) (model, sessionKey string, ok bool) { //nolint:unused // retained for alternate Responses payload paths
	if c == nil {
		return "", "", false
	}
	v, exists := c.Get(reasoningReplayCacheCtxKey)
	if !exists {
		return "", "", false
	}
	ctx, ok := v.(*reasoningReplayCacheContext)
	if !ok || ctx == nil {
		return "", "", false
	}
	return ctx.Model, ctx.SessionKey, ctx.Model != "" && ctx.SessionKey != ""
}

// cacheReasoningReplayFromCompletedSSE 从 SSE response.completed 事件数据中
// 提取 reasoning/function_call output items 并写入缓存。
func cacheReasoningReplayFromCompletedSSE(c *gin.Context, data []byte) { //nolint:unused // retained for alternate Responses payload paths
	model, sessionKey, ok := getReasoningReplayCacheContext(c)
	if !ok {
		return
	}
	cacheReasoningReplayFromCompletedData(data, model, sessionKey)
}

// cacheReasoningReplayFromCompletedData 从 response.completed 事件的 JSON 数据中
// 提取 output items 并缓存。
func cacheReasoningReplayFromCompletedData(data []byte, model, sessionKey string) {
	if len(data) == 0 || model == "" || sessionKey == "" {
		return
	}

	outputResult := gjson.GetBytes(data, "response.output")
	if !outputResult.Exists() || !outputResult.IsArray() {
		return
	}

	var items []map[string]any
	for _, item := range outputResult.Array() {
		if !item.IsObject() {
			continue
		}
		itemType := strings.TrimSpace(item.Get("type").String())
		switch itemType {
		case "reasoning":
			ec := item.Get("encrypted_content")
			if ec.Type != gjson.String {
				continue
			}
			items = append(items, map[string]any{
				"type":              "reasoning",
				"encrypted_content": ec.String(),
			})
		case "function_call":
			callID := item.Get("call_id").String()
			name := item.Get("name").String()
			arguments := item.Get("arguments")
			if callID == "" || name == "" || arguments.Type != gjson.String {
				continue
			}
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   callID,
				"name":      name,
				"arguments": arguments.String(),
			})
		}
	}

	if len(items) > 0 {
		CacheReasoningReplayItems(model, sessionKey, items)
	}
}

// clearReasoningReplayCacheOnError 在遇到 invalid_encrypted_content 错误时清除缓存。
func clearReasoningReplayCacheOnError(c *gin.Context) { //nolint:unused // retained for alternate Responses payload paths
	model, sessionKey, ok := getReasoningReplayCacheContext(c)
	if !ok {
		return
	}
	DeleteReasoningReplayItems(model, sessionKey)
}

func purgeExpiredReasoningReplayEntries(now time.Time) {
	reasoningReplayMu.Lock()
	for key, entry := range reasoningReplayEntries {
		if now.Sub(entry.Timestamp) > reasoningReplayCacheTTL {
			delete(reasoningReplayEntries, key)
		}
	}
	reasoningReplayMu.Unlock()
}
