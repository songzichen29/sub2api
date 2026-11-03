package qqcommunity

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// A live socket and NapCat's cached online flag do not verify QQ reception.
// Never use these diagnostics to log out, refresh QR, restart, or replay jobs.
type QQSessionHealth struct {
	State         string     `json:"state"`
	Probe         string     `json:"probe"`
	CheckedAt     *time.Time `json:"checked_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	HeartbeatAt   *time.Time `json:"heartbeat_at,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	Failures      int        `json:"failures"`
	Alert         bool       `json:"alert"`
}

// Guarded by oneBotConnection.mu; a new connection starts without old evidence.
type oneBotHealth struct {
	attempted, checked, succeeded, heartbeat, message time.Time
	online, good                                      *bool
	offlineNotice                                     bool
	probe                                             string
	failures                                          int
	alerted                                           bool
}

func healthTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (h oneBotHealth) snapshot(now time.Time) QQSessionHealth {
	out := QQSessionHealth{State: "unknown", Probe: h.probe, CheckedAt: healthTime(h.checked), LastSuccessAt: healthTime(h.succeeded), HeartbeatAt: healthTime(h.heartbeat), LastMessageAt: healthTime(h.message), Failures: h.failures, Alert: h.failures >= 3}
	if out.Probe == "" {
		out.Probe = "pending"
	}
	if h.offlineNotice || (h.online != nil && !*h.online) {
		out.State, out.Alert = "offline_reported", true
	} else if (!h.heartbeat.IsZero() && now.Sub(h.heartbeat) > 90*time.Second) || (!h.checked.IsZero() && now.Sub(h.checked) > 150*time.Second) {
		out.State = "stale"
	} else if (h.good != nil && !*h.good) || h.failures > 0 {
		out.State = "degraded"
	} else if h.probe == "ok" && !h.heartbeat.IsZero() && h.online != nil && *h.online {
		out.State = "probe_ok"
	}
	// A quiet group is not evidence of logout. Even probe_ok is not an
	// end-to-end guarantee; retain the last actual inbound message separately.
	return out
}

func (s *Service) sessionHealth(appID string) QQSessionHealth {
	s.onebot.mu.Lock()
	defer s.onebot.mu.Unlock()
	c := s.onebot.conn
	if c == nil || c.bot != appID {
		return QQSessionHealth{State: "disconnected", Probe: "pending"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.health.snapshot(time.Now())
}

func (c *oneBotConnection) observeHealth(raw []byte, now time.Time) {
	var e struct {
		PostType string `json:"post_type"`
		MetaType string `json:"meta_event_type"`
		Notice   string `json:"notice_type"`
		Status   struct {
			Online *bool `json:"online"`
			Good   *bool `json:"good"`
		} `json:"status"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.PostType == "meta_event" && e.MetaType == "heartbeat" {
		c.health.heartbeat = now
		if e.Status.Online != nil {
			c.health.online = e.Status.Online
		}
		if e.Status.Good != nil {
			c.health.good = e.Status.Good
		}
		if e.Status.Online != nil && *e.Status.Online {
			c.health.offlineNotice = false
		}
	}
	if e.PostType == "notice" && e.Notice == "bot_offline" {
		c.health.offlineNotice = true
	}
	if e.PostType == "message" {
		c.health.message = now
	}
	c.logHealthTransition(now)
}

// Requires c.mu. No message bodies, tokens, RKeys or upstream errors in logs.
func (c *oneBotConnection) logHealthTransition(now time.Time) {
	h := c.health.snapshot(now)
	if h.Alert && !c.health.alerted {
		slog.Warn("qqbot session health alert", "state", h.State, "probe", h.Probe, "consecutive_failures", h.Failures)
		c.health.alerted = true
	} else if c.health.alerted && h.State == "probe_ok" {
		slog.Info("qqbot live session probe recovered; inbound reception still requires independent verification")
		c.health.alerted = false
	}
}

// nc_get_rkey makes a read-only QQ protocol request in the deployed NapCat.
// get_status/get_login_info merely expose cached flags. Validate the response
// body as well as the OneBot envelope, and never retain or return the RKeys.
func validLiveProbe(raw json.RawMessage) bool {
	var keys []struct {
		RKey string `json:"rkey"`
	}
	if json.Unmarshal(raw, &keys) != nil || len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		if k.RKey == "" {
			return false
		}
	}
	return true
}

func (c *oneBotConnection) probeHealth(ctx context.Context, now time.Time) {
	c.mu.Lock()
	if !c.health.attempted.IsZero() && now.Sub(c.health.attempted) < time.Minute {
		c.mu.Unlock()
		return
	}
	c.health.attempted = now
	c.mu.Unlock()
	q, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	raw, err := c.call(q, "nc_get_rkey", map[string]any{})
	if ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.checked = time.Now()
	if err == nil && validLiveProbe(raw) {
		c.health.probe, c.health.failures, c.health.succeeded = "ok", 0, c.health.checked
	} else {
		// Unsupported APIs and malformed/empty results are unavailable evidence,
		// never proof of logout and never a reason to force reauthentication.
		c.health.probe = "failed"
		c.health.failures++
	}
	c.logHealthTransition(c.health.checked)
}

func (s *Service) sessionDiagnostic(appID string) DiagnosticCheck {
	h := s.sessionHealth(appID)
	messages := map[string]string{
		"disconnected":     "OneBot 未连接，QQ 收信链路不可验证",
		"unknown":          "QQ 会话尚未完成实时探测，不能仅凭已连接判断正常",
		"offline_reported": "NapCat 上报 QQ 离线；WebSocket 连接不代表已登录",
		"stale":            "QQ 会话探测或心跳已过期，当前状态未验证",
		"degraded":         "QQ 实时会话探测异常；未自动重登，请检查 NapCat/QQ 会话",
		"probe_ok":         "QQ 实时会话探测通过；实际收信仍以最后消息时间为准",
	}
	return DiagnosticCheck{"qq_session", h.State == "probe_ok", messages[h.State]}
}
