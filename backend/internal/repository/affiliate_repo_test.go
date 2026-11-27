package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestAffiliateRebateRecordsPreserveNonOrderAccruals(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, username TEXT)`,
		`CREATE TABLE user_affiliate_ledger (
			id INTEGER PRIMARY KEY, user_id INTEGER, source_user_id INTEGER,
			source_order_id INTEGER, action TEXT, amount REAL, created_at TIMESTAMP)`,
		`CREATE TABLE payment_orders (
			id INTEGER PRIMARY KEY, out_trade_no TEXT, amount REAL, pay_amount REAL,
			order_type TEXT, payment_type TEXT, status TEXT)`,
		`INSERT INTO users VALUES
			(1, 'inviter@example.com', 'inviter'),
			(2, 'invitee@example.com', '1336169509')`,
		`INSERT INTO payment_orders VALUES
			(10, 'order-10', 20, 140, 'balance', 'alipay', 'COMPLETED')`,
	} {
		_, err := db.ExecContext(ctx, statement)
		require.NoError(t, err)
	}
	createdAt := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	_, err = db.ExecContext(ctx, `INSERT INTO user_affiliate_ledger VALUES
		(1, 1, 2, NULL, 'accrue', 2, ?),
		(2, 1, 2, 10, 'accrue', 3, ?)`, createdAt, createdAt.Add(-time.Hour))
	require.NoError(t, err)

	repo := NewAffiliateRepository(client, db)
	records, total, err := repo.ListAffiliateRebateRecords(ctx, service.AffiliateRecordFilter{
		Search:   "1336169509",
		Page:     1,
		PageSize: 20,
		SortBy:   "created_at",
		SortDesc: true,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, records, 2)

	require.Nil(t, records[0].OrderID)
	require.Nil(t, records[0].OrderAmount)
	require.Nil(t, records[0].PayAmount)
	require.Empty(t, records[0].OutTradeNo)
	require.Empty(t, records[0].OrderType)
	require.Empty(t, records[0].PaymentType)
	require.Empty(t, records[0].OrderStatus)
	require.EqualValues(t, 1, records[0].InviterID)
	require.EqualValues(t, 2, *records[0].InviteeID)
	require.InDelta(t, 2, records[0].RebateAmount, 1e-9)
	require.Equal(t, createdAt, records[0].CreatedAt)

	require.EqualValues(t, 10, *records[1].OrderID)
	require.InDelta(t, 20, *records[1].OrderAmount, 1e-9)
	require.InDelta(t, 140, *records[1].PayAmount, 1e-9)
	require.Equal(t, "order-10", records[1].OutTradeNo)
	require.Equal(t, "balance", records[1].OrderType)
	require.Equal(t, "alipay", records[1].PaymentType)
	require.Equal(t, "COMPLETED", records[1].OrderStatus)
	require.InDelta(t, 3, records[1].RebateAmount, 1e-9)
}

func TestAffiliateUserOverviewSQLIncludesMaturedFrozenQuota(t *testing.T) {
	query := strings.Join(strings.Fields(affiliateUserOverviewSQL), " ")

	require.Contains(t, query, "ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0)")
	require.Contains(t, query, "frozen_until <= NOW()")
}

func TestAffiliateRecordQueriesUseLedgerAuditFields(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "JOIN payment_orders po ON po.id = ual.source_order_id")
	// 迁移到 MySQL 后不再做 ::double precision 类型转换；只验证字段被 SELECT 出来。
	require.Contains(t, content, "ual.amount,")
	require.Contains(t, content, "ual.balance_after,")
	require.NotContains(t, content, "::double precision")
	require.NotContains(t, content, "parseAffiliateRebateAmount")
	require.NotContains(t, content, `"current_balance": "u.balance"`)
}

func TestBuildAffiliateRecordWhereRepeatsSearchArgForEachColumn(t *testing.T) {
	start := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 16, 23, 59, 59, 0, time.UTC)

	where, args := buildAffiliateRecordWhere(service.AffiliateRecordFilter{
		Search:  "  1410134718@qq.com  ",
		StartAt: &start,
		EndAt:   &end,
	}, "ual.created_at", []string{
		"inviter.email",
		"invitee.email",
		"CAST(po.id AS CHAR)",
	})

	require.Contains(t, where, "ual.created_at >= ?")
	require.Contains(t, where, "ual.created_at <= ?")
	require.Contains(t, where, "LOWER(inviter.email) LIKE ?")
	require.Contains(t, where, "LOWER(invitee.email) LIKE ?")
	require.Contains(t, where, "LOWER(CAST(po.id AS CHAR)) LIKE ?")
	require.Equal(t, []any{
		start,
		end,
		"%1410134718@qq.com%",
		"%1410134718@qq.com%",
		"%1410134718@qq.com%",
	}, args)
}
