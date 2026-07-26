//go:build unit

// 验证 step 2 退出信号：Account.Tags 经 NormalizeAccountTags 处理后落到 service 层、
// 再通过 GetByID 读出来形态一致；Update 用指针区分"未提供"和"清空"语义。
//
// 这里的 stub 是 AccountRepository 的极简内存实现：只支持 Create + GetByID + Update。
// 其他方法触发 panic 防止被误用。

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// inMemoryAccountRepo 是 AccountRepository 接口的内存实现，仅覆盖 tags 相关测试需要的方法。
type inMemoryAccountRepo struct {
	store  map[int64]*Account
	nextID int64
}

func newInMemoryAccountRepo() *inMemoryAccountRepo {
	return &inMemoryAccountRepo{store: map[int64]*Account{}}
}

func (r *inMemoryAccountRepo) Create(ctx context.Context, account *Account) error {
	r.nextID++
	account.ID = r.nextID
	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()
	clone := *account
	// 复制 Tags 切片避免外部修改污染存储
	if account.Tags != nil {
		clone.Tags = append([]string{}, account.Tags...)
	}
	r.store[account.ID] = &clone
	return nil
}

func (r *inMemoryAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, ok := r.store[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	clone := *a
	if a.Tags != nil {
		clone.Tags = append([]string{}, a.Tags...)
	}
	return &clone, nil
}

func (r *inMemoryAccountRepo) Update(ctx context.Context, account *Account) error {
	if _, ok := r.store[account.ID]; !ok {
		return ErrAccountNotFound
	}
	clone := *account
	if account.Tags != nil {
		clone.Tags = append([]string{}, account.Tags...)
	}
	clone.UpdatedAt = time.Now()
	r.store[account.ID] = &clone
	return nil
}

// 以下方法本测试不会调用，触发 panic 防止误用。
func (r *inMemoryAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) Delete(ctx context.Context, id int64) error { panic("unexpected") }
func (r *inMemoryAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string, tags []string) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string, tags []string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListAllTags(ctx context.Context) ([]string, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListActive(ctx context.Context) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ClearError(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
}
func (r *inMemoryAccountRepo) ListSchedulable(ctx context.Context) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ClearRateLimit(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}
func (r *inMemoryAccountRepo) ResetQuotaUsed(ctx context.Context, id int64) error { return nil }
func (r *inMemoryAccountRepo) RevertProxyFallback(ctx context.Context, accountID int64) error {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListOAuthRefreshCandidates(ctx context.Context) ([]Account, error) {
	panic("unexpected")
}
func (r *inMemoryAccountRepo) ListShadowsByParent(ctx context.Context, parentID int64) ([]*Account, error) {
	panic("unexpected")
}

// TestAccountService_CreateThenRead_TagsNormalized 是 step 2 主退出信号——
// "创建账号附带 tags 后再读取" 闭环：输入 ["VIP", " prod ", "vip"] 经 service 规范化后
// 落到 account.Tags 应该是 ["prod", "vip"]，再读出来形态一致。
func TestAccountService_CreateThenRead_TagsNormalized(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc := &AccountService{accountRepo: repo}

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:     "test-acc",
		Platform: "anthropic",
		Type:     "oauth",
		Tags:     []string{"VIP", " prod ", "vip"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"prod", "vip"}, created.Tags)

	got, err := svc.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"prod", "vip"}, got.Tags)
}

// TestAccountService_Update_TagsNilDoesNotChange 验证 Update 时 tags 字段为 nil 时不修改。
func TestAccountService_Update_TagsNilDoesNotChange(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc := &AccountService{accountRepo: repo}

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:     "test-acc",
		Platform: "anthropic",
		Type:     "oauth",
		Tags:     []string{"vip"},
	})
	require.NoError(t, err)

	// 不传 Tags 字段（nil 指针），其他字段照改
	newName := "renamed"
	updated, err := svc.Update(context.Background(), created.ID, UpdateAccountRequest{
		Name: &newName,
		Tags: nil,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.Equal(t, []string{"vip"}, updated.Tags) // 标签保持不变
}

// TestAccountService_Update_TagsEmptyClears 验证 Update 时 tags 为 *[]string{} 时清空标签。
func TestAccountService_Update_TagsEmptyClears(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc := &AccountService{accountRepo: repo}

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:     "test-acc",
		Platform: "anthropic",
		Type:     "oauth",
		Tags:     []string{"vip", "prod"},
	})
	require.NoError(t, err)

	// 显式传空数组指针 → 清空所有标签
	emptyTags := []string{}
	updated, err := svc.Update(context.Background(), created.ID, UpdateAccountRequest{
		Tags: &emptyTags,
	})
	require.NoError(t, err)
	require.Empty(t, updated.Tags)
}

// TestAccountService_Update_TagsReplace 验证 Update 时 tags 为非空指针时替换标签。
func TestAccountService_Update_TagsReplace(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc := &AccountService{accountRepo: repo}

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:     "test-acc",
		Platform: "anthropic",
		Type:     "oauth",
		Tags:     []string{"vip"},
	})
	require.NoError(t, err)

	newTags := []string{"TEST", "  staging  "}
	updated, err := svc.Update(context.Background(), created.ID, UpdateAccountRequest{
		Tags: &newTags,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"staging", "test"}, updated.Tags) // 规范化后字典序
}

// TestAccountService_Create_InvalidTagsRejected 验证非法标签会直接返回错误而不落库。
func TestAccountService_Create_InvalidTagsRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc := &AccountService{accountRepo: repo}

	_, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:     "test-acc",
		Platform: "anthropic",
		Type:     "oauth",
		Tags:     []string{"vip!"}, // 非法字符集
	})
	require.Error(t, err)
	require.Empty(t, repo.store) // 落库应为空（早期校验失败）
}
