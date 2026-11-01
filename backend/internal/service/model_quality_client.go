package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const qualityMaxResponseBytes = 4 << 20
const qualityMaxArtifactBytes = 256 << 10

type qualityCompletionRequest struct {
	Model, Effort, ServiceTier, Instructions, Input string
	Schema                                          map[string]any
	MaxOutputTokens                                 int
}

type qualityCompletion struct {
	Text                                       string
	DurationMS                                 int64
	InputTokens, OutputTokens, ReasoningTokens *int64
	ServiceTier                                string
}

type qualityCompletionClient interface {
	Complete(context.Context, qualityCredentials, qualityCompletionRequest) (*qualityCompletion, error)
}

type qualityModelLister interface {
	Models(context.Context, qualityCredentials) ([]string, error)
}

// Read-only suggestions from the same gateway/key that will perform judging.
// A successful model listing is never used as a substitute for a real probe.
func (c *qualityHTTPClient) Models(ctx context.Context, credential qualityCredentials) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(credential.baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, errors.New("评分模型列表地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+credential.key)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("读取评分模型列表失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &qualityHTTPStatusError{StatusCode: resp.StatusCode}
	}
	const maxBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err != nil || len(data) > maxBytes || json.Unmarshal(data, &payload) != nil || payload.Data == nil {
		return nil, errors.New("评分模型列表响应无效")
	}
	models := make([]string, 0, len(payload.Data))
	seen := map[string]bool{}
	for _, entry := range payload.Data {
		model := strings.TrimSpace(entry.ID)
		if validateQualityJudgeModel(model) != nil || seen[model] || (credential.key != "" && strings.Contains(model, credential.key)) {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

type qualityHTTPClient struct{ client *http.Client }

// Only the numeric status is safe to persist; upstream bodies may echo keys.
type qualityHTTPStatusError struct{ StatusCode int }

func (e *qualityHTTPStatusError) Error() string {
	return fmt.Sprintf("检测接口返回 HTTP %d，未自动重发", e.StatusCode)
}

func newQualityHTTPClient(local bool) *qualityHTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Never send monitor credentials through a process-wide proxy. The generator
	// uses the existing monitor DNS/IP guard; the judge has a fixed loopback URL.
	transport.Proxy = nil
	if !local {
		transport.DialContext = safeDialContext
	}
	return &qualityHTTPClient{client: &http.Client{Transport: transport,
		Timeout:       8 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *qualityHTTPClient) Complete(ctx context.Context, credential qualityCredentials, in qualityCompletionRequest) (*qualityCompletion, error) {
	payload := map[string]any{
		"model": in.Model, "stream": true, "store": false,
		"input":             []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": in.Input}}}},
		"max_output_tokens": in.MaxOutputTokens,
	}
	if in.Instructions != "" {
		payload["instructions"] = in.Instructions
	}
	if in.Effort != "" {
		payload["reasoning"] = map[string]any{"effort": in.Effort}
	}
	if in.ServiceTier != "" {
		payload["service_tier"] = in.ServiceTier
	}
	if in.Schema != nil {
		payload["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "quality_assessment", "strict": true, "schema": in.Schema}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("无法编码检测请求")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(credential.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("检测接口地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+credential.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("检测请求中断或超时；结果不确定，未自动重发")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Bodies and URLs may echo secrets. Never persist/log them.
		return nil, &qualityHTTPStatusError{StatusCode: resp.StatusCode}
	}
	data, err := readQualityResponse(resp)
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(data) || gjson.GetBytes(data, "status").String() != "completed" || gjson.GetBytes(data, "error").IsObject() {
		return nil, errors.New("模型输出未完整结束，不能作为降智判定依据")
	}
	text := extractOpenAIResponsesText(data)
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("模型没有返回可判定的正文")
	}
	if len(text) > qualityMaxArtifactBytes {
		return nil, errors.New("模型正文超过检测上限，未截断后评分")
	}
	usage := func(path string) *int64 {
		v := gjson.GetBytes(data, path)
		if v.Type != gjson.Number || v.Int() < 0 {
			return nil
		}
		n := v.Int()
		return &n
	}
	return &qualityCompletion{Text: text, DurationMS: time.Since(start).Milliseconds(),
		InputTokens: usage("usage.input_tokens"), OutputTokens: usage("usage.output_tokens"), ReasoningTokens: usage("usage.output_tokens_details.reasoning_tokens"), ServiceTier: gjson.GetBytes(data, "service_tier").String()}, nil
}

func readQualityResponse(resp *http.Response) ([]byte, error) {
	limited := &io.LimitedReader{R: resp.Body, N: qualityMaxResponseBytes + 1}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		data, err := io.ReadAll(limited)
		if err != nil {
			return nil, errors.New("读取模型响应失败")
		}
		if len(data) > qualityMaxResponseBytes {
			return nil, errors.New("模型响应超过大小上限")
		}
		return data, nil
	}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), qualityMaxResponseBytes+1)
	var event strings.Builder
	consume := func() ([]byte, error) {
		data := strings.TrimSpace(event.String())
		event.Reset()
		if data == "" || data == "[DONE]" {
			return nil, nil
		}
		if !gjson.Valid(data) {
			return nil, errors.New("模型返回无效流事件")
		}
		switch gjson.Get(data, "type").String() {
		case "response.completed":
			return []byte(gjson.Get(data, "response").Raw), nil
		case "error", "response.failed", "response.incomplete":
			return nil, errors.New("模型流未成功完成，不能评分")
		}
		return nil, nil
	}
	for scanner.Scan() {
		if limited.N == 0 {
			return nil, errors.New("模型响应超过大小上限")
		}
		line := scanner.Text()
		if line == "" {
			if data, err := consume(); err != nil || data != nil {
				return data, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			event.WriteString(strings.TrimPrefix(line, "data:"))
			event.WriteByte('\n')
		}
	}
	if scanner.Err() != nil {
		return nil, errors.New("模型流读取失败或超过大小上限")
	}
	if data, err := consume(); err != nil || data != nil {
		return data, err
	}
	return nil, errors.New("模型流缺少完整结束事件，未对残缺输出评分")
}
