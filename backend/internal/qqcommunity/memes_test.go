package qqcommunity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemeSearchAcceptsOnlyResultImagesFromTrustedHost(t *testing.T) {
	raw := `<img src="https://img.soutula.com/ad.gif"><img class="bqppsearch" data-original="https://evil.test/a.gif">
	<img class="image bqppsearch lazy" data-original="https://img.soutula.com/a.gif" title="开心 &amp; 猫猫">
	<img class="bqppsearch" data-original="https://img.soutula.com/a.gif">
	<img class="bqppsearch" data-original="https://img.soutula.com/b.png" title="困">`
	items, err := parseMemeSearch(strings.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, []memeCandidate{{URL: "https://img.soutula.com/a.gif", Title: "开心 & 猫猫"}, {URL: "https://img.soutula.com/b.png", Title: "困"}}, items)
	for _, raw := range []string{"http://img.soutula.com/a.gif", "https://img.soutula.com.evil.test/a", "https://user@img.soutula.com/a", "https://img.soutula.com:443/a", "https://127.0.0.1/a", "file:///tmp/a.gif"} {
		require.False(t, trustedMemeURL(raw), raw)
	}
}

func TestMemeSearchUsesCanonicalResultsWithoutFollowingRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Mozilla/5.0", r.UserAgent())
		if strings.HasPrefix(r.URL.Path, "/search/search/") {
			http.Redirect(w, r, "/search/bqb/keyword/%E7%8C%AB%E7%8C%AB%20%E5%BC%80%E5%BF%83/type/bq/page/1.html", http.StatusFound)
			return
		}
		require.Equal(t, "/search/bqb/keyword/猫猫 开心/type/bq/page/1.html", r.URL.Path)
		_, _ = w.Write([]byte(`<img class="bqppsearch" data-original="https://img.soutula.com/happy.gif">`))
	}))
	defer server.Close()
	s := New(nil, nil, false, nil)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	s.memeHTTP.Transport = memeSearchTestTransport{target: server.URL, transport: transport}
	items, err := s.searchMemes(context.Background(), "猫猫 开心")
	require.NoError(t, err)
	require.Equal(t, []memeCandidate{{URL: "https://img.soutula.com/happy.gif"}}, items)
}

func TestMemeSearchSupportsOriginTLSWithCertificateVerification(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256}}
	server.StartTLS()
	defer server.Close()
	client := newMemeSearchHTTPClient()
	defer client.CloseIdleConnections()
	_, err := client.Get(server.URL)
	require.Error(t, err, "an untrusted certificate must still be rejected")
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	transport.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, uint16(tls.TLS_RSA_WITH_AES_128_GCM_SHA256), response.TLS.CipherSuite)
}

func TestMemeSearchFallsBackToPrimaryThemeWhenCompoundHasNoResults(t *testing.T) {
	for _, sourceErr := range []error{nil, errors.New("compound search redirected")} {
		t.Run(boolText(sourceErr != nil), func(t *testing.T) {
			s := New(nil, nil, false, nil)
			c := DefaultConfig()
			c.MemeEnabled = true
			var keywords []string
			s.memeSearch = func(_ context.Context, keyword string) ([]memeCandidate, error) {
				keywords = append(keywords, keyword)
				if keyword == "开心 大笑" {
					return nil, sourceErr
				}
				return []memeCandidate{{URL: "https://img.soutula.com/happy.gif"}}, nil
			}
			var img bytes.Buffer
			require.NoError(t, gif.Encode(&img, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}), nil))
			s.imageDownload = func(context.Context, string) ([]byte, error) { return img.Bytes(), nil }
			text, data, mime := s.resolveMemeReply(context.Background(), c, memeRequest{Keyword: "开心 大笑", Text: "好耶！"})
			require.Equal(t, []string{"开心 大笑", "开心"}, keywords)
			require.Equal(t, "好耶！", text)
			require.Equal(t, "image/gif", mime)
			require.Equal(t, img.Bytes(), data)
		})
	}
}

// Opt-in network smoke test; it never calls a model, sends a QQ message, or
// consumes a member quota. Run the compiled test in the production container to
// cover the actual DNS, TLS, result page and pinned image download together.
func TestMemeLiveSource(t *testing.T) {
	if os.Getenv("SUB2API_MEME_LIVE_TEST") != "1" {
		t.Skip("set SUB2API_MEME_LIVE_TEST=1 to access the public meme source")
	}
	s := New(nil, nil, false, nil)
	defer s.memeHTTP.CloseIdleConnections()
	c := DefaultConfig()
	c.MemeEnabled = true
	for _, keyword := range []string{"开心", "开心 大笑"} {
		t.Run(keyword, func(t *testing.T) {
			started := time.Now()
			text, data, mime := s.resolveMemeReply(context.Background(), c, memeRequest{Keyword: keyword})
			t.Logf("bytes=%d mime=%s duration=%v reply=%s", len(data), mime, time.Since(started), text)
			require.NotEmpty(t, data, "the real search and image download must succeed")
			require.True(t, strings.HasPrefix(mime, "image/"))
		})
	}
}

