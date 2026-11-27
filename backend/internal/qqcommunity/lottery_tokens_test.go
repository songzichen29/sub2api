package qqcommunity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestLotteryTokenBonusScalesNormalPoolAndPreservesTotal(t *testing.T) {
	c := LotteryConfig{
		Prizes: []LotteryPrize{
			{Name: "大礼", Amount: 1, ProbabilityBPS: 1000},
			{Name: "小礼", Amount: .5, ProbabilityBPS: 2000},
			{Name: "谢谢参与", Amount: 0, ProbabilityBPS: 7000},
		},
		TokenTiers: []CampaignTokenTier{{Tokens: 1_000_000, BonusBPS: 500}},
	}
	pool, bonus := lotteryPoolForTokens(c, 1_000_000)
	require.Equal(t, 500, bonus.BonusBPS)
	require.Equal(t, []int{1166, 2334, 6500}, []int{pool[0].ProbabilityBPS, pool[1].ProbabilityBPS, pool[2].ProbabilityBPS})
	require.Equal(t, 10000, pool[0].ProbabilityBPS+pool[1].ProbabilityBPS+pool[2].ProbabilityBPS)
}

func TestLotteryTokenBonusOnlyCountsAvailableRewards(t *testing.T) {
	limit := int64(1)
	c := LotteryConfig{
		Prizes: []LotteryPrize{
			{Name: "已耗尽", Amount: 1, ProbabilityBPS: 1000, StockLimit: &limit, Allocated: 1},
			{Name: "仍可得", Amount: .5, ProbabilityBPS: 2000},
			{Name: "谢谢参与", Amount: 0, ProbabilityBPS: 7000},
		},
		TokenTiers: []CampaignTokenTier{{Tokens: 1_000_000, BonusBPS: 500}},
	}
	pool, bonus := lotteryPoolForTokens(c, 1_000_000)
	require.Equal(t, 334, bonus.BonusBPS)
	require.Equal(t, "谢谢参与", pool[0].Name)
	require.Equal(t, 0.0, pool[0].Amount)
	require.Equal(t, 10000, pool[0].ProbabilityBPS+pool[1].ProbabilityBPS+pool[2].ProbabilityBPS)
}

func TestLotteryTokenTierValidationUsesOnlyNormalRewardProbability(t *testing.T) {
	c := defaultLotteryConfig()
	c.TokenTiers = []CampaignTokenTier{{Tokens: 1_000_000, BonusBPS: 500}}
	require.NoError(t, validateLottery(c))

	c.TokenTiers = []CampaignTokenTier{{Tokens: 1_000_000, BonusBPS: 9001}}
	require.Error(t, validateLottery(c))

	c.TokenTiers = []CampaignTokenTier{{Tokens: 2_000_000, BonusBPS: 500}, {Tokens: 1_000_000, BonusBPS: 600}}
	require.Error(t, validateLottery(c))

	c.TokenTiers = []CampaignTokenTier{{Tokens: 1, BonusBPS: 500}, {Tokens: 2, BonusBPS: 500}}
	require.Error(t, validateLottery(c))
	c.TokenTiers = []CampaignTokenTier{{Tokens: 9007199254740992, BonusBPS: 1}}
	require.Error(t, validateLottery(c))
	c.TokenTiers = make([]CampaignTokenTier, 21)
	require.Error(t, validateLottery(c))
	c.TokenTiers = []CampaignTokenTier{{Tokens: 1, BonusBPS: 1}}
	c.Prizes = []LotteryPrize{{Name: "谢谢参与", ProbabilityBPS: 10000}}
	require.Error(t, validateLottery(c))
	c.Prizes = []LotteryPrize{{Name: "已全中奖", Amount: 1, ProbabilityBPS: 10000}}
	require.Error(t, validateLottery(c))
}

