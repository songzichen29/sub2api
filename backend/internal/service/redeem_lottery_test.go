package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestRedeemLotteryValidationAllEntryPoints(t *testing.T) {
	ctx := context.Background()
	for _, value := range []float64{-1, 0, 0.5, 1.5, 1001, math.NaN(), math.Inf(1), math.Inf(-1)} {
		require.Error(t, validateLotteryRedeemValue(RedeemTypeLotteryChances, value))
		s := &RedeemService{}
		_, err := s.GenerateCodes(ctx, GenerateCodesRequest{Count: 1, Type: RedeemTypeLotteryChances, Value: value})
		require.Error(t, err)
		require.Error(t, s.CreateCode(ctx, &RedeemCode{Code: "invalid", Type: RedeemTypeLotteryChances, Value: value}))
		_, err = (&adminServiceImpl{}).GenerateRedeemCodes(ctx, &GenerateRedeemCodesInput{Count: 1, Type: RedeemTypeLotteryChances, Value: value})
		require.Error(t, err)
		r := &redeemRejectRepo{code: RedeemCode{ID: 1, Code: "invalid", Type: RedeemTypeLotteryChances, Value: value, Status: StatusUnused}}
		_, err = (&RedeemService{redeemRepo: r}).Redeem(ctx, 42, "invalid")
		require.Error(t, err)
		require.False(t, r.useCalled, "invalid persisted values cannot consume the code")
	}
	for _, value := range []float64{1, 5, 1000} {
		require.NoError(t, validateLotteryRedeemValue(RedeemTypeLotteryChances, value))
	}
	require.NoError(t, validateLotteryRedeemValue(RedeemTypeBalance, -5), "preserve existing balance refund codes")
}

type lotteryGenerateRepo struct {
	RedeemCodeRepository
	codes []RedeemCode
}

func (r *lotteryGenerateRepo) Create(_ context.Context, code *RedeemCode) error {
	code.ID = int64(len(r.codes) + 1)
	r.codes = append(r.codes, *code)
	return nil
}

func TestRedeemLotteryAdminBatchGeneration(t *testing.T) {
	repo := &lotteryGenerateRepo{}
	expires := time.Now().Add(24 * time.Hour)
	codes, err := (&adminServiceImpl{redeemCodeRepo: repo}).GenerateRedeemCodes(context.Background(), &GenerateRedeemCodesInput{
		Count: 3, Type: RedeemTypeLotteryChances, Value: 5, ExpiresAt: &expires,
	})
	require.NoError(t, err)
	require.Len(t, repo.codes, 3)
	unique := make(map[string]bool)
	for _, code := range codes {
		require.Len(t, code.Code, 32)
		require.False(t, unique[code.Code])
		unique[code.Code] = true
		require.Equal(t, RedeemTypeLotteryChances, code.Type)
		require.EqualValues(t, 5, code.Value)
		require.Equal(t, StatusUnused, code.Status)
		require.Equal(t, &expires, code.ExpiresAt)
	}
}

type lotteryRedeemUserStub struct{ UserRepository }

func (*lotteryRedeemUserStub) GetByID(_ context.Context, id int64) (*User, error) {
	return &User{ID: id, Balance: 100, Concurrency: 2}, nil
}

type lotteryTransactionalRedeemRepo struct {
	redeemRejectRepo
	useError error
}

func (r *lotteryTransactionalRedeemRepo) Use(ctx context.Context, id, uid int64) error {
	if r.useError != nil {
		return r.useError
	}
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return errors.New("missing redemption transaction")
	}
	tx.OnCommit(func(next dbent.Committer) dbent.Committer {
		return dbent.CommitFunc(func(ctx context.Context, tx *dbent.Tx) error {
			if err := next.Commit(ctx, tx); err != nil {
				return err
			}
			r.code.Status, r.code.UsedBy = StatusUsed, &uid
			return nil
		})
	})
	return nil
}

func TestRedeemLotteryAtomicGrantBothDialects(t *testing.T) {
	for _, dbDialect := range []string{dialect.Postgres, dialect.MySQL} {
		for _, scenario := range []string{"success", "grant failure", "already consumed"} {
			t.Run(dbDialect+"/"+scenario, func(t *testing.T) {
				db, m, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dbDialect, db)))
				r := &lotteryTransactionalRedeemRepo{redeemRejectRepo: redeemRejectRepo{code: RedeemCode{ID: 7, Code: "CHANCE-CODE", Type: RedeemTypeLotteryChances, Value: 5, Status: StatusUnused}}}
				s := NewRedeemService(r, &lotteryRedeemUserStub{}, nil, nil, nil, client, nil)
				m.ExpectBegin()
				m.ExpectQuery("SELECT .*user_id.* FROM .*qqc_code_claims").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
				if scenario == "already consumed" {
					r.useError = ErrRedeemCodeUsed
				} else {
					insert := m.ExpectExec("INSERT INTO .*qqc_lottery_chance_grants").WithArgs("redeem-code:7", int64(42), 5)
					if scenario == "grant failure" {
						insert.WillReturnError(errors.New("grant failed"))
					} else {
						insert.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				if scenario == "success" {
					m.ExpectCommit()
				} else {
					m.ExpectRollback()
				}
				out, err := s.Redeem(context.Background(), 42, r.code.Code)
				if scenario == "success" {
					require.NoError(t, err)
					require.Equal(t, StatusUsed, out.Status)
					require.EqualValues(t, 42, *out.UsedBy)
					for _, uid := range []int64{42, 43} {
						_, err = s.Redeem(context.Background(), uid, r.code.Code)
						require.ErrorIs(t, err, ErrRedeemCodeUsed)
					}
				} else {
					require.Error(t, err)
					require.Equal(t, StatusUnused, r.code.Status)
					require.Nil(t, r.code.UsedBy)
				}
				require.NoError(t, m.ExpectationsWereMet(), "must not update balances, concurrency or subscriptions")
			})
		}
	}
}

func TestRedeemLotteryRequiresTransaction(t *testing.T) {
	require.ErrorContains(t, grantRedeemLotteryChances(context.Background(), 1, 42, 5), "requires a transaction")
}

func TestRedeemLotteryExpiredAndDisabledCodesStayUnused(t *testing.T) {
	for _, status := range []string{StatusExpired, StatusDisabled} {
		r := &redeemRejectRepo{code: RedeemCode{ID: 1, Code: "unavailable", Type: RedeemTypeLotteryChances, Value: 5, Status: status}}
		_, err := (&RedeemService{redeemRepo: r}).Redeem(context.Background(), 42, r.code.Code)
		require.Error(t, err)
		require.False(t, r.useCalled)
	}
}
