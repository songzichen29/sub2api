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

func TestQQSessionHealthDoesNotConfuseSocketOrQuietGroupWithLogin(t *testing.T) {
	now := time.Now()
	c := &oneBotConnection{}
	require.Equal(t, "unknown", c.health.snapshot(now).State)
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":true,"good":true}}`), now)
	require.Equal(t, "unknown", c.health.snapshot(now).State, "cached heartbeat alone is not live evidence")
	c.health.checked, c.health.succeeded, c.health.probe = now, now, "ok"
	c.health.message = now.Add(-24 * time.Hour)
	require.Equal(t, "probe_ok", c.health.snapshot(now).State, "no group activity must not force logout")
	require.Equal(t, "stale", c.health.snapshot(now.Add(91*time.Second)).State)
	c.health.heartbeat = now.Add(151 * time.Second)
	require.Equal(t, "stale", c.health.snapshot(now.Add(151*time.Second)).State, "live probe also expires")
	c.health.heartbeat = now
	c.health.failures, c.health.probe = 1, "failed"
	require.Equal(t, "degraded", c.health.snapshot(now).State)
	require.False(t, c.health.snapshot(now).Alert)
	c.health.failures = 3
	require.True(t, c.health.snapshot(now).Alert)
	c.observeHealth([]byte(`{"post_type":"notice","notice_type":"bot_offline"}`), now)
	require.Equal(t, "offline_reported", c.health.snapshot(now).State)
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{}}`), now)
	require.Equal(t, "offline_reported", c.health.snapshot(now).State)
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":false,"good":true}}`), now)
	require.Equal(t, "offline_reported", c.health.snapshot(now).State)
	c.observeHealth([]byte(`{"post_type":"message"}`), now)
	require.Equal(t, now, *c.health.snapshot(now).LastMessageAt)
	require.Equal(t, "offline_reported", c.health.snapshot(now).State, "a queued message cannot clear offline evidence")
	c.observeHealth([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":true,"good":true}}`), now)
	require.Equal(t, "degraded", c.health.snapshot(now).State, "heartbeat must not erase probe failures")
	s := New(nil, nil, false, nil)
	s.onebot.conn = c
	c.bot = "old"
	require.Equal(t, "disconnected", s.sessionHealth("new").State)
	s.onebot.conn = &oneBotConnection{bot: "new"}
	require.Equal(t, "unknown", s.sessionHealth("new").State, "new socket never inherits old success")
}

func TestQQLiveProbeRequiresActualDataAndDoesNotExposeSecrets(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{}`, `""`, `[{}]`, `[{"rkey":""}]`, `invalid`} {
		require.False(t, validLiveProbe(json.RawMessage(raw)), raw)
	}
	require.True(t, validLiveProbe(json.RawMessage(`[{"rkey":"private-rkey","type":10}]`)))
	for _, tc := range []struct {
		name, status, data string
		code               int
		expected           string
	}{
		{"valid", "ok", `[{"rkey":"private-rkey"}]`, 0, "ok"},
		{"empty success", "ok", `null`, 0, "failed"},
		{"empty list", "ok", `[]`, 0, "failed"},
		{"rejected", "failed", `null`, 200, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 4)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				for {
					var req struct {
						Action, Echo string
						Params       map[string]any
					}
					if ws.ReadJSON(&req) != nil {
						return
					}
					requests <- req.Action
					_ = ws.WriteJSON(map[string]any{"echo": req.Echo, "status": tc.status, "retcode": tc.code, "data": json.RawMessage(tc.data)})
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
					var ack oneBotAck
					if ws.ReadJSON(&ack) != nil {
						return
					}
					c.mu.Lock()
					ch := c.pending[ack.Echo]
					c.mu.Unlock()
					if ch != nil {
						ch <- ack
					}
				}
			}()
			c.probeHealth(context.Background(), time.Now())
			require.Equal(t, "nc_get_rkey", <-requests, "only a read-only protocol request is allowed")
			c.mu.Lock()
			h := c.health.snapshot(time.Now())
			c.mu.Unlock()
			require.Equal(t, tc.expected, h.Probe)
			raw, err := json.Marshal(h)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "private-rkey")
			c.probeHealth(context.Background(), time.Now())
			require.Empty(t, requests, "probe is throttled; no QR/login/restart/send actions")
			_ = ws.Close()
			<-done
		})
	}
}

func TestQQFailedProbeCannotTriggerQRRefresh(t *testing.T) {
	n := &napcatLoginClient{wasOnline: true}
	require.False(t, n.shouldRefreshQR(NapCatLoginStatus{Online: true, Connected: true, SessionHealth: QQSessionHealth{State: "degraded", Alert: true}}, time.Now()))
}
