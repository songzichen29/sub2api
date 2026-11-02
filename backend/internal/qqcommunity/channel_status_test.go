package qqcommunity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

type channelStatusProviderFunc func(context.Context) (*ChannelStatusReport, error)

func (f channelStatusProviderFunc) ChannelStatus(ctx context.Context) (*ChannelStatusReport, error) {
	return f(ctx)
}

func channelStatusFixture() *ChannelStatusReport {
	cache, ttft, tps := .7625, int64(1280), 42.5
	metric := ChannelStatusMetric{OutputTPS: &tps, HasRequests: true, ErrorRate: .0032, CacheRate: &cache, TTFTP50Ms: &ttft, RPM: 12.34, TokensPerSecond: 56.78}
	return &ChannelStatusReport{
		UpdatedAt: time.Date(2026, 10, 16, 6, 30, 0, 0, time.UTC),
		Overall:   metric,
		Groups:    []ChannelStatusGroup{{Name: "OpenAI 标准组", Metrics: metric}, {Name: "Gemini 分组"}},
	}
}

func TestFormatChannelStatus(t *testing.T) {
	report := channelStatusFixture()
	chunks := formatChannelStatus(report)
	require.Equal(t, []string{`📊 **渠道状态**
近 90 分钟
更新：2026-10-16 14:30（北京时间）

**整体**
成功率：99.68%
错误率：0.32%
缓存率：76.25%
首 Token P50：1.28 秒
输出 TPS：42.50 token/s

**1. OpenAI 标准组**
成功率：99.68%
错误率：0.32%
缓存率：76.25%
首 Token P50：1.28 秒
输出 TPS：42.50 token/s

**2. Gemini 分组**
暂无请求数据`}, chunks)
	require.NotContains(t, chunks[0], "RPM")
	require.NotContains(t, chunks[0], "总吞吐")
	require.Contains(t, chunks[0], "输出 TPS：42.50 token/s")
	report.ShowThroughput = true
	text := strings.Join(formatChannelStatus(report), "")
	require.Equal(t, 2, strings.Count(text, "RPM：12.34\n总吞吐：56.78 token/s"))
}

func TestChannelStatusMissingSamplesAreNotZeroOrPerfectSuccess(t *testing.T) {
	report := &ChannelStatusReport{PartialCoverage: true}
	text := formatChannelStatus(report)[0]
	require.Contains(t, text, "更新：暂无数据")
	require.Contains(t, text, "暂无请求数据")
	require.Contains(t, text, "分组：暂无可展示项")
	require.Contains(t, text, "部分时段")
	require.NotContains(t, text, "100.00%")
	report.Backfilling = true
	require.Contains(t, formatChannelStatus(report)[0], "正在补齐数据")
	report.Overall = ChannelStatusMetric{HasRequests: true, ErrorRate: 1}
	text = formatChannelStatus(report)[0]
	require.Contains(t, text, "成功率：0.00%\n错误率：100.00%")
	require.Contains(t, text, "缓存率：-\n首 Token 暂无数据")
	zero := 0.0
	report.Overall.CacheRate = &zero
	require.Contains(t, formatChannelStatus(report)[0], "缓存率：0.00%")
}

func TestChannelStatusInsufficientSamplesDoNotReportExtremeRates(t *testing.T) {
	for _, errorRate := range []float64{0, 1} {
		metric := ChannelStatusMetric{HasRequests: true, InsufficientSamples: true, ErrorRate: errorRate}
		text := formatChannelStatusMetric(metric, false)
		require.Contains(t, text, "成功率样本不足")
		require.Contains(t, text, "错误率样本不足")
		require.Contains(t, text, "缓存率：-")
		require.NotContains(t, text, "0.00%")
		require.NotContains(t, text, "暂无请求数据")
	}
	// Enough failed requests are a real outage and must remain visible.
	text := formatChannelStatusMetric(ChannelStatusMetric{HasRequests: true, ErrorRate: 1}, false)
	require.Contains(t, text, "成功率：0.00%\n错误率：100.00%")
}

func TestChannelStatusSplitsWithoutLosingOrBreakingGroups(t *testing.T) {
	report := channelStatusFixture()
	report.ShowThroughput = true
	report.Groups = nil
	for i := 1; i <= 100; i++ {
		report.Groups = append(report.Groups, ChannelStatusGroup{Name: fmt.Sprintf("分组%03d%s", i, strings.Repeat("中", 90)), Metrics: report.Overall})
	}
	chunks := formatChannelStatus(report)
	require.Greater(t, len(chunks), 5)
	for _, chunk := range chunks {
		require.LessOrEqual(t, utf8.RuneCountInString(chunk), 1400)
		require.True(t, utf8.ValidString(chunk))
		require.NotContains(t, chunk, "截断")
	}
	for i, group := range report.Groups {
		block := fmt.Sprintf("**%d. %s**\n%s", i+1, group.Name, formatChannelStatusMetric(group.Metrics, true))
		count := 0
		for _, chunk := range chunks {
			count += strings.Count(chunk, block)
		}
		require.Equal(t, 1, count, group.Name)
	}
}

type channelStatusReply struct {
	Content  string `json:"content"`
	Sequence int    `json:"msg_seq"`
}

func channelStatusSender(t *testing.T, failAt int) (*Service, *[]channelStatusReply) {
	t.Helper()
	var replies []channelStatusReply
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"test-only","expires_in":7200}`))
		case "/v2/groups/group/messages":
			message := readQQTestReply(t, r)
			body := channelStatusReply{Content: message.text(), Sequence: message.Sequence}
			replies = append(replies, body)
			if failAt > 0 && body.Sequence == failAt {
				w.WriteHeader(500)
				return
			}
			_, _ = w.Write([]byte(`{"id":"sent"}`))
		default:
			t.Errorf("channel status unexpectedly called %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	// A nil database proves this command does not require binding, reserve
	// model quota, or write model history. Only monitor and QQ calls are allowed.
	s := New(nil, nil, false, nil)
	s.qqURL, s.tokenURL = server.URL, server.URL+"/token"
	return s, &replies
}
