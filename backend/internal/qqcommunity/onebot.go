package qqcommunity

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type oneBotTransport struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	conn   *oneBotConnection
	closed bool
}
type oneBotConnection struct {
	ws              *websocket.Conn
	write           sync.Mutex
	mu              sync.Mutex
	pending         map[string]chan oneBotAck
	since           time.Time
	bot, credential string
	cancel          context.CancelFunc
}
type oneBotAck struct {
	Echo    string          `json:"echo"`
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
}
type obID string

// A negative acknowledgement is safe to fall back from. A timeout, lost
// connection or missing receipt has uncertain delivery and must never resend.
type oneBotRejected struct {
	Action  string
	Retcode int
}

func (e *oneBotRejected) Error() string {
	return fmt.Sprintf("OneBot %s failed (retcode %d)", e.Action, e.Retcode)
}

func (v *obID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = obID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = obID(n.String())
	return nil
}

type obSegment struct {
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}
type obEvent struct {
	PostType    string      `json:"post_type"`
	MessageType string      `json:"message_type"`
	NoticeType  string      `json:"notice_type"`
	SubType     string      `json:"sub_type"`
	SelfID      obID        `json:"self_id"`
	UserID      obID        `json:"user_id"`
	GroupID     obID        `json:"group_id"`
	MessageID   obID        `json:"message_id"`
	OperatorID  obID        `json:"operator_id"`
	Time        int64       `json:"time"`
	Message     []obSegment `json:"message"`
	Sender      struct {
		Nickname string `json:"nickname"`
		Card     string `json:"card"`
		Role     string `json:"role"`
	} `json:"sender"`
}

