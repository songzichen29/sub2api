package qqcommunity

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// All fixture tables are connection-local; this test never changes live data.
func TestLotterySourcesMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TEMPORARY TABLE qqc_lottery_chance_grants(source_key VARCHAR(64),user_id BIGINT,chances BIGINT,created_at DATETIME(6))`,
		`CREATE TEMPORARY TABLE qqc_lottery_draws(request_key VARCHAR(64),user_id BIGINT,recharge_total DECIMAL(20,8),config_snapshot JSON,created_at DATETIME(6))`,
		`CREATE TEMPORARY TABLE payment_orders(id BIGINT,user_id BIGINT,order_type VARCHAR(32),paid_at DATETIME(6),completed_at DATETIME(6),pay_amount DECIMAL(20,2),fee_rate DECIMAL(10,4),amount DECIMAL(20,2),refund_amount DECIMAL(20,2),status VARCHAR(32),provider_key VARCHAR(32),payment_type VARCHAR(32),provider_snapshot JSON)`,
	} {
		_, err = db.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	at := func(hour int) time.Time { return lotteryPeriodStart.Add(time.Duration(hour) * time.Hour) }
	exec := func(q string, args ...any) { _, e := db.ExecContext(ctx, q, args...); require.NoError(t, e) }
	draw := func(key string, uid int, total int, snapshot string, hour int) {
		exec(`INSERT INTO qqc_lottery_draws VALUES(?,?,?,?,?)`, key, uid, total, snapshot, at(hour))
	}
	grant := func(key string, n, hour int) {
		exec(`INSERT INTO qqc_lottery_chance_grants VALUES(?,42,?,?)`, key, n, at(hour))
	}
	state := func() LotteryState {
		s, e := lotteryState(ctx, wrapDatabase(db), defaultLotteryConfig(), 42, false)
		require.NoError(t, e)
		return s
	}
	draw("legacy", 42, 10, `{"recharge_step":10,"chances_per_step":1}`, 1)
	grant("campaign", 1, 2)
	draw("old-gift", 42, 0, `{"recharge_currency":"CNY"}`, 3)
	s := state()
	require.EqualValues(t, 1, s.LegacyUsed)
	require.EqualValues(t, 1, s.BonusUsed)
	require.Zero(t, s.RechargeUsed)
	require.Zero(t, s.Remaining)
	require.Equal(t, float64(10), s.NextRecharge)
	exec(`INSERT INTO payment_orders VALUES(1,42,'balance',?,?,10.06,0.6,20,0,'COMPLETED','alipay','alipay',NULL)`, at(4), at(4))
	s = state()
	require.EqualValues(t, 1, s.RechargeRemaining)
	require.EqualValues(t, 1, s.Remaining)
	draw("paid", 42, 10, `{"chance_source":"recharge","recharge_currency":"CNY"}`, 5)
	grant("another-gift", 2, 6)
	draw("new-gift", 42, 10, `{"chance_source":"activity","recharge_currency":"CNY"}`, 7)
	exec(`UPDATE payment_orders SET status='REFUNDED',refund_amount=20 WHERE id=1`)
	s = state()
	require.EqualValues(t, 1, s.RechargeUsed)
	require.EqualValues(t, 2, s.BonusUsed)
	require.EqualValues(t, 1, s.BonusRemaining)
	require.EqualValues(t, 1, s.Remaining)
	require.Equal(t, float64(20), s.NextRecharge)
	// A paid legacy draw stays charged even after a full refund.
	exec(`INSERT INTO payment_orders VALUES(2,43,'balance',?,?,10,0,20,20,'REFUNDED','alipay','alipay',NULL)`, at(1), at(1))
	draw("refunded-legacy", 43, 20, `{"recharge_step":10,"chances_per_step":1}`, 2)
	usage, err := lotteryUsageBySource(ctx, wrapDatabase(db), 0)
	require.NoError(t, err)
	require.EqualValues(t, 0, usage[43].legacyUsed)
	require.EqualValues(t, 1, usage[42].legacyUsed)
	require.EqualValues(t, 2, usage[42].bonusUsed)
}
