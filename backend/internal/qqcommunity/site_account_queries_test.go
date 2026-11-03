package qqcommunity

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func accountQueryConfig() Config {
	c := DefaultConfig()
	c.Enabled, c.AppID, c.AllowedGroups = true, "trusted-app", []string{"group"}
	return c
}

func expectAccountQueryConfig(t *testing.T, mock sqlmock.Sqlmock, c Config) {
	t.Helper()
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
}

func TestSiteAccountBalanceUsesTrustedIdentityAndNeverWrites(t *testing.T) {
	for _, scenario := range []string{"bound", "unbound", "disabled", "deleted", "lookup failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			c := accountQueryConfig()
			expectAccountQueryConfig(t, mock, c)
			query := mock.ExpectQuery(regexp.QuoteMeta("SELECT u.balance,u.id FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.deleted_at IS NULL AND u.status='active'")).WithArgs("trusted-app", "trusted-member")
			switch scenario {
			case "bound":
				query.WillReturnRows(sqlmock.NewRows([]string{"balance", "user_id"}).AddRow(12.50, 42))
			case "lookup failure":
				query.WillReturnError(errors.New("secret-database-detail"))
			default:
				query.WillReturnRows(sqlmock.NewRows([]string{"balance", "user_id"}))
			}
			result, err := New(db, nil, false, nil).querySiteInfoDetailed(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "trusted-member"}, siteQuery{Kind: "account_balance"})
			require.NoError(t, err)
			require.Equal(t, "account_balance", result.Decision.Kind)
			require.Equal(t, 1, result.Decision.Page)
			require.Contains(t, result.Text, "查询时间：")
			switch scenario {
			case "bound":
				require.Contains(t, result.Text, "余额：USD 12.50")
				require.Equal(t, "succeeded", result.Decision.Status)
			case "lookup failure":
				require.Contains(t, result.Text, "账户查询失败")
				require.Equal(t, "account_query_failed", result.Decision.ErrorCode)
			default:
				require.Contains(t, result.Text, bindFirst)
				require.Equal(t, "binding_required", result.Decision.ErrorCode)
			}
			require.NotContains(t, result.Text, "secret-database-detail")
			require.NoError(t, mock.ExpectationsWereMet(), "only config and scoped SELECTs may run")
		})
	}
}

func TestSiteAccountQueriesRequireTrustedMemberAndApp(t *testing.T) {
	for _, kind := range []string{"account_balance", "today_usage", "lottery_chances"} {
		for _, j := range []Job{
			{AppID: "trusted-app", GroupID: "group"},
			{GroupID: "group", MemberID: "trusted-member"},
			{AppID: "different-app", GroupID: "group", MemberID: "trusted-member"},
		} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			result, err := New(db, nil, false, nil).querySiteInfoDetailed(context.Background(), accountQueryConfig(), j, siteQuery{Kind: kind})
			require.NoError(t, err)
			require.Equal(t, "identity_unavailable", result.Decision.ErrorCode)
			require.NoError(t, mock.ExpectationsWereMet())
			_ = db.Close()
		}
	}
}

func TestSiteAccountQueriesRecheckCurrentGroupAndBot(t *testing.T) {
	for _, scenario := range []string{"bot disabled", "group removed", "app changed", "activities disabled", "config failed"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			c := accountQueryConfig()
			current := c
			kind, code := "account_balance", "group_disabled"
			switch scenario {
			case "bot disabled":
				current.Enabled = false
			case "group removed":
				current.AllowedGroups = nil
			case "app changed":
				current.AppID = "new-app"
			case "activities disabled":
				current.GroupPolicies = []GroupPolicy{{GroupID: "group", DisableActivities: true}}
				kind, code = "lottery_chances", "activities_disabled"
			case "config failed":
				code = "config_read_failed"
			}
			if scenario == "config failed" {
				mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnError(errors.New("private-error"))
			} else {
				expectAccountQueryConfig(t, mock, current)
			}
			result, err := New(db, nil, false, nil).querySiteInfoDetailed(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "trusted-member"}, siteQuery{Kind: kind})
			require.NoError(t, err)
			require.Equal(t, code, result.Decision.ErrorCode)
			require.NotContains(t, result.Text, "private-error")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSiteAccountTodayUsageNeverWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	c := accountQueryConfig()
	expectAccountQueryConfig(t, mock, c)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WithArgs(c.AppID, "trusted-member").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectQuery("SELECT model,COUNT").WithArgs(42, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"model", "requests", "input", "output", "write", "read", "cost"}).AddRow("model-a", 2, 10, 20, 5, 6, 0.25))
	mock.ExpectCommit()
	result, err := New(db, nil, false, nil).querySiteInfoDetailed(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "trusted-member"}, siteQuery{Kind: "today_usage"})
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Decision.Status)
	require.Contains(t, result.Text, "model-a：2 次")
	require.NotNil(t, result.Page)
	require.Equal(t, "usage", result.Page.Kind)
	require.Contains(t, result.Text, "模型用量")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSiteAccountLotteryChancesNeverSettlesEligibleRewards(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	c := accountQueryConfig()
	expectAccountQueryConfig(t, mock, c)
	mock.ExpectBegin()
	lottery := defaultLotteryConfig()
	lottery.Enabled = true
	raw, err := json.Marshal(lottery)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(lotteryConfigKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT b.user_id FROM qqc_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL")+"$").WithArgs(c.AppID, "trusted-member").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(CASE")).WithArgs(42, lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("0"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(value),0)::text")).WithArgs(42, lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("0"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM qqc_lottery_draws")).WithArgs(42, lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(chances),0) FROM qqc_lottery_chance_grants")).WithArgs(42, lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM qqc_lottery_draws WHERE user_id=$1 AND amount>0")).WithArgs(42, lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectCommit()
	result, err := New(db, nil, false, nil).querySiteInfoDetailed(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "trusted-member"}, siteQuery{Kind: "lottery_chances"})
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Decision.Status)
	require.Contains(t, result.Text, "剩余抽奖次数：3 次")
	require.NoError(t, mock.ExpectationsWereMet(), "no settlement or balance mutation may occur")
}