func obValue(b json.RawMessage) string { var v obID; _ = json.Unmarshal(b, &v); return string(v) }
func (c *oneBotConnection) call(ctx context.Context, action string, params any) (json.RawMessage, error) {
	id := randomID()
	ch := make(chan oneBotAck, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	c.write.Lock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.ws.WriteJSON(map[string]any{"action": action, "params": params, "echo": id})
	c.write.Unlock()
	if err != nil {
		return nil, err
	}
	t := time.NewTimer(15 * time.Second)
	defer t.Stop()
	select {
	case a := <-ch:
		if a.Status != "ok" || a.Retcode != 0 {
			if a.Status == "failed" && a.Retcode != 0 {
				return nil, &oneBotRejected{Action: action, Retcode: a.Retcode}
			}
			return nil, fmt.Errorf("OneBot %s acknowledgement invalid; delivery unconfirmed", action)
		}
		return a.Data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.C:
		return nil, errors.New("OneBot response timed out; delivery unconfirmed")
	}
}
func (s *Service) oneBotCall(ctx context.Context, c Config, action string, params any) (json.RawMessage, error) {
	s.onebot.mu.Lock()
	conn := s.onebot.conn
	s.onebot.mu.Unlock()
	if conn == nil || conn.bot != c.AppID || conn.credential != codeHash(c.AppSecret) {
		return nil, errors.New("NapCat 未连接或凭据已变更")
	}
	return conn.call(ctx, action, params)
}
func (s *Service) closeOneBot() {
	s.onebot.mu.Lock()
	s.onebot.closed = true
	if c := s.onebot.conn; c != nil {
		c.cancel()
		_ = c.ws.Close()
	}
	s.onebot.mu.Unlock()
	s.onebot.wg.Wait()
}
func (s *Service) ConnectionStatus() map[string]any {
	s.onebot.mu.Lock()
	defer s.onebot.mu.Unlock()
	var since *time.Time
	if s.onebot.conn != nil {
		v := s.onebot.conn.since
		since = &v
	}
	return map[string]any{"connected": since != nil, "connected_since": since, "transport": "napcat_onebot_v11"}
}
func (s *Service) ServeOneBot(w http.ResponseWriter, r *http.Request) {
	qctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	cfg, err := s.Config(qctx)
	cancel()
	if err != nil || !cfg.Enabled {
		http.Error(w, "bot unavailable", 503)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if cfg.AppSecret == "" || subtle.ConstantTimeCompare([]byte(token), []byte(cfg.AppSecret)) != 1 || r.Header.Get("X-Self-ID") != cfg.AppID || !strings.EqualFold(r.Header.Get("X-Client-Role"), "Universal") {
		http.Error(w, "unauthorized", 401)
		return
	}
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }, HandshakeTimeout: 5 * time.Second}
	ws, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, since: time.Now(), bot: cfg.AppID, credential: codeHash(cfg.AppSecret), cancel: cancel}
	s.onebot.mu.Lock()
	if s.onebot.closed || s.onebot.conn != nil {
		s.onebot.mu.Unlock()
		cancel()
		_ = ws.Close()
		return
	}
	s.onebot.conn = conn
	s.onebot.wg.Add(1)
	s.onebot.mu.Unlock()
	var workers sync.WaitGroup
	defer func() {
		cancel()
		_ = ws.Close()
		workers.Wait()
		s.onebot.mu.Lock()
		if s.onebot.conn == conn {
			s.onebot.conn = nil
		}
		s.onebot.mu.Unlock()
		s.onebot.wg.Done()
	}()
	ws.SetReadLimit(256 << 10)
	_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	slots := make(chan struct{}, 8)
	workers.Add(1)
	go func() {
		defer workers.Done()
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				q, c := context.WithTimeout(ctx, 5*time.Second)
				current, e := s.Config(q)
				c()
				if e != nil || !current.Enabled || current.AppID != conn.bot || codeHash(current.AppSecret) != conn.credential {
					cancel()
					_ = ws.Close()
					return
				}
				conn.write.Lock()
				_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				conn.write.Unlock()
			}
		}
	}()
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		var ack oneBotAck
		if json.Unmarshal(raw, &ack) == nil && ack.Echo != "" {
			conn.mu.Lock()
			ch := conn.pending[ack.Echo]
			conn.mu.Unlock()
			if ch != nil {
				select {
				case ch <- ack:
				default:
				}
			}
			continue
		}
		var e obEvent
		if json.Unmarshal(raw, &e) != nil || string(e.SelfID) != cfg.AppID || e.UserID == e.SelfID {
			continue
		}
		if e.PostType != "message" && e.PostType != "notice" {
			continue
		}
		select {
		case slots <- struct{}{}:
			workers.Add(1)
			go func(e obEvent) {
				defer workers.Done()
				defer func() { <-slots }()
				q, c := context.WithTimeout(ctx, 20*time.Second)
				defer c()
				if err := s.enqueueOneBot(q, e); err != nil {
					slog.Warn("QQ event enqueue failed", "error", err)
				}
			}(e)
		default:
			slog.Warn("QQ event intake full")
		}
	}
}
func (s *Service) enqueueOneBot(ctx context.Context, in obEvent) error {
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !c.Enabled || c.AppID != string(in.SelfID) || !c.allows(string(in.GroupID)) {
		return nil
	}
	e := Event{ID: string(in.MessageID), GroupID: string(in.GroupID), MemberID: string(in.UserID)}
	at := time.Unix(in.Time, 0)
	if in.Time == 0 {
		at = time.Now()
	}
	e.Timestamp, _ = json.Marshal(at.UTC().Format(time.RFC3339))
	e.Author.MemberID = string(in.UserID)
	e.Author.MemberRole = in.Sender.Role
	e.Author.Username = in.Sender.Card
	if e.Author.Username == "" {
		e.Author.Username = in.Sender.Nickname
	}
	kind := groupMessageCreate
	if in.PostType == "message" {
		if in.MessageType != "group" || in.MessageID == "" {
			return nil
		}
		var text strings.Builder
		for _, seg := range in.Message {
			switch seg.Type {
			case "text":
				text.WriteString(obValue(seg.Data["text"]))
			case "at":
				id := obValue(seg.Data["qq"])
				e.Mentions = append(e.Mentions, Mention{ID: id, MemberID: id})
				if id == c.AppID {
					kind = groupAtMessageCreate
				}
			case "image":
				u := obValue(seg.Data["url"])
				if u != "" {
					e.Attachments = append(e.Attachments, Attachment{URL: u, ContentType: "image/jpeg"})
				}
			case "reply":
				e.MessageReference.MessageID = obValue(seg.Data["id"])
			}
		}
		e.Content = strings.TrimSpace(text.String())
		cmd, arg := command(e.Content, c.CustomCommands...)
		// Retain the existing /聊天 and /画图 aliases from the initial release.
		if strings.HasPrefix(e.Content, "/聊天 ") || strings.HasPrefix(e.Content, "聊天 ") {
			e.CommandReply = true
			e.Content = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(e.Content, "/"), "聊天"))
			kind = groupAtMessageCreate
		}
		if strings.HasPrefix(e.Content, "/画图 ") {
			e.Content = "/生图 " + strings.TrimSpace(strings.TrimPrefix(e.Content, "/画图"))
		}
		if cmd == "" && arg == "" && e.Content == "" && len(e.Attachments) == 0 {
			return nil
		}
	} else {
		e.Timestamp, _ = json.Marshal(at.Unix())
		e.ID = fmt.Sprintf("notice:%s:%s:%s:%d", in.NoticeType, in.GroupID, in.UserID, in.Time)
		switch in.NoticeType {
		case "group_increase":
			if in.SubType == "invite" && in.OperatorID != "" && in.OperatorID != in.UserID {
				invite := e
				invite.ApplySource = "invited"
				invite.InvitedBy = string(in.OperatorID)
				invite.ApplyAt = at.UTC().Format(time.RFC3339)
				raw, _ := json.Marshal(invite)
				if err = s.Enqueue(ctx, c, Payload{ID: e.ID + ":invite", Type: "GROUP_JOIN_REQUEST", Data: raw}); err != nil {
					return err
				}
			}
			kind = "GROUP_MEMBER_ADD"
		case "group_decrease":
			kind = "GROUP_MEMBER_REMOVE"
		default:
			return nil
		}
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.Enqueue(ctx, c, Payload{ID: e.ID, Type: kind, Data: raw})
}
func (s *Service) sendOneBot(ctx context.Context, c Config, j Job, text string, img []byte) ([]byte, error) {
	chunks := []string{""}
	if text != "" {
		chunks = splitReply(text)
	}
	var receipt []byte
	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part := j
		if i > 0 {
			part.SuppressMention = true
			part.ReplyReference = ""
		}
		var picture []byte
		if i == len(chunks)-1 {
			picture = img
		}
		var err error
		receipt, err = s.sendOneBotChunk(ctx, c, part, chunk, picture)
		if err != nil {
			// Earlier parts may already be visible. Never resend them or mark a
			// partially delivered reply as successful.
			return nil, err
		}
	}
	return receipt, nil
}

