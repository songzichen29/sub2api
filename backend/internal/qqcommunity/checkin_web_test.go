package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

var checkinTestAt = time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC)

func expectCheckinConfig(m sqlmock.Sqlmock, key string, value any) {
	raw, _ := json.Marshal(value)
	m.ExpectQuery("SELECT value FROM settings").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
}

func expectCheckinStart(m sqlmock.Sqlmock, mysql, active bool) {
	m.ExpectBegin()
	if mysql {
		m.ExpectQuery("SELECT id FROM qqc_transaction_lock").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	}
	lock := "SELECT pg_advisory"
	if mysql {
		lock = "DO 0"
	}
	m.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
	// Website activity remains available with a disabled/unconfigured QQ bot.
	expectCheckinConfig(m, configKey, Config{AppID: "bot", Enabled: false})
	for i := 0; i < 2; i++ {
		m.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	q := m.ExpectQuery("SELECT id FROM users").WithArgs(int64(42))
	if active {
		q.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	} else {
		q.WillReturnError(sql.ErrNoRows)
	}
}

func expectCheckinRecord(m sqlmock.Sqlmock, at time.Time, source string, amount float64) {
	start, _ := time.ParseInLocation(time.DateOnly, day(at), checkinZone)
	q := m.ExpectQuery("SELECT amount,app_id FROM qqc_rewards WHERE user_id").WithArgs(int64(42), start, start.AddDate(0, 0, 1))
	if source == "" {
		q.WillReturnError(sql.ErrNoRows)
	} else {
		q.WillReturnRows(sqlmock.NewRows([]string{"amount", "app_id"}).AddRow(amount, source))
	}
}

func expectNoCheckinBonus(m sqlmock.Sqlmock, date string) {
	key := "qq-streak:42:" + date
	m.ExpectQuery("SELECT reward_type,amount,quantity,prize_name,claim_instructions,status FROM qqc_reward_codes").WithArgs(key).WillReturnError(sql.ErrNoRows)
	m.ExpectQuery("SELECT chances FROM qqc_lottery_chance_grants").WithArgs(key).WillReturnError(sql.ErrNoRows)
}

func expectCheckinProgress(m sqlmock.Sqlmock, count int) {
	m.ExpectQuery("WITH dates AS").WillReturnRows(sqlmock.NewRows([]string{"total", "streak"}).AddRow(count, count))
}

func TestWebsiteCheckinSettlementBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		for _, amount := range []float64{0, 0.12345678} {
			t.Run(map[bool]string{false: "postgres", true: "mysql"}[mysql]+"/"+money(amount), func(t *testing.T) {
				db, m, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				s := New(db, nil, false, nil)
				s.db.mysql = mysql
				expectCheckinStart(m, mysql, true)
				expectCheckinRecord(m, checkinTestAt, "", 0)
				cfg := CheckinConfig{Enabled: true, MinAmount: amount, MaxAmount: amount, Growth: GrowthConfig{Tasks: []GrowthTask{{ID: "binding", Name: "绑定 QQ", Kind: "binding", Target: 1, Enabled: true, Chances: 99}}}}
				expectCheckinConfig(m, checkinConfigKey, cfg)
				if mysql {
					m.ExpectExec("INSERT INTO qqc_rewards").WithArgs("checkin:42:2026-11-07", int64(42), amount, checkinTestAt).WillReturnResult(sqlmock.NewResult(5, 1))
				} else {
					m.ExpectQuery("INSERT INTO qqc_rewards").WithArgs("checkin:42:2026-11-07", int64(42), amount, checkinTestAt).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
				}
				if amount > 0 {
					m.ExpectExec("UPDATE users SET balance").WithArgs(amount, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(6, 1))
				}
				expectCheckinProgress(m, 1)
				m.ExpectQuery("SELECT EXISTS.*qqc_bindings").WithArgs("bot", int64(42)).WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
				expectCheckinProgress(m, 1)
				// No binding task query or award is permitted for an unbound user.
				expectNoCheckinBonus(m, "2026-11-07")
				m.ExpectCommit()
				out, err := s.claimUserCheckin(context.Background(), 42, "2026-11-07", func() time.Time { return checkinTestAt })
				require.NoError(t, err)
				require.Equal(t, amount, out.Amount)
				require.Equal(t, "website", out.Source)
				require.False(t, out.AlreadyCheckedIn)
				require.NoError(t, m.ExpectationsWereMet())
			})
		}
	}
}

