package repository

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

const (
	claudeCodeAcceptEncoding        = "gzip, deflate, br, zstd"
	defaultOrderedH1IdleConnTimeout = 90 * time.Second
	defaultOrderedH1MaxIdlePerHost  = 120
)

// orderedH1RoundTripper sends Anthropic HTTPS requests over a uTLS connection
// using hand-written HTTP/1.1 bytes. The standard net/http transport sorts
// headers canonically and writes Host first; real Claude Code v2.1.197 preserves
// case-sensitive application header order and writes transport headers after
// those application headers.
type orderedH1RoundTripper struct {
	dialTLSContext        func(ctx context.Context, network, addr string) (net.Conn, error)
	responseHeaderTimeout time.Duration
	idleConnTimeout       time.Duration
	maxIdleConnsPerHost   int

	mu   sync.Mutex
	idle map[string][]*pooledH1Conn
}

func newOrderedH1RoundTripper(
	dialTLSContext func(ctx context.Context, network, addr string) (net.Conn, error),
	responseHeaderTimeout time.Duration,
	idleConnTimeout time.Duration,
	maxIdleConnsPerHost int,
) *orderedH1RoundTripper {
	if idleConnTimeout <= 0 {
		idleConnTimeout = defaultOrderedH1IdleConnTimeout
	}
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = defaultOrderedH1MaxIdlePerHost
	}
	return &orderedH1RoundTripper{
		dialTLSContext:        dialTLSContext,
		responseHeaderTimeout: responseHeaderTimeout,
		idleConnTimeout:       idleConnTimeout,
		maxIdleConnsPerHost:   maxIdleConnsPerHost,
		idle:                  make(map[string][]*pooledH1Conn),
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

	var writeBuf bytes.Buffer
	if err := writeOrderedH1Request(&writeBuf, req, body); err != nil {
		return nil, err
	}
	wireRequest := writeBuf.Bytes()

	addr := canonicalAddr(req.URL)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		pc, reused, err := rt.getConn(req.Context(), addr)
		if err != nil {
			return nil, err
		}

		resp, canRetry, err := rt.roundTripOnConn(req, pc, wireRequest, reused)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		pc.close()

		if ctxErr := req.Context().Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if attempt == 0 && canRetry {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func (rt *orderedH1RoundTripper) roundTripOnConn(req *http.Request, pc *pooledH1Conn, wireRequest []byte, reused bool) (*http.Response, bool, error) {
	stopWatchingContext := watchRequestContext(req.Context(), pc)

	written, err := writeFull(pc.conn, wireRequest)
	if err != nil {
		stopWatchingContext()
		return nil, reused && written == 0 && isRetryableStaleConnError(err), err
	}

	if rt.responseHeaderTimeout > 0 {
		_ = pc.conn.SetReadDeadline(time.Now().Add(rt.responseHeaderTimeout))
	}
	resp, err := http.ReadResponse(pc.br, req)
	if rt.responseHeaderTimeout > 0 {
		_ = pc.conn.SetReadDeadline(time.Time{})
	}
	if err != nil {
		stopWatchingContext()
		return nil, reused && isRetryableStaleConnError(err), err
	}

	mayReuse := !req.Close && !resp.Close
	fullyRead := responseBodyAlreadyConsumed(req, resp)
	resp.Body = &pooledH1ResponseBody{
		ReadCloser: resp.Body,
		rt:         rt,
		pc:         pc,
		stopCtx:    stopWatchingContext,
		mayReuse:   mayReuse,
		fullyRead:  fullyRead,
	}
	return resp, false, nil
}

func (rt *orderedH1RoundTripper) getConn(ctx context.Context, addr string) (*pooledH1Conn, bool, error) {
	for {
		pc := rt.popIdleConn(addr)
		if pc == nil {
			conn, err := rt.dialTLSContext(ctx, "tcp", addr)
			if err != nil {
				return nil, false, err
			}
			return &pooledH1Conn{addr: addr, conn: conn, br: bufio.NewReader(conn)}, false, nil
		}
		if pc.expired(rt.idleConnTimeout) || !pc.healthy() {
			pc.close()
			continue
		}
		return pc, true, nil
	}
}

func (rt *orderedH1RoundTripper) popIdleConn(addr string) *pooledH1Conn {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	q := rt.idle[addr]
	for len(q) > 0 {
		idx := len(q) - 1
		pc := q[idx]
		q = q[:idx]
		if len(q) == 0 {
			delete(rt.idle, addr)
		} else {
			rt.idle[addr] = q
		}
		if pc != nil {
			return pc
		}
	}
	delete(rt.idle, addr)
	return nil
}

func (rt *orderedH1RoundTripper) putIdleConn(pc *pooledH1Conn) {
	if rt == nil || pc == nil || pc.conn == nil || pc.br == nil {
		return
	}
	if pc.br.Buffered() != 0 || !pc.healthy() {
		pc.close()
		return
	}

	now := time.Now()
	pc.idleAt = now
	var toClose []*pooledH1Conn

	rt.mu.Lock()
	q := rt.idle[pc.addr]
	if rt.idleConnTimeout > 0 {
		kept := q[:0]
		for _, idleConn := range q {
			if idleConn == nil || idleConn.expiredAt(now, rt.idleConnTimeout) {
				toClose = append(toClose, idleConn)
				continue
			}
			kept = append(kept, idleConn)
		}
		q = kept
	}
	if rt.maxIdleConnsPerHost > 0 && len(q) >= rt.maxIdleConnsPerHost {
		toClose = append(toClose, pc)
	} else {
		q = append(q, pc)
	}
	if len(q) == 0 {
		delete(rt.idle, pc.addr)
	} else {
		rt.idle[pc.addr] = q
	}
	rt.mu.Unlock()

	for _, idleConn := range toClose {
		if idleConn != nil {
			idleConn.close()
		}
	}
}

func (rt *orderedH1RoundTripper) CloseIdleConnections() {
	if rt == nil {
		return
	}
	var toClose []*pooledH1Conn
	rt.mu.Lock()
	for _, q := range rt.idle {
		toClose = append(toClose, q...)
	}
	rt.idle = make(map[string][]*pooledH1Conn)
	rt.mu.Unlock()

	for _, pc := range toClose {
		if pc != nil {
			pc.close()
		}
	}
}

type pooledH1Conn struct {
	addr   string
	conn   net.Conn
	br     *bufio.Reader
	idleAt time.Time

	closeOnce sync.Once
}

func (pc *pooledH1Conn) close() {
	if pc == nil || pc.conn == nil {
		return
	}
	pc.closeOnce.Do(func() {
		_ = pc.conn.Close()
	})
}

func (pc *pooledH1Conn) expired(timeout time.Duration) bool {
	if timeout <= 0 || pc == nil || pc.idleAt.IsZero() {
		return false
	}
	return time.Since(pc.idleAt) > timeout
}

func (pc *pooledH1Conn) expiredAt(now time.Time, timeout time.Duration) bool {
	if timeout <= 0 || pc == nil || pc.idleAt.IsZero() {
		return false
	}
	return now.Sub(pc.idleAt) > timeout
}

func (pc *pooledH1Conn) healthy() bool {
	if pc == nil || pc.conn == nil || pc.br == nil {
		return false
	}
	if pc.br.Buffered() != 0 {
		return false
	}
	if err := pc.conn.SetReadDeadline(time.Now()); err != nil {
		return false
	}
	_, err := pc.br.Peek(1)
	_ = pc.conn.SetReadDeadline(time.Time{})
	if err == nil {
		return false // unexpected bytes on an idle connection
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

type pooledH1ResponseBody struct {
	io.ReadCloser
	rt       *orderedH1RoundTripper
	pc       *pooledH1Conn
	stopCtx  func()
	mayReuse bool

	fullyRead bool
	closeOnce sync.Once
	closeErr  error
}

func (b *pooledH1ResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.fullyRead = true
	}
	return n, err
}

func (b *pooledH1ResponseBody) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.ReadCloser.Close()
		if b.stopCtx != nil {
			b.stopCtx()
		}
		if b.mayReuse && b.fullyRead {
			b.rt.putIdleConn(b.pc)
		} else if b.pc != nil {
			b.pc.close()
		}
	})
	return b.closeErr
}

func watchRequestContext(ctx context.Context, pc *pooledH1Conn) func() {
	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() { close(done) })
	}
	go func() {
		select {
		case <-ctx.Done():
			pc.close()
		case <-done:
		}
	}()
	return stop
}

func writeFull(w io.Writer, b []byte) (int, error) {
	written := 0
	for len(b) > 0 {
		n, err := w.Write(b)
		written += n
		b = b[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func isRetryableStaleConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection was aborted")
}

func responseBodyAlreadyConsumed(req *http.Request, resp *http.Response) bool {
	if resp == nil || resp.Body == nil || resp.Body == http.NoBody {
		return true
	}
	if req != nil && req.Method == http.MethodHead {
		return true
	}
	if (resp.StatusCode >= 100 && resp.StatusCode <= 199) || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return true
	}
	return resp.ContentLength == 0 && len(resp.TransferEncoding) == 0
}

func drainRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer func() { _ = req.Body.Close() }()
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

	for _, key := range claude.ClaudeCodeHeaderWireOrder() {
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
