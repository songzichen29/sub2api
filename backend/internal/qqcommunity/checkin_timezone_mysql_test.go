package qqcommunity

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/migrations"
)

func TestCheckinMySQLLocalDaysAndStreakReward(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName)
	cfg.ParseTime, cfg.Loc = true, checkinZone
	db, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	// 临时表仅对本连接可见，重现生产原有的日期表达式和奖励约束。
	for _, q := range []string{
		`SET time_zone='+08:00'`,
		`CREATE TEMPORARY TABLE qqc_transaction_lock(id BIGINT PRIMARY KEY)`,
		`INSERT INTO qqc_transaction_lock VALUES(1)`,
		`CREATE TEMPORARY TABLE qqc_rewards(
 id BIGINT AUTO_INCREMENT PRIMARY KEY,reward_key VARCHAR(191) NOT NULL UNIQUE,
 user_id BIGINT NOT NULL,kind VARCHAR(32) NOT NULL,created_at DATETIME(6) NOT NULL,
 checkin_day DATE GENERATED ALWAYS AS (CASE WHEN kind='checkin' THEN DATE(DATE_ADD(created_at,INTERVAL 8 HOUR)) ELSE NULL END) STORED,
 UNIQUE KEY qqc_checkin_account_day(user_id,checkin_day))`,
		`CREATE TEMPORARY TABLE qqc_reward_codes(
 id BIGINT AUTO_INCREMENT PRIMARY KEY,app_id VARCHAR(191),code_hash VARCHAR(191),code_hint VARCHAR(191),
 batch_name VARCHAR(191),reward_type VARCHAR(32),amount DECIMAL(20,8),quantity INT,
 prize_name VARCHAR(191),claim_instructions TEXT,status VARCHAR(32),user_id BIGINT,
 member_id VARCHAR(191),group_id VARCHAR(191),redeemed_at DATETIME(6),fulfilled_at DATETIME(6),
 created_at DATETIME(6),source_kind VARCHAR(32),source_key VARCHAR(191) UNIQUE)`,
		`CREATE TEMPORARY TABLE qqc_lottery_chance_grants(
 source_key VARCHAR(191) PRIMARY KEY,user_id BIGINT,chances INT,created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6))`,
	} {
		_, err = db.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	at := time.Date(2026, 11, 27, 17, 57, 38, 0, checkinZone)
	for offset := -6; offset <= 0; offset++ {
		signedAt := at.AddDate(0, 0, offset)
		if offset < 0 {
			signedAt = time.Date(2026, 11, signedAt.Day(), 0, 5, 0, 0, checkinZone)
		}
		_, err = db.ExecContext(ctx, `INSERT INTO qqc_rewards(reward_key,user_id,kind,created_at) VALUES(?,42,'checkin',?)`, "checkin:42:"+day(signedAt), signedAt)
		require.NoError(t, err)
	}
	store := wrapDatabase(db)
	total, streak, err := checkinProgress(ctx, store, 42, at)
	require.NoError(t, err)
	require.Equal(t, 7, total)
	require.Equal(t, 7, streak)

	tomorrow := time.Date(2026, 11, 28, 0, 0, 0, 0, checkinZone)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_rewards(reward_key,user_id,kind,created_at) VALUES('checkin:42:2026-11-28',42,'checkin',?)`, tomorrow)
	var duplicate *mysql.MySQLError
	require.ErrorAs(t, err, &duplicate)
	require.EqualValues(t, 1062, duplicate.Number)

	migration, err := migrations.MySQLFS.ReadFile("118_qq_checkin_local_day.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	var date string
	err = db.QueryRowContext(ctx, `SELECT DATE_FORMAT(checkin_day,'%Y-%m-%d') FROM qqc_rewards WHERE reward_key='checkin:42:2026-11-27'`).Scan(&date)
	require.NoError(t, err)
	require.Equal(t, "2026-11-27", date)

	for _, wantGranted := range []bool{true, false} {
		tx, err := store.BeginTx(ctx, nil)
		require.NoError(t, err)
		streak, granted, grantErr := grantStreak(ctx, tx, Config{AppID: "bot"}, Job{}, 42, at, GrowthReward{Type: "chances", Quantity: 2}, 7)
		if grantErr != nil {
			_ = tx.Rollback()
		}
		require.NoError(t, grantErr)
		require.NoError(t, tx.Commit())
		require.Equal(t, 7, streak)
		require.Equal(t, wantGranted, granted)
	}
	var chances int
	err = db.QueryRowContext(ctx, `SELECT SUM(chances) FROM qqc_lottery_chance_grants WHERE user_id=42`).Scan(&chances)
	require.NoError(t, err)
	require.Equal(t, 2, chances)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_rewards(reward_key,user_id,kind,created_at) VALUES('checkin:42:2026-11-28',42,'checkin',?)`, tomorrow)
	require.NoError(t, err)
	total, streak, err = checkinProgress(ctx, store, 42, tomorrow)
	require.NoError(t, err)
	require.Equal(t, 8, total)
	require.Equal(t, 8, streak)
}
