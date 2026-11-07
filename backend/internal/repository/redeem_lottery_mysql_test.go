package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/Wei-Shaw/sub2api/internal/service"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestRedeemLotteryMySQL(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName, "test must never target production")
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(8)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.MySQL, db)))
	ctx := context.Background()
	u, err := client.User.Create().SetEmail(fmt.Sprintf("lottery-redeem-%d@example.test", time.Now().UnixNano())).SetPasswordHash("unused-test-only").SetBalance(12).SetConcurrency(2).Save(ctx)
	require.NoError(t, err)
	defer func() {
		_, err := db.ExecContext(ctx, `DELETE FROM qqc_lottery_chance_grants WHERE user_id=?`, u.ID)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `DELETE FROM redeem_codes WHERE used_by=? OR notes=?`, u.ID, u.Email)
		require.NoError(t, err)
		require.NoError(t, client.User.DeleteOneID(u.ID).Exec(ctx))
	}()
	codes := NewRedeemCodeRepository(client)
	s := service.NewRedeemService(codes, NewUserRepository(client, db), nil, nil, nil, client, nil)
	makeCode := func() *service.RedeemCode {
		raw, err := service.GenerateRedeemCode()
		require.NoError(t, err)
		code := &service.RedeemCode{Code: raw, Type: service.RedeemTypeLotteryChances, Value: 5, Notes: u.Email}
		require.NoError(t, s.CreateCode(ctx, code))
		return code
	}
	code := makeCode()
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := s.Redeem(ctx, u.ID, code.Code); errs <- err }()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			require.True(t, errors.Is(err, service.ErrRedeemCodeUsed), "unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, successes, "concurrent callers must receive exactly one grant")
	var total, count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(chances) FROM qqc_lottery_chance_grants WHERE user_id=?`, u.ID).Scan(&count, &total))
	require.Equal(t, 1, count)
	require.Equal(t, 5, total)
	state, err := qqcommunity.New(db, nil, false, nil).LotteryState(ctx, u.ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, state.BonusChances)
	require.EqualValues(t, 5, state.Remaining)
	require.Zero(t, state.RechargeTotal)
	unchanged, err := client.User.Get(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.Balance, unchanged.Balance)
	require.Equal(t, u.Concurrency, unchanged.Concurrency)
	history, _, err := s.GetUserHistoryPaginated(ctx, u.ID, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, service.RedeemTypeLotteryChances, history[0].Type)
	// A conflicting ledger source forces grant failure AFTER code consumption;
	// the code must still be unused after the transaction rolls back.
	conflict := makeCode()
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_lottery_chance_grants(source_key,user_id,chances) VALUES(?,?,?)`, fmt.Sprintf("redeem-code:%d", conflict.ID), u.ID, 1)
	require.NoError(t, err)
	_, err = s.Redeem(ctx, u.ID, conflict.Code)
	require.Error(t, err)
	stillUnused, err := codes.GetByID(ctx, conflict.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, stillUnused.Status)
	require.Nil(t, stillUnused.UsedBy)
	_, err = db.ExecContext(ctx, `DELETE FROM qqc_lottery_chance_grants WHERE source_key=?`, fmt.Sprintf("redeem-code:%d", conflict.ID))
	require.NoError(t, err)
	// Removing an already-used code must not remove its granted chances.
	require.NoError(t, codes.Delete(ctx, code.ID))
	state, err = qqcommunity.New(db, nil, false, nil).LotteryState(ctx, u.ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, state.Remaining)
}
