package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAutonomousConfigDefaultsAndValidation(t *testing.T) {
	var c Config
	require.NoError(t, json.Unmarshal([]byte(`{"autonomous":{"sample_percent":0,"daily_calls":0}}`), &c))
	require.Equal(t, 120, c.Autonomous.CooldownSeconds)
	require.Zero(t, c.Autonomous.SamplePercent)
	require.Zero(t, c.Autonomous.DailyCalls)
	c = DefaultConfig()
	require.False(t, c.Autonomous.Enabled)
	require.False(t, c.Autonomous.MediaEnabled)
	require.Equal(t, 10, c.Autonomous.MediaSamplePercent)
	require.Equal(t, 100, c.Autonomous.MediaDailyCalls)
	require.NoError(t, validateAutonomous(c))
	c.Autonomous.MediaSamplePercent = 101
	require.Error(t, validateAutonomous(c))
	c.Autonomous.MediaSamplePercent = 10
	c.Autonomous.MediaDailyCalls = -1
	require.Error(t, validateAutonomous(c))
	c.Autonomous.MediaDailyCalls = 100
	c.Autonomous.Enabled = true
	require.Error(t, validateAutonomous(c))
	c.Autonomous.Groups = []string{"group"}
	require.Error(t, validateAutonomous(c))
	c.AllowedGroups = []string{"group"}
	require.NoError(t, validateAutonomous(c))
	c.Autonomous.CooldownSeconds, c.Autonomous.MemberCooldownSeconds = 5, 5
	require.NoError(t, validateAutonomous(c))
	c.Autonomous.CooldownSeconds = 4
	require.Error(t, validateAutonomous(c))
	c.Autonomous.CooldownSeconds = 5
	c.Autonomous.SamplePercent = 101
	require.Error(t, validateAutonomous(c))
	c = DefaultConfig()
	c.Autonomous.Names = []string{"@all"}
	require.Error(t, validateAutonomous(c))
}
func TestAutonomousNamesAndLocalFilters(t *testing.T) {
	a := DefaultAutonomousConfig()
	a.Names = append(a.Names, "小爱")
	for _, text := range []string{"CTAV，帮我看看这个问题", "ctav 请问怎么安装", "CTAV你能帮忙吗", "今天CTAV不错", "CTAVX 帮我", "CTAV的功能怎么样", "CTAV", "这个问题想让cTaV看看", "请帮我看看，小爱", "小爱"} {
		require.True(t, containsBotName(a, text), text)
	}
	for _, text := range []string{"", "   ", "帮我看看这个问题", "CTA", "小明"} {
		require.False(t, containsBotName(a, text), text)
	}
	require.False(t, containsBotName(AutonomousConfig{Names: []string{"", " "}}, "普通消息"))
	require.True(t, botNameCall(a, Event{Content: "小爱"}))
	require.True(t, botNameCall(a, Event{Content: "CTAV 看看 https://example.com"}))
	require.False(t, botNameCall(a, Event{Content: "/生图 CTAV"}))
	bot := Event{Content: "CTAV"}
	bot.Author.Bot = true
	require.False(t, botNameCall(a, bot))
	elements := []MessageElement{{MsgElements: []MessageElement{{Content: "这个问题想让小爱看看"}}}}
	require.True(t, botNameCall(a, Event{MsgElements: elements}))
	require.False(t, botNameCall(a, Event{MessageType: 103, MsgElements: elements}))
	require.False(t, botNameCall(a, Event{MsgElements: []MessageElement{{MessageType: 103, Content: "CTAV", MsgElements: elements}}}))
	for _, text := range []string{"哈哈哈哈", "嗯", "/签到", "有人知道 https://example.com 吗", "@张三 部署好了吗", "sk-secret-key", "这是验证码 12345"} {
		_, reason := autonomousText(Event{Content: text})
		require.NotEmpty(t, reason, text)
	}
	text, reason := autonomousText(Event{Content: "今天这个错误怎么处理？"})
	require.Empty(t, reason)
	require.NotEmpty(t, text)
	for _, text := range []string{"你好", "好啊", "为什么", "怎么弄", "继续", "ok"} {
		_, reason := autonomousText(Event{Content: text})
		require.Empty(t, reason, text)
	}
	e := Event{Content: "这个问题怎么处理？"}
	e.MessageReference.MessageID = "another-user"
	_, reason = autonomousText(e)
	require.NotEmpty(t, reason)
	c := DefaultConfig()
	c.Autonomous.SamplePercent = 0
	require.False(t, sampledMessage(c, Event{ID: "a"}))
	c.Autonomous.SamplePercent = 100
	require.True(t, sampledMessage(c, Event{ID: "a"}))
}
func TestAutonomousActiveBeijingWindow(t *testing.T) {
	a := DefaultAutonomousConfig()
	at := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	require.True(t, autonomousActive(a, at))
	require.False(t, autonomousActive(a, at.Add(-time.Second)))
	require.False(t, autonomousActive(a, time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)))
	a.StartHour, a.EndHour = 23, 9
	require.True(t, autonomousActive(a, at.Add(-time.Second)))
	require.False(t, autonomousActive(a, at))
	a.StartHour = a.EndHour
	require.True(t, autonomousActive(a, at))
}
func TestAutonomousStrictOutput(t *testing.T) {
	text, err := parseAutonomousReply(`{"action":"reply","text":"可以先看看错误日志。"}`, 100)
	require.NoError(t, err)
	require.Equal(t, "可以先看看错误日志。", text)
	text, err = parseAutonomousReply(`{"action":"skip"}`, 100)
	require.NoError(t, err)
	require.Empty(t, text)
	for _, raw := range []string{`{"action":"reply","text":"@大家"}`, `{"action":"reply","text":"<qqbot-at-everyone />"}`, `{"action":"reply","text":""}`, `{"action":"reply","text":"answer","other":true}`, `{"action":"skip"} {}`, "普通回复", `{"action":"reply","text":"` + strings.Repeat("字", 101) + `"}`} {
		_, err = parseAutonomousReply(raw, 100)
		require.Error(t, err, raw)
	}
}

