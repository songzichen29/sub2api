package usage_provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// newapi 协议常量
const (
	// newAPIDefaultUserAgent 与用户提供脚本一致，便于上游识别调用方
	newAPIDefaultUserAgent = "cc-switch/1.0"

	// newAPIQuotaDivisor newapi 服务端把 quota 单位放大了 500000 倍存储，
	// 对外暴露时需要除以该系数换算为 USD。
	newAPIQuotaDivisor = 500000.0

	// newAPIDefaultTimeout newapi 接口默认超时
	newAPIDefaultTimeout = 15 * time.Second

	// newAPIUserSelfPath GET 用户自身信息的 endpoint
	newAPIUserSelfPath = "/api/user/self"
)

// newAPIProvider 实现 Provider 接口，对接 newapi 面板。
type newAPIProvider struct {
	// httpClient 允许测试注入自定义 client；nil 时按生产配置获取。
	httpClient *http.Client
}

// NewNewAPIProvider 构造 newapi Provider 实例（无状态，可全局复用）。
func NewNewAPIProvider() Provider {
	return &newAPIProvider{}
}

// NewNewAPIProviderWithClient 仅用于测试：注入自定义 *http.Client，
// 绕开生产环境的私网地址校验等限制。
func NewNewAPIProviderWithClient(c *http.Client) Provider {
	return &newAPIProvider{httpClient: c}
}

func (p *newAPIProvider) Type() ProviderType { return ProviderNewAPI }

// newAPIResponse newapi /api/user/self 响应结构。
//
// 仅声明本特性使用到的字段，其余字段忽略以保持向后兼容。
type newAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    *struct {
		Group     string  `json:"group"`
		Quota     float64 `json:"quota"`      // 当前剩余配额（放大单位）
		UsedQuota float64 `json:"used_quota"` // 历史已用配额（放大单位）
	} `json:"data"`
}

// Fetch 实现 Provider 接口：调 newapi /api/user/self，按用户提供的 extractor 规则
// 把 quota / used_quota 换算为 USD（除以 500000）后填充 QuotaInfo。
func (p *newAPIProvider) Fetch(ctx context.Context, cfg Config) (*QuotaInfo, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + newAPIUserSelfPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// header 与用户脚本一致
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	req.Header.Set("User-Agent", newAPIDefaultUserAgent)
	req.Header.Set("New-Api-User", cfg.UserID)

	client := p.httpClient
	if client == nil {
		var clientErr error
		client, clientErr = httpclient.GetClient(httpclient.Options{
			Timeout:            newAPIDefaultTimeout,
			ValidateResolvedIP: true,
		})
		if clientErr != nil {
			return nil, fmt.Errorf("create http client failed: %w", clientErr)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request newapi failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read newapi response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newapi returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed newAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode newapi response failed: %w", err)
	}

	if !parsed.Success || parsed.Data == nil {
		msg := strings.TrimSpace(parsed.Message)
		if msg == "" {
			msg = "newapi returned success=false"
		}
		return nil, fmt.Errorf("newapi business error: %s", msg)
	}

	used := parsed.Data.UsedQuota / newAPIQuotaDivisor
	remaining := parsed.Data.Quota / newAPIQuotaDivisor
	total := remaining + used

	utilization := 0.0
	if total > 0 {
		utilization = used / total
		if utilization < 0 {
			utilization = 0
		} else if utilization > 1 {
			utilization = 1
		}
	}

	planName := strings.TrimSpace(parsed.Data.Group)
	if planName == "" {
		planName = "默认套餐"
	}

	return &QuotaInfo{
		PlanName:    planName,
		Remaining:   remaining,
		Used:        used,
		Total:       total,
		Unit:        "USD",
		Utilization: utilization,
		TotalKnown:  true,
		UpdatedAt:   time.Now().UnixMilli(),
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
