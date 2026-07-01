package repository

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

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
