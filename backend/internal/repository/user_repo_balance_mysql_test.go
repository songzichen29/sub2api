package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newMySQLBalanceRepoMock(t *testing.T) (*userRepository, sqlmock.Sqlmock) {
	t.Helper()

	matcher := sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		upperSQL := strings.ToUpper(actualSQL)
		if strings.Contains(actualSQL, "$") ||
			strings.Contains(upperSQL, "RETURNING") ||
			(strings.HasPrefix(strings.TrimSpace(upperSQL), "UPDATE") && strings.Contains(upperSQL, " FROM ")) {
			return fmt.Errorf("PostgreSQL-only SQL generated for MySQL: %s", actualSQL)
		}
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.MySQL, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return newUserRepositoryWithSQL(client, db), mock
}

func expectMySQLLockedBalance(mock sqlmock.Sqlmock, id int64, balance *float64) {
	rows := sqlmock.NewRows([]string{"id", "balance"})
	if balance != nil {
		rows.AddRow(id, *balance)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `users`.`id`, `users`.`balance` FROM `users` WHERE (`users`.`id` = ? AND `users`.`deleted_at` IS NULL) AND `users`.`deleted_at` IS NULL LIMIT 2 FOR UPDATE")).
		WithArgs(id).
		WillReturnRows(rows)
}

func expectMySQLBalanceUpdate(mock sqlmock.Sqlmock, id int64, balance float64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `updated_at` = ?, `balance` = ? WHERE `users`.`id` = ? AND `users`.`deleted_at` IS NULL")).
		WithArgs(sqlmock.AnyArg(), balance, id).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestAdjustBalance_UsesMySQLTransactionAndReportsChange(t *testing.T) {
	repo, mock := newMySQLBalanceRepoMock(t)
	current := 10.0

	mock.ExpectBegin()
	expectMySQLLockedBalance(mock, 42, &current)
	expectMySQLBalanceUpdate(mock, 42, 15)
	mock.ExpectCommit()

	change, err := repo.AdjustBalance(context.Background(), 42, 5)
	require.NoError(t, err)
	require.Equal(t, service.BalanceChange{Old: 10, New: 15}, change)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetBalance_UsesMySQLTransactionAndReportsChange(t *testing.T) {
	repo, mock := newMySQLBalanceRepoMock(t)
	current := 10.0

	mock.ExpectBegin()
	expectMySQLLockedBalance(mock, 42, &current)
	expectMySQLBalanceUpdate(mock, 42, 3)
	mock.ExpectCommit()

	change, err := repo.SetBalance(context.Background(), 42, 3)
	require.NoError(t, err)
	require.Equal(t, service.BalanceChange{Old: 10, New: 3}, change)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdjustBalance_MySQLRejectsNegativeWithoutUpdate(t *testing.T) {
	repo, mock := newMySQLBalanceRepoMock(t)
	current := 2.0

	mock.ExpectBegin()
	expectMySQLLockedBalance(mock, 42, &current)
	mock.ExpectRollback()

	change, err := repo.AdjustBalance(context.Background(), 42, -3)
	require.ErrorIs(t, err, service.ErrBalanceNegative)
	require.Equal(t, service.BalanceChange{Old: 2, New: -1}, change)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdjustBalance_MySQLReturnsUserNotFound(t *testing.T) {
	repo, mock := newMySQLBalanceRepoMock(t)

	mock.ExpectBegin()
	expectMySQLLockedBalance(mock, 404, nil)
	mock.ExpectRollback()

	_, err := repo.AdjustBalance(context.Background(), 404, 1)
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
