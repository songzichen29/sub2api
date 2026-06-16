package repository

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

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
