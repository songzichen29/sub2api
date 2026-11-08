package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func keyPoolRequest(t *testing.T) (*http.Request, *Account) {
	t.Helper()
	account := &Account{ID: 1, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "first", "api_keys": []string{"first", "second", "third"},
	}}
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", strings.NewReader(`{"input":"test"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer first")
	return WithAccountAPIKeys(req, account), account
}

func keyPoolResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestAccountAPIKeyFailoverOrderAndReturnToPrimary(t *testing.T) {
	req, account := keyPoolRequest(t)
	var calls []string
	resp, err := DoWithAccountAPIKeyFailover(req, func(attempt *http.Request) (*http.Response, error) {
		key := attempt.Header.Get("Authorization")
		calls = append(calls, key)
		body, err := io.ReadAll(attempt.Body)
		require.NoError(t, err)
		require.Equal(t, `{"input":"test"}`, string(body))
		require.NoError(t, attempt.Body.Close())
		switch key {
		case "Bearer first":
			return keyPoolResponse(401, `{"error":"invalid_api_key"}`), nil
		case "Bearer second":
			return keyPoolResponse(402, "insufficient balance"), nil
		default:
			return keyPoolResponse(200, "success"), nil
		}
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{"Bearer first", "Bearer second", "Bearer third"}, calls)
	require.Equal(t, "Bearer first", req.Header.Get("Authorization"))
	require.Equal(t, "first", account.GetCredential("api_key"))

	req, _ = keyPoolRequest(t)
	resp, err = DoWithAccountAPIKeyFailover(req, func(attempt *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer first", attempt.Header.Get("Authorization"))
		return keyPoolResponse(200, "primary recovered"), nil
	})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestAccountAPIKeyFailoverProtectsAsyncCreateAndResolvesResources(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		status       int
		body         string
		attempts     int
	}{
		{"POST", "/api/v3/contents/generations/tasks", 500, "internal error", 1},
		{"POST", "/v1/videos/generations", 503, "unavailable", 1},
		{"POST", "/v1/videos/generations", 402, "quota exhausted", 2},
		{"POST", "/v1/responses", 400, `{"code":"previous_response_not_found"}`, 2},
		{"POST", "/v1beta/models/m:generateContent", 403, "CachedContent is not found or permission denied", 2},
		{"PATCH", "/v1beta/cachedContents/resource", 404, "not found", 2},
		{"DELETE", "/v1beta/files/owned-by-second", 404, "not found", 2},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req, account := keyPoolRequest(t)
			req.Method, req.URL.Path = tc.method, tc.path
			calls := 0
			resp, err := DoWithAccountAPIKeyFailover(WithAccountAPIKeys(req, account), func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return keyPoolResponse(tc.status, tc.body), nil
				}
				return keyPoolResponse(200, "ok"), nil
			})
			require.NoError(t, err)
			require.Equal(t, tc.attempts, calls)
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestAccountAPIKeyFailoverDoesNotReplayUnsafeFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"input", 400, `{"error":"invalid_request_error"}`},
		{"model", 404, `{"error":"model_not_found"}`},
		{"permissions", 403, `{"error":"model permission denied"}`},
		{"policy", 429, `{"error":"content_policy_violation"}`},
		{"success_stream", 200, "data: success\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := keyPoolRequest(t)
			calls := 0
			resp, err := DoWithAccountAPIKeyFailover(req, func(*http.Request) (*http.Response, error) {
				calls++
				return keyPoolResponse(tc.status, tc.body), nil
			})
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(body))
			require.Equal(t, 1, calls)
			require.NoError(t, resp.Body.Close())
		})
	}
	req, _ := keyPoolRequest(t)
	calls := 0
	_, err := DoWithAccountAPIKeyFailover(req, func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("ambiguous transport failure")
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestAccountAPIKeyFailoverBoundedAndCancelled(t *testing.T) {
	req, _ := keyPoolRequest(t)
	calls := 0
	resp, err := DoWithAccountAPIKeyFailover(req, func(*http.Request) (*http.Response, error) {
		calls++
		return keyPoolResponse(429, "exhausted"), nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, 429, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	req, _ = keyPoolRequest(t)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	calls = 0
	_, err = DoWithAccountAPIKeyFailover(req.WithContext(ctx), func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return keyPoolResponse(401, "rejected"), nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}

func TestAccountAPIKeyFailoverHeaderAndQueryAuth(t *testing.T) {
	for _, header := range []string{"X-Api-Key", "X-Goog-Api-Key", "Api-Key", "query"} {
		t.Run(header, func(t *testing.T) {
			req, _ := keyPoolRequest(t)
			req.Header.Del("Authorization")
			if header == "query" {
				req.URL.RawQuery = "key=first&alt=sse"
			} else {
				req.Header.Set(header, "first")
			}
			calls := 0
			resp, err := DoWithAccountAPIKeyFailover(req, func(attempt *http.Request) (*http.Response, error) {
				calls++
				key := attempt.Header.Get(header)
				if header == "query" {
					key = attempt.URL.Query().Get("key")
					require.Equal(t, "sse", attempt.URL.Query().Get("alt"))
				}
				if calls == 1 {
					require.Equal(t, "first", key)
					return keyPoolResponse(401, "bad"), nil
				}
				require.Equal(t, "second", key)
				return keyPoolResponse(200, "ok"), nil
			})
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestAccountAPIKeyFailoverSingleAndUnreplayable(t *testing.T) {
	req, account := keyPoolRequest(t)
	delete(account.Credentials, "api_keys")
	plain, err := http.NewRequest(http.MethodGet, "https://upstream.example", nil)
	require.NoError(t, err)
	require.Same(t, plain, WithAccountAPIKeys(plain, account))
	req.GetBody = nil
	calls := 0
	resp, err := DoWithAccountAPIKeyFailover(req, func(*http.Request) (*http.Response, error) {
		calls++
		return keyPoolResponse(401, "bad"), nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.NoError(t, resp.Body.Close())
}

func TestAccountAPIKeyFailoverConcurrentSnapshot(t *testing.T) {
	_, account := keyPoolRequest(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, "https://upstream.example/v1/models", nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Authorization", "Bearer first")
			resp, err := DoWithAccountAPIKeyFailover(WithAccountAPIKeys(req, account), func(attempt *http.Request) (*http.Response, error) {
				if attempt.Header.Get("Authorization") == "Bearer first" {
					return keyPoolResponse(401, "bad"), nil
				}
				return keyPoolResponse(200, "ok"), nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			if err := resp.Body.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, "first", account.GetCredential("api_key"))
}