func TestAutonomousJSONWrappersKeepStrictValidation(t *testing.T) {
	for _, raw := range []string{
		"```json\n{\"action\":\"reply\",\"text\":\"可以先看看日志。\"}\n```",
		"```JSON\r\n{\"action\":\"reply\",\"text\":\"可以先看看日志。\"}\r\n```",
		"~~~\n{\"action\":\"reply\",\"text\":\"可以先看看日志。\"}\n~~~",
		"\uFEFF {\"action\":\"reply\",\"text\":\"可以先看看日志。\"}",
	} {
		text, err := parseAutonomousReply(raw, 100)
		require.NoError(t, err)
		require.Equal(t, "可以先看看日志。", text)
	}
	for _, raw := range []string{
		"```json\n{\"action\":\"reply\",\"text\":\"@所有人\"}\n```",
		"```json\n{\"action\":\"skip\"} {}\n```",
		"```json\n{\"action\":\"skip\"}\n```\n额外文字",
		"说明如下：{\"action\":\"skip\"}",
		"```javascript\n{\"action\":\"skip\"}\n```",
		`{"action":"reply","text":"a","private_secret_field":"sensitive-value"}`,
		`{"action":"reply","text":`, `[]`, `null`,
	} {
		_, err := parseAutonomousReply(raw, 100)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private_secret_field")
		require.NotContains(t, err.Error(), "sensitive-value")
	}
	wrapped := "```json\n{\"action\":\"reply\",\"understood\":true,\"visual_token\":\"V123456\",\"text\":\"好可爱\"}\n```"
	media, err := parseAutonomousMediaReply(wrapped, "V123456", 100)
	require.NoError(t, err)
	require.Equal(t, "reply", media.Action)
	media, err = parseAutonomousMediaReply(wrapped, "V654321", 100)
	require.NoError(t, err)
	require.Equal(t, "skip", media.Action, "wrappers must never bypass actual vision verification")
}

func TestAutonomousHistoryHasOneOutputContract(t *testing.T) {
	s := New(nil, nil, false, nil)
	c := DefaultConfig()
	c.HistoryMessages = 0
	for _, shared := range []bool{false, true} {
		for _, instructions := range []string{autonomousInstructions, autonomousMediaInstructions} {
			messages := s.loadHistoryWithReplyInstructions(context.Background(), c, Job{}, shared, instructions)
			require.Len(t, messages, 2)
			require.Contains(t, messages[1].Content, historyContextInstructions)
			require.Contains(t, messages[1].Content, instructions)
			require.NotContains(t, messages[1].Content, naturalChatInstructions)
			if shared {
				require.Contains(t, messages[1].Content, sharedGroupHistoryScopeInstructions)
			} else {
				require.Contains(t, messages[1].Content, groupHistoryScopeInstructions)
			}
		}
	}
	ordinary := s.history(context.Background(), c, Job{})
	require.Contains(t, ordinary[1].Content, naturalChatInstructions)
}

func TestAutonomousJSONRequestProtocolsAndOrdinaryChat(t *testing.T) {
	for _, protocol := range []string{"chat_completions", "responses"} {
		for _, structured := range []bool{false, true} {
			t.Run(protocol+"/structured="+map[bool]string{true: "yes", false: "no"}[structured], func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "test-model", body["model"])
					require.Equal(t, false, body["stream"])
					require.NotContains(t, body, "tools")
					if protocol == "responses" {
						require.Equal(t, "/responses", r.URL.Path)
						require.NotContains(t, body, "response_format")
						if structured {
							require.Equal(t, map[string]any{"format": map[string]any{"type": "json_object"}}, body["text"])
						} else {
							require.NotContains(t, body, "text")
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output_text": `{"action":"reply","text":"你好"}`, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5}})
					} else {
						require.Equal(t, "/chat/completions", r.URL.Path)
						if structured {
							require.Equal(t, map[string]any{"type": "json_object"}, body["response_format"])
						} else {
							require.NotContains(t, body, "response_format")
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"action":"reply","text":"你好"}`}}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5}})
					}
				}))
				defer server.Close()
				s := New(nil, nil, false, nil)
				c := DefaultConfig()
				c.BaseURL, c.ChatProtocol, c.ChatModel = server.URL, protocol, "test-model"
				call := s.chat
				if structured {
					call = s.chatJSON
				}
				raw, in, out, err := call(context.Background(), c, []Message{{Role: "system", Content: autonomousInstructions}, {Role: "user", Content: "你好"}})
				require.NoError(t, err)
				reply, err := parseAutonomousReply(raw, 100)
				require.NoError(t, err)
				require.Equal(t, "你好", reply)
				require.Equal(t, int64(10), in)
				require.Equal(t, int64(5), out)
			})
		}
	}
}
