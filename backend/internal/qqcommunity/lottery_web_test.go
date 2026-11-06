package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const webRequestID = "12345678-1234-1234-1234-123456789012"

func expectWebStart(m sqlmock.Sqlmock, mysql bool) {
	m.ExpectBegin()
	if mysql {
		m.ExpectQuery("SELECT id FROM qqc_transaction_lock").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		for i := 0; i < 3; i++ {
			m.ExpectExec("DO 0").WillReturnResult(sqlmock.NewResult(0, 1))
		}
	} else {
		m.ExpectExec(`SELECT pg_advisory_xact_lock_shared\(71427101\)`).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`SELECT pg_advisory_xact_lock_shared`).WithArgs(lotteryConfigLock).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended`).WithArgs("42").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectQuery("SELECT id FROM users").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
}

func expectWebState(m sqlmock.Sqlmock, cfg LotteryConfig, used int64, recharge string) {
	raw, _ := json.Marshal(cfg)
	m.ExpectQuery("SELECT value FROM settings").WithArgs(lotteryConfigKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	m.ExpectQuery(`SELECT COALESCE\(SUM\(CASE`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(recharge))
	m.ExpectQuery(`SELECT COALESCE\(SUM\(value\)`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("0"))
	m.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(used))
	m.ExpectQuery(`SELECT COALESCE\(SUM\(chances\)`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"bonus"}).AddRow(0))
}

func TestWebLotterySettlementBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		for _, guaranteed := range []bool{false, true} {
			name := map[bool]string{false: "postgres", true: "mysql"}[mysql] + map[bool]string{false: "/zero", true: "/guaranteed"}[guaranteed]
			t.Run(name, func(t *testing.T) {
				db, m, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				s := New(db, nil, false, nil)
				s.db.mysql = mysql
				cfg := defaultLotteryConfig()
				cfg.Enabled = true
				cfg.Prizes = []LotteryPrize{{Name: "Thanks", Amount: 0, ProbabilityBPS: 10000}}
				cfg.GuaranteeEnabled = guaranteed
				cfg.GuaranteeEvery = 2
				used := int64(0)
				if guaranteed {
					used = 1
				}
				expectWebStart(m, mysql)
				m.ExpectQuery("SELECT result_text FROM qqc_lottery_draws").WithArgs("web:lottery:42:"+webRequestID, int64(42)).WillReturnError(sql.ErrNoRows)
				expectWebState(m, cfg, used, "30")
				if mysql {
					m.ExpectExec("INSERT INTO qqc_lottery_draws").WillReturnResult(sqlmock.NewResult(7, 1))
				} else {
					m.ExpectQuery("INSERT INTO qqc_lottery_draws").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
				}
				amount := float64(0)
				if guaranteed {
					amount = 1
				}
				m.ExpectExec("INSERT INTO qqc_rewards").WithArgs("lottery:7", "website", "", "", int64(42), amount).WillReturnResult(sqlmock.NewResult(8, 1))
				if guaranteed {
					m.ExpectExec("UPDATE users SET balance").WithArgs(amount, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(9, 1))
				}
				m.ExpectCommit()
				out, err := s.UserLotterySpin(context.Background(), 42, webRequestID)
				require.NoError(t, err)
				require.Equal(t, guaranteed, out.IsGuaranteed)
				require.Equal(t, amount, out.Prize.Amount)
				require.EqualValues(t, 2-used, out.Remaining)
				require.Equal(t, out.Prizes[out.PrizeIndex], out.Prize)
				require.NoError(t, m.ExpectationsWereMet())
			})
		}
	}
}

func TestWebLotteryRetryReplaysWithoutNewDebit(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	expectWebStart(m, false)
	saved := UserLotterySpin{RequestID: webRequestID, Prize: LotteryPrize{Name: "Saved", Amount: 1}, Remaining: 0}
	raw, _ := json.Marshal(saved)
	m.ExpectQuery("SELECT result_text FROM qqc_lottery_draws").WithArgs("web:lottery:42:"+webRequestID, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"result_text"}).AddRow(string(raw)))
	m.ExpectCommit()
	out, err := s.UserLotterySpin(context.Background(), 42, webRequestID)
	require.NoError(t, err)
	require.Equal(t, saved, out)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestWebLotteryRejectsWithoutSpending(t *testing.T) {
	for _, closed := range []bool{true, false} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		s := New(db, nil, false, nil)
		expectWebStart(m, false)
		m.ExpectQuery("SELECT result_text FROM qqc_lottery_draws").WillReturnError(sql.ErrNoRows)
		cfg := defaultLotteryConfig()
		cfg.Enabled = !closed
		if closed {
			raw, _ := json.Marshal(cfg)
			m.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
		} else {
			expectWebState(m, cfg, 1, "10")
		}
		m.ExpectRollback()
		_, err = s.UserLotterySpin(context.Background(), 42, webRequestID)
		if closed {
			require.ErrorIs(t, err, ErrLotteryClosed)
		} else {
			require.ErrorIs(t, err, ErrLotteryNoChances)
		}
		require.NoError(t, m.ExpectationsWereMet())
		db.Close()
	}
}

func TestWebLotteryRollsBackWhenRewardWriteFails(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	expectWebStart(m, false)
	m.ExpectQuery("SELECT result_text FROM qqc_lottery_draws").WillReturnError(sql.ErrNoRows)
	cfg := defaultLotteryConfig()
	cfg.Enabled = true
	expectWebState(m, cfg, 0, "10")
	m.ExpectQuery("INSERT INTO qqc_lottery_draws").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	m.ExpectExec("INSERT INTO qqc_rewards").WillReturnError(errors.New("write failed"))
	m.ExpectRollback()
	_, err = s.UserLotterySpin(context.Background(), 42, webRequestID)
	require.ErrorContains(t, err, "write failed")
	require.NoError(t, m.ExpectationsWereMet())
}

func TestWebLotteryRejectsInvalidIdentityAndRequest(t *testing.T) {
	s := New(nil, nil, false, nil)
	for _, id := range []string{"", "../x", "123", "1234567890123456:other"} {
		_, err := s.UserLotterySpin(context.Background(), 42, id)
		require.ErrorIs(t, err, ErrLotteryRequest)
	}
	_, err := s.UserLotterySpin(context.Background(), 0, webRequestID)
	require.ErrorIs(t, err, ErrLotteryRequest)
}
