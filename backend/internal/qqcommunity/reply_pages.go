package qqcommunity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ReplyPage is a snapshot of safe business data, never a user session or API key.
// The browser renders the same Vue components used by the corresponding pages.
type ReplyPage struct {
	BoundUserID int64     `json:"-"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	SourcePath  string    `json:"source_path"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Data        any       `json:"data"`
}

type MonitorPageProvider interface {
	MonitorPage(context.Context, int64) (any, error)
}

type replyPageEntry struct {
	Page               ReplyPage
	App, Group, Member string
	UserID             int64
}

type replyPageStore struct {
	mu      sync.Mutex
	entries map[[32]byte]replyPageEntry
}

var ErrReplyPageExpired = errors.New("页面链接已失效，请重新发送查询指令")

func isUnbound(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func (s *Service) ConfigureReplyPages(internalURL string, provider MonitorPageProvider) error {
	u, err := url.Parse(internalURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("reply page renderer requires a fixed loopback origin")
	}
	s.pageOrigin, s.monitorPages = internalURL, provider
	s.pageGate = make(chan struct{}, 1)
	return nil
}

func (s *Service) rememberReplyPage(c Config, j Job, uid int64, page ReplyPage) (string, error) {
	if page.BoundUserID > 0 && page.BoundUserID != uid {
		return "", ErrReplyPageExpired
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret[:])
	page.CreatedAt = time.Now()
	page.ExpiresAt = page.CreatedAt.Add(10 * time.Minute)
	b, err := json.Marshal(page)
	if err != nil || len(b) > 2<<20 {
		return "", errors.New("报告内容过大，请缩小筛选范围")
	}
	s.pages.mu.Lock()
	defer s.pages.mu.Unlock()
	if s.pages.entries == nil {
		s.pages.entries = make(map[[32]byte]replyPageEntry)
	}
	for key, entry := range s.pages.entries {
		if !entry.Page.ExpiresAt.After(page.CreatedAt) {
			delete(s.pages.entries, key)
		}
	}
	if len(s.pages.entries) >= 64 {
		return "", errors.New("报告请求较多，请稍后重试")
	}
	s.pages.entries[sha256.Sum256([]byte(token))] = replyPageEntry{page, c.AppID, j.GroupID, j.MemberID, uid}
	return token, nil
}

// A fragment carries the report capability; it is absent from HTTP access logs
// and Referer. Only this POST endpoint accepts it, with a bounded request body.
func (s *Service) ReplyPage(ctx context.Context, token string) (ReplyPage, error) {
	if len(token) != 64 {
		return ReplyPage{}, ErrReplyPageExpired
	}
	if _, err := hex.DecodeString(token); err != nil {
		return ReplyPage{}, ErrReplyPageExpired
	}
	s.pages.mu.Lock()
	entry, ok := s.pages.entries[sha256.Sum256([]byte(token))]
	if ok && !entry.Page.ExpiresAt.After(time.Now()) {
		delete(s.pages.entries, sha256.Sum256([]byte(token)))
		ok = false
	}
	s.pages.mu.Unlock()
	if !ok {
		return ReplyPage{}, ErrReplyPageExpired
	}
	c, err := s.Config(ctx)
	if err != nil || !c.Enabled || c.AppID != entry.App || !c.allows(entry.Group) {
		return ReplyPage{}, ErrReplyPageExpired
	}
	if entry.UserID > 0 {
		uid, err := boundUser(ctx, s.db, entry.App, entry.Member)
		if err != nil || uid != entry.UserID {
			return ReplyPage{}, ErrReplyPageExpired
		}
	}
	return entry.Page, nil
}

func (s *Service) sendReplyPage(ctx context.Context, c Config, j Job, text string) ([]byte, bool, error) {
	uid, err := boundUser(ctx, s.db, c.AppID, j.MemberID)
	if err != nil && !isUnbound(err) {
		return nil, false, err
	}
	if j.ReplyPage.BoundUserID > 0 && uid != j.ReplyPage.BoundUserID {
		return nil, false, ErrReplyPageExpired
	}
	token, err := s.rememberReplyPage(c, j, uid, *j.ReplyPage)
	if err != nil {
		return s.sendPageFallback(ctx, c, j, text, "", "report_unavailable")
	}
	link := ""
	if s.bindingEmail != nil {
		base := strings.TrimRight(s.bindingEmail.FrontendURL(ctx), "/")
		if u, e := url.Parse(base); e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
			link = base + "/qq/report#" + token
		}
	}
	caption := "打开页面（10分钟内有效，持链接可查看本次查询）：" + link
	if link == "" {
		caption = ""
	}
	if c.CommandReplyFormat == "text" {
		raw, err := s.sendOneBot(ctx, c, j, text+"\n"+caption, nil)
		return raw, false, err
	}
	img, renderErr := s.renderReplyPage(ctx, token)
	if renderErr != nil {
		// The URL includes a capability. Never log browser errors or its URL.
		slog.WarnContext(ctx, "QQ page screenshot unavailable", "component", "qqbot.reply_page", "reason", "render_failed", "kind", j.ReplyPage.Kind, "job_id", j.ID)
		return s.sendPageFallback(ctx, c, j, text, caption, "screenshot_unavailable")
	}
	raw, err := s.sendOneBot(ctx, c, j, caption, img)
	var rejected *oneBotRejected
	if errors.As(err, &rejected) {
		return s.sendPageFallback(ctx, c, j, text, caption, "image_rejected")
	}
	// Unknown/timeout delivery is never retried, to avoid duplicate messages.
	return raw, err == nil, err
}

func (s *Service) sendPageFallback(ctx context.Context, c Config, j Job, text, link, reason string) ([]byte, bool, error) {
	slog.WarnContext(ctx, "QQ page reply fallback", "component", "qqbot.reply_page", "reason", reason, "job_id", j.ID)
	if link != "" {
		text = "截图暂不可用，请打开完整页面。\n" + link
	} else {
		text = fmt.Sprintf("%s\n页面暂不可用，请稍后重试。", text)
	}
	raw, err := s.sendOneBot(ctx, c, j, text, nil)
	return raw, false, err
}

func (s *Service) prepareMonitorPage(ctx context.Context, c Config, j Job) (*ReplyPage, error) {
	if s.monitorPages == nil {
		return nil, errors.New("监控页面暂不可用")
	}
	uid, err := boundUser(ctx, s.db, c.AppID, j.MemberID)
	if err != nil && !isUnbound(err) {
		return nil, err
	}
	data, err := s.monitorPages.MonitorPage(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &ReplyPage{BoundUserID: uid, Kind: "monitor", Title: "渠道状态", SourcePath: "/monitor", Data: data}, nil
}

func (s *Service) CaptureReplyPage(ctx context.Context, path string) ([]byte, error) {
	if !strings.HasPrefix(path, "/qq/report#") {
		return nil, ErrReplyPageExpired
	}
	token := strings.TrimPrefix(path, "/qq/report#")
	if _, err := s.ReplyPage(ctx, token); err != nil {
		return nil, err
	}
	return s.renderReplyPage(ctx, token)
}
