package qqcommunity

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type heartbeatIDArgument struct{ rows *sqlmock.Rows }

func (a heartbeatIDArgument) Match(v driver.Value) bool {
	id, ok := v.(string)
	if ok && id != "" {
		a.rows.AddRow(id)
	}
	return ok && id != ""
}

func heartbeatTestConfig() Config {
	c := DefaultConfig()
	c.Enabled, c.AppID = true, "1849138657"
	c.AllowedGroups = []string{"1025362951"}
	c.HealthNotifications.HeartbeatEnabled = true
	c.HealthNotifications.HeartbeatGroup = "1025362951"
	return c
}

func TestQQHeartbeatRequiresExplicitAllowedGroupAndKeepsOldSettingsOff(t *testing.T) {
	var old Config
	require.NoError(t, json.Unmarshal([]byte(`{"health_notifications":{"enabled":true}}`), &old))
	require.False(t, old.HealthNotifications.HeartbeatEnabled)
	require.Equal(t, 5, old.HealthNotifications.HeartbeatMinutes)
	now := time.Date(2026, 11, 3, 13, 30, 0, 0, time.UTC)
	c := heartbeatTestConfig()
	require.NoError(t, validateHealthNotifications(c))
	first := qqHeartbeatSlot(c, now)
	require.NotEmpty(t, first)
	require.Equal(t, first, qqHeartbeatSlot(c, now.Add(299*time.Second)))
	require.NotEqual(t, first, qqHeartbeatSlot(c, now.Add(300*time.Second)))
	for _, change := range []func(*Config){
		func(c *Config) { c.Enabled = false },
		func(c *Config) { c.HealthNotifications.HeartbeatEnabled = false },
		func(c *Config) { c.HealthNotifications.HeartbeatGroup = "" },
		func(c *Config) { c.HealthNotifications.HeartbeatGroup = "1093968528" },
		func(c *Config) { c.HealthNotifications.HeartbeatMinutes = 0 },
		func(c *Config) { c.HealthNotifications.HeartbeatMinutes = 61 },
	} {
		x := c
		change(&x)
		require.Empty(t, qqHeartbeatSlot(x, now))
	}
	c.NotificationQuietHours = QuietHours{Enabled: true, StartHour: 0, EndHour: 23}
	require.Equal(t, first, qqHeartbeatSlot(c, now), "explicit online check-ins run 24h")
}

func TestQQHeartbeatCannotAnnounceCachedOnlineAndDeduplicatesAcrossReconnects(t *testing.T) {
	for _, tc := range []struct {
		name, probe         string
		duplicate, disabled bool
		expected            string
	}{
		{"cached online only", "", false, false, "skipped"},
		{"failed live session", "failed", false, false, "skipped"},
		{"duplicate on reconnect", "ok", true, false, ""},
		{"disabled during probe", "ok", false, true, "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			cfg, now := heartbeatTestConfig(), time.Now()
			conn := &oneBotConnection{bot: cfg.AppID, credential: codeHash("")}
			online := true
			conn.health = oneBotHealth{online: &online, heartbeat: now, checked: now, probe: tc.probe}
			if tc.probe == "failed" {
				conn.health.failures = 3
			}
			rows := sqlmock.NewRows([]string{"id"})
			var idArg sqlmock.Argument = heartbeatIDArgument{rows}
			if tc.duplicate {
				idArg = sqlmock.AnyArg()
				rows.AddRow("previous-owner")
			}
			mock.ExpectExec("INSERT INTO qqc_jobs").WithArgs(idArg, cfg.AppID, qqHeartbeatSlot(cfg, now), qqHeartbeatEvent, "1025362951").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT id FROM qqc_jobs").WithArgs(cfg.AppID, qqHeartbeatSlot(cfg, now)).WillReturnRows(rows)
			if tc.disabled {
				current := cfg
				current.HealthNotifications.HeartbeatEnabled = false
				raw, _ := json.Marshal(current)
				mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
			}
			if tc.expected != "" {
				mock.ExpectExec("UPDATE qqc_jobs SET status").WithArgs(sqlmock.AnyArg(), tc.expected, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			// No socket exists: any accidental send would fail this test.
			s.sendQQHeartbeat(context.Background(), cfg, conn, now)
			s.sendQQHeartbeat(context.Background(), cfg, conn, now.Add(time.Second))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestQQHeartbeatNeedsActualQQMessageReceiptAndNeverRetries(t *testing.T) {
	for _, tc := range []struct {
		name, status, data, expected string
		code                         int
	}{
		{"delivered", "ok", `{"message_id":42}`, "completed", 0},
		{"missing receipt", "ok", `{}`, "delivery_failed", 0},
		{"zero receipt", "ok", `{"message_id":0}`, "delivery_failed", 0},
		{"rejected", "failed", `null`, "delivery_failed", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan map[string]json.RawMessage, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				for {
					var msg map[string]json.RawMessage
					if ws.ReadJSON(&msg) != nil {
						return
					}
					received <- msg
					_ = ws.WriteJSON(map[string]any{"echo": msg["echo"], "status": tc.status, "retcode": tc.code, "data": json.RawMessage(tc.data)})
				}
			}))
			defer srv.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			require.NoError(t, err)
			defer ws.Close()
			cfg, now := heartbeatTestConfig(), time.Now()
			conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, bot: cfg.AppID, credential: codeHash("")}
			online := true
			conn.health = oneBotHealth{online: &online, heartbeat: now, checked: now, probe: "ok"}
			done := make(chan struct{})
			go func() {
				defer close(done)
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
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			rows := sqlmock.NewRows([]string{"id"})
			mock.ExpectExec("INSERT INTO qqc_jobs").WithArgs(heartbeatIDArgument{rows}, cfg.AppID, qqHeartbeatSlot(cfg, now), qqHeartbeatEvent, "1025362951").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT id FROM qqc_jobs").WithArgs(cfg.AppID, qqHeartbeatSlot(cfg, now)).WillReturnRows(rows)
			raw, _ := json.Marshal(cfg)
			mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
			mock.ExpectExec("UPDATE qqc_jobs SET status").WithArgs(sqlmock.AnyArg(), tc.expected, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			s.sendQQHeartbeat(context.Background(), cfg, conn, now)
			msg := <-received
			require.JSONEq(t, `"send_group_msg"`, string(msg["action"]))
			require.Contains(t, string(msg["params"]), "1025362951")
			require.Contains(t, string(msg["params"]), "小碗在线")
			s.sendQQHeartbeat(context.Background(), cfg, conn, now)
			require.Empty(t, received)
			require.NoError(t, mock.ExpectationsWereMet())
			_ = ws.Close()
			<-done
		})
	}
}
