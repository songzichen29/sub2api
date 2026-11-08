package repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamOrderedAPIKeysRealTransport(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "tls"}[useTLS], func(t *testing.T) {
			var recovered atomic.Bool
			var primaryCalls, backupCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"input":"hello"}` {
					w.WriteHeader(400)
					return
				}
				switch r.Header.Get("Authorization") {
				case "Bearer first":
					primaryCalls.Add(1)
					if !recovered.Load() {
						w.WriteHeader(402)
						_, _ = w.Write([]byte("insufficient balance"))
						return
					}
				case "Bearer second":
					backupCalls.Add(1)
				default:
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: success\n\n"))
			}))
			defer server.Close()
			upstream := NewHTTPUpstream(nil)
			account := &service.Account{ID: 33, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "first", "api_keys": []string{"first", "second"}}}
			for i := 0; i < 2; i++ {
				req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"input":"hello"}`))
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer first")
				req = service.WithAccountAPIKeys(req, account)
				var resp *http.Response
				if useTLS {
					resp, err = upstream.DoWithTLS(req, "", account.ID, 1, &tlsfingerprint.Profile{Name: "test"})
				} else {
					resp, err = upstream.Do(req, "", account.ID, 1)
				}
				require.NoError(t, err)
				require.Equal(t, 200, resp.StatusCode)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, "data: success\n\n", string(body))
				require.NoError(t, resp.Body.Close())
				recovered.Store(true)
			}
			require.Equal(t, int32(2), primaryCalls.Load())
			require.Equal(t, int32(1), backupCalls.Load())
		})
	}
}
