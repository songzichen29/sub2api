package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsOpenAIWSTokenEvent_TerminalEventsExcluded 覆盖 isOpenAIWSTokenEvent 的回归用例。
// 重点验证终止事件（response.completed / response.done）不再被当作 token event，
// 否则当上游没有可识别的 delta 时，firstTokenMs 会被填到终止时刻，
// 等于把"总耗时"误报为"首 token 延迟"（issue #2651）。
func TestIsOpenAIWSTokenEvent_TerminalEventsExcluded(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      bool
	}{
		{name: "empty", eventType: "", want: false},
		{name: "whitespace_trimmed_empty", eventType: "   ", want: false},

		{name: "response.created", eventType: "response.created", want: false},
		{name: "response.in_progress", eventType: "response.in_progress", want: false},
		{name: "response.output_item.added", eventType: "response.output_item.added", want: false},
		{name: "response.output_item.done", eventType: "response.output_item.done", want: false},

		{name: "terminal_response.completed", eventType: "response.completed", want: false},
		{name: "terminal_response.done", eventType: "response.done", want: false},
		{name: "terminal_response.completed_padded", eventType: "  response.completed  ", want: false},
		{name: "terminal_response.done_padded", eventType: "  response.done  ", want: false},

		{name: "delta_text", eventType: "response.output_text.delta", want: true},
		{name: "delta_audio_transcript", eventType: "response.audio_transcript.delta", want: true},
		{name: "delta_function_call_arguments", eventType: "response.function_call_arguments.delta", want: true},

		{name: "output_text_done", eventType: "response.output_text.done", want: true},
		{name: "output_text_annotation_added", eventType: "response.output_text.annotation.added", want: true},

		{name: "output_audio_done", eventType: "response.output_audio.done", want: true},

		{name: "reasoning_summary_delta", eventType: "response.reasoning_summary_text.delta", want: true},

		{name: "unrelated_event_error", eventType: "error", want: false},
		{name: "unknown_event_without_match", eventType: "response.reasoning_summary_part.added", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isOpenAIWSTokenEvent(tc.eventType)
			require.Equal(t, tc.want, got, "isOpenAIWSTokenEvent(%q)", tc.eventType)
		})
	}
}

// TestIsOpenAIWSTokenEvent_DisjointWithTerminal 守护「token 事件集合与终止事件集合互斥」的不变量。
// firstTokenMs 的计算依赖于 isTokenEvent && !isTerminalEvent；
// 若两者再次出现交集，则 issue #2651 描述的 latency 误报会重现。
func TestIsOpenAIWSTokenEvent_DisjointWithTerminal(t *testing.T) {
	terminalEvents := []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
	}
	for _, ev := range terminalEvents {
		ev := ev
		t.Run(ev, func(t *testing.T) {
			require.True(t, isOpenAIWSTerminalEvent(ev), "expected terminal event %q to be classified as terminal", ev)
			require.False(t, isOpenAIWSTokenEvent(ev), "terminal event %q must NOT be classified as token event (issue #2651)", ev)
		})
	}
}

func TestIsOpenAIWSTokenEventMessage_UsesForkLikeEventTypeSemantics(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		message   []byte
		want      bool
	}{
		{name: "text_delta", eventType: "response.output_text.delta", message: []byte(`{"type":"response.output_text.delta","delta":"hi"}`), want: true},
		{name: "function_call_arguments_delta", eventType: "response.function_call_arguments.delta", message: []byte(`{"type":"response.function_call_arguments.delta","delta":"{}"}`), want: true},
		{name: "reasoning_summary_delta", eventType: "response.reasoning_summary_text.delta", message: []byte(`{"type":"response.reasoning_summary_text.delta","delta":"think"}`), want: true},
		{name: "unknown_delta", eventType: "response.custom.delta", message: []byte(`{"type":"response.custom.delta","delta":"x"}`), want: true},
		{name: "empty_delta", eventType: "response.output_text.delta", message: []byte(`{"type":"response.output_text.delta","delta":""}`), want: true},
		{name: "whitespace_delta", eventType: "response.output_text.delta", message: []byte(`{"type":"response.output_text.delta","delta":"   "}`), want: true},
		{name: "missing_delta", eventType: "response.output_text.delta", message: []byte(`{"type":"response.output_text.delta"}`), want: true},
		{name: "output_text_done", eventType: "response.output_text.done", message: []byte(`{"type":"response.output_text.done","text":"hi"}`), want: true},
		{name: "response_output", eventType: "response.output", message: []byte(`{"type":"response.output"}`), want: true},
		{name: "preamble_with_delta", eventType: "response.created", message: []byte(`{"type":"response.created","delta":"x"}`), want: false},
		{name: "terminal_with_delta", eventType: "response.completed", message: []byte(`{"type":"response.completed","delta":"x"}`), want: false},
		{name: "lifecycle_with_delta", eventType: "response.output_item.added", message: []byte(`{"type":"response.output_item.added","delta":"x"}`), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isOpenAIWSTokenEventMessage(tc.message, tc.eventType))
		})
	}
}
