package repository

import (
	"context"
	"database/sql"
	"encoding/json"
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
		for _, table := range []string{"qqc_lottery_draws", "qqc_rewards"} {
			_, err := db.ExecContext(ctx, "DELETE FROM "+table+" WHERE user_id=?", u.ID)
			require.NoError(t, err)
		}
		_, err := db.ExecContext(ctx, `DELETE FROM qqc_code_claims WHERE user_id=?`, u.ID)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `DELETE FROM qqc_lottery_chance_grants WHERE user_id=?`, u.ID)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `DELETE FROM redeem_codes WHERE used_by=? OR notes=?`, u.ID, u.Email)
		require.NoError(t, err)
		require.NoError(t, client.User.DeleteOneID(u.ID).Exec(ctx))
	}()
	// Reproduce the production incident: an older draw has no current CNY
	// entitlement. Redeeming a new code must not pay that historical deficit.
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_lottery_draws(request_key,app_id,member_id,group_id,user_id,prize_name,amount,config_snapshot,recharge_total,earned,remaining,result_text,created_at) VALUES(?,'website','','',?,'old fixture',0,'{}',0,1,0,'{}',DATE_SUB(NOW(),INTERVAL 1 HOUR))`, fmt.Sprintf("historical-redeem-%d", u.ID), u.ID)
	require.NoError(t, err)
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
	require.EqualValues(t, 1, state.Used)
	// A claimed activity code rejects every other account without consumption.
	owned := makeCode()
	_, err = db.ExecContext(ctx, `INSERT INTO qqc_code_claims(id,campaign_id,app_id,group_id,member_id,user_id,period_key,redeem_code_id) VALUES(?,?,'test','test','test',?,'all',?)`, owned.Code, owned.Code, u.ID, owned.ID)
	require.NoError(t, err)
	other, err := client.User.Create().SetEmail("other-" + u.Email).SetPasswordHash("unused").Save(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, client.User.DeleteOneID(other.ID).Exec(ctx)) }()
	_, err = s.Redeem(ctx, other.ID, owned.Code)
	require.ErrorContains(t, err, "领取时绑定")
	unchangedCode, err := codes.GetByID(ctx, owned.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, unchangedCode.Status)
	_, err = s.Redeem(ctx, u.ID, owned.Code)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM qqc_lottery_chance_grants WHERE source_key=?`, fmt.Sprintf("redeem-code:%d", owned.ID))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM qqc_code_claims WHERE redeem_code_id=?`, owned.ID)
	require.NoError(t, err)
	require.NoError(t, codes.Delete(ctx, owned.ID))
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
	// Spend the redeemed chances through the real website settlement path.
	bot := qqcommunity.New(db, nil, false, nil)
	var original string
	oldErr := db.QueryRow("SELECT value FROM settings WHERE `key`='qqc_lottery_config'").Scan(&original)
	require.True(t, oldErr == nil || errors.Is(oldErr, sql.ErrNoRows))
	defer func() {
		if oldErr == nil {
			_, err := db.Exec("UPDATE settings SET value=? WHERE `key`='qqc_lottery_config'", original)
			require.NoError(t, err)
		} else {
			_, err := db.Exec("DELETE FROM settings WHERE `key`='qqc_lottery_config'")
			require.NoError(t, err)
		}
	}()
	lottery, err := bot.LotteryConfig(ctx)
	require.NoError(t, err)
	lottery.Enabled, lottery.GuaranteeEnabled = true, false
	lottery.Prizes = []qqcommunity.LotteryPrize{{Name: "fixture", Amount: 0, ProbabilityBPS: 10000}}
	raw, err := json.Marshal(lottery)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO settings (`key`,value,updated_at) VALUES('qqc_lottery_config',?,NOW()) ON DUPLICATE KEY UPDATE value=VALUES(value)", string(raw))
	require.NoError(t, err)
	type spinOutcome struct {
		result qqcommunity.UserLotterySpin
		err    error
	}
	spins := make(chan spinOutcome, 8)
	start = make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := bot.UserLotterySpin(ctx, u.ID, fmt.Sprintf("redeem-regression-%d-%d", u.ID, i))
			spins <- spinOutcome{result, err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(spins)
	successes = 0
	var replay qqcommunity.UserLotterySpin
	for spin := range spins {
		if spin.err == nil {
			successes++
			replay = spin.result
		} else {
			require.ErrorIs(t, spin.err, qqcommunity.ErrLotteryNoChances)
		}
	}
	require.Equal(t, 5, successes)
	replayed, err := bot.UserLotterySpin(ctx, u.ID, replay.RequestID)
	require.NoError(t, err)
	require.Equal(t, replay, replayed)
	state, err = bot.LotteryState(ctx, u.ID)
	require.NoError(t, err)
	require.Zero(t, state.Remaining)
	require.EqualValues(t, 6, state.Used)
	require.EqualValues(t, 5, state.BonusChances)
	stats, err := bot.ActivityStats(ctx, qqcommunity.ActivityStatsFilter{Search: u.Email, Page: 1, PageSize: 20}, false)
	require.NoError(t, err)
	found := false
	for _, item := range stats.Items {
		if item.UserID == u.ID {
			require.Equal(t, state, item.LotteryState)
			found = true
		}
	}
	require.True(t, found, "the redeemed account must be present in administrator statistics")
}
