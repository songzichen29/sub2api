package service

import (
	"context"
	"errors"
	"net/http"
)

// Only a rejected handshake can move to another key. Once connected, the
// connection owns its credential for its entire session/continuation lifetime.
func dialOpenAIWSWithAccountKeys(ctx context.Context, account *Account, dialer openAIWSClientDialer, target string, headers http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	keys := account.OrderedAPIKeys()
	req, parseErr := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if parseErr != nil || len(keys) < 2 {
		return dialer.Dial(ctx, target, headers, proxy)
	}
	req.Header = headers.Clone()
	if !requestUsesAPIKey(req, keys[0]) {
		return dialer.Dial(ctx, target, headers, proxy)
	}
	for index, key := range keys {
		attempt := req.Clone(ctx)
		replaceRequestAPIKey(attempt, keys[0], key)
		conn, status, responseHeaders, err := dialer.Dial(ctx, attempt.URL.String(), attempt.Header, proxy)
		var handshakeErr *openAIWSHandshakeError
		var body []byte
		if errors.As(err, &handshakeErr) {
			body = handshakeErr.Body
		}
		if err == nil || conn != nil || index == len(keys)-1 || !AccountAPIKeyFailureRetryable(status, body) || ctx.Err() != nil {
			return conn, status, responseHeaders, err
		}
	}
	panic("unreachable API key websocket loop")
}
