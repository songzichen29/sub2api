package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
