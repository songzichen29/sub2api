package routes

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestCommonRoutesDoNotConflictWithGatewayProxies 是 1f84ee30 引入回归的防回归测试：
// common 兼容层不得注册 /api/features/:clientKey 与 /api/claude_cli/bootstrap——这两条路径
// 已由 gateway.go 的 FeaturesProxy / BootstrapProxy 认证代理接管。若 common 重复注册，
// Gin radix tree 在同前缀 :clientKey 与 :key 参数名不一致（或 bootstrap 路径重复）时会在
// 注册阶段 panic，导致服务启动失败（生产曾触发：gateway.go:147 panic）。
//
// 本测试复刻生产启动顺序——先 common 后 gateway 叠加注册到同一 engine——并断言：
//  1. 注册阶段不再 panic；
//  2. gateway 代理路由存在且未被 common stub 覆盖；
//  3. common 保留的 GrowthBook SDK 兼容端点（/api/eval）仍可用。
func TestCommonRoutesDoNotConflictWithGatewayProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)
	attachGatewayRoutesForTest(router) // 回归点：若存在路由冲突，此处 panic

	seen := make(map[string]bool)
	for _, r := range router.Routes() {
		seen[r.Method+" "+r.Path] = true
	}

	// gateway 认证代理路由存在，未被 common 兼容层覆盖。
	require.True(t, seen["GET /api/features/:key"],
		"gateway FeaturesProxy 代理路由 GET /api/features/:key 应存在")
	require.True(t, seen["GET /api/claude_cli/bootstrap"],
		"gateway BootstrapProxy 代理路由 GET /api/claude_cli/bootstrap 应存在")

	// common 兼容层不得注册与 gateway features 代理冲突的 :clientKey 参数前缀。
	require.False(t, seen["GET /api/features/:clientKey"],
		"common 不得注册 GET /api/features/:clientKey（与 gateway :key 参数名冲突）")

	// common 保留的 GrowthBook SDK 兼容端点仍存在（无认证 stub，不与 gateway 冲突）。
	require.True(t, seen["POST /api/eval/:clientKey"],
		"common 应保留 POST /api/eval/:clientKey SDK 兼容端点")
}
