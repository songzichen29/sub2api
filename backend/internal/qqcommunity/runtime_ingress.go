package qqcommunity

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// 路由只接受认证桥传入的原始身份，不接收模型给出的用户、群或绑定状态。
func (s *Service) ServeRuntimeIngress(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	c, err := s.Config(req.Context())
	if err != nil {
		fail(503, "configuration_unavailable")
		return
	}
	if !runtimeAuthorized(c, req, strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")) {
		fail(401, "unauthorized")
		return
	}
	if req.Method != http.MethodPost {
		fail(405, "method_not_allowed")
		return
	}
	var in struct {
		Phase string  `json:"phase"`
		Event obEvent `json:"event"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 18<<20))
	var extra json.RawMessage
	if d.Decode(&in) != nil || d.Decode(&extra) != io.EOF || (in.Phase != "route" && in.Phase != "delivery") || string(in.Event.SelfID) != c.AppID || string(in.Event.UserID) == c.AppID || !runtimeNumericID.MatchString(string(in.Event.UserID)) || !runtimeMessageID.MatchString(string(in.Event.MessageID)) || in.Event.PostType != "message" {
		fail(400, "invalid_event")
		return
	}
	out := map[string]any{"allowed": false}
	if !c.Enabled {
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	if in.Event.MessageType == "private" && in.Phase == "route" {
		_, sessionErr := s.privateBindingSession(req.Context(), c, string(in.Event.UserID))
		if sessionErr != nil && !errors.Is(sessionErr, sql.ErrNoRows) {
			fail(503, "binding_state_unavailable")
			return
		}
		text := oneBotMessageEvent(in.Event).Content
		out["allowed"] = true
		out["business"] = sessionErr == nil || privateBindingMessage.MatchString(text) || privateAccountCommand(in.Event) != "" || !c.allowsPrivateChat(string(in.Event.UserID))
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	if in.Event.MessageType != "group" || !runtimeNumericID.MatchString(string(in.Event.GroupID)) || !c.allows(string(in.Event.GroupID)) {
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	blocked := false
	if in.Phase == "route" {
		blocked, err = s.moderateIncoming(req.Context(), c, oneBotMessageEvent(in.Event))
	}
	if in.Phase == "delivery" {
		blocked, err = s.moderationBlocked(req.Context(), c, string(in.Event.GroupID), string(in.Event.MessageID))
	}
	if err != nil {
		fail(503, "moderation_unavailable")
		return
	}
	out["allowed"] = !blocked
	if blocked {
		out["reason"] = "moderation_blocked"
	}
	_ = json.NewEncoder(w).Encode(out)
}

var privateBindingMessage = regexp.MustCompile(`(?i)^(?:/?(?:绑定|确认|解绑|取消)(?:\s|$)|[a-z\d_-]{24}$|[^\s@]+@[^\s@]+\.[^\s@]+$)`)

func privateAccountCommand(in obEvent) string {
	for _, segment := range in.Message {
		if segment.Type != "text" {
			return ""
		}
	}
	text := oneBotMessageEvent(in).Content
	cmd, arg := command(text)
	if cmd == "" {
		cmd = naturalAccountCommand(text)
	}
	if arg != "" {
		return ""
	}
	switch cmd {
	case "我的余额", "今日用量", "抽奖次数":
		return cmd
	}
	return ""
}

// 与 OneBot 业务入口共用转换，附件和引用不得被误认为相同的纯文本刷屏。
func oneBotMessageEvent(in obEvent) Event {
	e := Event{ID: string(in.MessageID), GroupID: string(in.GroupID), MemberID: string(in.UserID)}
	if in.MessageType == "private" {
		e.GroupID = ""
	}
	at := time.Unix(in.Time, 0)
	if in.Time == 0 {
		at = time.Now()
	}
	e.Timestamp, _ = json.Marshal(at.UTC().Format(time.RFC3339))
	e.Author.MemberID, e.Author.MemberRole = string(in.UserID), in.Sender.Role
	e.Author.Username = in.Sender.Card
	if e.Author.Username == "" {
		e.Author.Username = in.Sender.Nickname
	}
	var text strings.Builder
	for _, seg := range in.Message {
		switch seg.Type {
		case "text":
			text.WriteString(obValue(seg.Data["text"]))
		case "at":
			id := obValue(seg.Data["qq"])
			e.Mentions = append(e.Mentions, Mention{ID: id, MemberID: id})
		case "reply":
			e.MessageReference.MessageID = obValue(seg.Data["id"])
		default:
			appendOneBotMedia(&e, seg)
		}
	}
	e.Content = strings.TrimSpace(text.String())
	return e
}
