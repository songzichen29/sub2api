package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultManxueOrigin = "https://manxue.ai"

// ManxueClient talks to the private task API. The origin is fixed in production;
// the alternate constructor is deliberately kept for httptest coverage.
type ManxueClient interface {
	Create(context.Context, ManxueCreateRequest) (*ManxueTask, error)
	Get(context.Context, string) (*ManxueTask, error)
	Cancel(context.Context, string) error
}

type ManxueCreateRequest struct {
	Benchmark       string `json:"benchmark"`
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
	HTML            string `json:"html,omitempty"`
	IdempotencyKey  string `json:"-"`
}

type ManxueTask struct {
	ID         string            `json:"id"`
	Benchmark  string            `json:"benchmark"`
	Status     string            `json:"status"`
	Phase      string            `json:"phase"`
	Error      json.RawMessage   `json:"error,omitempty"`
	Candy      *ManxueCandy      `json:"candy,omitempty"`
	Assessment *ManxueAssessment `json:"assessment,omitempty"`
	Result     json.RawMessage   `json:"result,omitempty"`
}

type ManxueCandy struct {
	Status          string    `json:"status"`
	Answer          string    `json:"answer,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	DurationMS      int64     `json:"duration_ms"`
	InputTokens     *int64    `json:"input_tokens,omitempty"`
	OutputTokens    *int64    `json:"output_tokens,omitempty"`
	ReasoningTokens *int64    `json:"reasoning_tokens,omitempty"`
	ServiceTier     string    `json:"service_tier,omitempty"`
}

type ManxueAssessment struct {
	Quality string `json:"quality"`
	Label   string `json:"label"`
	Reason  string `json:"reason"`
	Source  string `json:"source"`
}

type manxueHTTPError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *manxueHTTPError) Error() string {
	return fmt.Sprintf("manxue API returned HTTP %d: %s", e.Status, e.Message)
}

type manxueClient struct {
	origin     string
	httpClient *http.Client
	maxBody    int64
}

func NewManxueClient() ManxueClient {
	return newManxueClient(defaultManxueOrigin, &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	})
}

func newManxueClient(origin string, client *http.Client) ManxueClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &manxueClient{origin: strings.TrimRight(origin, "/"), httpClient: client, maxBody: 4 << 20}
}

func (c *manxueClient) Create(ctx context.Context, payload ManxueCreateRequest) (*ManxueTask, error) {
	if err := validateManxueRequest(payload); err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, "/api/v1/tests", payload, true)
}

func (c *manxueClient) Get(ctx context.Context, id string) (*ManxueTask, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return nil, errors.New("invalid manxue task id")
	}
	return c.do(ctx, http.MethodGet, "/api/v1/tests/"+url.PathEscape(id), nil, false)
}

func (c *manxueClient) Cancel(ctx context.Context, id string) error {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return errors.New("invalid manxue task id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.origin+"/api/v1/tests/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return c.readHTTPError(resp)
}

func (c *manxueClient) do(ctx context.Context, method, path string, payload any, accepted202 bool) (*ManxueTask, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if p, ok := payload.(ManxueCreateRequest); ok && p.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", p.IdempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || (accepted202 && resp.StatusCode != http.StatusAccepted) {
		return nil, c.readHTTPError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.maxBody {
		return nil, errors.New("manxue response is too large")
	}
	var task ManxueTask
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("decode manxue response: %w", err)
	}
	return &task, nil
}

func (c *manxueClient) readHTTPError(resp *http.Response) error {
	// Remote error bodies can echo credentials. Do not put them in errors/logs.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16<<10))
	msg := http.StatusText(resp.StatusCode)
	return &manxueHTTPError{Status: resp.StatusCode, Message: msg, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(min(seconds, 86400)) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func validateManxueRequest(p ManxueCreateRequest) error {
	switch p.Benchmark {
	case "candy", "pelican", "creative", "html":
	default:
		return errors.New("invalid manxue benchmark")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("manxue base_url must be a public HTTPS URL")
	}
	if p.APIKey == "" {
		return errors.New("manxue api_key is required")
	}
	if p.Benchmark != "candy" && p.Model != "gpt-6-astra" {
		return errors.New("manxue requires gpt-6-astra for this benchmark")
	}
	if p.Benchmark == "html" && len(p.HTML) == 0 {
		return errors.New("html benchmark requires html")
	}
	return nil
}
