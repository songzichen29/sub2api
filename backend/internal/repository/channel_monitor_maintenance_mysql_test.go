package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newMySQLChannelMonitorRepositoryTest(t *testing.T) (*channelMonitorRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.MySQL, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return &channelMonitorRepository{client: client, db: db}, mock
}

func TestChannelMonitorDailyRollupUsesMySQLSyntax(t *testing.T) {
	repo, mock := newMySQLChannelMonitorRepositoryTest(t)
	target := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(channelMonitorDailyRollupMySQL)).
		WithArgs("2026-09-18", "2026-09-18", "2026-09-18").
		WillReturnResult(sqlmock.NewResult(0, 3))

	affected, err := repo.UpsertDailyRollupsFor(context.Background(), target)
	require.NoError(t, err)
	require.Equal(t, int64(3), affected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorWatermarkUsesMySQLUpsert(t *testing.T) {
	repo, mock := newMySQLChannelMonitorRepositoryTest(t)
	target := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(channelMonitorWatermarkMySQL)).
		WithArgs("2026-09-18").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.UpdateAggregationWatermark(context.Background(), target))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorRetentionUsesMySQLBoundedDelete(t *testing.T) {
	repo, mock := newMySQLChannelMonitorRepositoryTest(t)
	cutoff := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(channelMonitorPruneHistoryMySQL)).
		WithArgs(cutoff, channelMonitorPruneBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(channelMonitorPruneHistoryMySQL)).
		WithArgs(cutoff, channelMonitorPruneBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 0))

	deleted, err := repo.DeleteHistoryBefore(context.Background(), cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorMySQLMaintenanceSQLHasNoPostgresSyntax(t *testing.T) {
	for _, query := range []string{
		channelMonitorDailyRollupMySQL,
		channelMonitorPruneHistoryMySQL,
		channelMonitorPruneRollupMySQL,
		channelMonitorWatermarkMySQL,
	} {
		require.NotContains(t, query, "$1")
		require.NotContains(t, query, "::date")
		require.NotContains(t, query, "FILTER (")
		require.NotContains(t, query, "ON CONFLICT")
	}
}
