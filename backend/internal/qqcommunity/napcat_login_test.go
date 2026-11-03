package qqcommunity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNapCatLoginStatesAndRefreshSafety(t *testing.T) {
	for _, tc := range []struct {
		name, phase, qq, qr       string
		online, accepted, refresh bool
		refreshCalls              int
	}{
		{name: "offline QR", qr: "https://ssl.ptlogin2.qq.com/qr/test"},
		{name: "online hides QR and never refreshes", phase: "ready", qq: "1849138657", qr: "https://qq.com/old", online: true, refresh: true},
		{name: "accepted scan never refreshes", phase: "logging_in", accepted: true, refresh: true, qr: "https://qq.com/old"},
		{name: "initializing never refreshes", phase: "initializing", refresh: true, qr: "https://qq.com/old"},
		{name: "reconnecting never refreshes", phase: "reconnecting", refresh: true, qr: "https://qq.com/old"},
		{name: "account mismatch", phase: "ready", qq: "222222", online: true},
		{name: "unsafe QR rejected", qr: "https://qq.com.evil.test/secret"},
		{name: "refresh gets new QR", refresh: true, refreshCalls: 1, qr: "https://qq.com/new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auths, refreshes := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				var data any
				switch r.URL.Path {
				case "/api/auth/login":
					auths++
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					hash := sha256.Sum256([]byte("test-token.napcat"))
					require.Equal(t, hex.EncodeToString(hash[:]), body["hash"])
					data = map[string]any{"Credential": "test-credential"}
				case "/api/QQLogin/CheckLoginStatus":
					data = map[string]any{"isLogin": tc.online, "loginPhase": tc.phase, "qrLoginAccepted": tc.accepted, "qrcodeurl": tc.qr}
				case "/api/QQLogin/GetQQLoginInfo":
					data = map[string]any{"uin": tc.qq}
				case "/api/QQLogin/RefreshQRcode":
					refreshes++
					data = map[string]any{"qrcodeurl": tc.qr}
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				if r.URL.Path != "/api/auth/login" {
					require.Equal(t, "Bearer test-credential", r.Header.Get("Authorization"))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer srv.Close()
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			require.NoError(t, s.ConfigureNapCatLogin(srv.URL, "test-token"))
			for i := 0; i < 2; i++ {
				mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"app_id":"1849138657"}`))
				out, err := s.NapCatLogin(context.Background(), tc.refresh && i == 0)
				require.NoError(t, err)
				require.True(t, out.Configured)
				require.Equal(t, tc.online, out.Online)
				require.Equal(t, tc.online && tc.qq != "1849138657", out.Mismatch)
				if tc.online || tc.accepted || tc.phase == "initializing" || tc.phase == "reconnecting" || strings.Contains(tc.name, "unsafe") {
					require.Empty(t, out.QRCode)
				} else {
					require.Equal(t, tc.qr, out.QRCode)
				}
				raw, _ := json.Marshal(out)
				require.NotContains(t, string(raw), "test-token")
				require.NotContains(t, string(raw), "test-credential")
			}
			require.Equal(t, 1, auths)
			require.Equal(t, tc.refreshCalls, refreshes)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAutomaticLoginQRRecoveryProtectsActiveFlowsAndExpiresIdleCodes(t *testing.T) {
	now := time.Now()
	n := &napcatLoginClient{}
	offline := NapCatLoginStatus{Configured: true, QRCode: "https://qq.com/old"}
	require.False(t, n.shouldRefreshQR(offline, now), "an existing QR can be scanned before expiry")
	require.False(t, n.shouldRefreshQR(offline, now.Add(100*time.Second)))
	require.True(t, n.shouldRefreshQR(offline, now.Add(111*time.Second)))
	n.lastRefresh = now.Add(111 * time.Second)
	require.False(t, n.shouldRefreshQR(offline, now.Add(120*time.Second)), "do not repeatedly refresh the same QR")
	for _, status := range []NapCatLoginStatus{
		{Online: true, Connected: false},
		{ScanAccepted: true},
		{Phase: "initializing"},
		{Phase: "reconnecting"},
	} {
		require.False(t, n.shouldRefreshQR(status, now.Add(300*time.Second)), "never log out or disrupt a pending login")
	}
	n = &napcatLoginClient{}
	require.False(t, n.shouldRefreshQR(NapCatLoginStatus{Online: true, Connected: true}, now))
	require.True(t, n.shouldRefreshQR(offline, now.Add(time.Second)), "replace a QR left over from before disconnect")
	n = &napcatLoginClient{lastRefresh: now}
	require.False(t, n.shouldRefreshQR(NapCatLoginStatus{}, now.Add(15*time.Second)))
	require.True(t, n.shouldRefreshQR(NapCatLoginStatus{}, now.Add(31*time.Second)), "retry a missing QR with backoff")
}

func TestLoginRecoveryRequestsQRWithoutBrowserAndRechecksScan(t *testing.T) {
	for _, scanRaces := range []bool{false, true} {
		t.Run(boolText(scanRaces), func(t *testing.T) {
			checks, refreshes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch r.URL.Path {
				case "/api/auth/login":
					data = map[string]any{"Credential": "test-credential"}
				case "/api/QQLogin/CheckLoginStatus":
					checks++
					data = map[string]any{"isLogin": false, "qrLoginAccepted": scanRaces && checks == 2}
				case "/api/QQLogin/RefreshQRcode":
					refreshes++
					data = map[string]any{"qrcodeurl": "https://qq.com/new"}
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			require.NoError(t, s.ConfigureNapCatLogin(server.URL, "test-token"))
			for i := 0; i < 2; i++ {
				mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"enabled":true,"app_id":"1849138657"}`))
			}
			s.recoverNapCatLogin(context.Background())
			require.Equal(t, 2, checks)
			expected := 1
			if scanRaces {
				expected = 0
			}
			require.Equal(t, expected, refreshes)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestNapCatRejectsInvalidEnvelopeAndKeepsErrorsPrivate(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":{}}`, `{"code":403,"message":"secret-token internal trace"}`, `not-json`, strings.Repeat("x", (128<<10)+2)} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		s := New(nil, nil, false, nil)
		require.NoError(t, s.ConfigureNapCatLogin(srv.URL, "test-token"))
		err := s.napcatLogin.authenticate(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-token")
		require.NotContains(t, err.Error(), "internal trace")
		srv.Close()
	}
	for _, raw := range []string{"http://qq.com/qr", "https://qq.com@evil.test/qr", "https://qq.com.evil.test/qr", "https://qq.com:443/qr", "javascript:alert(1)"} {
		require.Empty(t, safeLoginQRCode(raw))
	}
}
