package qqcommunity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestConfirmTransferCancelsRequestWithoutMovingMoney(t *testing.T) {
	for _, tc := range []struct {
		name          string
		senderBalance string
		recipientGone bool
	}{
		{name: "insufficient balance", senderBalance: "1.00000000"},
		{name: "recipient removed", senderBalance: "3.00000000", recipientGone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			c := DefaultConfig()
			c.Enabled = true
			c.TransferEnabled = true
			c.AppID = "bot"
			raw, err := json.Marshal(c)
			require.NoError(t, err)
			token := randomID() + randomID()
			hash := codeHash(token)
			expectStart := func() {
				mock.ExpectBegin()
				mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).
					WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
				mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(c.AppID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			expectStart()
			mock.ExpectQuery("SELECT r.from_user_id").WithArgs(hash, c.AppID).
				WillReturnRows(sqlmock.NewRows([]string{
					"from_user_id", "to_user_id", "from_email", "to_email", "amount", "net_amount", "expires_at",
				}).AddRow(1, 2, "sender@example.test", "recipient@example.test", "2.00000000", "2.00000000", time.Now().Add(time.Minute)))
			mock.ExpectQuery("SELECT balance::text FROM users").WithArgs(int64(1)).
				WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(tc.senderBalance))
			if tc.recipientGone {
				mock.ExpectQuery("SELECT id FROM users WHERE id").WithArgs(int64(2)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
			}
			mock.ExpectExec("DELETE FROM qqc_transfer_requests").WithArgs(hash, c.AppID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			err = s.ConfirmTransfer(context.Background(), token)
			require.ErrorContains(t, err, "已取消")

			// A later balance top-up or recipient recovery cannot revive the link.
			expectStart()
			mock.ExpectQuery("SELECT r.from_user_id").WithArgs(hash, c.AppID).
				WillReturnRows(sqlmock.NewRows([]string{"from_user_id", "to_user_id", "from_email", "to_email", "amount", "net_amount", "expires_at"}))
			mock.ExpectRollback()
			err = s.ConfirmTransfer(context.Background(), token)
			require.ErrorIs(t, err, ErrTransferInvalid)
			require.False(t, strings.Contains(err.Error(), "已取消"))
			// Any balance update or history INSERT would violate the expectations.
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
