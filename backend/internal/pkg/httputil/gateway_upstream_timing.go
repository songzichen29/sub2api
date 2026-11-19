package httputil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// TraceGatewayUpstream 为每次实际 HTTP 尝试独立计时。回调可能并发或晚于 Do
// 返回，因此只修改加锁的局部状态；完成时记录一个快照，不等待后台回调。
func TraceGatewayUpstream(req *http.Request) (*http.Request, func(*http.Response, error)) {
	timing, enabled := req.Context().Value(gatewayTimingKey{}).(*gatewayTiming)
	if !enabled {
		return req, func(*http.Response, error) {}
	}
	started := time.Now()
	timing.mu.Lock()
	timing.attempts++
	attempt := timing.attempts
	timing.mu.Unlock()
	var mu sync.Mutex
	var getConn, gotConn, wroteRequest, firstByte, dnsStart, tlsStart time.Time
	var dnsTime, tcpTime, tlsTime time.Duration
	var connections, writes, dnsCount, tcpCount, tlsCount int
	var reused, writeFailed bool
	tcpStarts := make(map[string]time.Time)
	trace := &httptrace.ClientTrace{
		GetConn: func(string) {
			mu.Lock()
			defer mu.Unlock()
			getConn = time.Now()
			connections++
		},
		GotConn: func(info httptrace.GotConnInfo) {
			mu.Lock()
			defer mu.Unlock()
			gotConn = time.Now()
			reused = info.Reused
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			mu.Lock()
			defer mu.Unlock()
			dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			mu.Lock()
			defer mu.Unlock()
			if !dnsStart.IsZero() {
				dnsTime += time.Since(dnsStart)
				dnsCount++
			}
		},
		ConnectStart: func(network, addr string) {
			mu.Lock()
			defer mu.Unlock()
			tcpStarts[network+"\x00"+addr] = time.Now()
		},
		ConnectDone: func(network, addr string, _ error) {
			mu.Lock()
			defer mu.Unlock()
			key := network + "\x00" + addr
			if start, ok := tcpStarts[key]; ok {
				tcpTime += time.Since(start)
				tcpCount++
				delete(tcpStarts, key)
			}
		},
		TLSHandshakeStart: func() {
			mu.Lock()
			defer mu.Unlock()
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			mu.Lock()
			defer mu.Unlock()
			if !tlsStart.IsZero() {
				tlsTime += time.Since(tlsStart)
				tlsCount++
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			mu.Lock()
			defer mu.Unlock()
			writes++
			writeFailed = info.Err != nil
			if info.Err == nil {
				wroteRequest = time.Now()
			}
		},
		GotFirstResponseByte: func() {
			mu.Lock()
			defer mu.Unlock()
			if firstByte.IsZero() {
				firstByte = time.Now()
			}
		},
	}
	finish := func(resp *http.Response, err error) {
		mu.Lock()
		fields := []zap.Field{
			zap.Int("upstream_attempt", attempt),
			zap.Int64("upstream_dispatch_at_ms", started.Sub(timing.started).Milliseconds()),
			zap.Int64("upstream_roundtrip_ms", time.Since(started).Milliseconds()),
			zap.Int64("upstream_request_bytes", req.ContentLength),
			zap.Int("upstream_connection_attempts", connections),
			zap.Int("upstream_write_callbacks", writes),
			zap.Bool("upstream_transport_error", err != nil),
			zap.Bool("upstream_write_failed", writeFailed),
			zap.Bool(logger.OpsSystemLogSkipField, true),
		}
		if id, ok := req.Context().Value(ctxkey.AccountID).(int64); ok {
			fields = append(fields, zap.Int64("account_id", id))
		}
		if resp != nil {
			fields = append(fields, zap.Int("upstream_status", resp.StatusCode))
		}
		if dnsCount > 0 {
			fields = append(fields, zap.Int64("upstream_dns_total_ms", dnsTime.Milliseconds()))
		}
		if tcpCount > 0 {
			// 多地址拨号可能重叠，该累计值不能与连接耗时相加。
			fields = append(fields, zap.Int64("upstream_tcp_total_ms", tcpTime.Milliseconds()))
		}
		if tlsCount > 0 {
			fields = append(fields, zap.Int64("upstream_tls_total_ms", tlsTime.Milliseconds()))
		}
		// Transport 内部重试或重定向可能触发多组回调；不把不同尝试拼成一段。
		if connections == 1 && !gotConn.IsZero() {
			fields = append(fields,
				zap.Bool("upstream_conn_reused", reused),
				zap.Int64("upstream_conn_ms", gotConn.Sub(getConn).Milliseconds()),
			)
			if writes == 1 && !writeFailed && !wroteRequest.IsZero() {
				fields = append(fields, zap.Int64("upstream_write_ms", wroteRequest.Sub(gotConn).Milliseconds()))
				// 首字节是响应头（包括 1xx），不是首个 SSE，也不代表模型思考完成。
				if !firstByte.IsZero() && !firstByte.Before(wroteRequest) {
					fields = append(fields, zap.Int64("upstream_wait_first_byte_ms", firstByte.Sub(wroteRequest).Milliseconds()))
				}
			}
			fields = append(fields, zap.Bool("upstream_response_before_write_complete",
				!firstByte.IsZero() && (wroteRequest.IsZero() || firstByte.Before(wroteRequest))))
		}
		mu.Unlock()
		logger.FromContext(req.Context()).Info("gateway.upstream_timing", fields...)
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace)), finish
}
