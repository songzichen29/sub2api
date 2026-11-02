package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func newQQRepoTest(t *testing.T, isMySQL bool) (*qqBotRepository, sqlmock.Sqlmock, qqbot.Config) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); db.Close() })
	c := qqbot.DefaultConfig()
	c.Enabled = true
	c.BotQQ = "123456"
	c.AllowedGroups = []string{"987654"}
	c.CheckinReward = 0.25
	c.ChatEnabled = true
	return &qqBotRepository{db: db, mysql: isMySQL}, mock, c
}
func expectQQTx(t *testing.T, r *qqBotRepository, m sqlmock.Sqlmock, c qqbot.Config) {
	t.Helper()
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	suffix := `ON CONFLICT \("key"\) DO NOTHING`
	if r.mysql {
		suffix = "ON DUPLICATE KEY UPDATE `key` = `key`"
	}
	m.ExpectExec("INSERT INTO settings .*"+suffix).WithArgs(qqbot.ConfigKey, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectBegin()
	m.ExpectQuery(regexp.QuoteMeta(r.q("SELECT value FROM settings WHERE " + r.key() + " = ? FOR UPDATE"))).WithArgs(qqbot.ConfigKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
}
func expectQQBound(r *qqBotRepository, m sqlmock.Sqlmock) {
	m.ExpectQuery(regexp.QuoteMeta(r.q("SELECT user_id FROM qq_bot_bindings WHERE qq_id = ? FOR UPDATE"))).WithArgs("777777").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	m.ExpectQuery(regexp.QuoteMeta(r.q("SELECT balance, status FROM users WHERE id = ? AND deleted_at IS NULL FOR UPDATE"))).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"balance", "status"}).AddRow(10, "active"))
}
func rewardArgs(now time.Time) []driver.Value {
	return []driver.Value{sqlmock.AnyArg(), int64(42), "777777", "987654", "2026-11-03", "0.25000000", 0, now}
}

