package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepositoryBatchUpdateLastUsed_DialectLock(t *testing.T) {
	for _, dbDialect := range []string{dialect.MySQL, dialect.Postgres} {
		t.Run(dbDialect, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dbDialect, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, nil)
			earlier := time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)
			later := earlier.Add(time.Minute)

			mock.ExpectBegin()
			if dbDialect == dialect.MySQL {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM accounts WHERE id IN (?,?) AND deleted_at IS NULL ORDER BY id FOR UPDATE")).
					WithArgs(int64(3), int64(9)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3).AddRow(9)).RowsWillBeClosed()
			}
			if dbDialect == dialect.Postgres {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM accounts WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY id FOR NO KEY UPDATE")).
					WithArgs("{3,9}").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3).AddRow(9)).RowsWillBeClosed()
			}
			mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET last_used_at = CASE id WHEN ? THEN ? WHEN ? THEN ? END, updated_at = NOW() WHERE id IN (?,?) AND deleted_at IS NULL")).
				WithArgs(int64(3), earlier, int64(9), later, int64(3), int64(9)).
				WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectCommit()
			mock.ExpectExec("INSERT INTO scheduler_outbox").
				WithArgs(service.SchedulerOutboxEventAccountLastUsed, nil, nil, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))

			require.NoError(t, repo.BatchUpdateLastUsed(context.Background(), map[int64]time.Time{9: later, 3: earlier}))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountRepositoryBatchUpdateLastUsed_MySQLRollsBack(t *testing.T) {
	for _, failAt := range []string{"lock", "update"} {
		t.Run(failAt, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, nil)
			wantErr := errors.New("write failed")
			mock.ExpectBegin()
			lock := mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM accounts WHERE id IN (?) AND deleted_at IS NULL ORDER BY id FOR UPDATE")).WithArgs(int64(3))
			if failAt == "lock" {
				lock.WillReturnError(wantErr)
			}
			if failAt == "update" {
				lock.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3)).RowsWillBeClosed()
				mock.ExpectExec("UPDATE accounts SET last_used_at").WillReturnError(wantErr)
			}
			mock.ExpectRollback()

			require.ErrorIs(t, repo.BatchUpdateLastUsed(context.Background(), map[int64]time.Time{3: time.Now()}), wantErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
