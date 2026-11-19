package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// MySQL 索引 DDL 不会留下 PostgreSQL 的 invalid index，无需查询 pg_catalog 或 DROP INDEX CONCURRENTLY。
func TestPrepareNonTransactionalMigrationAPIKeyCostIndexUsesMySQLIndexSemantics(t *testing.T) {
	db, mock := newSQLMock(t)
	err := prepareNonTransactionalMigration(context.Background(), db, usageLogsAPIKeyCostIndexMigration)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
