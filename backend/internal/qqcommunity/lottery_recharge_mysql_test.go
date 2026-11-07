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

func TestLotteryRechargeMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName, "test must never target a production database")
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	// Connection-local temporary data cannot alter existing fixture tables.
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TEMPORARY TABLE payment_orders (
        id BIGINT PRIMARY KEY,user_id BIGINT,order_type VARCHAR(32),paid_at DATETIME(6),completed_at DATETIME(6),
        pay_amount DECIMAL(20,2),fee_rate DECIMAL(10,4),amount DECIMAL(20,2),refund_amount DECIMAL(20,2),
        status VARCHAR(32),provider_key VARCHAR(32),payment_type VARCHAR(32),provider_snapshot JSON)`)
	require.NoError(t, err)
	for _, r := range []struct {
		id, uid                int
		kind, status, snapshot string
		completed              time.Time
	}{
		{1, 42, "balance", "COMPLETED", `{"provider_key":"alipay"}`, lotteryPeriodStart},
		{2, 42, "balance", "COMPLETED", `{"currency":"CNY"}`, lotteryPeriodStart.Add(-time.Microsecond)},
		{3, 42, "balance", "COMPLETED", `{"currency":"USD"}`, lotteryPeriodStart},
		{4, 42, "subscription", "COMPLETED", `{"currency":"CNY"}`, lotteryPeriodStart},
		{5, 43, "balance", "COMPLETED", `{"currency":"CNY"}`, lotteryPeriodStart},
		{6, 42, "balance", "REFUNDING", `{"currency":"CNY"}`, lotteryPeriodStart},
	} {
		_, err = db.ExecContext(ctx, `INSERT INTO payment_orders VALUES(?,?,?,?,?,30.18,0.6,60,0,?,'alipay','alipay',?)`, r.id, r.uid, r.kind, lotteryPeriodStart, r.completed, r.status, r.snapshot)
		require.NoError(t, err)
	}
	total, err := lotteryRechargeSum(ctx, wrapDatabase(db), 42, false)
	require.NoError(t, err)
	require.Equal(t, "30", total.String())
	rawTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rawTx.Rollback()
	total, err = lotteryRechargeSum(ctx, &transaction{raw: rawTx, mysql: true}, 42, true)
	require.NoError(t, err)
	require.Equal(t, "30", total.String())
	require.NoError(t, rawTx.Rollback())
}
