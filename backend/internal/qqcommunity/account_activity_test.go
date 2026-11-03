package qqcommunity

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountActivityScopesBothLedgersAndPaginates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	userID := int64(42)
	now := time.Date(2026, 11, 3, 6, 30, 0, 0, time.UTC)
	columns := []string{"id", "code", "type", "value", "status", "used_at", "created_at", "notes", "group_id", "validity_days", "group_name", "counterparty", "amount", "fee", "net_amount"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*COUNT.* FROM redeem_codes").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(5))
	mock.ExpectQuery("SELECT id,code,type,value,status").WithArgs(userID, 2, 2).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(9, "", "transfer_in", 0.95, "completed", now, now, "", nil, 0, "", "sender@example.test", "1.00000000", "0.05000000", "0.95000000").
			AddRow(8, "", "transfer_out", -2.0, "completed", now.Add(-time.Minute), now.Add(-time.Minute), "", nil, 0, "", "recipient@example.test", "2.00000000", "0.10000000", "1.90000000"))
	mock.ExpectCommit()
	items, total, err := s.AccountActivity(context.Background(), userID, 2, 2)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	require.Len(t, items, 2)
	require.Equal(t, "transfer_in", items[0].Type)
	require.Equal(t, "sender@example.test", items[0].Counterparty)
	require.InDelta(t, 0.95, items[0].Value, 1e-8)
	require.Equal(t, "transfer_out", items[1].Type)
	require.Equal(t, "recipient@example.test", items[1].Counterparty)
	require.InDelta(t, -2, items[1].Value, 1e-8)
	require.Equal(t, "0.10000000", items[1].Fee)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountActivityRejectsInvalidIdentityAndPagination(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	for _, args := range [][3]int{{0, 1, 20}, {42, 0, 20}, {42, 1, 101}, {42, 1000002, 1}} {
		_, _, err := s.AccountActivity(context.Background(), int64(args[0]), args[1], args[2])
		require.Error(t, err)
	}
}

func TestAccountActivityQuotesGroupsInMySQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	s.db.mysql = true
	userID := int64(42)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*COUNT.* FROM redeem_codes").WithArgs(userID, userID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	mock.ExpectQuery("LEFT JOIN `groups` g").WithArgs(userID, userID, userID, userID, userID, userID, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "type", "value", "status", "used_at", "created_at", "notes", "group_id", "validity_days", "group_name", "counterparty", "amount", "fee", "net_amount"}))
	mock.ExpectCommit()
	items, total, err := s.AccountActivity(context.Background(), userID, 1, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
