package qqcommunity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const qqHeartbeatEvent = "QQ_HEARTBEAT_NOTIFICATION"

func qqHeartbeatSlot(c Config, now time.Time) string {
	h := c.HealthNotifications
	if !c.Enabled || !h.HeartbeatEnabled || h.HeartbeatMinutes < 1 || h.HeartbeatMinutes > 60 || h.HeartbeatGroup == "" || !c.allows(h.HeartbeatGroup) || c.groupPolicy(h.HeartbeatGroup).DisableNotifications {
		return ""
	}
	return fmt.Sprintf("qq-heartbeat:%s:%d:%d", h.HeartbeatGroup, h.HeartbeatMinutes, now.Unix()/int64(60*h.HeartbeatMinutes))
}

// A real QQ group message, never a WebSocket ping or a model call. Unlike
// channel alerts this explicit opt-in runs around the clock (no quiet hours).
// Only the current time window can be sent; reconnects never replay old ones.
func (s *Service) sendQQHeartbeat(ctx context.Context, cfg Config, conn *oneBotConnection, now time.Time) {
	slot := qqHeartbeatSlot(cfg, now)
	if slot == "" {
		return
	}
	conn.mu.Lock()
	if conn.heartbeatSlot == slot {
		conn.mu.Unlock()
		return
	}
	conn.heartbeatSlot = slot
	health := conn.health.snapshot(now)
	conn.mu.Unlock()
	q, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	// Deduplicate durably, including uncertain sends and process restarts.
	// Start as running: normal job workers must never replay a heartbeat.
	id := randomID()
	_, err := s.db.ExecContext(q, `INSERT INTO qqc_jobs(id,app_id,event_key,event_type,group_id,payload,kind,status,started_at) VALUES($1,$2,$3,$4,$5,'{}','heartbeat_notification','running',NOW()) ON CONFLICT(app_id,event_key) DO NOTHING`, id, cfg.AppID, slot, qqHeartbeatEvent, cfg.HealthNotifications.HeartbeatGroup)
	if err != nil {
		slog.Warn("qqbot heartbeat reservation failed")
		return
	}
	// Checking ownership also works with MySQL CLIENT_FOUND_ROWS enabled.
	var owner string
	if err = s.db.QueryRowContext(q, `SELECT id FROM qqc_jobs WHERE app_id=$1 AND event_key=$2`, cfg.AppID, slot).Scan(&owner); err != nil || owner != id {
		return
	}
	j := Job{ID: id, AppID: cfg.AppID, GroupID: cfg.HealthNotifications.HeartbeatGroup}
	if health.State != "probe_ok" {
		s.finish(j, "skipped", "QQ 实时会话未验证或检测异常，未发送在线心跳")
		return
	}
	// Honor settings changed while the live probe/DB request was in flight.
	current, err := s.Config(q)
	if err != nil || current.AppID != conn.bot || codeHash(current.AppSecret) != conn.credential || qqHeartbeatSlot(current, time.Now()) != slot {
		s.finish(j, "cancelled", "心跳设置已变更或当前时段已过期，未发送")
		return
	}
	conn.mu.Lock()
	ready := conn.health.snapshot(time.Now()).State == "probe_ok"
	conn.mu.Unlock()
	if !ready || q.Err() != nil {
		s.finish(j, "skipped", "QQ 会话证据失效，未发送在线心跳")
		return
	}
	text := "🟢 小碗在线 · " + time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("01-02 15:04:05") + "（北京时间）"
	raw, err := conn.call(q, "send_group_msg", map[string]any{"group_id": j.GroupID, "message": []map[string]any{{"type": "text", "data": map[string]string{"text": text}}}})
	var receipt struct {
		MessageID obID `json:"message_id"`
	}
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.MessageID == "" || receipt.MessageID == "0" {
		// A lost receipt is uncertain delivery: do not retry this time window.
		s.finish(j, "delivery_failed", "QQ 在线心跳未收到有效发送回执，本时段不重发")
		slog.Warn("qqbot heartbeat delivery unconfirmed")
		return
	}
	s.finish(j, "completed", "")
}
