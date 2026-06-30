package service

import (
	"context"
	"time"
)

// AnthropicProxyCache caches responses from proxied Anthropic endpoints that
// real Claude Code fetches alongside /v1/messages (GrowthBook feature flags,
// bootstrap config). Cached per account so each OAuth account's org-scoped
// response is served from cache after the first fetch.
//
// Implementations should treat a cache miss as ("" , nil) — not an error — so
// callers can distinguish miss from failure with a simple empty-string check.
type AnthropicProxyCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}
