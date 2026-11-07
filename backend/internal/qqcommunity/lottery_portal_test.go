package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectPortalConfig(t *testing.T, m sqlmock.Sqlmock, c Config, l LotteryConfig) {
	t.Helper()
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	m.ExpectQuery(`SELECT value FROM settings WHERE key=\$1`).WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	raw, err = json.Marshal(l)
	require.NoError(t, err)
	m.ExpectQuery(`SELECT value FROM settings WHERE key=\$1`).WithArgs(lotteryConfigKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
}

func TestUserLotteryIncludesUnboundEntitlementAndOnlyPublishedEnabledGroups(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := DefaultConfig()
	c.Enabled = true
	c.AppID = "12345"
	c.AllowedGroups = []string{"10001", "10002", "10004"}
	c.GroupPolicies = []GroupPolicy{{GroupID: "10002", DisableActivities: true}}
	l := defaultLotteryConfig()
	l.Enabled = true
	l.GuaranteeEnabled = true
	l.PublicGroups = []LotteryPublicGroup{{"10001", "visible", "https://qun.qq.com/abc"}, {"10002", "disabled", ""}, {"10003", "removed", ""}}
	m.ExpectBegin()
	expectPortalConfig(t, m, c, l)
	expectLotteryRecharge(m, 42, "25")
	m.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(1))
	m.ExpectQuery(`SELECT COALESCE\(SUM\(chances\)`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"grants"}).AddRow(3))
	m.ExpectQuery("SELECT user_id,chance_delta FROM").WithArgs(lotteryPeriodStart, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "chance_delta"}).AddRow(42, 3).AddRow(42, -1))
	m.ExpectQuery(`SELECT member_id FROM qqc_bindings`).WithArgs(c.AppID, int64(42)).WillReturnError(sql.ErrNoRows)
	m.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM payment_orders`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"pending"}).AddRow(true))
	m.ExpectCommit()
	out, err := s.UserLottery(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, out.Enabled)
	require.Empty(t, out.BoundQQ)
	require.True(t, out.PendingRecharge)
	require.EqualValues(t, 25, out.State.RechargeTotal)
	require.EqualValues(t, 5, out.State.Earned)
	require.EqualValues(t, 4, out.State.Remaining)
	require.EqualValues(t, 9, out.State.DrawsUntilGuarantee)
	require.Equal(t, lotteryPeriodStart, out.PeriodStart)
	require.Equal(t, l.PublicGroups[:1], out.PublicGroups)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestUserLotteryHistoryScopesAndPaginatesInBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		t.Run(map[bool]string{false: "postgres", true: "mysql"}[mysql], func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			s.db.mysql = mysql
			marker := "$1"
			if mysql {
				marker = "?"
			}
			m.ExpectBegin()
			m.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM qqc_lottery_draws WHERE user_id="+marker)).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(22))
			m.ExpectQuery(regexp.QuoteMeta("SELECT id,prize_name,amount,config_snapshot,created_at FROM qqc_lottery_draws WHERE user_id="+marker)).WithArgs(int64(42), lotteryPeriodStart, 20, 20).WillReturnRows(sqlmock.NewRows([]string{"id", "prize_name", "amount", "config_snapshot", "created_at"}).AddRow(1, "prize", 1, `{"is_guaranteed":true}`, time.Now()))
			m.ExpectCommit()
			out, err := s.UserLotteryDraws(context.Background(), 42, 2, 20)
			require.NoError(t, err)
			require.EqualValues(t, 22, out.Total)
			require.True(t, out.Items[0].IsGuaranteed)
			raw, _ := json.Marshal(out)
			require.NotContains(t, string(raw), "email")
			require.NotContains(t, string(raw), "member_id")
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestLotteryPublishedGroupsValidateLinksAndDuplicates(t *testing.T) {
	for _, link := range []string{"", "https://qun.qq.com/universal-share/share?key=abc", "https://qm.qq.com/q/test"} {
		require.NoError(t, validateLotteryPublicGroups([]LotteryPublicGroup{{"10001", "group", link}}))
	}
	for _, link := range []string{"javascript:alert(1)", "http://qun.qq.com/a", "https://qun.qq.com.evil.test/a", "https://user@qun.qq.com/a", "https://qun.qq.com:444/a", "https://example.com/a"} {
		require.Error(t, validateLotteryPublicGroups([]LotteryPublicGroup{{"10001", "group", link}}))
	}
	require.Error(t, validateLotteryPublicGroups([]LotteryPublicGroup{{"10001", "group", ""}, {"10001", "again", ""}}))
	require.Error(t, validateLotteryPublicGroups([]LotteryPublicGroup{{"x", "group", ""}}))
	require.Error(t, validateLotteryPublicGroups(make([]LotteryPublicGroup, 21)))
}

func TestLotteryMembershipNeverTreatsTransportFailuresAsAbsent(t *testing.T) {
	c := Config{AppID: "12345"}
	for _, tc := range []struct {
		name, info, list, want string
		infoError, listError   bool
	}{
		{"member", `{"group_id":10001,"user_id":67890}`, "", "member", false, false},
		{"old member in list", `{}`, `[{"group_id":"10001","user_id":"12345"},{"group_id":10001,"user_id":67890}]`, "member", false, false},
		{"confirmed absent", "", `[{"group_id":10001,"user_id":12345}]`, "not_member", true, false},
		{"offline", "", "", "unknown", true, true},
		{"empty list", "", `[]`, "unknown", true, false},
		{"wrong group", `{"group_id":99999,"user_id":67890}`, `[{"group_id":99999,"user_id":12345}]`, "unknown", false, false},
		{"malformed", `{`, `null`, "unknown", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := func(_ context.Context, _ Config, action string, params any) (json.RawMessage, error) {
				p := params.(map[string]any)
				require.Equal(t, "10001", p["group_id"])
				require.True(t, p["no_cache"].(bool))
				if action == "get_group_member_info" {
					require.Equal(t, "67890", p["user_id"])
					if tc.infoError {
						return nil, errors.New("not confirmed")
					}
					return json.RawMessage(tc.info), nil
				}
				require.Equal(t, "get_group_member_list", action)
				if tc.listError {
					return nil, errors.New("offline")
				}
				return json.RawMessage(tc.list), nil
			}
			require.Equal(t, tc.want, probeLotteryMembership(context.Background(), c, "10001", "67890", call))
		})
	}
}

func TestUserLotteryUnboundMembershipDoesNotNeedQQConnection(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := DefaultConfig()
	c.AppID = "12345"
	c.Enabled = true
	l := defaultLotteryConfig()
	l.Enabled = true
	expectPortalConfig(t, m, c, l)
	m.ExpectQuery(`SELECT member_id FROM qqc_bindings`).WithArgs(c.AppID, int64(42)).WillReturnError(sql.ErrNoRows)
	out, err := s.UserLotteryMembership(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "unbound", out.Status)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestUserLotteryInvalidIdentityAndPagination(t *testing.T) {
	s := New(nil, nil, false, nil)
	_, err := s.UserLottery(context.Background(), 0)
	require.Error(t, err)
	_, err = s.UserLotteryMembership(context.Background(), 0)
	require.Error(t, err)
	for _, v := range [][3]int{{0, 1, 20}, {42, 0, 20}, {42, 1, 101}, {42, 100001, 20}} {
		_, err = s.UserLotteryDraws(context.Background(), int64(v[0]), v[1], v[2])
		require.Error(t, err)
	}
}
