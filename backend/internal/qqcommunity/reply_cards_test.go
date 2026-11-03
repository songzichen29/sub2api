package qqcommunity

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func replyTestFont(t *testing.T) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(goregular.TTF)
	require.NoError(t, err)
	return f
}

func TestExplicitReplyFontCannotHideFailureBehindSystemFallback(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.ttf"), filepath.Join(dir, "bad.ttf")
	require.NoError(t, os.WriteFile(good, goregular.TTF, 0600))
	broken := append([]byte(nil), goregular.TTF...)
	// Emulate the packaged TTC's unaligned table offsets rejected by sfnt.
	binary.BigEndian.PutUint32(broken[20:24], binary.BigEndian.Uint32(broken[20:24])|1)
	require.NoError(t, os.WriteFile(bad, broken, 0600))
	f, source, err := resolveCardFont(bad, []string{good})
	require.ErrorContains(t, err, "invalid table offset")
	require.Nil(t, f)
	require.Empty(t, source)
	f, source, err = resolveCardFont(good, []string{bad})
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Equal(t, good, source)
	f, source, err = resolveCardFont("", []string{bad, good})
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Equal(t, good, source)
}

func TestReplyCardPaginationRetainsUnicodeAndSuffix(t *testing.T) {
	f := replyTestFont(t)
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 26, DPI: 72})
	require.NoError(t, err)
	defer face.Close()
	text := strings.Repeat("混合中文 model-6.1 $0.10\n", 500) + "END-最后一段"
	pages := layoutReplyCards(text, nil, face)
	require.Greater(t, len(pages), 5)
	var lines []string
	for i, p := range pages {
		require.LessOrEqual(t, len(p.Lines), 40)
		lines = append(lines, p.Lines...)
		if i == len(pages)-1 {
			data, err := renderReplyCard(f, "分页", p, i, len(pages))
			require.NoError(t, err)
			cfg, err := png.DecodeConfig(bytes.NewReader(data))
			require.NoError(t, err)
			require.Equal(t, 960, cfg.Width)
			require.LessOrEqual(t, cfg.Height, 1700)
		}
	}
	require.Equal(t, text, strings.Join(lines, "\n"))
	require.Equal(t, "https://example.com/bind?code=abc\nhttps://example.com/transfer", replyLinks("绑定：https://example.com/bind?code=abc\n[转账](https://example.com/transfer)\nhttps://example.com/bind?code=abc"))
	var c Config
	require.NoError(t, json.Unmarshal([]byte(`{"enabled":false}`), &c))
	require.Equal(t, "image", c.CommandReplyFormat)
	require.NoError(t, json.Unmarshal([]byte(`{"command_reply_format":"text"}`), &c))
	require.Equal(t, "text", c.CommandReplyFormat)
}

