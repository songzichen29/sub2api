//go:build integration

package repository

import (
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// mustInsertUsageLogForSubscription 直接通过原生 SQL 插入一条 usage_logs，
// 用于驱动 last_used_at 聚合查询。复用与 user_repo_sort_integration_test.go 同款写法。
func (s *UserSubscriptionRepoSuite) mustInsertUsageLogForSubscription(userID int64, subscriptionID int64, createdAt time.Time) {
	s.T().Helper()

	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sub-usage-log-account"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: userID})

	sqlq := sqlExecutorFromEntClient(s.client)
	s.Require().NotNil(sqlq, "resolve tx-aware SQL executor")
	_, err := sqlq.ExecContext(
		s.ctx,
		`INSERT INTO usage_logs (user_id, api_key_id, account_id, subscription_id, request_id, model, input_tokens, output_tokens, total_cost, actual_cost, created_at)
		 VALUES (?, ?, ?, ?, ?, 'gpt-test', 1, 1, 0.01, 0.01, ?)`,
		userID,
		apiKey.ID,
		account.ID,
		subscriptionID,
		fmt.Sprintf("req-sub-%d-%d", subscriptionID, createdAt.UnixNano()),
		createdAt.UTC(),
	)
	s.Require().NoError(err)
}

// TestGetLatestUsedAtBySubscriptionIDs_UsesUsageLogs 验证聚合 SQL 取的是 usage_logs.MAX(created_at)，
// 没有 usage_log 的订阅不出现在结果 map 中。
func (s *UserSubscriptionRepoSuite) TestGetLatestUsedAtBySubscriptionIDs_UsesUsageLogs() {
	older := time.Now().Add(-4 * time.Hour).UTC().Truncate(time.Second)
	newer := time.Now().Add(-90 * time.Minute).UTC().Truncate(time.Second)

	user := s.mustCreateUser("sub-last-used@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-sub-last-used")
	subWithUsage := s.mustCreateSubscription(user.ID, group.ID, nil)
	subWithoutUsage := s.mustCreateSubscription(user.ID, group.ID, nil)

	s.mustInsertUsageLogForSubscription(user.ID, subWithUsage.ID, older)
	s.mustInsertUsageLogForSubscription(user.ID, subWithUsage.ID, newer)

	got, err := s.repo.GetLatestUsedAtBySubscriptionIDs(s.ctx, []int64{subWithUsage.ID, subWithoutUsage.ID})
	s.Require().NoError(err)
	s.Require().Contains(got, subWithUsage.ID)
	s.Require().NotContains(got, subWithoutUsage.ID, "无 usage_log 的订阅不应出现在结果中")
	s.Require().NotNil(got[subWithUsage.ID])
	s.Require().True(got[subWithUsage.ID].Equal(newer), "应取 MAX(created_at)，期望 %v 实际 %v", newer, got[subWithUsage.ID])
}

// TestGetLatestUsedAtBySubscriptionIDs_EmptyInput 入参为空时应返回空 map 不报错。
func (s *UserSubscriptionRepoSuite) TestGetLatestUsedAtBySubscriptionIDs_EmptyInput() {
	got, err := s.repo.GetLatestUsedAtBySubscriptionIDs(s.ctx, nil)
	s.Require().NoError(err)
	s.Require().Empty(got)

	got, err = s.repo.GetLatestUsedAtBySubscriptionIDs(s.ctx, []int64{})
	s.Require().NoError(err)
	s.Require().Empty(got)
}

// TestList_SortByLastUsedAt_Desc 用 sortBy=last_used_at + sort_order=desc 时，
// 最近用过的订阅排前面，从未使用过的（last_used_at = nil）排最末。
func (s *UserSubscriptionRepoSuite) TestList_SortByLastUsedAt_Desc() {
	older := time.Now().Add(-6 * time.Hour).UTC().Truncate(time.Second)
	newer := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)

	user := s.mustCreateUser("sub-list-sort-desc@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-sub-list-sort-desc")

	// 用 notes 区分订阅顺序，避免依赖 ID 顺序
	subOlder := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("older-usage")
	})
	subNewer := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("newer-usage")
	})
	s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("no-usage")
	})

	s.mustInsertUsageLogForSubscription(user.ID, subOlder.ID, older)
	s.mustInsertUsageLogForSubscription(user.ID, subNewer.ID, newer)

	results, _, err := s.repo.List(s.ctx, pagination.PaginationParams{
		Page:     1,
		PageSize: 10,
	}, &user.ID, nil, "", "", "last_used_at", "desc")
	s.Require().NoError(err)
	s.Require().Len(results, 3)
	s.Require().Equal("newer-usage", results[0].Notes, "最近使用的订阅应排第一")
	s.Require().Equal("older-usage", results[1].Notes, "较早使用的订阅应排第二")
	s.Require().Equal("no-usage", results[2].Notes, "从未使用的订阅应排最后")
	s.Require().NotNil(results[0].LastUsedAt)
	s.Require().True(results[0].LastUsedAt.Equal(newer))
	s.Require().NotNil(results[1].LastUsedAt)
	s.Require().True(results[1].LastUsedAt.Equal(older))
	s.Require().Nil(results[2].LastUsedAt, "no-usage 订阅 LastUsedAt 应为 nil")
}

// TestList_SortByLastUsedAt_Asc 升序时较早使用的订阅排前面，nil 仍排末尾。
func (s *UserSubscriptionRepoSuite) TestList_SortByLastUsedAt_Asc() {
	older := time.Now().Add(-6 * time.Hour).UTC().Truncate(time.Second)
	newer := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)

	user := s.mustCreateUser("sub-list-sort-asc@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-sub-list-sort-asc")

	subOlder := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("older-usage")
	})
	subNewer := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("newer-usage")
	})
	s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetNotes("no-usage")
	})

	s.mustInsertUsageLogForSubscription(user.ID, subOlder.ID, older)
	s.mustInsertUsageLogForSubscription(user.ID, subNewer.ID, newer)

	results, _, err := s.repo.List(s.ctx, pagination.PaginationParams{
		Page:     1,
		PageSize: 10,
	}, &user.ID, nil, "", "", "last_used_at", "asc")
	s.Require().NoError(err)
	s.Require().Len(results, 3)
	s.Require().Equal("older-usage", results[0].Notes, "升序：较早使用的订阅应排第一")
	s.Require().Equal("newer-usage", results[1].Notes)
	s.Require().Equal("no-usage", results[2].Notes, "nil 不论 asc/desc 都排末尾")
}

// TestList_FillsLastUsedAt_OnDefaultSort 即使非 last_used_at 排序，
// 返回结果上也应填好 LastUsedAt 字段供前端展示。
func (s *UserSubscriptionRepoSuite) TestList_FillsLastUsedAt_OnDefaultSort() {
	usedAt := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)

	user := s.mustCreateUser("sub-list-fill@test.com", service.RoleUser)
	group := s.mustCreateGroup("g-sub-list-fill")
	sub := s.mustCreateSubscription(user.ID, group.ID, nil)
	s.mustInsertUsageLogForSubscription(user.ID, sub.ID, usedAt)

	results, _, err := s.repo.List(s.ctx, pagination.PaginationParams{
		Page:     1,
		PageSize: 10,
	}, &user.ID, nil, "", "", "created_at", "desc")
	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().NotNil(results[0].LastUsedAt, "默认排序也应填充 LastUsedAt")
	s.Require().True(results[0].LastUsedAt.Equal(usedAt))
}