type memeSearchTestTransport struct {
	target    string
	transport http.RoundTripper
}

func (t memeSearchTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Keep the production URL construction and no-redirect behavior, only
	// route the fixed search host to this test's HTTP server.
	if req.URL.Scheme != "https" || req.URL.Host != "fabiaoqing.com" {
		return nil, errors.New("unexpected search origin")
	}
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = "http", strings.TrimPrefix(t.target, "http://")
	copy.URL = &u
	return t.transport.RoundTrip(copy)
}

func TestMemeArgumentsRejectPrivateDataAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"keyword":"","text":""}`, `{"keyword":"2904566522","text":""}`, `{"keyword":"a@example.com","text":""}`, `{"keyword":"https://qq.com","text":""}`, `{"keyword":"绑定码 abcd","text":""}`, `{"keyword":"开心","text":"","url":"https://evil.test"}`, `{"keyword":"开心","text":""}{}`} {
		_, err := parseMemeRequest(raw)
		require.Error(t, err, raw)
	}
	request, err := parseMemeRequest(`{"keyword":" 猫猫 困 ","text":"摸鱼一会儿。"}`)
	require.NoError(t, err)
	require.Equal(t, "猫猫 困", request.Keyword)
}

func TestMemeReplyPreservesAnimatedGIFAndFallsBackWithoutImageGeneration(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second.Pix[0] = 1
	var encoded bytes.Buffer
	require.NoError(t, gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 10}}))
	s := New(nil, nil, false, nil)
	c := DefaultConfig()
	c.MemeEnabled = true
	s.memeSearch = func(_ context.Context, keyword string) ([]memeCandidate, error) {
		require.Equal(t, "猫猫", keyword)
		return []memeCandidate{{URL: "https://img.soutula.com/test.gif"}}, nil
	}
	downloads := 0
	s.imageDownload = func(_ context.Context, u string) ([]byte, error) {
		downloads++
		require.Equal(t, "https://img.soutula.com/test.gif", u)
		return encoded.Bytes(), nil
	}
	text, raw, mime := s.resolveMemeReply(context.Background(), c, memeRequest{Keyword: "猫猫", Text: "来点精神支柱。"})
	require.Equal(t, "来点精神支柱。", text)
	require.Equal(t, "image/gif", mime)
	require.Equal(t, encoded.Bytes(), raw)
	decoded, err := gif.DecodeAll(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Len(t, decoded.Image, 2)
	c.MemeEnabled = false
	_, raw, _ = s.resolveMemeReply(context.Background(), c, memeRequest{Keyword: "猫猫"})
	require.Empty(t, raw)
	require.Equal(t, 1, downloads)
	c.MemeEnabled = true
	s.memeSearch = func(context.Context, string) ([]memeCandidate, error) {
		return nil, errors.New("upstream private detail")
	}
	text, raw, _ = s.resolveMemeReply(context.Background(), c, memeRequest{Keyword: "猫猫", Text: "已找到。"})
	require.Empty(t, raw)
	require.Contains(t, text, "没找到")
	require.NotContains(t, text, "已找到")
	require.NotContains(t, text, "private")
}

func TestMemeToolWorksAcrossChatProtocolsWithImagesDisabled(t *testing.T) {
	for _, protocol := range []string{"chat_completions", "responses"} {
		for _, enabled := range []bool{false, true} {
			t.Run(protocol+"/"+boolText(enabled), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					tools, _ := json.Marshal(body["tools"])
					require.Equal(t, enabled, strings.Contains(string(tools), memeSearchToolName))
					raw := imageToolTestJSON(t, protocol, `{"keyword":"开心","text":"好耶！"}`)
					_, _ = w.Write(bytes.ReplaceAll(raw, []byte(chatImageToolName), []byte(memeSearchToolName)))
				}))
				defer server.Close()
				s := New(nil, nil, false, nil)
				c := DefaultConfig()
				c.BaseURL, c.ChatProtocol, c.MemeEnabled = server.URL, protocol, enabled
				turn, err := s.siteChatTurn(context.Background(), c, []Message{{Role: "user", Content: "来张开心的表情包"}})
				if !enabled {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, &memeRequest{Keyword: "开心", Text: "好耶！"}, turn.Meme)
				require.Empty(t, turn.ImagePrompt)
				require.Nil(t, turn.SiteQuery)
			})
		}
	}
}
