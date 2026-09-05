package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetLatestUsedAtByUserIDsUsesDatabaseTimezoneEpoch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	want := time.Date(2026, time.July, 22, 11, 36, 25, 970972000, time.UTC)
	mock.ExpectQuery(`SELECT user_id, UNIX_TIMESTAMP\(MAX\(created_at\)\) AS last_used_at_unix`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "last_used_at_unix"}).
			AddRow(int64(1), strconv.FormatInt(want.Unix(), 10)+".970972"))

	repo := newUserRepositoryWithSQL(nil, db)
	gotByUser, err := repo.GetLatestUsedAtByUserIDs(context.Background(), []int64{1})
	require.NoError(t, err)
	require.Equal(t, want, *gotByUser[1])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLUnixTimestampToUTCPreservesMicroseconds(t *testing.T) {
	want := time.Date(2026, time.July, 22, 11, 36, 25, 970972000, time.UTC)
	got, err := mysqlUnixTimestampToUTC(strconv.FormatInt(want.Unix(), 10) + ".970972")
	require.NoError(t, err)
	require.Equal(t, want, got)
}