func TestQQBotCheckinAtomicBothDialects(t *testing.T) {
	for _, isMySQL := range []bool{true, false} {
		for _, outcome := range []string{"commit", "duplicate", "balance_failure", "ledger_failure", "disabled"} {
			t.Run(map[bool]string{true: "mysql/", false: "postgres/"}[isMySQL]+outcome, func(t *testing.T) {
				r, m, c := newQQRepoTest(t, isMySQL)
				if outcome == "disabled" {
					c.Enabled = false
				}
				expectQQTx(t, r, m, c)
				now := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC) // Beijing date is already Nov 3.
				if outcome == "disabled" {
					m.ExpectRollback()
					_, err := r.Checkin(context.Background(), "777777", "987654", "123456", now)
					require.ErrorIs(t, err, qqbot.ErrDisabled)
					return
				}
				expectQQBound(r, m)
				insert := m.ExpectExec("INSERT INTO qq_bot_rewards").WithArgs(rewardArgs(now)...)
				if outcome == "duplicate" {
					insert.WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
					m.ExpectRollback()
					m.ExpectQuery("SELECT amount, balance_after FROM qq_bot_rewards").WithArgs(int64(42), "2026-11-03").WillReturnRows(sqlmock.NewRows([]string{"amount", "balance_after"}).AddRow(0.25, 10.25))
					_, err := r.Checkin(context.Background(), "777777", "987654", "123456", now)
					require.ErrorIs(t, err, qqbot.ErrAlreadyCheckedIn)
					return
				}
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				update := m.ExpectExec(regexp.QuoteMeta(r.q("UPDATE users SET balance = balance + CAST(? AS DECIMAL(20,8)), updated_at = ? WHERE id = ?"))).WithArgs("0.25000000", now, int64(42))
				if outcome == "balance_failure" {
					update.WillReturnError(errors.New("storage failure"))
					m.ExpectRollback()
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectQuery("SELECT balance FROM users WHERE id =").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(10.25))
					ledger := m.ExpectExec("UPDATE qq_bot_rewards SET balance_after").WithArgs(int64(42), sqlmock.AnyArg())
					if outcome == "ledger_failure" {
						ledger.WillReturnError(errors.New("storage failure"))
						m.ExpectRollback()
					} else {
						ledger.WillReturnResult(sqlmock.NewResult(0, 1))
						m.ExpectCommit()
					}
				}
				reward, err := r.Checkin(context.Background(), "777777", "987654", "123456", now)
				if outcome == "balance_failure" || outcome == "ledger_failure" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, "2026-11-03", reward.Day)
					require.EqualValues(t, 42, reward.UserID)
					require.Equal(t, 10.25, reward.BalanceAfter)
				}
			})
		}
	}
}
func TestQQBotConfirmRequiresMatchingWebsiteQQ(t *testing.T) {
	r, m, c := newQQRepoTest(t, true)
	expectQQTx(t, r, m, c)
	m.ExpectQuery("SELECT qq_id, group_id, bot_id, expires_at FROM qq_bot_bind_requests").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"qq_id", "group_id", "bot_id", "expires_at"}).AddRow("777777", "987654", "123456", time.Now().Add(time.Minute)))
	m.ExpectRollback()
	_, err := r.ConfirmBinding(context.Background(), 42, "888888")
	require.ErrorIs(t, err, qqbot.ErrConflict)
}
func TestQQBotRejectExpiredOrClaimedCode(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "already_claimed"}[claimed], func(t *testing.T) {
			r, m, c := newQQRepoTest(t, false)
			expectQQTx(t, r, m, c)
			qq := ""
			expiry := time.Now().Add(-time.Minute)
			if claimed {
				qq = "888888"
				expiry = time.Now().Add(time.Minute)
			}
			m.ExpectQuery("SELECT user_id, qq_id, expires_at FROM qq_bot_bind_requests").WithArgs("hash").WillReturnRows(sqlmock.NewRows([]string{"user_id", "qq_id", "expires_at"}).AddRow(42, qq, expiry))
			m.ExpectRollback()
			require.ErrorIs(t, r.ClaimBindCode(context.Background(), "hash", "777777", "987654", "123456"), qqbot.ErrInvalidCode)
		})
	}
}
func TestQQBotChatQuotaUsesSiteAccountAcrossGroups(t *testing.T) {
	for _, outcome := range []string{"site_limit", "member_limit", "cooldown", "stale_config"} {
		t.Run(outcome, func(t *testing.T) {
			r, m, c := newQQRepoTest(t, true)
			expectQQTx(t, r, m, c)
			now := time.Now()
			day := now.In(qqbot.Shanghai).Format("2006-01-02")
			version := c.Version
			if outcome == "stale_config" {
				version--
				m.ExpectRollback()
				_, err := r.ReserveChat(context.Background(), "777777", "987654", "123456", "event", now, version)
				require.ErrorIs(t, err, qqbot.ErrConflict)
				return
			}
			expectQQBound(r, m)
			siteCount := 1
			if outcome == "site_limit" {
				siteCount = c.TotalDailyChatLimit
			}
			m.ExpectQuery("SELECT request_count,last_request_at FROM qq_bot_daily_usage").WithArgs(day, "site").WillReturnRows(sqlmock.NewRows([]string{"request_count", "last_request_at"}).AddRow(siteCount, now.Add(-time.Hour)))
			if outcome != "site_limit" {
				m.ExpectExec("UPDATE qq_bot_daily_usage").WithArgs(now, day, "site").WillReturnResult(sqlmock.NewResult(0, 1))
				memberCount := 1
				last := now.Add(-time.Hour)
				if outcome == "member_limit" {
					memberCount = c.DailyChatLimit
				} else {
					last = now
				}
				m.ExpectQuery("SELECT request_count,last_request_at FROM qq_bot_daily_usage").WithArgs(day, "user:42").WillReturnRows(sqlmock.NewRows([]string{"request_count", "last_request_at"}).AddRow(memberCount, last))
			}
			m.ExpectRollback()
			_, err := r.ReserveChat(context.Background(), "777777", "987654", "123456", "event", now, version)
			if outcome == "cooldown" {
				require.ErrorIs(t, err, qqbot.ErrCooldown)
			} else {
				require.ErrorIs(t, err, qqbot.ErrChatLimit)
			}
		})
	}
}
func TestQQBotConversationLeaseAndHistoryAreScoped(t *testing.T) {
	r, m, c := newQQRepoTest(t, false)
	expectQQTx(t, r, m, c)
	expectQQBound(r, m)
	now := time.Now()
	day := now.In(qqbot.Shanghai).Format("2006-01-02")
	for _, scope := range []string{"site", "user:42"} {
		m.ExpectQuery("SELECT request_count,last_request_at FROM qq_bot_daily_usage").WithArgs(day, scope).WillReturnRows(sqlmock.NewRows([]string{"request_count", "last_request_at"}).AddRow(1, now.Add(-time.Hour)))
		m.ExpectExec("UPDATE qq_bot_daily_usage").WithArgs(now, day, scope).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectQuery("SELECT messages_json,lease_until FROM qq_bot_conversations").WithArgs("777777", "987654").WillReturnRows(sqlmock.NewRows([]string{"messages_json", "lease_until"}).AddRow(`[]`, now.Add(time.Minute)))
	m.ExpectRollback()
	_, err := r.ReserveChat(context.Background(), "777777", "987654", "123456", "event", now, c.Version)
	require.ErrorIs(t, err, qqbot.ErrCooldown)
	m.ExpectExec("UPDATE qq_bot_conversations.*AND lease_token =").WithArgs("[]", sqlmock.AnyArg(), sqlmock.AnyArg(), "777777", "987654", "original-event").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, r.FinishChat(context.Background(), "777777", "987654", "original-event", []qqbot.Message{}))
}
func TestQQBotConfigCASAndEventReplay(t *testing.T) {
	r, m, c := newQQRepoTest(t, true)
	expectQQTx(t, r, m, c)
	m.ExpectRollback()
	require.ErrorIs(t, r.SaveConfig(context.Background(), "{}", 99), qqbot.ErrConflict)
	m.ExpectExec("INSERT INTO qq_bot_events").WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	accepted, err := r.BeginEvent(context.Background(), qqbot.EventRecord{ID: "hash"})
	require.NoError(t, err)
	require.False(t, accepted)
}
