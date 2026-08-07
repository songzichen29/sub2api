package usage_provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	sub2APIUsagePath      = "/v1/usage"
	sub2APIDefaultTimeout = 15 * time.Second
)

type sub2APIProvider struct {
	httpClient *http.Client
}

func NewSub2APIProvider() Provider {
	return &sub2APIProvider{}
}

func NewSub2APIProviderWithClient(client *http.Client) Provider {
	return &sub2APIProvider{httpClient: client}
}

func (p *sub2APIProvider) Type() ProviderType { return ProviderSub2API }

type sub2APIUsageResponse struct {
	Mode      string   `json:"mode"`
	IsValid   *bool    `json:"isValid"`
	Status    string   `json:"status"`
	PlanName  string   `json:"planName"`
	Remaining *float64 `json:"remaining"`
	Balance   *float64 `json:"balance"`
	Unit      string   `json:"unit"`
	Quota     *struct {
		Limit     float64 `json:"limit"`
		Used      float64 `json:"used"`
		Remaining float64 `json:"remaining"`
		Unit      string  `json:"unit"`
	} `json:"quota"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *sub2APIProvider) Fetch(ctx context.Context, cfg Config) (*QuotaInfo, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	endpoint := buildSub2APIUsageURL(cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)

	client := p.httpClient
	if client == nil {
		client, err = httpclient.GetClient(httpclient.Options{
			Timeout:            sub2APIDefaultTimeout,
			ValidateResolvedIP: true,
		})
		if err != nil {
			return nil, fmt.Errorf("create http client failed: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request sub2api failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read sub2api response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sub2api returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed sub2APIUsageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode sub2api response failed: %w", err)
	}
	if parsed.Error != nil {
		message := strings.TrimSpace(parsed.Error.Message)
		if message == "" {
			message = strings.TrimSpace(parsed.Error.Type)
		}
		if message == "" {
			message = "unknown upstream error"
		}
		return nil, fmt.Errorf("sub2api business error: %s", message)
	}
	if parsed.IsValid != nil && !*parsed.IsValid {
		return nil, fmt.Errorf("sub2api API key is not valid (status=%s)", strings.TrimSpace(parsed.Status))
	}

	now := time.Now().UnixMilli()
	if parsed.Quota != nil {
		if err := validateSub2APIAmount("quota.limit", parsed.Quota.Limit); err != nil {
			return nil, err
		}
		if err := validateSub2APIAmount("quota.used", parsed.Quota.Used); err != nil {
			return nil, err
		}
		if err := validateSub2APIAmount("quota.remaining", parsed.Quota.Remaining); err != nil {
			return nil, err
		}

		total := parsed.Quota.Limit
		if total <= 0 {
			total = parsed.Quota.Used + parsed.Quota.Remaining
		}
		utilization := quotaUtilization(parsed.Quota.Used, total)
		unit := firstNonEmpty(parsed.Quota.Unit, parsed.Unit, "USD")
		planName := firstNonEmpty(parsed.PlanName, "API Key 额度")
		return &QuotaInfo{
			PlanName:    planName,
			Remaining:   parsed.Quota.Remaining,
			Used:        parsed.Quota.Used,
			Total:       total,
			Unit:        unit,
			Utilization: utilization,
			TotalKnown:  total > 0,
			UpdatedAt:   now,
		}, nil
	}

	remaining := parsed.Remaining
	if remaining == nil {
		remaining = parsed.Balance
	}
	if remaining == nil {
		return nil, fmt.Errorf("sub2api response does not contain balance or quota information")
	}
	if err := validateSub2APIAmount("remaining", *remaining); err != nil {
		return nil, err
	}

	return &QuotaInfo{
		PlanName:   firstNonEmpty(parsed.PlanName, "钱包余额"),
		Remaining:  *remaining,
		Unit:       firstNonEmpty(parsed.Unit, "USD"),
		TotalKnown: false,
		UpdatedAt:  now,
	}, nil
}

func buildSub2APIUsageURL(baseURL string) string {
	normalized := strings.TrimSpace(baseURL)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(normalized, "/") + sub2APIUsagePath
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(strings.ToLower(path), "/usage"):
	case hasAPIVersionSuffix(path):
		path += "/usage"
	default:
		path += sub2APIUsagePath
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}

func hasAPIVersionSuffix(path string) bool {
	segment := path
	if slash := strings.LastIndex(segment, "/"); slash >= 0 {
		segment = segment[slash+1:]
	}
	segment = strings.ToLower(strings.TrimSpace(segment))
	if len(segment) < 2 || segment[0] != 'v' || segment[1] < '0' || segment[1] > '9' {
		return false
	}
	for i := 2; i < len(segment); i++ {
		if segment[i] < '0' || segment[i] > '9' {
			return false
		}
	}
	return true
}

func validateSub2APIAmount(field string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return fmt.Errorf("sub2api returned invalid %s", field)
	}
	return nil
}

func quotaUtilization(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Max(0, math.Min(1, used/total))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
