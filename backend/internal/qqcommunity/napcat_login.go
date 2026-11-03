package qqcommunity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type NapCatLoginStatus struct {
	Configured   bool   `json:"configured"`
	Online       bool   `json:"online"`
	Connected    bool   `json:"connected"`
	Offline      bool   `json:"offline"`
	Phase        string `json:"phase"`
	ScanAccepted bool   `json:"scan_accepted"`
	ExpectedQQ   string `json:"expected_qq"`
	LoggedInQQ   string `json:"logged_in_qq"`
	Mismatch     bool   `json:"account_mismatch"`
	QRCode       string `json:"qr_code,omitempty"`
	LoginFailed  bool   `json:"login_failed"`
	AutoRecovery bool   `json:"auto_recovery"`
}

type napcatLoginClient struct {
	mu          sync.Mutex
	base        string
	hash        string
	http        *http.Client
	credential  string
	validUntil  time.Time
	lastRefresh time.Time
	lastQRCode  string
	qrObserved  time.Time
	wasOnline   bool
	recoverQR   bool
}

// ConfigureNapCatLogin only accepts the operator's fixed server configuration.
// Request parameters must never choose an upstream URL or supply WebUI secrets.
func (s *Service) ConfigureNapCatLogin(base, token string) error {
	if base == "" && token == "" {
		return nil
	}
	if err := validateURL(base); err != nil || strings.TrimSpace(token) == "" {
		return errors.New("NapCat WebUI 地址和凭据配置不完整")
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(token) + ".napcat"))
	s.napcatLogin = &napcatLoginClient{base: strings.TrimRight(base, "/"), hash: hex.EncodeToString(hash[:]), http: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: noRedirect}}
	return nil
}

var errNapCatAuth = errors.New("NapCat WebUI 认证失败，请检查服务器配置")
var errNapCatUnavailable = errors.New("NapCat 暂时不可用，请检查服务后重试")

func (n *napcatLoginClient) call(ctx context.Context, path string, body any, auth bool, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return errNapCatUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.base+path, bytes.NewReader(raw))
	if err != nil {
		return errNapCatUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer "+n.credential)
	}
	res, err := n.http.Do(req)
	if err != nil {
		return errNapCatUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errNapCatAuth
	}
	if res.StatusCode != http.StatusOK {
		return errNapCatUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if err != nil || len(data) > 128<<10 {
		return errNapCatUnavailable
	}
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code == nil {
		return errNapCatUnavailable
	}
	if *envelope.Code != 0 {
		// Never expose upstream messages, URLs, credential material or stack traces.
		return errNapCatAuth
	}
	if out != nil && json.Unmarshal(envelope.Data, out) != nil {
		return errNapCatUnavailable
	}
	return nil
}

func (n *napcatLoginClient) authenticate(ctx context.Context) error {
	if n.credential != "" && time.Now().Before(n.validUntil) {
		return nil
	}
	var result struct {
		Credential string `json:"Credential"`
		Require2FA bool   `json:"require2FA"`
	}
	if err := n.call(ctx, "/api/auth/login", map[string]string{"hash": n.hash}, false, &result); err != nil {
		return err
	}
	if result.Require2FA || result.Credential == "" {
		return errNapCatAuth
	}
	n.credential, n.validUntil = result.Credential, time.Now().Add(50*time.Minute)
	return nil
}

func (n *napcatLoginClient) request(ctx context.Context, path string, out any) error {
	if err := n.authenticate(ctx); err != nil {
		return err
	}
	err := n.call(ctx, path, map[string]any{}, true, out)
	if errors.Is(err, errNapCatAuth) {
		n.credential = ""
	}
	return err
}

func safeLoginQRCode(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "qq.com" && !strings.HasSuffix(u.Hostname(), ".qq.com")) || len(raw) > 4096 {
		return ""
	}
	return raw
}