func TestReplyCardDeliveryAndSafeFallback(t *testing.T) {
	for _, mode := range []string{"success", "rejected second image", "uncertain", "missing receipt"} {
		t.Run(mode, func(t *testing.T) {
			type segment struct {
				Type string            `json:"type"`
				Data map[string]string `json:"data"`
			}
			type request struct {
				Echo   string `json:"echo"`
				Params struct {
					Message []segment `json:"message"`
				} `json:"params"`
			}
			requests := make(chan request, 20)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				for n := 1; ; n++ {
					var req request
					if ws.ReadJSON(&req) != nil {
						return
					}
					requests <- req
					if mode == "uncertain" {
						continue
					}
					status, code := "ok", 0
					if mode == "rejected second image" && n == 2 {
						status, code = "failed", 100
					}
					data := map[string]any{"message_id": n}
					if mode == "missing receipt" {
						data = map[string]any{}
					}
					if ws.WriteJSON(map[string]any{"echo": req.Echo, "status": status, "retcode": code, "data": data}) != nil {
						return
					}
				}
			}))
			defer srv.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			require.NoError(t, err)
			defer ws.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if mode == "uncertain" {
				var short context.CancelFunc
				ctx, short = context.WithTimeout(ctx, 200*time.Millisecond)
				defer short()
			}
			conn := &oneBotConnection{ws: ws, pending: map[string]chan oneBotAck{}, bot: "bot", credential: codeHash("token")}
			s := New(nil, nil, false, nil)
			s.onebot.conn = conn
			go func() {
				for {
					var ack oneBotAck
					if ws.ReadJSON(&ack) != nil {
						return
					}
					conn.mu.Lock()
					ch := conn.pending[ack.Echo]
					conn.mu.Unlock()
					if ch != nil {
						ch <- ack
					}
				}
			}()
			text := strings.Repeat("line\n", 85) + "https://example.com/finish"
			_, hasImage, err := s.sendCommandCardsWithFont(ctx, Config{AppID: "bot", AppSecret: "token"}, Job{GroupID: "group", MemberID: "member", ReplyReference: "42"}, text, replyTestFont(t))
			if mode == "uncertain" || mode == "missing receipt" {
				require.Error(t, err)
				require.Len(t, requests, 1)
				return
			}
			require.NoError(t, err)
			require.True(t, hasImage)
			expected := 4
			if mode == "rejected second image" {
				expected++
			}
			require.Len(t, requests, expected)
			var fallback, links string
			for i := 0; i < expected; i++ {
				req := <-requests
				for _, seg := range req.Params.Message {
					if i > 0 && !(mode == "rejected second image" && i == 2) {
						require.NotEqual(t, "at", seg.Type)
						require.NotEqual(t, "reply", seg.Type)
					}
					if seg.Type == "image" {
						require.True(t, strings.HasPrefix(seg.Data["file"], "base64://"))
					}
					if seg.Type == "text" {
						if strings.HasPrefix(seg.Data["text"], "可点击链接") {
							links = seg.Data["text"]
						} else {
							fallback += seg.Data["text"]
						}
					}
				}
			}
			require.Contains(t, links, "https://example.com/finish")
			if mode == "rejected second image" {
				require.Equal(t, 39, strings.Count(fallback, "\n"))
			}
		})
	}
}

// Optional local visual review; CI does not need Chinese system fonts.
func TestReplyCardVisualSamples(t *testing.T) {
	dir := os.Getenv("QQBOT_CARD_PREVIEW_DIR")
	if dir == "" {
		t.Skip("local preview only")
	}
	f, err := loadCardFont()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0755))
	report := channelStatusFixture()
	for i := 0; i < 18; i++ {
		m := ChannelStatusMetric{HasRequests: true, ErrorRate: float64(i%6) / 50}
		report.Trend = append(report.Trend, ChannelStatusPoint{At: time.Date(2026, 11, 3, 5, i*5, 0, 0, time.UTC), Metrics: m, Health: []string{"healthy", "warning", "critical"}[i%3]})
	}
	for i := range report.Groups {
		report.Groups[i].Buckets = report.Trend
	}
	samples := []struct {
		name, text string
		doc        *replyDocument
	}{
		{"monitor", strings.Join(formatChannelStatus(report), "\n\n"), channelReplyDocument(report)},
		{"usage", "今日用量\n请求：204 次\n费用：USD 3.3157\n余额扣费：USD 1.2880\n套餐消耗：USD 2.0277", &replyDocument{Title: "今日用量", Bars: []replyBar{{"输入 Token", 2150000, "2.15M"}, {"输出 Token", 63110, "63.11K"}, {"缓存读取 Token", 653450, "653.45K"}}, CostBars: []replyBar{{"gpt-6.1-sol", 2.8963, "$2.8963"}, {"gpt-6-astra", .403, "$0.4030"}}}},
		{"help", "机器人帮助\n\n/绑定 绑定码\n/余额 · 查看本人余额\n/用量 · 今日 Token 和费用\n/签到 · 每日签到奖励\n/监控 · 当前渠道健康趋势\n/价格 模型名\n/帮助 · 查看全部功能\n\n长内容自动分页，链接另附文字。", nil},
	}
	for _, sample := range samples {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 26, DPI: 72})
		require.NoError(t, err)
		pages := layoutReplyCards(sample.text, sample.doc, face)
		_ = face.Close()
		for i, p := range pages {
			title := "机器人帮助"
			if sample.doc != nil {
				title = sample.doc.Title
			}
			data, err := renderReplyCard(f, title, p, i, len(pages))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.png", sample.name, i+1)), data, 0644))
		}
	}
}
