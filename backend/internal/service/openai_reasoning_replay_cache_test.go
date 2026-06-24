package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testValidEC = "gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"

func TestCacheReasoningReplayItems_StoreAndRetrieve(t *testing.T) {
	ClearAllReasoningReplayCache()

	items := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
		{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": `{"cmd":"ls"}`},
	}
	require.True(t, CacheReasoningReplayItems("gpt-5.1", "session-1", items))

	cached, ok := GetReasoningReplayItems("gpt-5.1", "session-1")
	require.True(t, ok)
	require.Len(t, cached, 2)
	require.Equal(t, "reasoning", cached[0]["type"])
	require.Equal(t, testValidEC, cached[0]["encrypted_content"])
	require.Equal(t, "function_call", cached[1]["type"])
	require.Equal(t, "call_1", cached[1]["call_id"])
}

func TestCacheReasoningReplayItems_NormalizesAndFiltersInvalid(t *testing.T) {
	ClearAllReasoningReplayCache()

	items := []map[string]any{
		{"type": "reasoning", "encrypted_content": "gAAA"},
		{"type": "reasoning", "encrypted_content": testValidEC},
		{"type": "function_call", "call_id": "", "name": "bad", "arguments": "{}"},
		{"type": "function_call", "call_id": "call_2", "name": "edit", "arguments": `{"file":"a.go"}`},
		{"type": "message", "role": "assistant"},
	}
	require.True(t, CacheReasoningReplayItems("gpt-5.1", "session-2", items))

	cached, ok := GetReasoningReplayItems("gpt-5.1", "session-2")
	require.True(t, ok)
	require.Len(t, cached, 2, "只保留签名合法的 reasoning 和字段完整的 function_call")
	require.Equal(t, "reasoning", cached[0]["type"])
	require.Equal(t, "function_call", cached[1]["type"])
	require.Equal(t, "call_2", cached[1]["call_id"])
}

func TestCacheReasoningReplayItems_EmptyKeyReject(t *testing.T) {
	ClearAllReasoningReplayCache()

	items := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
	}
	require.False(t, CacheReasoningReplayItems("", "session-1", items))
	require.False(t, CacheReasoningReplayItems("gpt-5.1", "", items))
}

func TestGetReasoningReplayItems_MissReturnsNil(t *testing.T) {
	ClearAllReasoningReplayCache()

	cached, ok := GetReasoningReplayItems("gpt-5.1", "nonexistent")
	require.False(t, ok)
	require.Nil(t, cached)
}

func TestDeleteReasoningReplayItems(t *testing.T) {
	ClearAllReasoningReplayCache()

	items := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
	}
	CacheReasoningReplayItems("gpt-5.1", "session-del", items)

	_, ok := GetReasoningReplayItems("gpt-5.1", "session-del")
	require.True(t, ok)

	DeleteReasoningReplayItems("gpt-5.1", "session-del")

	_, ok = GetReasoningReplayItems("gpt-5.1", "session-del")
	require.False(t, ok, "删除后不应再能读取")
}

func TestFilterReasoningReplayItemsForInput_DeduplicatesByCallID(t *testing.T) {
	cached := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
		{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": `{}`},
		{"type": "function_call", "call_id": "call_2", "name": "edit", "arguments": `{}`},
	}
	input := []any{
		map[string]any{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
	}

	filtered := filterReasoningReplayItemsForInput(cached, input)
	require.Len(t, filtered, 2, "reasoning + call_2 应保留，call_1 已在 input 中")
	require.Equal(t, "reasoning", filtered[0]["type"])
	require.Equal(t, "call_2", filtered[1]["call_id"])
}

func TestFilterReasoningReplayItemsForInput_SkipsReasoningIfAlreadyExists(t *testing.T) {
	cached := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
		{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": `{}`},
	}
	input := []any{
		map[string]any{"type": "reasoning", "encrypted_content": "some-other-ec"},
		map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
	}

	filtered := filterReasoningReplayItemsForInput(cached, input)
	require.Len(t, filtered, 1, "input 已有 reasoning 时跳过缓存的 reasoning")
	require.Equal(t, "function_call", filtered[0]["type"])
}

func TestInsertReasoningReplayItemsIntoInput(t *testing.T) {
	input := []any{
		map[string]any{"type": "input_text", "text": "hello"},
		map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
	}
	replayItems := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
		{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": `{}`},
	}

	result := insertReasoningReplayItemsIntoInput(input, replayItems)
	require.Len(t, result, 4)

	item0, _ := result[0].(map[string]any)
	require.Equal(t, "input_text", item0["type"])

	item1, _ := result[1].(map[string]any)
	require.Equal(t, "reasoning", item1["type"], "reasoning 应插在 function_call_output 之前")

	item2, _ := result[2].(map[string]any)
	require.Equal(t, "function_call", item2["type"], "function_call 应紧接 reasoning 之后")

	item3, _ := result[3].(map[string]any)
	require.Equal(t, "function_call_output", item3["type"], "function_call_output 保持原位")
}

func TestInsertReasoningReplayItemsIntoInput_NoFunctionCallOutput(t *testing.T) {
	input := []any{
		map[string]any{"type": "input_text", "text": "hello"},
	}
	replayItems := []map[string]any{
		{"type": "reasoning", "encrypted_content": testValidEC},
	}

	result := insertReasoningReplayItemsIntoInput(input, replayItems)
	require.Len(t, result, 2, "没有 function_call_output 时追加到末尾")

	item1, _ := result[1].(map[string]any)
	require.Equal(t, "reasoning", item1["type"])
}

func TestResolveReasoningReplaySessionKey(t *testing.T) {
	require.Equal(t, "pck:test-key", resolveReasoningReplaySessionKey("test-key", "", ""))
	require.Equal(t, "sid:sess-1", resolveReasoningReplaySessionKey("", "sess-1", ""))
	require.Equal(t, "cid:conv-1", resolveReasoningReplaySessionKey("", "", "conv-1"))
	require.Equal(t, "pck:test-key", resolveReasoningReplaySessionKey("test-key", "sess-1", "conv-1"), "promptCacheKey 优先")
	require.Equal(t, "", resolveReasoningReplaySessionKey("", "", ""))
}

func TestCacheReasoningReplayFromCompletedData(t *testing.T) {
	ClearAllReasoningReplayCache()

	data := []byte(`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"reasoning","encrypted_content":"` + testValidEC + `"},{"type":"function_call","call_id":"call_1","name":"run_cmd","arguments":"{}"},{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`)

	cacheReasoningReplayFromCompletedData(data, "gpt-5.1", "session-sse")

	cached, ok := GetReasoningReplayItems("gpt-5.1", "session-sse")
	require.True(t, ok)
	require.Len(t, cached, 2, "应缓存 reasoning 和 function_call，忽略 message")
	require.Equal(t, "reasoning", cached[0]["type"])
	require.Equal(t, "function_call", cached[1]["type"])
}

func TestCacheReasoningReplayItems_CloneIsolation(t *testing.T) {
	ClearAllReasoningReplayCache()

	items := []map[string]any{
		{"type": "function_call", "call_id": "call_1", "name": "run_cmd", "arguments": `{}`},
	}
	CacheReasoningReplayItems("gpt-5.1", "session-clone", items)

	cached1, _ := GetReasoningReplayItems("gpt-5.1", "session-clone")
	cached1[0]["call_id"] = "mutated"

	cached2, _ := GetReasoningReplayItems("gpt-5.1", "session-clone")
	require.Equal(t, "call_1", cached2[0]["call_id"], "修改副本不应影响缓存")
}
