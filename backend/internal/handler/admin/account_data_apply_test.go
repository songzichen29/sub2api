package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// applyImportApplyToAccount 是 feature 2026-05-06-account-import-apply 的核心 helper。
// 这一组测试覆盖 design 第 3.4 节"helper 行为"功能点的 7 个 case，
// 每条测试约束都对应一个独立用例。

func TestApplyImportApply_NilApply_NoOp(t *testing.T) {
	item := DataAccount{
		Name:        "acc",
		Platform:    "anthropic",
		Type:        "oauth",
		Concurrency: 3,
		Priority:    50,
		Tags:        []string{"x"},
		GroupIDs:    []int64{7},
		Credentials: map[string]any{"access_token": "tok"},
	}
	original := item // 拷贝（slice / map 共享底层，但本测试只比较没写入的字段）

	resolved := applyImportApplyToAccount(&item, nil)
	require.Nil(t, resolved)
	require.Equal(t, original.Concurrency, item.Concurrency)
	require.Equal(t, original.Priority, item.Priority)
	require.Equal(t, original.Tags, item.Tags)
	require.Equal(t, original.GroupIDs, item.GroupIDs)
	require.Equal(t, original.Credentials, item.Credentials)
}

func TestApplyImportApply_Tags_Overrides(t *testing.T) {
	item := DataAccount{Tags: []string{"legacy"}}
	newTags := []string{"vip", "prod"}
	apply := &DataImportApply{Tags: &newTags}

	resolved := applyImportApplyToAccount(&item, apply)
	require.Nil(t, resolved)
	require.Equal(t, []string{"vip", "prod"}, item.Tags)

	// 显式清空场景：传 [] 应该把 item.Tags 设为空 slice
	emptyTags := []string{}
	apply2 := &DataImportApply{Tags: &emptyTags}
	resolved2 := applyImportApplyToAccount(&item, apply2)
	require.Nil(t, resolved2)
	require.Equal(t, []string{}, item.Tags)
}

func TestApplyImportApply_GroupIDs_OverridesEmptyArray(t *testing.T) {
	item := DataAccount{GroupIDs: []int64{1, 2, 3}}

	// 显式空数组：不绑任何分组
	emptyGroups := []int64{}
	apply := &DataImportApply{GroupIDs: &emptyGroups}
	resolved := applyImportApplyToAccount(&item, apply)
	require.Nil(t, resolved)
	require.Equal(t, []int64{}, item.GroupIDs)

	// 非空数组：覆盖
	newGroups := []int64{5, 7}
	apply2 := &DataImportApply{GroupIDs: &newGroups}
	resolved2 := applyImportApplyToAccount(&item, apply2)
	require.Nil(t, resolved2)
	require.Equal(t, []int64{5, 7}, item.GroupIDs)
}

func TestApplyImportApply_ProxyID_ReturnsResolvedID(t *testing.T) {
	fileProxyKey := "http|1.2.3.4|8080|u|p"
	item := DataAccount{ProxyKey: &fileProxyKey}

	// Apply 启用 ProxyID=42 → 返回 &42 并清空 item.ProxyKey
	pid := int64(42)
	apply := &DataImportApply{ProxyID: &pid}
	resolved := applyImportApplyToAccount(&item, apply)
	require.NotNil(t, resolved)
	require.Equal(t, int64(42), *resolved)
	require.Nil(t, item.ProxyKey, "Apply.ProxyID 启用后必须显式置空 item.ProxyKey")

	// ProxyID=0 → 返回 &0（"显式不绑代理"语义，由上层 handler 处理）
	zero := int64(0)
	apply2 := &DataImportApply{ProxyID: &zero}
	item2 := DataAccount{ProxyKey: &fileProxyKey}
	resolved2 := applyImportApplyToAccount(&item2, apply2)
	require.NotNil(t, resolved2)
	require.Equal(t, int64(0), *resolved2)
	require.Nil(t, item2.ProxyKey)
}

func TestApplyImportApply_ConcurrencyPriority(t *testing.T) {
	item := DataAccount{Concurrency: 3, Priority: 50}
	c := 10
	p := 1
	apply := &DataImportApply{Concurrency: &c, Priority: &p}

	resolved := applyImportApplyToAccount(&item, apply)
	require.Nil(t, resolved)
	require.Equal(t, 10, item.Concurrency)
	require.Equal(t, 1, item.Priority)
}

func TestApplyImportApply_ModelMapping_PreservesOtherCredentials(t *testing.T) {
	item := DataAccount{
		Credentials: map[string]any{
			"access_token":  "tok-secret",
			"refresh_token": "ref-secret",
			"model_mapping": map[string]string{"old": "old"},
		},
	}
	mapping := map[string]string{"claude-3-5-sonnet-20241022": "claude-3-5-sonnet-20241022"}
	apply := &DataImportApply{ModelMapping: &mapping}

	resolved := applyImportApplyToAccount(&item, apply)
	require.Nil(t, resolved)

	// 关键约束（design 3.2.4）：写 model_mapping 不能破坏 credentials 其他键
	require.Equal(t, "tok-secret", item.Credentials["access_token"])
	require.Equal(t, "ref-secret", item.Credentials["refresh_token"])
	// model_mapping 已被覆盖
	require.Equal(t, mapping, item.Credentials["model_mapping"])
}

func TestApplyImportApply_NilCredentials_InitializesMap(t *testing.T) {
	item := DataAccount{Credentials: nil}
	mapping := map[string]string{"a": "b"}
	apply := &DataImportApply{ModelMapping: &mapping}

	require.NotPanics(t, func() {
		applyImportApplyToAccount(&item, apply)
	})
	require.NotNil(t, item.Credentials, "Credentials==nil 时 helper 必须先初始化为空 map")
	require.Equal(t, mapping, item.Credentials["model_mapping"])
}
