package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 构造一个仅承载本测试所需账号的 handler + router。
// 复用 stubAdminService 的 ListAllAccountTags 默认实现：聚合 accounts.Tags 后去重排序。
func newAccountHandlerWithAccounts(accounts []service.Account) (*AccountHandler, *stubAdminService, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	stub := newStubAdminService()
	stub.accounts = accounts
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	// 路由注册顺序与 internal/server/routes/admin.go 保持一致：/tags 在 /:id 之前
	router.GET("/api/v1/admin/accounts", h.List)
	router.GET("/api/v1/admin/accounts/tags", h.ListTags)
	return h, stub, router
}

// TestAccountHandler_ListTags_ReturnsDedupedSorted 覆盖测试约束：
// "ListAllTags 测试：软删除账号标签不出现、不同账号相同标签去重、返回字典序"
// handler 层只能验证去重和字典序——软删除是 repo 层职责，已在 repo 集成测试覆盖。
func TestAccountHandler_ListTags_ReturnsDedupedSorted(t *testing.T) {
	accounts := []service.Account{
		{ID: 1, Tags: []string{"vip", "prod"}},
		{ID: 2, Tags: []string{"prod", "test"}}, // prod 与账号 1 重复
		{ID: 3, Tags: []string{"vip", "alpha"}}, // vip 与账号 1 重复
		{ID: 4, Tags: nil},                      // 无标签账号
	}
	_, _, router := newAccountHandlerWithAccounts(accounts)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/tags", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Tags []string `json:"tags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"alpha", "prod", "test", "vip"}, resp.Data.Tags,
		"tags 必须按字典序去重返回")
}

// TestAccountHandler_ListTags_EmptyReturnsEmptyArray 验证零账号场景下返回 [] 而非 null。
// 前端 chip 渲染依赖该约束，避免 v-for 报错。
func TestAccountHandler_ListTags_EmptyReturnsEmptyArray(t *testing.T) {
	_, _, router := newAccountHandlerWithAccounts(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/tags", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// 直接断言 JSON 文本中不出现 "null"（覆盖 handler 内 nil → []string{} 兜底逻辑）
	require.Contains(t, rec.Body.String(), `"tags":[]`)
	require.NotContains(t, rec.Body.String(), `"tags":null`)
}

// TestAccountHandler_List_TagsQueryParsing 覆盖测试约束：
// "Handler 解析 query 参数测试：?tags=vip&tags=prod、?tags=、?tags=VIP（大写兼容）三种入参各一例"
//
// 契约：len(wantTags)==0 表示"不过滤"，nil 和 []string{} 在 repo 层等价处理，
// 因此用 require.Empty 而非严格 nil 比较。
func TestAccountHandler_List_TagsQueryParsing(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantTags []string // nil 或空切片均表示不过滤
	}{
		{
			name:     "multi value OR",
			query:    "?tags=vip&tags=prod",
			wantTags: []string{"vip", "prod"},
		},
		{
			name:     "empty value treated as no filter",
			query:    "?tags=",
			wantTags: nil,
		},
		{
			name:     "uppercase normalized to lowercase",
			query:    "?tags=VIP",
			wantTags: []string{"vip"},
		},
		{
			name:     "missing param treated as no filter",
			query:    "",
			wantTags: nil,
		},
		{
			name:     "duplicate values deduped",
			query:    "?tags=vip&tags=VIP&tags=vip",
			wantTags: []string{"vip"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stub, router := newAccountHandlerWithAccounts(nil)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts"+tc.query, nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
			require.Equal(t, 1, stub.lastListAccounts.calls, "ListAccounts 应被调用一次")
			if len(tc.wantTags) == 0 {
				require.Empty(t, stub.lastListAccounts.tags,
					"未提供或空值入参时 service 应收到空 tags（不过滤）")
			} else {
				require.Equal(t, tc.wantTags, stub.lastListAccounts.tags,
					"service 收到的 tags 与期望不符")
			}
		})
	}
}
