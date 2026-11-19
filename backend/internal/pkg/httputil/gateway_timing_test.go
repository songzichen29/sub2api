package httputil

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestGatewayTimingCompressedRequest(t *testing.T) {
	body := []byte(`{"model":"test","input":"` + strings.Repeat("A", 1024) + `"}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	wireBytes := compressed.Len()
	req := httptest.NewRequest(http.MethodPost, "/", &compressed)
	req.Header.Set("Content-Encoding", "gzip")
	req = req.WithContext(WithGatewayTiming(req.Context()))
	got, err := ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	require.NoError(t, err)
	require.Equal(t, body, got)
	core, logs := observer.New(zap.InfoLevel)
	zap.New(core).Info("timing", GatewayTimingFields(req.Context())...)
	fields := logs.All()[0].ContextMap()
	require.EqualValues(t, wireBytes, fields["request_body_wire_bytes"])
	require.Equal(t, true, fields["request_body_read_complete"])
	require.Contains(t, fields, "request_body_read_ms")
	require.Contains(t, fields, "request_body_decode_ms")
	require.Empty(t, GatewayTimingFields(context.Background()))
}

func TestTraceGatewayUpstreamTLSAndReuse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	core, logs := observer.New(zap.InfoLevel)
	ctx := WithGatewayTiming(logger.IntoContext(context.Background(), zap.New(core)))
	ctx = context.WithValue(ctx, ctxkey.AccountID, int64(4622))
	client := server.Client()
	for attempt := 1; attempt <= 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(strings.Repeat("A", 64<<10)))
		require.NoError(t, err)
		req, finish := TraceGatewayUpstream(req)
		resp, err := client.Do(req)
		finish(resp, err)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		fields := logs.All()[attempt-1].ContextMap()
		require.EqualValues(t, attempt, fields["upstream_attempt"])
		require.EqualValues(t, 4622, fields["account_id"])
		require.EqualValues(t, 64<<10, fields["upstream_request_bytes"])
		require.Equal(t, attempt == 2, fields["upstream_conn_reused"])
		require.Equal(t, false, fields["upstream_transport_error"])
		require.Contains(t, fields, "upstream_write_ms")
		require.GreaterOrEqual(t, fields["upstream_wait_first_byte_ms"].(int64), int64(15))
		if attempt == 1 {
			require.Contains(t, fields, "upstream_tls_total_ms")
		}
		if attempt == 2 {
			require.NotContains(t, fields, "upstream_tls_total_ms")
		}
	}
}

func TestTraceGatewayUpstreamCancellation(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	ctx, cancel := context.WithCancel(WithGatewayTiming(logger.IntoContext(context.Background(), zap.New(core))))
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:1", strings.NewReader("secret"))
	require.NoError(t, err)
	req, finish := TraceGatewayUpstream(req)
	resp, err := http.DefaultClient.Do(req)
	finish(resp, err)
	require.ErrorIs(t, err, context.Canceled)
	fields := logs.All()[0].ContextMap()
	require.Equal(t, true, fields["upstream_transport_error"])
	require.NotContains(t, fields, "upstream_wait_first_byte_ms")
	require.NotContains(t, fields, "upstream_write_ms")
	require.NotContains(t, fields, "error")
}

func TestTraceGatewayUpstreamEarlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	core, logs := observer.New(zap.InfoLevel)
	ctx := WithGatewayTiming(logger.IntoContext(context.Background(), zap.New(core)))
	// 上传停在 Read，确保收到 413 时请求仍未写完；不依赖本机缓冲区速度。
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, reader)
	require.NoError(t, err)
	req.ContentLength = 1 << 20
	req, finish := TraceGatewayUpstream(req)
	resp, err := http.DefaultClient.Do(req)
	finish(resp, err)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	fields := logs.All()[0].ContextMap()
	require.EqualValues(t, 413, fields["upstream_status"])
	require.Equal(t, true, fields["upstream_response_before_write_complete"])
	require.NotContains(t, fields, "upstream_wait_first_byte_ms")
}

// 模拟 net/http 文档允许的迟到及并发回调，确保日志快照不读写 Gin 或产生竞态。
func TestTraceGatewayUpstreamConcurrentLateCallbacks(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	ctx := WithGatewayTiming(logger.IntoContext(context.Background(), zap.New(core)))
	req := httptest.NewRequest(http.MethodPost, "http://example.test", nil).WithContext(ctx)
	req, finish := TraceGatewayUpstream(req)
	trace := httptrace.ContextClientTrace(req.Context())
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trace.GetConn("example.test")
			trace.GotConn(httptrace.GotConnInfo{Reused: true})
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			trace.GotFirstResponseByte()
		}()
	}
	finish(nil, context.Canceled)
	wg.Wait()
	require.Equal(t, 1, logs.Len())
}
