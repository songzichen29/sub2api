package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestOneBotNativeImagesAndAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		code         int
		data         any
		fail         bool
	}{
		{"numeric message ID", "ok", 0, map[string]any{"message_id": 12345}, false},
		{"string message ID", "ok", 0, map[string]any{"message_id": "12345"}, false},
		{"rejected send", "failed", 100, nil, true},
		{"missing receipt", "ok", 0, map[string]any{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan map[string]json.RawMessage, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				var msg map[string]json.RawMessage
				if ws.ReadJSON(&msg) != nil {
					return
				}
				received <- msg
				_ = ws.WriteJSON(map[string]any{"echo": json.RawMessage(msg["echo"]), "status": tc.status, "retcode": tc.code, "data": tc.data})
				for {
					if _, _, err := ws.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer ws.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
			defer cancel()
			conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, bot: "1849138657", credential: codeHash("test-only-token"), cancel: cancel}
			s := New(nil, nil, false, nil)
			s.onebot.conn = conn
			go func() {
				var ack oneBotAck
				if ws.ReadJSON(&ack) != nil {
					return
				}
				conn.mu.Lock()
				ch := conn.pending[ack.Echo]
				conn.mu.Unlock()
				if ch != nil {
					ch <- ack
				}
			}()
			raw, err := s.sendOneBot(ctx, Config{AppID: conn.bot, AppSecret: "test-only-token"}, Job{GroupID: "1025362951", MemberID: "123456789", ReplyReference: "42"}, "hello", []byte("png-data"))
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `{"id":"12345"}`, string(raw))
			}
			msg := <-received
			var params struct {
				Group   string `json:"group_id"`
				Message []struct {
					Type string            `json:"type"`
					Data map[string]string `json:"data"`
				} `json:"message"`
			}
			require.NoError(t, json.Unmarshal(msg["params"], &params))
			require.Equal(t, "1025362951", params.Group)
			require.Len(t, params.Message, 4)
			require.Equal(t, "reply", params.Message[0].Type)
			require.Equal(t, "at", params.Message[1].Type)
			require.Equal(t, "text", params.Message[2].Type)
			require.Equal(t, "image", params.Message[3].Type)
			require.Equal(t, "base64://cG5nLWRhdGE=", params.Message[3].Data["file"])
		})
	}
}

func TestOneBotDeliversAllLongReplyPartsAndStopsOnFailure(t *testing.T) {
	for _, failAt := range []int{0, 2} {
		t.Run(map[int]string{0: "full text and one image", 2: "partial failure"}[failAt], func(t *testing.T) {
			type segment struct {
				Type string            `json:"type"`
				Data map[string]string `json:"data"`
			}
			type request struct {
				Echo   string `json:"echo"`
				Params struct {
					Message []segment `json:"message"`
				} `json:"params"`
			}
			text := strings.Repeat("完整内容🙂\n", 1800) + "这是必须保留的最后一段。"
			expected := splitReply(text)
			require.Greater(t, len(expected), 5)
			received := make(chan request, len(expected))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				for n := 1; ; n++ {
					var msg request
					if ws.ReadJSON(&msg) != nil {
						return
					}
					received <- msg
					status, code := "ok", 0
					if n == failAt {
						status, code = "failed", 100
					}
					if ws.WriteJSON(map[string]any{"echo": msg.Echo, "status": status, "retcode": code, "data": map[string]any{"message_id": n}}) != nil {
						return
					}
				}
			}))
			defer server.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer ws.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, bot: "bot", credential: codeHash("test-token"), cancel: cancel}
			s := New(nil, nil, false, nil)
			s.onebot.conn = conn
			go func() {
				for {
					var ack oneBotAck
					if ws.ReadJSON(&ack) != nil {
						return
					}
					conn.mu.Lock()
					ch := conn.pending[ack.Echo]
					conn.mu.Unlock()
					if ch != nil {
						ch <- ack
					}
				}
			}()
			raw, err := s.sendOneBot(ctx, Config{AppID: "bot", AppSecret: "test-token"}, Job{GroupID: "group", MemberID: "member", ReplyReference: "42"}, text, []byte("png-data"))
			count := len(expected)
			if failAt > 0 {
				require.Error(t, err)
				count = failAt
			} else {
				require.NoError(t, err)
				var receipt map[string]string
				require.NoError(t, json.Unmarshal(raw, &receipt))
				require.NotEmpty(t, receipt["id"])
			}
			var delivered strings.Builder
			images, mentions, replies := 0, 0, 0
			for i := 0; i < count; i++ {
				msg := <-received
				for _, part := range msg.Params.Message {
					switch part.Type {
					case "text":
						require.LessOrEqual(t, utf8.RuneCountInString(part.Data["text"]), 1500)
						delivered.WriteString(part.Data["text"])
					case "image":
						images++
						require.Equal(t, len(expected)-1, i)
					case "at":
						mentions++
						require.Zero(t, i)
					case "reply":
						replies++
						require.Zero(t, i)
					}
				}
			}
			require.Empty(t, received)
			require.Equal(t, 1, mentions)
			require.Equal(t, 1, replies)
			if failAt == 0 {
				require.Equal(t, text, delivered.String())
				require.Equal(t, 1, images)
			} else {
				require.Equal(t, strings.Join(expected[:failAt], ""), delivered.String())
				require.Zero(t, images)
			}
		})
	}
}
