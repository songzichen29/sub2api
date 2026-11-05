package qqcommunity

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif" // Register GIF headers without decoding/re-encoding animation frames.
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const memeSearchToolName = "search_meme"
const memeSearchInstructions = `本轮还提供 search_meme：根据简短关键词搜索现成表情包并由系统实际发送，支持静态图与 GIF。这是专门的表情搜索，不是通用网页、新闻或 X 检索。用户要找表情包、斗图时可使用；轻松闲聊中也可偶尔用贴合语境的表情接梗，不要每轮都用，认真处理账户、费用或故障时优先文字和站点工具。每轮仍只能选择一个操作，search_meme 也属于可选操作。只想找现成表情时不要调用生图；用户明确要求创作或修改图片时继续使用对应图片工具。keyword 优先只用一个最核心的表情主题或情绪词，例如“猫猫”“困”“开心”，不要堆叠同义词或擅自添加“大笑”等修饰，不得包含用户原话中的账号、QQ号、邮箱、绑定码、密钥或其他私人资料。text 是随图发送的简短自然回复，可以为空；不要承诺搜索成功或描述尚未见到的具体画面。搜索失败时系统会直接给出失败说明，不编造结果。`

// This public search origin still negotiates RSA key exchange with AES-GCM.
// Keep that compatibility in a dedicated client: model/API clients retain their
// default suites, certificates are verified, and TLS 1.2 remains the minimum.
func newMemeSearchHTTPClient() *http.Client {
	return &http.Client{
		// The production origin can take 20-30 seconds to serve a results page.
		// The overall reply deadline below still bounds retries and downloads.
		Timeout: 32 * time.Second,
		Transport: &http.Transport{Proxy: nil, TLSHandshakeTimeout: 12 * time.Second, TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			},
		}},
		CheckRedirect: noRedirect,
	}
}

type memeRequest struct {
	Keyword string `json:"keyword"`
	Text    string `json:"text"`
}

type memeCandidate struct {
	URL   string
	Title string
}

func memeSearchTool(protocol string) map[string]any {
	return wrapChatImageTool(protocol, map[string]any{
		"name": memeSearchToolName, "description": "搜索并发送一张现成表情包，支持 GIF；不调用生图模型。关键词只含主题和情绪，不含私人账号资料。", "strict": true,
		"parameters": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"keyword", "text"},
			"properties": map[string]any{
				"keyword": map[string]any{"type": "string", "minLength": 1, "maxLength": 40},
				"text":    map[string]any{"type": "string", "maxLength": 300},
			},
		},
	})
}

var privateMemeKeyword = regexp.MustCompile(`(?i)(https?://|www\.|@|[0-9]{5,}|sk-[a-z0-9]|绑定码|验证码|密钥|密码)`)

func parseMemeRequest(raw string) (*memeRequest, error) {
	var request memeRequest
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&request) != nil || d.Decode(&extra) != io.EOF {
		return nil, errors.New("表情搜索参数不正确")
	}
	request.Keyword, request.Text = strings.TrimSpace(request.Keyword), normalizeChatReply(request.Text)
	if n := len([]rune(request.Keyword)); n < 1 || n > 40 || len([]rune(request.Text)) > 300 || privateMemeKeyword.MatchString(request.Keyword) || strings.ContainsFunc(request.Keyword, unicode.IsControl) {
		return nil, errors.New("表情关键词须为 1～40 字的主题或情绪，不得包含账号或敏感资料")
	}
	return &request, nil
}

// Only accept the search site's actual result thumbnails, never arbitrary URLs
// supplied by the model or page scripts. Downloads additionally pin public IPs.
func trustedMemeURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	return u.Hostname() == "img.soutula.com"
}

func parseMemeSearch(reader io.Reader) ([]memeCandidate, error) {
	root, err := html.Parse(reader)
	if err != nil {
		return nil, err
	}
	items := []memeCandidate{}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(items) >= 8 {
			return
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if strings.Contains(" "+attrs["class"]+" ", " bqppsearch ") && trustedMemeURL(attrs["data-original"]) && !seen[attrs["data-original"]] {
				seen[attrs["data-original"]] = true
				items = append(items, memeCandidate{URL: attrs["data-original"], Title: attrs["title"]})
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return items, nil
}

func (s *Service) searchMemes(ctx context.Context, keyword string) ([]memeCandidate, error) {
	if s.memeSearch != nil {
		return s.memeSearch(ctx, keyword)
	}
	// The legacy /search/search entry point responds with a redirect. Request
	// the actual results page so the client's no-redirect policy stays intact.
	endpoint := "https://fabiaoqing.com/search/bqb/keyword/" + url.PathEscape(keyword) + "/type/bq/page/1.html"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := s.memeHTTP.Do(req)
	if err != nil {
		return nil, errors.New("表情搜索来源暂时不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("表情搜索来源暂时不可用")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (512<<10)+1))
	if err != nil || len(raw) > 512<<10 {
		return nil, errors.New("表情搜索响应不完整")
	}
	return parseMemeSearch(bytes.NewReader(raw))
}

func (s *Service) resolveMemeReply(ctx context.Context, c Config, request memeRequest) (string, []byte, string) {
	if !c.MemeEnabled {
		return "表情搜索尚未启用。", nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	items, err := s.searchMemes(ctx, request.Keyword)
	// Space-separated themes can return no matches or a redirect at this source.
	// Retry the main theme once within the same overall deadline and chat turn.
	if fields := strings.Fields(request.Keyword); len(items) == 0 && len(fields) > 1 && ctx.Err() == nil {
		items, err = s.searchMemes(ctx, fields[0])
	}
	if err == nil {
		for i, item := range items {
			if i >= 3 || ctx.Err() != nil {
				break
			}
			if !trustedMemeURL(item.URL) {
				continue
			}
			downloadCtx, done := context.WithTimeout(ctx, 7*time.Second)
			data, downloadErr := s.imageDownload(downloadCtx, item.URL)
			done()
			if downloadErr != nil || len(data) > 6<<20 {
				continue
			}
			cfg, format, decodeErr := image.DecodeConfig(bytes.NewReader(data))
			if decodeErr != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 12_000_000 || (format != "gif" && format != "png" && format != "jpeg") {
				continue
			}
			text := strings.TrimSpace(request.Text)
			if text == "" {
				text = "找到一张「" + request.Keyword + "」表情包。"
			}
			// Preserve the original bytes; converting a GIF to JPEG loses animation.
			return text, data, http.DetectContentType(data)
		}
	}
	return "这次没找到能发送的「" + request.Keyword + "」表情包，换个关键词再试试。", nil, ""
}