func (s *Service) sendOneBotChunk(ctx context.Context, c Config, j Job, text string, img []byte) ([]byte, error) {
	segments := []map[string]any{}
	if j.ReplyReference != "" {
		segments = append(segments, map[string]any{"type": "reply", "data": map[string]string{"id": j.ReplyReference}})
	}
	if j.MemberID != "" && !j.SuppressMention {
		segments = append(segments, map[string]any{"type": "at", "data": map[string]string{"qq": j.MemberID}})
	}
	if text != "" {
		segments = append(segments, map[string]any{"type": "text", "data": map[string]string{"text": text}})
	}
	if len(img) > 0 {
		segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": "base64://" + base64.StdEncoding.EncodeToString(img)}})
	}
	raw, err := s.oneBotCall(ctx, c, "send_group_msg", map[string]any{"group_id": j.GroupID, "message": segments})
	if err != nil {
		return nil, err
	}
	var ack struct {
		ID obID `json:"message_id"`
	}
	if json.Unmarshal(raw, &ack) != nil || ack.ID == "" {
		return nil, errors.New("OneBot send acknowledged without message_id")
	}
	return json.Marshal(map[string]string{"id": string(ack.ID)})
}

// Translate only QQ operations actually used by the upstream business engine.
func (s *Service) oneBotRequest(ctx context.Context, c Config, method, path string, body any) ([]byte, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "v2" || parts[1] != "groups" {
		return nil, errors.New("unsupported QQ operation")
	}
	group, _ := url.PathUnescape(parts[2])
	if !c.allows(group) {
		return nil, ErrDisabled
	}
	switch parts[3] {
	case "bot_state":
		return json.Marshal(map[string]string{"member_openid": c.AppID})
	case "info":
		raw, err := s.oneBotCall(ctx, c, "get_group_info", map[string]any{"group_id": group})
		if err != nil {
			return nil, err
		}
		var v struct {
			Name string `json:"group_name"`
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"group_openid": group, "group_name": v.Name})
	case "restrict_chat_setting":
		b, _ := json.Marshal(body)
		var v struct {
			Members []struct {
				ID    string `json:"member_openid"`
				Until string `json:"mute_expire_at"`
			} `json:"members"`
		}
		if json.Unmarshal(b, &v) != nil || len(v.Members) != 1 {
			return nil, errors.New("invalid mute operation")
		}
		until, err := time.Parse(time.RFC3339, v.Members[0].Until)
		if err != nil {
			return nil, err
		}
		secs := int(time.Until(until).Seconds())
		if secs < 1 || secs > 30*86400 {
			return nil, errors.New("invalid mute duration")
		}
		_, err = s.oneBotCall(ctx, c, "set_group_ban", map[string]any{"group_id": group, "user_id": v.Members[0].ID, "duration": secs})
		if err != nil {
			return nil, err
		}
		return []byte(`{}`), nil
	case "messages":
		if method == http.MethodDelete && len(parts) == 5 {
			id, _ := url.PathUnescape(parts[4])
			_, err := s.oneBotCall(ctx, c, "delete_msg", map[string]string{"message_id": id})
			if err != nil {
				return nil, err
			}
			return []byte(`{}`), nil
		}
		if method == http.MethodPost {
			b, _ := json.Marshal(body)
			var v struct {
				Markdown struct {
					Content string `json:"content"`
				} `json:"markdown"`
			}
			if json.Unmarshal(b, &v) != nil {
				return nil, errors.New("invalid notification")
			}
			return s.sendOneBot(ctx, c, Job{GroupID: group, SuppressMention: true}, v.Markdown.Content, nil)
		}
	}
	return nil, errors.New("unsupported QQ operation: " + strconv.Quote(parts[3]))
}
