package repository

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteOrderedH1RequestMatchesClaudeCodeOrder(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", strings.NewReader(`{"model":"x"}`))
	require.NoError(t, err)
	req.Header["anthropic-version"] = []string{"2023-06-01"}
	req.Header["X-Stainless-Runtime"] = []string{"node"}
	req.Header["Content-Type"] = []string{"application/json"}
	req.Header["User-Agent"] = []string{"claude-cli/2.1.197 (external, cli)"}
	req.Header["X-Stainless-Package-Version"] = []string{"0.94.0"}
	req.Header["anthropic-beta"] = []string{"claude-code-20250219,oauth-2025-04-20"}
	req.Header["X-Stainless-Arch"] = []string{"x64"}
	req.Header["X-Stainless-OS"] = []string{"Linux"}
	req.Header["X-Stainless-Timeout"] = []string{"600"}
	req.Header["Authorization"] = []string{"Bearer token"}
	req.Header["Accept-Encoding"] = []string{claudeCodeAcceptEncoding}
	req.Header["Accept"] = []string{"application/json"}
	req.Header["X-Stainless-Lang"] = []string{"js"}
	req.Header["X-Stainless-Retry-Count"] = []string{"0"}
	req.Header["X-Stainless-Runtime-Version"] = []string{"v26.3.0"}
	req.Header["anthropic-dangerous-direct-browser-access"] = []string{"true"}
	req.Header["x-app"] = []string{"cli"}
	req.Header["X-Claude-Code-Session-Id"] = []string{"sess"}

	var buf bytes.Buffer
	require.NoError(t, writeOrderedH1Request(&buf, req, []byte(`{"model":"x"}`)))
	lines := strings.Split(strings.Split(buf.String(), "\r\n\r\n")[0], "\r\n")
	require.Equal(t, "POST /v1/messages?beta=true HTTP/1.1", lines[0])

	var got []string
	for _, line := range lines[1:] {
		got = append(got, strings.SplitN(line, ":", 2)[0])
	}
	require.Equal(t, []string{
		"Accept",
		"Authorization",
		"Content-Type",
		"User-Agent",
		"X-Claude-Code-Session-Id",
		"X-Stainless-Arch",
		"X-Stainless-Lang",
		"X-Stainless-OS",
		"X-Stainless-Package-Version",
		"X-Stainless-Retry-Count",
		"X-Stainless-Runtime",
		"X-Stainless-Runtime-Version",
		"X-Stainless-Timeout",
		"anthropic-beta",
		"anthropic-dangerous-direct-browser-access",
		"anthropic-version",
		"x-app",
		"Connection",
		"Host",
		"Accept-Encoding",
		"Content-Length",
	}, got)
	require.Contains(t, buf.String(), "\r\nAccept-Encoding: gzip, deflate, br, zstd\r\n")
}

func TestOrderedH1RoundTripperReusesIdleConnectionAfterBodyEOF(t *testing.T) {
	addr, closeServer, _ := startRawH1TestServer(t, func(_ int64, conn net.Conn, _ *http.Request) bool {
		writeRawH1Response(t, conn, "ok")
		return true
	})
	defer closeServer()

	var dialCount atomic.Int64
	rt := newOrderedH1RoundTripper(testDialer(addr, &dialCount), time.Second, time.Minute, 4)
	url := "https://" + addr + "/v1/messages"

	for i := 0; i < 3; i++ {
		require.Equal(t, "ok", roundTripBody(t, rt, url))
	}
	require.Equal(t, int64(1), dialCount.Load(), "fully-read responses should return the connection to the idle pool")
}

