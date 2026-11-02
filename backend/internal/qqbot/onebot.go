package qqbot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type onebotID string

func (id *onebotID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*id = onebotID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*id = onebotID(n.String())
	return nil
}

type segment struct {
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}
type groupEvent struct {
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	SelfID      onebotID        `json:"self_id"`
	UserID      onebotID        `json:"user_id"`
	GroupID     onebotID        `json:"group_id"`
	MessageID   onebotID        `json:"message_id"`
	Message     json.RawMessage `json:"message"`
}
type connection struct {
	ws        *websocket.Conn
	write     sync.Mutex
	cancel    context.CancelFunc
	started   time.Time
	version   int
	bot       string
	pendingMu sync.Mutex
	pending   map[string]chan bool
}

func (c *connection) ack(raw []byte) bool {
	var ack struct {
		Echo    string `json:"echo"`
		Status  string `json:"status"`
		Retcode int    `json:"retcode"`
	}
	if json.Unmarshal(raw, &ack) != nil || ack.Echo == "" {
		return false
	}
	c.pendingMu.Lock()
	ch := c.pending[ack.Echo]
	c.pendingMu.Unlock()
	if ch != nil {
		select {
		case ch <- ack.Status == "ok" && ack.Retcode == 0:
		default:
		}
	}
	return true
}
func (c *connection) send(ctx context.Context, id string, action any) error {
	ch := make(chan bool, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	defer func() { c.pendingMu.Lock(); delete(c.pending, id); c.pendingMu.Unlock() }()
	c.write.Lock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.ws.WriteJSON(action)
	c.write.Unlock()
	if err != nil {
		return err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case ok := <-ch:
		if !ok {
			return errors.New("OneBot rejected reply")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("OneBot reply acknowledgement timed out")
	}
}

func (c *connection) close() { c.cancel(); _ = c.ws.Close() }
func (s *Service) disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.sessions {
		c.close()
	}
}
func (s *Service) Stop() {
	if s.Community != nil {
		s.Community.Stop()
	}
	s.mu.Lock()
	s.stopped = true
	for c := range s.sessions {
		c.close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.client.CloseIdleConnections()
}
func (s *Service) ConnectionStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var since *time.Time
	for c := range s.sessions {
		v := c.started
		since = &v
	}
	return map[string]any{"connected": len(s.sessions) > 0, "connections": len(s.sessions), "connected_since": since, "scope": "current_instance"}
}
func (s *Service) ServeOneBot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	c, err := s.config(ctx)
	cancel()
	if err != nil || !c.Enabled {
		http.Error(w, "bot unavailable", http.StatusServiceUnavailable)
		return
	}
	token, err := s.encryptor.Decrypt(c.AccessToken)
	supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err != nil || token == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 || r.Header.Get("X-Self-ID") != c.BotQQ || !strings.EqualFold(r.Header.Get("X-Client-Role"), "Universal") {
		http.Error(w, "unauthorized OneBot connection", http.StatusUnauthorized)
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }, HandshakeTimeout: 5 * time.Second}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ctx, cancel = context.WithCancel(r.Context())
	defer cancel()
	conn := &connection{ws: ws, cancel: cancel, started: time.Now(), version: c.Version, bot: c.BotQQ, pending: make(map[string]chan bool)}
	s.mu.Lock()
	if s.stopped || len(s.sessions) >= 1 {
		s.mu.Unlock()
		conn.close()
		return
	}
	s.sessions[conn] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()
	var workers sync.WaitGroup
	defer func() {
		conn.close()
		workers.Wait()
		s.mu.Lock()
		delete(s.sessions, conn)
		s.mu.Unlock()
		s.wg.Done()
	}()
	ws.SetReadLimit(256 << 10)
	_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	// Poll current stored configuration to revoke a session updated on another instance.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, done := context.WithTimeout(ctx, 3*time.Second)
				current, err := s.config(checkCtx)
				done()
				if err != nil || !current.Enabled || current.Version != conn.version {
					conn.close()
					return
				}
			}
		}
	}()
	slots := make(chan struct{}, 8)
	last := map[string]time.Time{}
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		if conn.ack(raw) {
			continue
		}
		var e groupEvent
		if json.Unmarshal(raw, &e) != nil || e.PostType != "message" || e.MessageType != "group" || e.SubType != "normal" || e.SelfID != onebotID(c.BotQQ) || e.UserID == e.SelfID || !numericID.MatchString(string(e.UserID)) || !c.Allows(string(e.SelfID), string(e.GroupID)) || len(e.MessageID) == 0 || len(e.MessageID) > 32 {
			continue
		}
		command, arg, ok := parseCommand(e)
		if !ok {
			continue
		}
		// Bound both outstanding work and the sender rate, including free commands.
		now := time.Now()
		if now.Sub(last[string(e.UserID)]) < time.Second {
			continue
		}
		if len(last) > 5000 {
			for k, v := range last {
				if now.Sub(v) > time.Minute {
					delete(last, k)
				}
			}
		}
		if len(last) > 5000 {
			continue
		}
		last[string(e.UserID)] = now
		select {
		case slots <- struct{}{}:
		default:
			continue
		}
		workers.Add(1)
		go func(e groupEvent, command, arg string) {
			defer workers.Done()
			defer func() { <-slots }()
			s.process(ctx, conn, c, e, command, arg)
		}(e, command, arg)
	}
}
func parseCommand(e groupEvent) (string, string, bool) {
	var segments []segment
	if json.Unmarshal(e.Message, &segments) != nil {
		return "", "", false
	}
	var text strings.Builder
	addressed := false
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			var value string
			if json.Unmarshal(seg.Data["text"], &value) == nil {
				text.WriteString(value)
			}
		case "at":
			var id onebotID
			if json.Unmarshal(seg.Data["qq"], &id) == nil && id == e.SelfID {
				addressed = true
			}
		case "reply":
			// A quoted message carries only an ID. Never fetch or infer its identity.
		default:
			return "", "", false
		}
	}
	input := strings.TrimSpace(text.String())
	explicit := strings.HasPrefix(input, "/") || strings.HasPrefix(input, "／")
	if !explicit && !addressed {
		return "", "", false
	}
	input = strings.TrimSpace(strings.TrimLeft(input, "/／"))
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "帮助", "", true
	}
	command := fields[0]
	arg := strings.TrimSpace(strings.TrimPrefix(input, command))
	switch command {
	case "帮助", "余额", "签到", "渠道", "清空对话":
		if arg != "" {
			return "帮助", "", true
		}
		return command, "", true
	case "绑定", "聊天":
		return command, arg, true
	}
	if addressed {
		return "聊天", input, true
	}
	return "", "", false
}
func (s *Service) process(ctx context.Context, conn *connection, c Config, e groupEvent, command, arg string) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	current, err := s.config(ctx)
	if err != nil || current.Version != c.Version || !current.Allows(string(e.SelfID), string(e.GroupID)) {
		return
	}
	id := Hash(string(e.SelfID) + ":" + string(e.GroupID) + ":" + string(e.MessageID))
	inserted, err := s.repo.BeginEvent(ctx, EventRecord{ID: id, QQ: string(e.UserID), GroupID: string(e.GroupID), Command: command})
	if err != nil || !inserted {
		return
	}
	reply, err := s.execute(ctx, current, e, id, command, arg)
	status := "success"
	if err != nil {
		log.Printf("[QQBot] command=%s event=%s error=%v", command, id, err)
		status = "failed"
		reply = publicError(err)
	}
	reply = truncate(reply, 6000)
	// Recheck immediately before sending. Credential rotation/disable cancels the connection.
	final, loadErr := s.config(ctx)
	if loadErr != nil || final.Version != c.Version || !final.Allows(string(e.SelfID), string(e.GroupID)) {
		status = "cancelled"
	} else {
		action := map[string]any{"action": "send_group_msg", "params": map[string]any{"group_id": json.Number(e.GroupID), "message": []any{map[string]any{"type": "at", "data": map[string]any{"qq": string(e.UserID)}}, map[string]any{"type": "text", "data": map[string]any{"text": " " + reply}}}}, "echo": id}
		sendErr := conn.send(ctx, id, action)
		if sendErr != nil {
			status = "send_failed"
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	if err = s.repo.FinishEvent(finishCtx, id, status, reply); err != nil {
		log.Printf("[QQBot] finish event=%s error=%v", id, err)
	}
}
func publicError(err error) string {
	if errors.Is(err, ErrModelRequest) {
		return err.Error()
	}
	for _, known := range []error{ErrDisabled, ErrInvalidCode, ErrBindingConflict, ErrNotBound, ErrInactiveUser, ErrAlreadyCheckedIn, ErrRewardsDisabled, ErrChatLimit, ErrCooldown, ErrConflict} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	// Arbitrary database/upstream errors must never be reflected into a public group.
	return "操作失败，请稍后再试或联系站点管理员。"
}
