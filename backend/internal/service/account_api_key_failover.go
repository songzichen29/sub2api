package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

type accountAPIKeysContextKey struct{}

// WithAccountAPIKeys opts an account-authenticated request into ordered failover.
// It does not change the account, header map, or request body shared by callers.
func WithAccountAPIKeys(req *http.Request, account *Account) *http.Request {
	if req == nil || account == nil || account.Type != AccountTypeAPIKey || account.Credentials["api_keys"] == nil {
		return req
	}
	return req.WithContext(withAccountAPIKeysContext(req.Context(), account))
}

func withAccountAPIKeysContext(ctx context.Context, account *Account) context.Context {
	if keys := account.OrderedAPIKeys(); len(keys) > 1 {
		return context.WithValue(ctx, accountAPIKeysContextKey{}, keys)
	}
	return ctx
}

// DoWithAccountAPIKeyFailover runs below response parsing/stream delivery and
// above the existing HTTP client. Successful streams and ambiguous transport
// failures are never replayed. Every independent request starts with the primary.
func DoWithAccountAPIKeyFailover(req *http.Request, send func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if req == nil {
		return send(req)
	}
	keys, _ := req.Context().Value(accountAPIKeysContextKey{}).([]string)
	if len(keys) < 2 || !requestUsesAPIKey(req, keys[0]) {
		return send(req)
	}
	// A caller streaming an unreplayable upload owns the retry decision. Reading
	// and buffering it here would remove backpressure and allow unbounded memory.
	canReplay := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	for i, key := range keys {
		attempt := req.Clone(req.Context())
		if i > 0 && req.Body != nil && req.Body != http.NoBody {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			attempt.Body = body
		}
		replaceRequestAPIKey(attempt, keys[0], key)
		resp, err := send(attempt)
		if err != nil || resp == nil || resp.StatusCode < 400 || !canReplay || i == len(keys)-1 {
			return resp, err
		}
		// Keep a bounded prefix and restore it if this failure is not eligible.
		// No bytes from a successful response are read here.
		var prefix []byte
		if resp.Body != nil {
			prefix, err = io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body = &apiKeyReplayResponseBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), Closer: resp.Body}
			if err != nil {
				return resp, nil
			}
		}
		if apiKeyAsyncCreateAmbiguous(attempt, resp.StatusCode) || (!AccountAPIKeyFailureRetryable(resp.StatusCode, prefix) && !apiKeyResourceLookupRetryable(attempt, resp.StatusCode, prefix)) {
			return resp, nil
		}
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
	}
	panic("unreachable API key failover loop")
}

type apiKeyReplayResponseBody struct {
	io.Reader
	io.Closer
}

func requestUsesAPIKey(req *http.Request, key string) bool {
	if key == "" {
		return false
	}
	for _, name := range []string{"X-Api-Key", "X-Goog-Api-Key", "Api-Key"} {
		if req.Header.Get(name) == key {
			return true
		}
	}
	auth := strings.Fields(req.Header.Get("Authorization"))
	if len(auth) == 2 && strings.EqualFold(auth[0], "Bearer") && auth[1] == key {
		return true
	}
	return req.URL != nil && req.URL.Query().Get("key") == key
}

func replaceRequestAPIKey(req *http.Request, primary, key string) {
	for _, name := range []string{"X-Api-Key", "X-Goog-Api-Key", "Api-Key"} {
		if req.Header.Get(name) == primary {
			req.Header.Set(name, key)
		}
	}
	auth := strings.Fields(req.Header.Get("Authorization"))
	if len(auth) == 2 && strings.EqualFold(auth[0], "Bearer") && auth[1] == primary {
		req.Header.Set("Authorization", auth[0]+" "+key)
	}
	if req.URL != nil && req.URL.Query().Get("key") == primary {
		query := req.URL.Query()
		query.Set("key", key)
		req.URL.RawQuery = query.Encode()
	}
}

// AccountAPIKeyFailureRetryable excludes ordinary input/model/permission errors.
// Only explicit authentication/quota failures and rejected availability responses
// fall through to another credential. A successful HTTP stream is never retried.
func AccountAPIKeyFailureRetryable(status int, body []byte) bool {
	message := strings.ToLower(string(body))
	for _, policy := range []string{"content_policy", "content_filter", "safety_policy", "moderation_blocked"} {
		if strings.Contains(message, policy) {
			return false
		}
	}
	switch status {
	case 401, 402, 429, 500, 502, 503, 504, 529:
		return true
	case 400, 403:
		for _, reason := range []string{
			"invalid_api_key", "api_key_invalid", "api key is invalid", "invalid api key",
			"api key not valid", "authentication_error", "api key has been revoked",
			"invalid x-api-key", "api_key_expired", "expired api key", "key_expired",
			"insufficient_quota", "insufficient_balance", "insufficient balance",
			"quota_exceeded", "quota exceeded", "quota exhausted", "credit balance is too low",
			"insufficient credits", "payment required", "余额不足", "额度不足", "额度已用尽", "余额已用完",
		} {
			if strings.Contains(message, reason) {
				return true
			}
		}
	}
	return false
}

// Async creates may already have accepted a billable job before returning 5xx.
// Auth/quota rejections can fail over, but an ambiguous create is never replayed.
func apiKeyAsyncCreateAmbiguous(req *http.Request, status int) bool {
	if status < 500 || req.Method != http.MethodPost || req.URL == nil {
		return false
	}
	path := strings.ToLower(strings.TrimRight(req.URL.Path, "/"))
	for _, endpoint := range []string{"/videos", "/videos/generations", "/videos/edits", "/videos/extensions", "/contents/generations/tasks", "/batches", "/cachedcontents", "/files", ":batchgeneratecontent"} {
		if strings.HasSuffix(path, endpoint) {
			return true
		}
	}
	return false
}

// Read-only resource lookups may belong to a later key. Try another credential
// on a missing resource, without classifying the earlier credential as unhealthy.
func apiKeyResourceLookupRetryable(req *http.Request, status int, body []byte) bool {
	// A resource created by a fallback key may be invisible to earlier keys.
	// Only definitive missing-resource responses are replayed, never a success.
	if status == http.StatusNotFound && (req.Method == http.MethodGet || req.Method == http.MethodPatch || req.Method == http.MethodDelete || strings.HasSuffix(req.URL.Path, ":cancel")) {
		return true
	}
	if status != http.StatusBadRequest && status != http.StatusNotFound && status != http.StatusForbidden {
		return false
	}
	message := strings.ToLower(string(body))
	return strings.Contains(message, "previous_response_not_found") ||
		(strings.Contains(message, "previous response") && strings.Contains(message, "not found")) ||
		(strings.Contains(message, "cachedcontent") && (strings.Contains(message, "not found") || strings.Contains(message, "does not exist")))
}
