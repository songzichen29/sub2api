// Package usage_provider 提供第三方面板（如 newapi、oneapi 等）账号用量查询的统一抽象。
//
// 当账号 type 为 apikey 且开启了第三方用量查询时，AccountUsageService 会
// 通过本包定义的 Provider 接口获取上游面板暴露的额度/已用/剩余等信息，
// 而不再走 OAuth profile scope 路径。
package usage_provider

import (
	"context"
	"errors"
	"fmt"
)

// ProviderType 第三方面板提供商类型。
//
// 支持独立面板凭据的 newapi，以及复用账号上游凭据的 sub2api。
type ProviderType string

const (
	ProviderNewAPI  ProviderType = "newapi"
	ProviderSub2API ProviderType = "sub2api"
)

// Config 第三方面板查询所需配置（解密后的明文凭据）。
type Config struct {
	Provider    ProviderType
	BaseURL     string // 例如 https://newapi.example.com
	AccessToken string // Bearer token（已解密）
	UserID      string // 上游用户 ID（newapi 通过 New-Api-User header 携带）
}

// Validate 基础校验，避免未填写关键字段导致下游 HTTP 报错难以定位。
func (c Config) Validate() error {
	if c.Provider == "" {
		return errors.New("provider is required")
	}
	if c.BaseURL == "" {
		return errors.New("base_url is required")
	}
	if c.AccessToken == "" {
		return errors.New("access_token is required")
	}
	if c.Provider == ProviderNewAPI && c.UserID == "" {
		return errors.New("user_id is required")
	}
	return nil
}

// QuotaInfo 第三方面板返回的额度信息（已规整化为 USD 单位）。
//
// 字段语义参考 newapi /api/user/self 响应，其它 Provider 适配时若没有
// 完全对应字段，应尽力做合理映射；缺失字段允许为零值。
type QuotaInfo struct {
	PlanName    string  `json:"plan_name"`            // 套餐名/分组名（如 "默认套餐"、"VIP"）
	Remaining   float64 `json:"remaining"`            // 剩余额度
	Used        float64 `json:"used"`                 // 已用额度
	Total       float64 `json:"total"`                // 总额（remaining + used）
	Unit        string  `json:"unit"`                 // 单位（USD / CNY / quota 等）
	Utilization float64 `json:"utilization"`          // 使用率 0~1，由 used/total 计算
	TotalKnown  bool    `json:"total_known"`          // false 表示上游只提供当前余额，无法可靠推导历史总额
	UpdatedAt   int64   `json:"updated_at,omitempty"` // 拉取时间戳（毫秒，可选）
}

// Provider 第三方面板用量查询的统一抽象。
//
// 实现需保证：
//   - 网络错误、HTTP 非 2xx、解析失败 等都返回非 nil error；
//   - 上游业务级失败（如 success=false）应返回错误，便于上层做负缓存；
//   - 不应在内部做缓存，缓存由 AccountUsageService 统一处理。
type Provider interface {
	Type() ProviderType
	Fetch(ctx context.Context, cfg Config) (*QuotaInfo, error)
}

// New 工厂函数：根据 ProviderType 构造对应实现。
//
// 不支持的取值返回 ErrUnsupportedProvider。
func New(provider ProviderType) (Provider, error) {
	switch provider {
	case ProviderNewAPI:
		return NewNewAPIProvider(), nil
	case ProviderSub2API:
		return NewSub2APIProvider(), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, provider)
	}
}

// ErrUnsupportedProvider 不支持的提供商类型。
var ErrUnsupportedProvider = errors.New("unsupported usage provider")