func TestLotteryTokenTierBoundariesAndExhaustion(t *testing.T) {
	c := defaultLotteryConfig()
	c.TokenTiers = []CampaignTokenTier{{Tokens: 100000, BonusBPS: 500}, {Tokens: 500000, BonusBPS: 1000}, {Tokens: 2000000, BonusBPS: 9000}}
	for _, tc := range []struct {
		tokens int64
		win    int
		bonus  int
	}{{0, 1000, 0}, {99999, 1000, 0}, {100000, 1500, 500}, {499999, 1500, 500}, {500000, 2000, 1000}, {2000000, 10000, 9000}} {
		pool, bonus := lotteryPoolForTokens(c, tc.tokens)
		require.Equal(t, tc.bonus, bonus.BonusBPS)
		require.Equal(t, tc.tokens, bonus.UsageTokens)
		wins := 0
		for roll := int64(0); roll < 10000; roll++ {
			if lotteryPrizeAt(pool, roll).Amount > 0 {
				wins++
			}
		}
		require.Equal(t, tc.win, wins)
	}
	limit := int64(0)
	c.Prizes[0].StockLimit = &limit
	pool, bonus := lotteryPoolForTokens(c, 2000000)
	require.Zero(t, bonus.BonusBPS)
	require.Equal(t, "谢谢参与", pool[0].Name)
	require.Zero(t, lotteryPrizeAt(pool, 9999).Amount)
	c.TokenTiers = nil
	_, bonus = lotteryPoolForTokens(c, 2000000)
	require.Nil(t, bonus)
}

type lotteryBonusSnapshot struct {
	bonus      *LotteryTokenBonus
	guaranteed bool
}

func (want lotteryBonusSnapshot) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var snapshot struct {
		TokenBonus   *LotteryTokenBonus `json:"token_bonus"`
		IsGuaranteed bool               `json:"is_guaranteed"`
	}
	if json.Unmarshal([]byte(raw), &snapshot) != nil || snapshot.IsGuaranteed != want.guaranteed {
		return false
	}
	if want.bonus == nil {
		return snapshot.TokenBonus == nil
	}
	return snapshot.TokenBonus != nil && *want.bonus == *snapshot.TokenBonus
}

func TestWebLotteryUsesDailyTokenBonusAndLeavesGuaranteeIndependent(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		for _, guaranteed := range []bool{false, true} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			s := New(db, nil, false, nil)
			s.db.mysql = mysql
			c := defaultLotteryConfig()
			c.Enabled, c.GuaranteeEnabled, c.GuaranteeEvery = true, true, 2
			c.Prizes[0].Amount = 2
			c.TokenTiers = []CampaignTokenTier{{Tokens: 1000000, BonusBPS: 9000}}
			used, amount := int64(0), float64(2)
			bonus := &LotteryTokenBonus{UsageTokens: 1000000, BonusBPS: 9000}
			if guaranteed {
				used, amount, bonus = 1, 1, nil
			}
			expectWebStart(mock, mysql)
			mock.ExpectQuery("SELECT result_text").WillReturnError(sql.ErrNoRows)
			expectWebState(mock, c, used, "30", mysql)
			if !guaranteed {
				mock.ExpectQuery(`SELECT COALESCE\(SUM\(input_tokens\)`).WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"tokens"}).AddRow(1000000))
			}
			args := []driver.Value{sqlmock.AnyArg(), "website", "", "", int64(42), sqlmock.AnyArg(), amount, lotteryBonusSnapshot{bonus: bonus, guaranteed: guaranteed}, float64(30), int64(3), int64(2) - used, sqlmock.AnyArg(), ""}
			if mysql {
				mock.ExpectExec("INSERT INTO qqc_lottery_draws").WithArgs(args...).WillReturnResult(sqlmock.NewResult(7, 1))
			} else {
				mock.ExpectQuery("INSERT INTO qqc_lottery_draws").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			}
			mock.ExpectExec("INSERT INTO qqc_rewards").WillReturnResult(sqlmock.NewResult(8, 1))
			mock.ExpectExec("UPDATE users SET balance").WithArgs(amount, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(9, 1))
			mock.ExpectCommit()
			out, err := s.UserLotterySpin(context.Background(), 42, webRequestID)
			require.NoError(t, err)
			require.Equal(t, amount, out.Prize.Amount)
			require.Equal(t, bonus, out.TokenBonus)
			require.Equal(t, guaranteed, out.IsGuaranteed)
			require.Equal(t, out.Prizes[out.PrizeIndex], out.Prize)
			require.NoError(t, mock.ExpectationsWereMet())
			_ = db.Close()
		}
	}
}

