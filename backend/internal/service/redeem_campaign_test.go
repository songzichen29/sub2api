package service

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestCampaignRecipientAtomicBothDialects(t *testing.T) {
	for _, d := range []string{dialect.Postgres, dialect.MySQL} {
		for _, scenario := range []string{"owner", "other account", "ledger unavailable"} {
			t.Run(d+"/"+scenario, func(t *testing.T) {
				db, m, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				client := dbent.NewClient(dbent.Driver(entsql.OpenDB(d, db)))
				r := &lotteryTransactionalRedeemRepo{redeemRejectRepo: redeemRejectRepo{code: RedeemCode{ID: 7, Code: "claimed", Type: RedeemTypeLotteryChances, Value: 3, Status: StatusUnused}}}
				s := NewRedeemService(r, &lotteryRedeemUserStub{}, nil, nil, nil, client, nil)
				m.ExpectBegin()
				q := m.ExpectQuery("SELECT .*user_id.* FROM .*qqc_code_claims").WithArgs(int64(7))
				if scenario == "ledger unavailable" {
					q.WillReturnError(errors.New("db unavailable"))
				} else {
					owner := int64(42)
					if scenario == "other account" {
						owner = 99
					}
					q.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(owner))
				}
				if scenario == "owner" {
					m.ExpectExec("INSERT INTO .*qqc_lottery_chance_grants").WithArgs("redeem-code:7", int64(42), 3).WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectCommit()
				} else {
					m.ExpectRollback()
				}
				_, err = s.Redeem(context.Background(), 42, "claimed")
				if scenario == "owner" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.Equal(t, StatusUnused, r.code.Status)
					require.Nil(t, r.code.UsedBy)
				}
				require.NoError(t, m.ExpectationsWereMet())
			})
		}
	}
}