func (s *Service) NapCatLogin(ctx context.Context, refresh bool) (*NapCatLoginStatus, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return nil, errors.New("读取机器人配置失败")
	}
	result := &NapCatLoginStatus{ExpectedQQ: c.AppID, Connected: s.ConnectionStatus()["connected"] == true}
	n := s.napcatLogin
	if n == nil {
		return result, nil
	}
	result.Configured = true
	result.AutoRecovery = c.Enabled
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	n.mu.Lock()
	defer n.mu.Unlock()
	var status struct {
		Online       bool   `json:"isLogin"`
		Offline      bool   `json:"isOffline"`
		Phase        string `json:"loginPhase"`
		ScanAccepted bool   `json:"qrLoginAccepted"`
		QRCode       string `json:"qrcodeurl"`
		LoginError   string `json:"loginError"`
	}
	if err := n.request(ctx, "/api/QQLogin/CheckLoginStatus", &status); err != nil {
		return nil, err
	}
	// Refreshing a QR must never log out a working bot or invalidate a scan that
	// is waiting for confirmation or core initialization.
	if refresh && !status.Online && !status.ScanAccepted && status.Phase != "reconnecting" && status.Phase != "initializing" {
		if time.Since(n.lastRefresh) < 5*time.Second {
			return nil, errors.New("刷新过于频繁，请稍后重试")
		}
		n.lastRefresh = time.Now()
		var data json.RawMessage
		if err := n.request(ctx, "/api/QQLogin/RefreshQRcode", &data); err != nil {
			return nil, err
		}
		// Older WebUI versions acknowledge refresh with null or a boolean.
		var refreshed struct {
			QRCode string `json:"qrcodeurl"`
		}
		_ = json.Unmarshal(data, &refreshed)
		status.QRCode = ""
		if refreshed.QRCode != "" {
			status.QRCode = refreshed.QRCode
		}
	}
	result.Online, result.Offline, result.Phase, result.ScanAccepted = status.Online, status.Offline, status.Phase, status.ScanAccepted
	result.LoginFailed = status.LoginError != ""
	if !status.Online && !status.ScanAccepted && status.Phase != "initializing" && status.Phase != "reconnecting" {
		result.QRCode = safeLoginQRCode(status.QRCode)
	}
	if status.Online {
		var info struct {
			QQ json.RawMessage `json:"uin"`
		}
		if err := n.request(ctx, "/api/QQLogin/GetQQLoginInfo", &info); err != nil {
			return nil, err
		}
		var qq obID
		if json.Unmarshal(info.QQ, &qq) != nil || qq == "" {
			return nil, errNapCatUnavailable
		}
		result.LoggedInQQ = string(qq)
		result.Mismatch = result.LoggedInQQ != c.AppID
	}
	return result, nil
}

// NapCat owns quick-login and reverse-WebSocket reconnects. Only request a QR
// after its login flow is idle: a broken OneBot socket alone is not a logout.
func (s *Service) startNapCatRecovery(ctx context.Context) {
	if s.napcatLogin == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			s.recoverNapCatLogin(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) recoverNapCatLogin(ctx context.Context) {
	status, err := s.NapCatLogin(ctx, false)
	if err != nil || !status.AutoRecovery {
		return
	}
	if !s.napcatLogin.shouldRefreshQR(*status, time.Now()) {
		return
	}
	// NapCatLogin re-reads the actual login status while holding the same client
	// lock, so a scan or successful login racing this check is still protected.
	if _, err := s.NapCatLogin(ctx, true); err == nil {
		s.napcatLogin.mu.Lock()
		s.napcatLogin.recoverQR = false
		s.napcatLogin.mu.Unlock()
	}
}

func (n *napcatLoginClient) shouldRefreshQR(status NapCatLoginStatus, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.wasOnline && !status.Online {
		n.recoverQR = true
	}
	n.wasOnline = status.Online
	if status.Online {
		n.recoverQR, n.lastQRCode, n.qrObserved = false, "", time.Time{}
		return false
	}
	if status.ScanAccepted || status.Phase == "initializing" || status.Phase == "reconnecting" {
		return false
	}
	if status.QRCode != n.lastQRCode || n.qrObserved.IsZero() {
		n.lastQRCode, n.qrObserved = status.QRCode, now
	}
	if now.Sub(n.lastRefresh) < 30*time.Second {
		return false
	}
	// Refresh stable, unscanned QR codes periodically instead of leaving an
	// expired code on screen indefinitely. Back off requests when no QR exists.
	return n.recoverQR || status.QRCode == "" || (now.Sub(n.qrObserved) >= 110*time.Second && now.Sub(n.lastRefresh) >= 110*time.Second)
}