func TestOrderedH1RoundTripperDoesNotReuseConnectionUntilStreamingBodyClosed(t *testing.T) {
	addr, closeServer, _ := startRawH1TestServer(t, func(_ int64, conn net.Conn, _ *http.Request) bool {
		writeRawH1Response(t, conn, "stream-body")
		return true
	})
	defer closeServer()

	var dialCount atomic.Int64
	rt := newOrderedH1RoundTripper(testDialer(addr, &dialCount), time.Second, time.Minute, 4)
	url := "https://" + addr + "/v1/messages"

	firstReq, err := http.NewRequest(http.MethodGet, url+"?stream=1", nil)
	require.NoError(t, err)
	firstResp, err := rt.RoundTrip(firstReq)
	require.NoError(t, err)
	defer func() { _ = firstResp.Body.Close() }()

	// The first response body is still checked out, so a concurrent request must
	// use a different connection instead of reusing the streaming connection.
	require.Equal(t, "stream-body", roundTripBody(t, rt, url+"?second=1"))
	require.Equal(t, int64(2), dialCount.Load())

	// Closing without reading to EOF must close (not pool) the first connection.
	require.NoError(t, firstResp.Body.Close())
	require.Equal(t, "stream-body", roundTripBody(t, rt, url+"?third=1"))
	require.Equal(t, int64(2), dialCount.Load(), "only the fully-read second response should have been pooled")
}

func TestOrderedH1RoundTripperRedialsStaleIdleConnection(t *testing.T) {
	firstServed := make(chan struct{})
	closeFirst := make(chan struct{})
	firstClosed := make(chan struct{})

	addr, closeServer, _ := startRawH1TestServer(t, func(connID int64, conn net.Conn, _ *http.Request) bool {
		writeRawH1Response(t, conn, "ok")
		if connID == 1 {
			close(firstServed)
			<-closeFirst
			_ = conn.Close()
			close(firstClosed)
			return false
		}
		return true
	})
	defer closeServer()

	var dialCount atomic.Int64
	rt := newOrderedH1RoundTripper(testDialer(addr, &dialCount), time.Second, time.Minute, 4)
	url := "https://" + addr + "/v1/messages"

	require.Equal(t, "ok", roundTripBody(t, rt, url))
	<-firstServed
	close(closeFirst)
	<-firstClosed

	require.Equal(t, "ok", roundTripBody(t, rt, url))
	require.Equal(t, int64(2), dialCount.Load(), "stale pooled connection should be discarded and redialed")
}

func TestOrderedH1RoundTripperCloseIdleConnectionsClearsPool(t *testing.T) {
	addr, closeServer, _ := startRawH1TestServer(t, func(_ int64, conn net.Conn, _ *http.Request) bool {
		writeRawH1Response(t, conn, "ok")
		return true
	})
	defer closeServer()

	var dialCount atomic.Int64
	rt := newOrderedH1RoundTripper(testDialer(addr, &dialCount), time.Second, time.Minute, 4)
	url := "https://" + addr + "/v1/messages"

	require.Equal(t, "ok", roundTripBody(t, rt, url))
	require.Equal(t, int64(1), dialCount.Load())

	rt.CloseIdleConnections()

	require.Equal(t, "ok", roundTripBody(t, rt, url))
	require.Equal(t, int64(2), dialCount.Load())
}

func testDialer(addr string, dialCount *atomic.Int64) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialCount.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
}

func roundTripBody(t *testing.T, rt *orderedH1RoundTripper, url string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"model":"x"}`))
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return string(body)
}

func writeRawH1Response(t *testing.T, conn net.Conn, body string) {
	t.Helper()
	_, err := fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n%s", len(body), body)
	require.NoError(t, err)
}

func startRawH1TestServer(t *testing.T, handle func(connID int64, conn net.Conn, req *http.Request) bool) (string, func(), *atomic.Int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var connCount atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connID := connCount.Add(1)
			go func() {
				defer func() { _ = conn.Close() }()
				br := bufio.NewReader(conn)
				for {
					req, err := http.ReadRequest(br)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					_ = req.Body.Close()
					if !handle(connID, conn, req) {
						return
					}
				}
			}()
		}
	}()

	closeFn := func() {
		_ = ln.Close()
		<-done
	}
	return ln.Addr().String(), closeFn, &connCount
}
