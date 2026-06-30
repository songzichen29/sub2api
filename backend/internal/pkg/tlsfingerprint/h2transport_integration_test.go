//go:build integration

package tlsfingerprint

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestH2RoundTripper_NegotiatesH2 verifies the H2RoundTripper advertises h2 in
// ALPN and the resulting request runs over HTTP/2. Run with:
//
//	go test -tags integration -run TestH2RoundTripper -v ./internal/pkg/tlsfingerprint/
func TestH2RoundTripper_NegotiatesH2(t *testing.T) {
	// h1 fallback: utls dialer advertising ["http/1.1"].
	h1 := &http.Transport{
		ForceAttemptHTTP2: false,
		IdleConnTimeout:   30 * time.Second,
	}
	h1.DialTLSContext = NewDialer(&Profile{}, nil).DialTLSContext

	// h2 path: identical fingerprint, ALPN ["h2","http/1.1"].
	h2Profile := Profile{ALPNProtocols: []string{"h2", "http/1.1"}}
	h2Dialer := NewDialer(&h2Profile, nil)

	rt := NewH2RoundTripper(h1, h2Dialer.DialTLSContext)
	client := &http.Client{Transport: rt}

	// 1) Well-known h2 endpoint: the response must come back over HTTP/2.0,
	//    which is only possible if we advertised h2 and the server selected it.
	resp, err := client.Get("https://http2.golang.org/")
	if err != nil {
		t.Fatalf("GET http2.golang.org: %v", err)
	}
	resp.Body.Close()
	t.Logf("http2.golang.org: Proto=%s status=%d", resp.Proto, resp.StatusCode)
	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("http2.golang.org expected HTTP/2.0, got %s", resp.Proto)
	}

	// 2) tls.peet.ws echoes the caller's TLS fingerprint (JA4 + negotiated ALPN).
	resp2, err := client.Get("https://tls.peet.ws/api/all")
	if err != nil {
		t.Fatalf("GET tls.peet.ws: %v", err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	t.Logf("tls.peet.ws: Proto=%s", resp2.Proto)

	var rep struct {
		TLS map[string]any `json:"tls"`
	}
	if json.Unmarshal(body, &rep) == nil {
		t.Logf("tls.peet.ws tls fingerprint: %+v", rep.TLS)
	}

	// resp2.Proto == HTTP/2.0 already proves h2 was offered in ALPN and the
	// server selected it. Confirm the JA4 a-segment ALPN marker is now h2
	// (previously http/1.1 only → "h1"). JA4 a-segment looks like
	// "t13d<cc><ec><alpn>" before the first underscore.
	if resp2.Proto != "HTTP/2.0" {
		t.Fatalf("tls.peet.ws expected HTTP/2.0, got %s", resp2.Proto)
	}
	ja4, _ := rep.TLS["ja4"].(string)
	t.Logf("JA4 = %q", ja4)
	aSeg := ja4
	if i := strings.Index(ja4, "_"); i >= 0 {
		aSeg = ja4[:i]
	}
	t.Logf("JA4 a-segment = %q", aSeg)
	if !strings.HasSuffix(aSeg, "h2") {
		t.Fatalf("JA4 ALPN segment is not h2 (was h1 before R-P5): ja4=%q a-segment=%q", ja4, aSeg)
	}
}
