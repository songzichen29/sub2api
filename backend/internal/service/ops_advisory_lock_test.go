package service

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestTryAcquireDBAdvisoryLock_MySQLNamedLock(t *testing.T) {
	lockID := int64(12345)
	lockName := fmt.Sprintf("sub2api:advisory:%d", lockID)

	t.Run("acquire_and_release_success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT GET_LOCK\\(\\?, 0\\)").
			WithArgs(lockName).
			WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK(?, 0)"}).AddRow(1))
		mock.ExpectQuery("SELECT RELEASE_LOCK\\(\\?\\)").
			WithArgs(lockName).
			WillReturnRows(sqlmock.NewRows([]string{"RELEASE_LOCK(?)"}).AddRow(1))

		release, ok := tryAcquireDBAdvisoryLock(context.Background(), db, lockID)
		require.True(t, ok)
		require.NotNil(t, release)
		release()

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("lock_busy_returns_false", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT GET_LOCK\\(\\?, 0\\)").
			WithArgs(lockName).
			WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK(?, 0)"}).AddRow(0))

		release, ok := tryAcquireDBAdvisoryLock(context.Background(), db, lockID)
		require.False(t, ok)
		require.Nil(t, release)

		require.NoError(t, mock.ExpectationsWereMet())
	})
}
