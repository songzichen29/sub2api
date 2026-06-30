package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
)

// H2RoundTripper routes HTTPS requests over HTTP/2 when the upstream negotiates
// h2 via ALPN, falling back to the provided HTTP/1.1 round tripper otherwise.
//
// Both paths use the same utls ClientHello fingerprint (Node.js 24.x); only the
// advertised ALPN differs:
//   - h2 path (h2Dial):      ["h2", "http/1.1"]
//   - h1 fallback (wrapped): ["http/1.1"]
//
// For hosts that support h2 (e.g. api.anthropic.com) every request takes a
// single dial and runs over h2, which is what real Bun-based Claude Code does.
// Hosts that fail h2 negotiation are remembered so we don't pay the double-dial
// cost on every subsequent request.
type H2RoundTripper struct {
	h1 http.RoundTripper
	h2 *http2.Transport

	mu       sync.Mutex
	h2Failed map[string]bool
}

// NewH2RoundTripper wraps h1 (an HTTP/1.1 transport already configured with a
// utls dialer advertising ["http/1.1"]) and uses h2Dial to establish HTTP/2
// connections. h2Dial must return a utls-handshaked connection that advertised
// ["h2","http/1.1"] as ALPN.
func NewH2RoundTripper(h1 http.RoundTripper, h2Dial func(ctx context.Context, network, addr string) (net.Conn, error)) *H2RoundTripper {
	rt := &H2RoundTripper{h1: h1, h2Failed: make(map[string]bool)}
	rt.h2 = &http2.Transport{
		AllowHTTP: false,
		// http2.Transport hands the returned conn to NewClientConn (HTTP/2
		// framing). cfg is ignored — the utls handshake derives ServerName from
		// addr and uses the fingerprint's own ClientHello.
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return h2Dial(context.Background(), network, addr)
		},
	}
	return rt
}

// RoundTrip routes the request over h2 when possible, otherwise h1.
func (rt *H2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// HTTP/2 transport is HTTPS-only; route plain HTTP through h1.
	if req.URL.Scheme != "https" {
		return rt.h1.RoundTrip(req)
	}
	host := req.URL.Host
	if rt.h2FailedHost(host) {
		return rt.h1.RoundTrip(req)
	}
	resp, err := rt.h2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}
	// h2 was not negotiated (or the h2 session failed) — remember it for this
	// host and fall back to HTTP/1.1.
	rt.markH2Failed(host)
	return rt.h1.RoundTrip(req)
}

// CloseIdleConnections delegates to both transports so http.Client eviction
// (entry.client.CloseIdleConnections) reaches the pooled h1/h2 connections.
func (rt *H2RoundTripper) CloseIdleConnections() {
	type closer interface{ CloseIdleConnections() }
	if c, ok := rt.h1.(closer); ok {
		c.CloseIdleConnections()
	}
	rt.h2.CloseIdleConnections()
}

func (rt *H2RoundTripper) h2FailedHost(host string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.h2Failed[host]
}

func (rt *H2RoundTripper) markH2Failed(host string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.h2Failed[host] = true
}