func TestWebLotteryUsageFailureRollsBackBeforeSpending(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := defaultLotteryConfig()
	c.Enabled = true
	c.TokenTiers = []CampaignTokenTier{{Tokens: 1000000, BonusBPS: 500}}
	expectWebStart(mock, false)
	mock.ExpectQuery("SELECT result_text").WillReturnError(sql.ErrNoRows)
	expectWebState(mock, c, 0, "30", false)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(input_tokens\)`).WillReturnError(errors.New("usage unavailable"))
	mock.ExpectRollback()
	_, err = s.UserLotterySpin(context.Background(), 42, webRequestID)
	require.ErrorContains(t, err, "usage unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQQLotteryTokenBonusBatchKeepsGuaranteeAndLastStock(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		s := New(db, nil, false, nil)
		s.db.mysql = mysql
		bot := Config{AppID: "bot", Enabled: true, AllowedGroups: []string{"group"}}
		job := Job{ID: "job", MemberID: "member", GroupID: "group"}
		cfg := defaultLotteryConfig()
		cfg.Enabled, cfg.GuaranteeEnabled, cfg.GuaranteeEvery = true, true, 2
		limit := int64(1)
		cfg.Prizes = []LotteryPrize{
			{ID: "last", Name: "普通奖", Amount: 2, ProbabilityBPS: 5000, StockLimit: &limit},
			{Name: "谢谢参与", ProbabilityBPS: 5000},
		}
		cfg.TokenTiers = []CampaignTokenTier{{Tokens: 1000000, BonusBPS: 5000}}
		rawBot, err := json.Marshal(bot)
		require.NoError(t, err)
		rawConfig, err := json.Marshal(cfg)
		require.NoError(t, err)
		mock.ExpectBegin()
		lock := "SELECT pg_advisory_xact_lock"
		if mysql {
			mock.ExpectQuery("SELECT id FROM qqc_transaction_lock").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			lock = "DO 0"
		}
		for range 3 {
			mock.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(rawBot)))
		mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(rawConfig)))
		mock.ExpectQuery("SELECT b.user_id").WithArgs(bot.AppID, job.MemberID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
		mock.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT result_text").WillReturnError(sql.ErrNoRows)
		expectLotteryRecharge(mock, 42, "30")
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws`).WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(0))
		mock.ExpectQuery(`SELECT COALESCE\(SUM\(chances\)`).WillReturnRows(sqlmock.NewRows([]string{"bonus"}).AddRow(0))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws`).WithArgs("last").WillReturnRows(sqlmock.NewRows([]string{"allocated"}).AddRow(0))
		mock.ExpectQuery(`SELECT COALESCE\(SUM\(input_tokens\)`).WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"tokens"}).AddRow(1000000))
		for i, outcome := range []struct {
			name       string
			amount     float64
			prizeID    string
			guaranteed bool
			bonus      *LotteryTokenBonus
		}{
			{"普通奖", 2, "last", false, &LotteryTokenBonus{UsageTokens: 1000000, BonusBPS: 5000}},
			{"保底奖励", 1, "", true, nil},
			{"谢谢参与", 0, "", false, &LotteryTokenBonus{UsageTokens: 1000000, BonusBPS: 0}},
		} {
			args := []driver.Value{sqlmock.AnyArg(), bot.AppID, job.MemberID, job.GroupID, int64(42), outcome.name, outcome.amount, lotteryBonusSnapshot{bonus: outcome.bonus, guaranteed: outcome.guaranteed}, float64(30), int64(3), int64(2 - i), sqlmock.AnyArg(), outcome.prizeID}
			if mysql {
				mock.ExpectExec("INSERT INTO qqc_lottery_draws").WithArgs(args...).WillReturnResult(sqlmock.NewResult(int64(i+7), 1))
			} else {
				mock.ExpectQuery("INSERT INTO qqc_lottery_draws").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(i + 7))
			}
			mock.ExpectExec("INSERT INTO qqc_rewards").WillReturnResult(sqlmock.NewResult(int64(i+10), 1))
			if outcome.amount > 0 {
				mock.ExpectExec("UPDATE users SET balance").WithArgs(outcome.amount, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(int64(i+20), 1))
			}
		}
		mock.ExpectExec("UPDATE qqc_jobs SET kind='lottery'").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		text, err := s.drawLotteryCount(context.Background(), bot, job, 3)
		require.NoError(t, err)
		require.Contains(t, text, "普通奖 × 1")
		require.Contains(t, text, "保底奖励 × 1")
		require.Contains(t, text, "谢谢参与 × 1")
		require.Contains(t, text, "剩余抽奖次数：0 次")
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}
