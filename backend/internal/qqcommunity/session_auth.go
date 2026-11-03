package qqcommunity

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var errQQSessionExpired = errors.New("QQ 登录身份已失效，请在机器人配置中重新登录并验证收发")

// NapCat wraps QQ's EventRet error in message/wording, usually with the generic
// OneBot code 1200. Never treat that code alone, timeouts, bans or a successful
// response containing quoted chat text as evidence that the session expired.
// Inspect only explicit failed acknowledgements; never persist upstream text.
func qqAuthenticationRejected(a oneBotAck) bool {
	if a.Status != "failed" || a.Retcode == 0 {
		return false
	}
	for _, text := range []string{a.Message, a.Wording} {
		if (strings.Contains(text, "身份验证失败") && strings.Contains(text, "重新登录")) ||
			strings.Contains(text, "你的用户身份已失效") ||
			strings.Contains(text, "你的账号当前登录已失效") ||
			strings.Contains(text, "登录态已失效") {
			return true
		}
	}
	return false
}

// Cached heartbeats, incoming queued messages and RKey reads cannot clear an
// authentication rejection. A later send with a real message receipt can;
// replies to requests started before the rejection cannot overwrite it.
// A fresh transport starts unverified and must establish its own evidence.
func (c *oneBotConnection) observeSuccessfulSend(action string, raw json.RawMessage, started time.Time) {
	if action != "send_group_msg" && action != "send_private_msg" {
		return
	}
	var receipt struct {
		MessageID obID `json:"message_id"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.MessageID == "" || receipt.MessageID == "0" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.health.authRejectedAt.IsZero() && started.After(c.health.authRejectedAt) {
		c.health.authRejectedAt = time.Time{}
		c.logHealthTransition(time.Now())
	}
}