func TestWebsiteCheckinReplaysQQAndWebsiteRewards(t *testing.T) {
	for _, source := range []string{"bot", "website"} {
		for _, mysql := range []bool{false, true} {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			s := New(db, nil, false, nil)
			s.db.mysql = mysql
			expectCheckinStart(m, mysql, true)
			expectCheckinRecord(m, checkinTestAt, source, .25)
			expectNoCheckinBonus(m, "2026-11-07")
			m.ExpectCommit()
			out, err := s.claimUserCheckin(context.Background(), 42, "2026-11-07", func() time.Time { return checkinTestAt })
			require.NoError(t, err)
			require.True(t, out.AlreadyCheckedIn)
			require.Equal(t, .25, out.Amount)
			require.Equal(t, checkinSource(source), out.Source)
			require.NoError(t, m.ExpectationsWereMet())
			db.Close()
		}
	}
}

func TestWebsiteCheckinRejectsUnavailableAndStaleRequests(t *testing.T) {
	for _, scenario := range []string{"inactive", "midnight", "closed", "ineligible"} {
		t.Run(scenario, func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			expectCheckinStart(m, false, scenario != "inactive")
			want := ErrCheckinAccount
			requested := "2026-11-07"
			switch scenario {
			case "midnight":
				requested = "2026-11-06"
				want = ErrCheckinDay
			case "closed", "ineligible":
				expectCheckinRecord(m, checkinTestAt, "", 0)
				cfg := CheckinConfig{Enabled: scenario == "ineligible"}
				if cfg.Enabled {
					cfg.MinRechargeTotal = 10
				}
				expectCheckinConfig(m, checkinConfigKey, cfg)
				want = ErrCheckinClosed
				if scenario == "ineligible" {
					for i := 0; i < 4; i++ {
						m.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("0"))
					}
					want = ErrCheckinEligibility
				}
			}
			m.ExpectRollback()
			_, err = s.claimUserCheckin(context.Background(), 42, requested, func() time.Time { return checkinTestAt })
			require.ErrorIs(t, err, want)
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestWebsiteCheckinRollsBackAllRewardWrites(t *testing.T) {
	for _, failure := range []string{"balance", "history"} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		s := New(db, nil, false, nil)
		expectCheckinStart(m, false, true)
		expectCheckinRecord(m, checkinTestAt, "", 0)
		expectCheckinConfig(m, checkinConfigKey, CheckinConfig{Enabled: true, MinAmount: 1, MaxAmount: 1})
		m.ExpectQuery("INSERT INTO qqc_rewards").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
		if failure == "balance" {
			m.ExpectExec("UPDATE users").WillReturnError(errors.New("write failed"))
		} else {
			m.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec("INSERT INTO redeem_codes").WillReturnError(errors.New("write failed"))
		}
		m.ExpectRollback()
		_, err = s.claimUserCheckin(context.Background(), 42, "2026-11-07", func() time.Time { return checkinTestAt })
		require.ErrorContains(t, err, "write failed")
		require.NoError(t, m.ExpectationsWereMet())
		db.Close()
	}
}

func TestWebsiteCheckinPhysicalStreakRemainsPendingAndRetryDoesNotGrantAgain(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	reward := GrowthReward{Type: "physical", Quantity: 1, PrizeName: "礼物", ClaimInstructions: "联系管理员"}
	expectCheckinStart(m, false, true)
	expectCheckinRecord(m, checkinTestAt, "", 0)
	expectCheckinConfig(m, checkinConfigKey, CheckinConfig{Enabled: true, StreakDays: 1, StreakReward: &reward, Growth: GrowthConfig{Tasks: []GrowthTask{}}})
	m.ExpectQuery("INSERT INTO qqc_rewards").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	expectCheckinProgress(m, 1)
	m.ExpectQuery("SELECT EXISTS.*qqc_reward_codes").WithArgs("qq-streak:42:2026-11-07").WillReturnRows(sqlmock.NewRows([]string{"claimed"}).AddRow(false))
	m.ExpectQuery("INSERT INTO qqc_reward_codes").WithArgs("bot", "连续签到 1 天奖励", "physical", float64(0), 1, "礼物", "联系管理员", "pending", int64(42), "", "", checkinTestAt, "streak", "qq-streak:42:2026-11-07").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(6))
	m.ExpectQuery("SELECT EXISTS.*qqc_bindings").WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(false))
	expectCheckinProgress(m, 1)
	bonusRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"type", "amount", "quantity", "name", "instructions", "status"}).AddRow("physical", 0, 1, "礼物", "联系管理员", "pending")
	}
	m.ExpectQuery("SELECT reward_type").WillReturnRows(bonusRows())
	m.ExpectCommit()
	out, err := s.claimUserCheckin(context.Background(), 42, "2026-11-07", func() time.Time { return checkinTestAt })
	require.NoError(t, err)
	require.Equal(t, "pending", out.Bonus.Status)
	expectCheckinStart(m, false, true)
	expectCheckinRecord(m, checkinTestAt, "website", 0)
	m.ExpectQuery("SELECT reward_type").WillReturnRows(bonusRows())
	m.ExpectCommit()
	again, err := s.claimUserCheckin(context.Background(), 42, "2026-11-07", func() time.Time { return checkinTestAt })
	require.NoError(t, err)
	require.True(t, again.AlreadyCheckedIn)
	require.Equal(t, out.Bonus, again.Bonus)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestWebsiteCheckinCalendarIsUserScopedAcrossOrigins(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		s := New(db, nil, false, nil)
		s.db.mysql = mysql
		m.ExpectBegin() // Read-only transaction never takes the MySQL write lock.
		expectCheckinConfig(m, configKey, Config{Enabled: false, AppID: "bot"})
		expectCheckinConfig(m, checkinConfigKey, CheckinConfig{Enabled: true, MinAmount: .1, MaxAmount: .5})
		m.ExpectQuery("SELECT amount,app_id FROM qqc_rewards WHERE user_id").WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(sql.ErrNoRows)
		expectCheckinProgress(m, 2)
		m.ExpectQuery("SELECT COALESCE.*FROM qqc_rewards WHERE user_id").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(.3))
		start := time.Date(2020, 2, 1, 0, 0, 0, 0, checkinZone)
		m.ExpectQuery("SELECT created_at,amount,app_id FROM qqc_rewards WHERE user_id").WithArgs(int64(42), start, start.AddDate(0, 1, 0)).WillReturnRows(sqlmock.NewRows([]string{"created_at", "amount", "app_id"}).AddRow(start, .1, "website").AddRow(start.AddDate(0, 0, 1), .2, "bot"))
		m.ExpectCommit()
		out, err := s.UserCheckin(context.Background(), 42, "2020-02")
		require.NoError(t, err)
		require.True(t, out.CanCheckin)
		require.Len(t, out.Days, 2)
		require.Equal(t, "website", out.Days[0].Source)
		require.Equal(t, "qq", out.Days[1].Source)
		require.Equal(t, .3, out.TotalAmount)
		require.Equal(t, "2020-02-01", out.Days[0].Date)
		require.NoError(t, m.ExpectationsWereMet())
		db.Close()
	}
}

