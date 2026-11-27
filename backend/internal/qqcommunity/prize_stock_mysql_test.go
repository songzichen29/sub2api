package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/migrations"
)

// 只使用连接私有临时表，校验实际迁移、NULL 唯一约束和库存/用量 SQL。
func TestPrizeStockMySQLTemporaryTables(t *testing.T) {
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
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	for _, statement := range []string{
		`SET time_zone='+08:00'`,
		`CREATE TEMPORARY TABLE qqc_lottery_draws(id BIGINT PRIMARY KEY)`,
		`CREATE TEMPORARY TABLE users(id BIGINT PRIMARY KEY,email VARCHAR(255))`,
		`CREATE TEMPORARY TABLE redeem_codes(id BIGINT PRIMARY KEY,code VARCHAR(32),type VARCHAR(32),value DECIMAL(20,8),validity_days INT,status VARCHAR(32),expires_at DATETIME(6),used_at DATETIME(6))`,
		`CREATE TEMPORARY TABLE usage_logs(user_id BIGINT,input_tokens BIGINT,output_tokens BIGINT,cache_creation_tokens BIGINT,cache_read_tokens BIGINT,created_at DATETIME(6))`,
	} {
		_, err = db.ExecContext(ctx, statement)
		require.NoError(t, err)
	}
	schema, err := migrations.MySQLFS.ReadFile("110_qq_code_campaigns.sql")
	require.NoError(t, err)
	for _, statement := range strings.Split(string(schema), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		_, err = db.ExecContext(ctx, strings.Replace(statement, "CREATE TABLE IF NOT EXISTS", "CREATE TEMPORARY TABLE", 1))
		require.NoError(t, err)
	}
	migration, err := migrations.MySQLFS.ReadFile("119_qq_prize_stock.sql")
	require.NoError(t, err)
	for _, statement := range strings.Split(string(migration), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		_, err = db.ExecContext(ctx, statement)
		require.NoError(t, err)
	}
	c := probabilityCampaignFixture()
	c.ID = "aaaaaaaaaaaaaaaaaaaaaaaa"
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_code_campaigns(id,app_id,config_json) VALUES(?,'bot',?)`, c.ID, raw)
	require.NoError(t, err)
	for _, uid := range []int64{42, 43} {
		_, err = db.ExecContext(ctx, `INSERT INTO qqc_code_claims(id,campaign_id,app_id,group_id,member_id,user_id,period_key,redeem_code_id,outcome,delivery_status,usage_tokens,probability_bps,bonus_bps) VALUES(?,?,'bot','group',?,?,'all',NULL,'none','not_required',100001,3500,500)`, "none-"+strconv.FormatInt(uid, 10), c.ID, uid, uid)
		require.NoError(t, err)
	}
	store := wrapDatabase(db)
	claim, err := scanCodeClaim(store.QueryRowContext(ctx, claimSelect+` WHERE q.user_id=$1`, 42).Scan)
	require.NoError(t, err)
	require.Equal(t, "none", claim.Status)
	require.Empty(t, claim.Type)
	require.Zero(t, claim.Value)
	require.Zero(t, claim.CodeID)
	require.EqualValues(t, 100001, claim.UsageTokens)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_code_claims(id,campaign_id,app_id,group_id,member_id,user_id,period_key,redeem_code_id) VALUES('duplicate',?,'bot','group','new-qq',42,'all',NULL)`, c.ID)
	var duplicate *mysql.MySQLError
	require.ErrorAs(t, err, &duplicate)
	require.EqualValues(t, 1062, duplicate.Number)

	snapshot, err := json.Marshal(c.Prizes[0])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_code_claims(id,campaign_id,app_id,group_id,member_id,user_id,period_key,redeem_code_id,prize_id,reward_snapshot) VALUES('winner',?,'bot','group','winner',44,'all',99,'a',?)`, c.ID, snapshot)
	require.NoError(t, err)
	claim, err = scanCodeClaim(store.QueryRowContext(ctx, claimSelect+` WHERE q.id='winner'`).Scan)
	require.NoError(t, err)
	require.Equal(t, "deleted", claim.Status)
	require.Equal(t, c.Prizes[0].Name, claim.PrizeName)
	require.Equal(t, c.Prizes[0].Value, claim.Value)
	require.NoError(t, loadCampaignStock(ctx, store, &c))
	require.EqualValues(t, 1, c.Prizes[0].Allocated)
	require.Zero(t, c.Prizes[1].Allocated)
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_lottery_draws(id,prize_id) VALUES(1,'stable'),(2,''),(3,'stable')`)
	require.NoError(t, err)
	limit := int64(2)
	pool := []LotteryPrize{{ID: "stable", Name: "Renamed", Amount: 1, StockLimit: &limit}}
	require.NoError(t, loadLotteryStock(ctx, store, pool))
	require.EqualValues(t, 2, pool[0].Allocated)
	require.Equal(t, "谢谢参与", availableLotteryPool(pool)[0].Name)

	now := time.Date(2026, 11, 28, 0, 0, 1, 0, checkinZone)
	for _, entry := range []struct {
		at     time.Time
		tokens int
	}{{now.Add(-2 * time.Second), 999}, {now.Add(-time.Second), 100}, {now, 200}, {now.Add(time.Second), 999}} {
		_, err = db.ExecContext(ctx, `INSERT INTO usage_logs VALUES(42,?,?,?,?,?)`, entry.tokens, entry.tokens, entry.tokens, entry.tokens, entry.at)
		require.NoError(t, err)
	}
	rawTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = rawTx.Rollback() }()
	tokens, err := campaignUsageTokens(ctx, &transaction{raw: rawTx, mysql: true}, 42, now)
	require.NoError(t, err)
	require.EqualValues(t, 1200, tokens)
	require.NoError(t, rawTx.Rollback())
}
