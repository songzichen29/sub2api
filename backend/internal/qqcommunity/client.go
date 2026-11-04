package qqcommunity

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

func signingKey(secret string) (ed25519.PrivateKey, error) {
	if secret == "" {
		return nil, errors.New("AppSecret 未配置")
	}
	for len(secret) < ed25519.SeedSize {
		secret += secret
	}
	return ed25519.NewKeyFromSeed([]byte(secret[:ed25519.SeedSize])), nil
}
func VerifySignature(secret, timestamp, signature string, body []byte, now time.Time) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(ts, 0)) > 5*time.Minute || time.Unix(ts, 0).Sub(now) > time.Minute {
		return false
	}
	key, err := signingKey(secret)
	if err != nil {
		return false
	}
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	publicKey, ok := key.Public().(ed25519.PublicKey)
	return ok && ed25519.Verify(publicKey, append([]byte(timestamp), body...), sig)
}

var challengeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_=-]{1,128}$`)
var challengeTimePattern = regexp.MustCompile(`^[0-9]{1,20}$`)

func Challenge(secret string, data json.RawMessage) (map[string]string, error) {
	var v struct {
		PlainToken string `json:"plain_token"`
		EventTS    string `json:"event_ts"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	// The unsigned URL-validation endpoint must never sign arbitrary JSON. QQ
	// event signatures use the same timestamp+body format: accepting a JSON
	// document as plain_token would turn this into an event-forgery oracle.
	if !challengeTokenPattern.MatchString(v.PlainToken) || !challengeTimePattern.MatchString(v.EventTS) {
		return nil, errors.New("invalid challenge")
	}
	key, err := signingKey(secret)
	if err != nil {
		return nil, err
	}
	return map[string]string{"plain_token": v.PlainToken, "signature": hex.EncodeToString(ed25519.Sign(key, []byte(v.EventTS+v.PlainToken)))}, nil
}

func noRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// Preserve retry evidence without exposing upstream bodies, URLs or credentials.
// Callers decide whether their operation is safe to retry.
type requestFailure struct {
	StatusCode int
	RetryAfter string
	cause      error
	message    string
}

func (e *requestFailure) Error() string { return e.message }
func (e *requestFailure) Unwrap() error { return e.cause }

func (s *Service) request(ctx context.Context, method, endpoint, key string, body any, limit int64) ([]byte, string, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		reader = bytes.NewReader(raw)
	}
	return s.requestBody(ctx, method, endpoint, key, reader, "application/json", limit)
}

func (s *Service) requestBody(ctx context.Context, method, endpoint, key string, reader io.Reader, contentType string, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, "", errors.New("请求地址无效")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", &requestFailure{cause: err, message: "服务连接失败或超时，请查看调用记录"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &requestFailure{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After"), message: fmt.Sprintf("服务返回 HTTP %d，请管理员检查接口、模型和凭据", resp.StatusCode)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return raw, resp.Header.Get("Content-Type"), &requestFailure{cause: err, message: "响应读取失败"}
	}
	if int64(len(raw)) > limit {
		return nil, "", errors.New("响应超过大小限制")
	}
	return raw, resp.Header.Get("Content-Type"), nil
}
func endpoint(base, path string) string { return strings.TrimRight(base, "/") + path }

func (s *Service) chat(ctx context.Context, c Config, messages []Message) (string, int64, int64, error) {
	raw, ct, err := s.chatRequest(ctx, c, messages, false)
	if err != nil {
		return "", 0, 0, err
	}
	return parseChat(raw, ct)
}

// Autonomous replies are decisions consumed by the application, not prose.
func (s *Service) chatJSON(ctx context.Context, c Config, messages []Message) (string, int64, int64, error) {
	raw, ct, err := s.chatRequestWithFormat(ctx, c, messages, nil, "", true)
	if err != nil {
		return "", 0, 0, err
	}
	return parseChat(raw, ct)
}

func (s *Service) chatRequest(ctx context.Context, c Config, messages []Message, imageTool bool) ([]byte, string, error) {
	var tools []any
	if imageTool {
		tools = []any{chatImageTool(c.ChatProtocol)}
		if len(currentChatImages(messages)) > 0 {
			tools = append(tools, chatEditImageTool(c.ChatProtocol))
		}
		tools = append(tools, chatTextTool(c.ChatProtocol))
	}
	return s.chatRequestWithTools(ctx, c, messages, tools)
}

func (s *Service) chatRequestWithTools(ctx context.Context, c Config, messages []Message, tools []any) ([]byte, string, error) {
	return s.chatRequestWithToolChoice(ctx, c, messages, tools, "required")
}

func (s *Service) chatRequestWithToolChoice(ctx context.Context, c Config, messages []Message, tools []any, choice string) ([]byte, string, error) {
	return s.chatRequestWithFormat(ctx, c, messages, tools, choice, false)
}