func TestWebsiteCheckinDatesAndUnboundGrowth(t *testing.T) {
	for _, month := range []string{"invalid", "2026-1", "2026-13", "2027-01", "1999-12"} {
		_, err := checkinMonth(month, checkinTestAt)
		require.ErrorIs(t, err, ErrCheckinMonth)
	}
	start, err := checkinMonth("", checkinTestAt)
	require.NoError(t, err)
	require.Equal(t, "2026-11", start.Format("2006-01"))
	require.Equal(t, "2026-11-07", day(checkinTestAt))
	require.Equal(t, "2026-11-06", day(checkinTestAt.Add(-time.Nanosecond)))
	cfg := CheckinConfig{Growth: GrowthConfig{Tasks: []GrowthTask{{ID: "bound", Kind: "binding"}, {ID: "sign", Kind: "checkin_total"}}}}
	filtered := withoutBindingTask(cfg)
	require.Len(t, filtered.Growth.Tasks, 1)
	require.Len(t, cfg.Growth.Tasks, 2)
	require.Equal(t, "sign", filtered.Growth.Tasks[0].ID)
	s := New(nil, nil, false, nil)
	_, err = s.ClaimUserCheckin(context.Background(), 0, "2026-11-07")
	require.ErrorIs(t, err, ErrCheckinAccount)
	_, err = s.ClaimUserCheckin(context.Background(), 42, "2026-02-30")
	require.ErrorIs(t, err, ErrCheckinDay)
}
