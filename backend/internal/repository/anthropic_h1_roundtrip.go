package repository

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const claudeCodeAcceptEncoding = "gzip, deflate, br, zstd"

// orderedH1RoundTripper sends Anthropic HTTPS requests over a uTLS connection
// using hand-written HTTP/1.1 bytes. The standard net/http transport sorts
// headers canonically and writes Host first; real Claude Code v2.1.197 preserves
// case-sensitive application header order and writes transport headers after
// those application headers.
type orderedH1RoundTripper struct {
	dialTLSContext        func(ctx context.Context, network, addr string) (net.Conn, error)
	responseHeaderTimeout time.Duration
}

func newOrderedH1RoundTripper(
	dialTLSContext func(ctx context.Context, network, addr string) (net.Conn, error),
	responseHeaderTimeout time.Duration,
) *orderedH1RoundTripper {
	return &orderedH1RoundTripper{
		dialTLSContext:        dialTLSContext,
		responseHeaderTimeout: responseHeaderTimeout,
	}
}

func (rt *orderedH1RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt == nil || rt.dialTLSContext == nil {
		return nil, fmt.Errorf("ordered h1 round tripper missing TLS dialer")
	}
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("request url is nil")
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("ordered h1 round tripper requires https, got %q", req.URL.Scheme)
	}

	body, err := drainRequestBody(req)
	if err != nil {
		return nil, err
	}

	addr := canonicalAddr(req.URL)
	conn, err := rt.dialTLSContext(req.Context(), "tcp", addr)
	if err != nil {
		return nil, err
	}
	var connOnce sync.Once
	closeConn := func() {
		connOnce.Do(func() {
			_ = conn.Close()
		})
	}

	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			closeConn()
		case <-ctxDone:
		}
	}()

	var writeBuf bytes.Buffer
	if err := writeOrderedH1Request(&writeBuf, req, body); err != nil {
		close(ctxDone)
		closeConn()
		return nil, err
	}
	if _, err := conn.Write(writeBuf.Bytes()); err != nil {
		close(ctxDone)
		closeConn()
		return nil, err
	}

	if rt.responseHeaderTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(rt.responseHeaderTimeout))
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if rt.responseHeaderTimeout > 0 {
		_ = conn.SetReadDeadline(time.Time{})
	}
	if err != nil {
		close(ctxDone)
		closeConn()
		return nil, err
	}
	resp.Body = &connClosingBody{
		ReadCloser: resp.Body,
		closeConn: func() {
			close(ctxDone)
			closeConn()
		},
	}
	return resp, nil
}

func (rt *orderedH1RoundTripper) CloseIdleConnections() {}

type connClosingBody struct {
	io.ReadCloser
	closeOnce sync.Once
	closeConn func()
}

func (b *connClosingBody) Close() error {
	err := b.ReadCloser.Close()
	b.closeOnce.Do(b.closeConn)
	return err
}

func drainRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func writeOrderedH1Request(w io.Writer, req *http.Request, body []byte) error {
	uri := req.URL.RequestURI()
	if uri == "" {
		uri = "/"
	}
	if _, err := fmt.Fprintf(w, "%s %s HTTP/1.1\r\n", req.Method, uri); err != nil {
		return err
	}

	written := make(map[string]struct{}, len(req.Header)+8)
	writeHeader := func(key string, values []string) error {
		if len(values) == 0 {
			return nil
		}
		lk := strings.ToLower(key)
		if _, ok := written[lk]; ok {
			return nil
		}
		written[lk] = struct{}{}
		for _, value := range values {
			if _, err := fmt.Fprintf(w, "%s: %s\r\n", key, value); err != nil {
				return err
			}
		}
		return nil
	}

	for _, key := range service.ClaudeCodeHeaderWireOrder() {
		if values := headerValues(req.Header, key); len(values) > 0 {
			if err := writeHeader(key, values); err != nil {
				return err
			}
		}
	}

	for _, key := range orderedExtraHeaderKeys(req.Header, written) {
		if err := writeHeader(key, req.Header[key]); err != nil {
			return err
		}
	}

	if err := writeHeader("Connection", []string{"keep-alive"}); err != nil {
		return err
	}
	if err := writeHeader("Host", []string{hostHeader(req)}); err != nil {
		return err
	}
	acceptEncoding := firstHeaderValue(req.Header, "Accept-Encoding")
	if acceptEncoding == "" {
		acceptEncoding = claudeCodeAcceptEncoding
	}
	if err := writeHeader("Accept-Encoding", []string{acceptEncoding}); err != nil {
		return err
	}
	if shouldWriteContentLength(req.Method, body) {
		if err := writeHeader("Content-Length", []string{fmt.Sprintf("%d", len(body))}); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}
	if len(body) > 0 {
		_, err := w.Write(body)
		return err
	}
	return nil
}

func orderedExtraHeaderKeys(h http.Header, written map[string]struct{}) []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		lk := strings.ToLower(key)
		if _, ok := written[lk]; ok {
			continue
		}
		if isTransportHeader(lk) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isTransportHeader(lowerKey string) bool {
	switch lowerKey {
	case "connection", "host", "accept-encoding", "content-length", "transfer-encoding":
		return true
	default:
		return false
	}
}

func headerValues(h http.Header, key string) []string {
	if vals := h[key]; len(vals) > 0 {
		return vals
	}
	lowerKey := strings.ToLower(key)
	for k, vals := range h {
		if strings.ToLower(k) == lowerKey {
			return vals
		}
	}
	return nil
}

func firstHeaderValue(h http.Header, key string) string {
	if vals := headerValues(h, key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func shouldWriteContentLength(method string, body []byte) bool {
	if len(body) > 0 {
		return true
	}
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func hostHeader(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	return req.URL.Host
}

func canonicalAddr(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(host, port)
}
