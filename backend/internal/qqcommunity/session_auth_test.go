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

func TestQQAuthClassificationRequiresExplicitFailure(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       bool
	}{
		{"native auth", `EventRet: {"result":-10003,"errMsg":"身份验证失败，请你重新登录。(w21)"}`, true},
		{"expired identity", "你的用户身份已失效，为保证账号安全，请你重新登录。", true},
		{"expired session", "登录态已失效，请重新登录。", true},
		{"expired account", "你的账号当前登录已失效，请重新登录。", true},
		{"network", "网络连接异常!", false},
		{"group mute", "群禁言，发送失败", false},
		{"permission", "权限不足", false},
		{"number alone", "result=-10003", false},
		{"webui auth", "WebUI token invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, qqAuthenticationRejected(oneBotAck{Status: "failed", Retcode: 1200, Message: tc.text}))
			require.Equal(t, tc.want, qqAuthenticationRejected(oneBotAck{Status: "failed", Retcode: 1200, Wording: tc.text}))
			require.False(t, qqAuthenticationRejected(oneBotAck{Status: "ok", Data: json.RawMessage(`{}`), Message: tc.text}))
		})
	}
}

func TestQQAuthenticationFailureReachesHealthAndDoesNotLeakOrRetry(t *testing.T) {
	requests := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			var req struct{ Action, Echo string }
			if ws.ReadJSON(&req) != nil {
				return
			}
			requests <- req.Action
			ack := oneBotAck{Echo: req.Echo, Status: "ok", Data: json.RawMessage(`[{"rkey":"private-rkey"}]`)}
			if req.Action == "send_group_msg" {
				ack.Status, ack.Retcode = "failed", 1200
				ack.Message = "EventChecker Failed: EventRet:\n{\"result\": -10003, \"errMsg\": \"身份验证失败，请你重新登录。(w21)\"}\nprivate-secret"
			}
			if ws.WriteJSON(ack) != nil {
				return
			}
		}
	}))
	defer srv.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer ws.Close()
	c := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var a oneBotAck
			if ws.ReadJSON(&a) != nil {
				return
			}
			c.mu.Lock()
			ch := c.pending[a.Echo]
			c.mu.Unlock()
			if ch != nil {
				ch <- a
			}
		}
	}()
	now := time.Now()
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":true,"good":true}}`), now)
	_, err = c.call(context.Background(), "send_group_msg", map[string]any{})
	require.ErrorIs(t, err, errQQSessionExpired)
	var fallback *oneBotRejected
	require.NotErrorAs(t, err, &fallback, "an expired session must not retry image replies as text")
	require.NotContains(t, err.Error(), "private-secret")
	require.Equal(t, "send_group_msg", <-requests)
	h := c.health.snapshot(time.Now())
	require.Equal(t, "authentication_failed", h.State)
	require.True(t, h.Alert)
	require.NotNil(t, h.AuthFailedAt)
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":true,"good":true}}`), time.Now())
	c.observeHealth([]byte(`{"post_type":"message"}`), time.Now())
	c.probeHealth(context.Background(), time.Now())
	require.Equal(t, "nc_get_rkey", <-requests)
	h = c.health.snapshot(time.Now())
	require.Equal(t, "ok", h.Probe)
	require.Equal(t, "authentication_failed", h.State, "cached status, queued messages and successful reads cannot erase a send authentication failure")
	raw, err := json.Marshal(h)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-")
	require.Empty(t, requests, "no automatic retries, QR refresh or restart")
	_ = ws.Close()
	<-done
}

func TestQQAuthRecoveryRequiresNewSendReceipt(t *testing.T) {
	now := time.Now()
	online := true
	c := &oneBotConnection{health: oneBotHealth{online: &online, heartbeat: now, checked: now, probe: "ok", authRejectedAt: now}}
	c.observeSuccessfulSend("send_group_msg", json.RawMessage(`{"message_id":123}`), now.Add(-time.Second))
	c.observeSuccessfulSend("get_msg", json.RawMessage(`{"message_id":123}`), now.Add(time.Second))
	c.observeSuccessfulSend("send_private_msg", json.RawMessage(`{"message_id":0}`), now.Add(time.Second))
	c.observeSuccessfulSend("send_private_msg", json.RawMessage(`{}`), now.Add(time.Second))
	require.Equal(t, "authentication_failed", c.health.snapshot(now).State)
	c.observeSuccessfulSend("send_private_msg", json.RawMessage(`{"message_id":"42"}`), now.Add(time.Second))
	require.Equal(t, "probe_ok", c.health.snapshot(now).State)
	require.Nil(t, c.health.snapshot(now).AuthFailedAt)
}
