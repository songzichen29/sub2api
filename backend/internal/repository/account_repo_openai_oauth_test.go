package repository

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_UpdateOpenAIOAuthCredentialsIfUnchanged_MySQL(t *testing.T) {
	proxyID := int64(29)
	failure := errors.New("database failure")
	for _, tc := range []struct {
		name      string
		proxyID   *int64
		affected  int64
		execErr   error
		outboxErr error
		commitErr error
	}{
		{name: "success", proxyID: &proxyID, affected: 1},
		{name: "null proxy", affected: 1},
		{name: "concurrent credentials changed"},
		{name: "update failure", execErr: failure},
		{name: "outbox failure rolls back credentials", affected: 1, outboxErr: failure},
		{name: "commit failure", affected: 1, commitErr: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
			repo := newAccountRepositoryWithSQL(client, db, nil)
			mock.ExpectBegin()
			update := mock.ExpectExec(`UPDATE accounts AS a SET credentials = CAST\(\? AS JSON\), updated_at = NOW\(\) WHERE a.id = \? AND a.deleted_at IS NULL AND a.platform = \? AND a.type = \? AND a.credentials = CAST\(\? AS JSON\) AND a.proxy_id <=> \? AND a.parent_account_id IS NULL`).
				WithArgs(`{"refresh_token":"rotated"}`, int64(42), service.PlatformOpenAI, service.AccountTypeOAuth, `{"refresh_token":"attempted"}`, tc.proxyID)
			if tc.execErr != nil {
				update.WillReturnError(tc.execErr)
			}
			if tc.execErr == nil {
				update.WillReturnResult(sqlmock.NewResult(0, tc.affected))
			}
			if tc.execErr == nil && tc.affected > 0 {
				outbox := mock.ExpectExec(`INSERT INTO scheduler_outbox`).
					WithArgs(service.SchedulerOutboxEventAccountChanged, int64(42), nil, nil, sqlmock.AnyArg())
				if tc.outboxErr != nil {
					outbox.WillReturnError(tc.outboxErr)
				}
				if tc.outboxErr == nil {
					outbox.WillReturnResult(sqlmock.NewResult(1, 1))
					commit := mock.ExpectCommit()
					if tc.commitErr != nil {
						commit.WillReturnError(tc.commitErr)
					}
				}
			}
			if tc.execErr != nil || tc.affected == 0 || tc.outboxErr != nil {
				mock.ExpectRollback()
			}

			applied, err := repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
				context.Background(), 42, map[string]any{"refresh_token": "attempted"},
				tc.proxyID, map[string]any{"refresh_token": "rotated"})

			if tc.execErr != nil || tc.outboxErr != nil || tc.commitErr != nil {
				require.ErrorIs(t, err, failure)
				require.False(t, applied)
			}
			if tc.execErr == nil && tc.outboxErr == nil && tc.commitErr == nil {
				require.NoError(t, err)
				require.Equal(t, tc.affected > 0, applied)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