func (s *Service) chatRequestWithFormat(ctx context.Context, c Config, messages []Message, tools []any, choice string, jsonOutput bool) ([]byte, string, error) {
	ctx, cancel, err := modelContext(ctx)
	if err != nil {
		return nil, "", err
	}
	defer cancel()
	path := "/chat/completions"
	body := map[string]any{"model": c.ChatModel, "messages": messages, "stream": false, "max_completion_tokens": c.MaxOutputTokens}
	if c.ChatProtocol == "responses" {
		path = "/responses"
		body = map[string]any{"model": c.ChatModel, "input": responsesMessages(messages), "stream": false, "store": false, "max_output_tokens": c.MaxOutputTokens}
	}
	if jsonOutput {
		if c.ChatProtocol == "responses" {
			body["text"] = map[string]any{"format": map[string]string{"type": "json_object"}}
		} else {
			body["response_format"] = map[string]string{"type": "json_object"}
		}
	}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = choice
		body["parallel_tool_calls"] = false
	}
	return s.request(ctx, "POST", endpoint(c.BaseURL, path), c.APIKey, body, 4<<20)
}

func responsesMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		item := map[string]any{"role": message.Role}
		if len(message.Images) == 0 {
			item["content"] = message.Content
		} else {
			parts := make([]map[string]any, 0, len(message.Images)+1)
			if message.Content != "" {
				parts = append(parts, map[string]any{"type": "input_text", "text": message.Content})
			}
			for _, imageURL := range message.Images {
				if imageURL != "" {
					parts = append(parts, map[string]any{"type": "input_image", "image_url": imageURL})
				}
			}
			item["content"] = parts
		}
		out = append(out, item)
	}
	return out
}
func parseChat(raw []byte, ct string) (string, int64, int64, error) {
	if strings.Contains(ct, "text/event-stream") {
		var out strings.Builder
		var in, output int64
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		scanner.Buffer(make([]byte, 4096), 4<<20)
		done := false
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				done = true
				continue
			}
			var v struct {
				Type    string          `json:"type"`
				Code    string          `json:"code"`
				Error   json.RawMessage `json:"error"`
				Delta   string          `json:"delta"`
				Choices []struct {
					FinishReason string `json:"finish_reason"`
					Delta        struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Response json.RawMessage `json:"response"`
				Usage    struct {
					Prompt     int64 `json:"prompt_tokens"`
					Completion int64 `json:"completion_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &v) != nil {
				continue
			}
			if v.Type == "error" || v.Type == "response.failed" || v.Type == "response.incomplete" || (len(v.Error) > 0 && string(v.Error) != "null") {
				rawError := v.Error
				if len(rawError) == 0 && v.Code != "" {
					rawError, _ = json.Marshal(map[string]string{"code": v.Code})
				}
				status := ""
				if v.Type == "response.incomplete" {
					status = "incomplete"
				}
				return "", 0, 0, chatCompletionFailure(status, rawError, nil)
			}
			if v.Type == "response.output_text.delta" {
				_, _ = out.WriteString(v.Delta)
			}
			for _, ch := range v.Choices {
				if ch.FinishReason == "length" || ch.FinishReason == "content_filter" {
					details, _ := json.Marshal(map[string]string{"reason": ch.FinishReason})
					return "", 0, 0, chatCompletionFailure("incomplete", nil, details)
				}
				_, _ = out.WriteString(ch.Delta.Content)
			}
			if v.Usage.Prompt > 0 {
				in = v.Usage.Prompt
				output = v.Usage.Completion
			}
			if v.Type == "response.completed" {
				done = true
				text, i, o, e := parseChat(v.Response, "application/json")
				if e == nil {
					if out.Len() == 0 {
						_, _ = out.WriteString(text)
					}
					in = i
					output = o
				}
			}
		}
		if scanner.Err() != nil || !done || out.Len() == 0 {
			return "", 0, 0, errors.New("模型流式响应不完整")
		}
		return out.String(), in, output, nil
	}
	var v struct {
		Status            string          `json:"status"`
		Error             json.RawMessage `json:"error"`
		IncompleteDetails json.RawMessage `json:"incomplete_details"`
		OutputText        string          `json:"output_text"`
		Choices           []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", 0, 0, chatResponseError{Code: "malformed_response", Message: "模型响应格式不兼容", Retryable: true}
	}
	if v.Status == "failed" || v.Status == "incomplete" || (len(v.Error) > 0 && string(v.Error) != "null") {
		return "", 0, 0, chatCompletionFailure(v.Status, v.Error, v.IncompleteDetails)
	}
	text := v.OutputText
	if len(v.Choices) > 0 {
		if finish := v.Choices[0].FinishReason; finish == "length" || finish == "content_filter" {
			details, _ := json.Marshal(map[string]string{"reason": finish})
			return "", 0, 0, chatCompletionFailure("incomplete", nil, details)
		}
		text = v.Choices[0].Message.Content
	}
	if text == "" {
		for _, o := range v.Output {
			for _, p := range o.Content {
				if p.Type == "output_text" {
					text += p.Text
				}
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", 0, 0, chatResponseError{Code: "empty_response", Message: "模型没有返回文本", Retryable: true}
	}
	if v.Usage.Input == 0 {
		v.Usage.Input = v.Usage.Prompt
	}
	if v.Usage.Output == 0 {
		v.Usage.Output = v.Usage.Completion
	}
	return text, v.Usage.Input, v.Usage.Output, nil
}

func (s *Service) generateImage(ctx context.Context, c Config, prompt string) ([]byte, error) {
	if err := checkModelBudget(ctx, minimumImageBudget); err != nil {
		return nil, err
	}
	ctx, cancel, err := modelContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	base, key := imageConnection(c)
	raw, _, err := s.request(ctx, "POST", endpoint(base, "/images/generations"), key, map[string]any{"model": c.ImageModel, "prompt": prompt, "n": 1, "size": c.ImageSize}, 32<<20)
	if err != nil {
		return nil, err
	}
	return imageResult(ctx, raw)
}

func imageConnection(c Config) (string, string) {
	base, key := c.BaseURL, c.APIKey
	if c.ImageBaseURL != "" {
		base = c.ImageBaseURL
		key = c.ImageAPIKey
	} else if c.ImageAPIKey != "" {
		key = c.ImageAPIKey
	}
	return base, key
}

func imageResult(ctx context.Context, raw []byte) ([]byte, error) {
	var v struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Data) == 0 {
		return nil, errors.New("生图接口没有返回图片")
	}
	var data []byte
	var err error
	if v.Data[0].B64 != "" {
		data, err = base64.StdEncoding.DecodeString(v.Data[0].B64)
	} else if v.Data[0].URL != "" {
		data, err = downloadImage(ctx, v.Data[0].URL)
	} else {
		return nil, errors.New("图片响应缺少 b64_json 或 url")
	}
	if err != nil {
		return nil, errors.New("获取生成图片失败")
	}
	return normalizeImage(data)
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	v4 := ip.To4()
	return v4 == nil || v4[0] != 100 || v4[1] < 64 || v4[1] > 127
}

func downloadImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return nil, errors.New("图片地址必须为公网 HTTPS")
	}
	tr := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("图片域名无法解析")
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, errors.New("图片地址禁止访问内网")
			}
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: noRedirect}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	if trustedMemeURL(rawURL) {
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Referer", "https://fabiaoqing.com/")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, errors.New("图片下载失败")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (20<<20)+1))
	if len(data) > 20<<20 {
		return nil, errors.New("图片过大")
	}
	return data, err
}
func normalizeImage(data []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, errors.New("图片尺寸或格式不受支持")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("图片解码失败")
	}
	if cfg.Width > 2048 || cfg.Height > 2048 {
		w, h := cfg.Width, cfg.Height
		if w >= h {
			h = h * 2048 / w
			w = 2048
		} else {
			w = w * 2048 / h
			h = 2048
		}
		dst := image.NewRGBA(image.Rect(0, 0, max(1, w), max(1, h)))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = dst
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	if out.Len() > 3<<20 {
		return nil, errors.New("图片压缩后仍超过 QQ 上传限制")
	}
	return out.Bytes(), nil
}

func (s *Service) token(ctx context.Context, c Config) (string, error) {
	s.onebot.mu.Lock()
	defer s.onebot.mu.Unlock()
	if s.onebot.conn == nil || s.onebot.conn.bot != c.AppID || s.onebot.conn.credential != codeHash(c.AppSecret) {
		return "", errors.New("NapCat 未连接")
	}
	return "onebot-connected", nil
}

func (s *Service) qqRequest(ctx context.Context, c Config, path string, body any) ([]byte, error) {
	return s.qqRequestMethod(ctx, c, http.MethodPost, path, body)
}
func (s *Service) qqRequestMethod(ctx context.Context, c Config, method, path string, body any) ([]byte, error) {
	return s.oneBotRequest(ctx, c, method, path, body)
}

func (e *qqRequestError) Error() string {
	return fmt.Sprintf("QQ HTTP %d / code %d", e.StatusCode, e.Code)
}

// The caller may prepare credentials before acquiring a final authorization lock.
func (s *Service) qqRequestMethodWithToken(ctx context.Context, c Config, token, method, path string, body any) ([]byte, error) {
	return s.oneBotRequest(ctx, c, method, path, body)
}

func (s *Service) send(ctx context.Context, c Config, j Job, text string, img []byte, seq int) error {
	if len(img) == 0 {
		text = normalizeReplyLineBreaks(text)
	}
	return s.sendFormatted(ctx, c, j, text, img, seq)
}

// sendFormatted also accepts chunks normalized before splitting. Normalizing
// those again would change HTML code in a chunk whose opening fence was in the
// previous message.
func (s *Service) sendFormatted(ctx context.Context, c Config, j Job, text string, img []byte, seq int) error {
	return s.sendFormattedWithKeyboard(ctx, c, j, text, img, seq, nil)
}

func (s *Service) sendFormattedWithKeyboard(ctx context.Context, c Config, j Job, text string, img []byte, seq int, keyboard *messageKeyboard) error {
	if j.PrivateReply {
		if err := s.checkPrivateReplyBinding(ctx, c, j); err != nil {
			return err
		}
		// Save the first result before delivery. Failure must not replay the
		// original financial operation or leak a private reply into group history.
		if _, err := s.db.ExecContext(ctx, `UPDATE qqc_jobs SET result_text=$2 WHERE id=$1`, j.ID, text); err != nil {
			return err
		}
	}
	if blocked, err := s.moderationBlocked(ctx, c, j.GroupID, j.MessageID); err != nil {
		return err
	} else if blocked {
		return errors.New("防刷屏已拦截原消息")
	}
	if time.Since(j.CreatedAt) >= passiveReplyWindow {
		return errReplyWindow
	}
	var raw []byte
	var err error
	hasImage := len(img) > 0
	if j.ReplyPage != nil && !j.TransientReply && !hasImage && s.pageOrigin != "" {
		raw, hasImage, err = s.sendReplyPage(ctx, c, j, text)
	} else if j.CommandReply && !j.TransientReply && !hasImage && c.CommandReplyFormat != "text" {
		raw, hasImage, err = s.sendCommandCards(ctx, c, j, text)
	} else {
		raw, err = s.sendOneBot(ctx, c, j, text, img)
	}
	if err != nil {
		return err
	}
	if j.TransientReply {
		return nil
	}
	if j.PrivateReply {
		return nil
	}
	if err = s.recordAutonomousReply(ctx, c, j); err != nil {
		return errors.New("消息已发送，但自主接话状态保存失败")
	}
	shareable := (j.Kind == "chat" || j.Kind == "ambient_chat" || j.Kind == "image") && j.ShareHistory && j.Error == ""
	return s.rememberSentMessage(ctx, c, j, raw, text, hasImage, shareable)
}

func limitText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func (s *Service) Probe(ctx context.Context, kind string) (string, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return "", err
	}
	if kind == "qq" {
		_, err = s.token(ctx, c)
		return "NapCat 已连接；请在测试群验证指令与管理员权限。", err
	}
	if kind == "image" || kind == "image_edit" {
		base, key := imageConnection(c)
		if base == "" || key == "" || c.ImageModel == "" {
			return "", errors.New("请先保存生图连接和模型")
		}
	} else if c.BaseURL == "" || c.APIKey == "" || c.ChatModel == "" {
		return "", errors.New("请先保存聊天连接和模型")
	}
	if kind == "image_edit" {
		_, err = s.editImage(ctx, c, "Change this blue square to red; retain its dimensions.", probeImage())
		return "图片编辑接口测试成功（已调用付费接口，不发送到群）。", err
	}
	if kind == "vision" {
		marker, expected, err := visualMarker()
		if err != nil {
			return "", err
		}
		result, _, _, err := s.chat(ctx, c, []Message{{Role: "user", Content: "Read the characters in the attached image. Return only those characters, without any other text.", Images: []string{marker}}})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(result) != expected {
			return "", errors.New("视觉识别校验未通过；当前模型可能未接收图片，请检查模型与网关")
		}
		return "视觉识别校验通过（随机图片字符识别成功，未发送群消息）。", nil
	}
	if kind == "tools" {
		c.ImageEnabled = true
		result, err := s.chatTurn(ctx, c, []Message{{Role: "user", Content: "Call reply_text with the text OK. Do not generate or edit any image."}})
		if err == nil && result.Text == "" {
			err = errors.New("工具测试未返回 reply_text 操作")
		}
		return "工具调用测试成功（仅验证工具协议，不执行生图）。", err
	}
	if kind == "image" {
		_, err = s.generateImage(ctx, c, "A small blue circle on a white background, minimal illustration.")
		return "生图测试成功（使用站长 API Key，可能产生费用）。", err
	}
	if kind != "chat" {
		return "", errors.New("未知测试类型")
	}
	result, _, _, err := s.chat(ctx, c, []Message{{Role: "user", Content: "Reply with OK."}})
	return limitText(result, 300), err
}

func probeImage() string {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 30, G: 90, B: 240, A: 255})
		}
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

type qqRequestError struct {
	StatusCode int
	Code       int
}
