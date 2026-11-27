package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const campaignClaimColumns = "id,campaign,user,email,member,group,codeID,code,type,value,days,status,delivery,attempts,created,expires,used,sent,campaignJSON,outcome,rewardJSON,tokens,probability,bonus"

func probabilityCampaignFixture() CodeCampaign {
	return CodeCampaign{
		Name: "Probability", RewardMode: "probability", ClaimMode: "daily", CodeValidHours: 24,
		Prizes: []CampaignPrize{
			{ID: "a", Name: "A", Type: "balance", Value: 1, ProbabilityBPS: 1000, StockLimit: 1},
			{ID: "b", Name: "B", Type: "lottery_chances", Value: 2, ProbabilityBPS: 2000, StockLimit: 1},
		},
		TokenTiers:   []CampaignTokenTier{{Tokens: 100000, BonusBPS: 500}, {Tokens: 500000, BonusBPS: 1000}, {Tokens: 2000000, BonusBPS: 1500}},
		Compensation: &CampaignPrize{ID: "c", Name: "Compensation", Type: "balance", Value: .01, StockLimit: 100},
	}
}

func TestCampaignTokenTiersAndExhaustedPrizeDistribution(t *testing.T) {
	c := probabilityCampaignFixture()
	require.NoError(t, validateCodeCampaign(c))
	for _, scenario := range []struct {
		tokens int64
		total  int
		bonus  int
	}{{99999, 3000, 0}, {100000, 3500, 500}, {499999, 3500, 500}, {500000, 4000, 1000}, {2000000, 4500, 1500}} {
		weights, total, bonus := campaignDistribution(c, scenario.tokens)
		require.Equal(t, scenario.total, total)
		require.Equal(t, scenario.bonus, bonus)
		counts := map[string]int{}
		for roll := range 10000 {
			prize, outcome := campaignPrizeAt(c, weights, roll)
			require.NotNil(t, prize)
			counts[outcome]++
		}
		require.Equal(t, total, counts["prize"])
		require.Equal(t, 10000-total, counts["compensation"])
	}
	c.Prizes[0].Allocated = 1
	weights, total, _ := campaignDistribution(c, 2000000)
	require.Equal(t, []int{1500, 3000}, weights)
	require.Equal(t, 3000, total)
	prize, outcome := campaignPrizeAt(c, weights, 1499)
	require.Equal(t, "compensation", outcome)
	require.Equal(t, "c", prize.ID)
	prize, outcome = campaignPrizeAt(c, weights, 1500)
	require.Equal(t, "prize", outcome)
	require.Equal(t, "b", prize.ID)
	c.Compensation.Allocated = 100
	prize, outcome = campaignPrizeAt(c, weights, 1499)
	require.Nil(t, prize)
	require.Equal(t, "none", outcome)
}

func TestCampaignProbabilityValidation(t *testing.T) {
	for _, mutate := range []func(*CodeCampaign){
		func(c *CodeCampaign) { c.Prizes = nil },
		func(c *CodeCampaign) { c.Prizes[0].ProbabilityBPS = 0 },
		func(c *CodeCampaign) { c.Prizes[0].ProbabilityBPS = 9000 },
		func(c *CodeCampaign) { c.TokenTiers[0].BonusBPS = 8000 },
		func(c *CodeCampaign) { c.TokenTiers[1].Tokens = 100000 },
		func(c *CodeCampaign) { c.TokenTiers[1].BonusBPS = 100 },
		func(c *CodeCampaign) { c.CompensationRequired, c.Compensation = true, nil },
		func(c *CodeCampaign) { c.Compensation.Type = "unknown" },
		func(c *CodeCampaign) { c.Prizes[0].StockLimit = -1 },
	} {
		c := probabilityCampaignFixture()
		mutate(&c)
		require.Error(t, validateCodeCampaign(c))
	}
}

func TestCampaignSavePreservesStockAndRewardContract(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*CodeCampaign)
		error  string
		read   bool
	}{
		{"rename and reorder", func(c *CodeCampaign) {
			c.Prizes[0], c.Prizes[1] = c.Prizes[1], c.Prizes[0]
			c.Prizes[0].Name = "Renamed"
		}, "", true},
		{"lower stock", func(c *CodeCampaign) { c.Prizes[0].StockLimit = 0 }, "不能小于已分配", true},
		{"reset identity", func(c *CodeCampaign) { c.Prizes[0].ID = "" }, "不可删除", false},
		{"change reward", func(c *CodeCampaign) { c.Prizes[0].Value = 9 }, "兑换内容不可变更", false},
		{"change compensation guarantee", func(c *CodeCampaign) { c.CompensationRequired = true }, "补偿规则不可变更", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			raw, err := db.Begin()
			require.NoError(t, err)
			defer func() { _ = raw.Rollback() }()
			old := probabilityCampaignFixture()
			old.ID = "aaaaaaaaaaaaaaaaaaaaaaaa"
			next := old
			next.Prizes = append([]CampaignPrize(nil), old.Prizes...)
			compensation := *old.Compensation
			next.Compensation = &compensation
			scenario.mutate(&next)
			if scenario.read {
				for _, p := range next.campaignPrizes() {
					mock.ExpectQuery("SELECT COUNT").WithArgs(old.ID, p.ID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				}
			}
			err = prepareCampaignPrizes(context.Background(), &transaction{raw: raw}, old, &next, 1)
			if scenario.error == "" {
				require.NoError(t, err)
				require.Equal(t, "b", next.Prizes[0].ID)
				require.Equal(t, "a", next.Prizes[1].ID)
			}
			if scenario.error != "" {
				require.ErrorContains(t, err, scenario.error)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCampaignUsageBeijingMidnightBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		raw, err := db.Begin()
		require.NoError(t, err)
		at := time.Date(2026, 11, 27, 16, 0, 1, 0, time.UTC)
		mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(42), time.Date(2026, 11, 28, 0, 0, 0, 0, checkinZone), at).
			WillReturnRows(sqlmock.NewRows([]string{"tokens"}).AddRow(123456))
		tokens, err := campaignUsageTokens(context.Background(), &transaction{raw: raw, mysql: mysql}, 42, at)
		require.NoError(t, err)
		require.EqualValues(t, 123456, tokens)
		require.NoError(t, mock.ExpectationsWereMet())
		_ = raw.Rollback()
		_ = db.Close()
	}
}

func TestCampaignNoneResultReplaysBeforeStockAndUsageChecks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	s := New(db, fixtureEncryptor{}, true, nil)
	cfg := Config{AppID: "bot", AllowedGroups: []string{"group"}}
	rawConfig, err := json.Marshal(cfg)
	require.NoError(t, err)
	c := probabilityCampaignFixture()
	c.Enabled = false
	rawCampaign, err := json.Marshal(c)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(rawConfig)))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT b.user_id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery("SELECT c.id").WillReturnRows(sqlmock.NewRows([]string{"id", "config"}).AddRow("campaign", string(rawCampaign)))
	mock.ExpectQuery("SELECT id FROM qqc_code_claims").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("claim"))
	mock.ExpectQuery("SELECT q.id").WillReturnRows(sqlmock.NewRows(strings.Split(campaignClaimColumns, ",")).AddRow("claim", "campaign", 42, "", "member", "group", 0, "", "", 0, 0, "deleted", "not_required", 0, time.Now(), nil, nil, nil, string(rawCampaign), "none", "", 100001, 3500, 500))
	mock.ExpectCommit()
	result, err := s.allocateCampaignCode(context.Background(), cfg, Job{MemberID: "member", GroupID: "group"}, time.Now())
	require.NoError(t, err)
	require.Equal(t, "none", result.Outcome)
	require.EqualValues(t, 100001, result.UsageTokens)
	require.Equal(t, 3500, result.ProbabilityBPS)
	require.Equal(t, "none", result.Status)
	require.Empty(t, result.Type)
	require.Zero(t, result.Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCampaignProbabilityAllocationBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		for _, scenario := range []string{"winner", "main_exhausted", "compensation_exhausted", "claim_failure"} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			s := New(db, fixtureEncryptor{}, true, nil)
			s.db.mysql = mysql
			cfg := Config{AppID: "bot", Enabled: true, AllowedGroups: []string{"group"}}
			rawConfig, err := json.Marshal(cfg)
			require.NoError(t, err)
			c := probabilityCampaignFixture()
			c.Enabled = true
			c.Prizes = c.Prizes[:1]
			c.Prizes[0].ProbabilityBPS = 10000
			c.TokenTiers = nil
			c.CompensationRequired = true
			rawCampaign, err := json.Marshal(c)
			require.NoError(t, err)
			now := time.Date(2026, 11, 27, 12, 0, 0, 0, checkinZone)
			mock.ExpectBegin()
			lock := "SELECT pg_advisory_xact_lock"
			if mysql {
				mock.ExpectQuery("SELECT id FROM qqc_transaction_lock").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				lock = "DO 0"
			}
			mock.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(rawConfig)))
			mock.ExpectExec(lock).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT b.user_id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
			mock.ExpectQuery("SELECT c.id").WillReturnRows(sqlmock.NewRows([]string{"id", "config"}).AddRow("campaign", string(rawCampaign)))
			mock.ExpectQuery("SELECT id FROM qqc_code_claims").WillReturnError(sql.ErrNoRows)
			mainStock, compensationStock := 0, 0
			if scenario == "main_exhausted" {
				mainStock = 1
			}
			if scenario == "compensation_exhausted" {
				compensationStock = 100
			}
			mock.ExpectQuery("SELECT COUNT").WithArgs("campaign", "a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(mainStock))
			mock.ExpectQuery("SELECT COUNT").WithArgs("campaign", "c").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(compensationStock))
			if scenario == "main_exhausted" || scenario == "compensation_exhausted" {
				mock.ExpectRollback()
				_, err = s.allocateCampaignCode(context.Background(), cfg, Job{MemberID: "member", GroupID: "group"}, now)
				require.ErrorContains(t, err, "已领完")
				require.NoError(t, mock.ExpectationsWereMet())
				_ = db.Close()
				continue
			}
			mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"tokens"}).AddRow(123456))
			if mysql {
				mock.ExpectExec("INSERT INTO redeem_codes").WillReturnResult(sqlmock.NewResult(7, 1))
			} else {
				mock.ExpectQuery("INSERT INTO redeem_codes").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			}
			insert := mock.ExpectExec("INSERT INTO qqc_code_claims").WithArgs(sqlmock.AnyArg(), "campaign", "bot", "group", "member", int64(42), "2026-11-27", int64(7), "a", "prize", sqlmock.AnyArg(), int64(123456), 10000, 0, "pending")
			if scenario == "claim_failure" {
				insert.WillReturnError(sql.ErrConnDone)
				mock.ExpectRollback()
				_, err = s.allocateCampaignCode(context.Background(), cfg, Job{MemberID: "member", GroupID: "group"}, now)
				require.ErrorIs(t, err, sql.ErrConnDone)
				require.NoError(t, mock.ExpectationsWereMet())
				_ = db.Close()
				continue
			}
			insert.WillReturnResult(sqlmock.NewResult(0, 1))
			snapshot, err := json.Marshal(c.Prizes[0])
			require.NoError(t, err)
			mock.ExpectQuery("SELECT q.id").WillReturnRows(sqlmock.NewRows(strings.Split(campaignClaimColumns, ",")).AddRow("claim", "campaign", 42, "", "member", "group", 7, "original-code", "balance", 1, 0, "unused", "pending", 0, now, now.Add(24*time.Hour), nil, nil, string(rawCampaign), "prize", string(snapshot), 123456, 10000, 0))
			mock.ExpectCommit()
			result, err := s.allocateCampaignCode(context.Background(), cfg, Job{MemberID: "member", GroupID: "group"}, now)
			require.NoError(t, err)
			require.Equal(t, "A", result.PrizeName)
			require.Equal(t, "original-code", result.Code)
			require.EqualValues(t, 123456, result.UsageTokens)
			require.NoError(t, mock.ExpectationsWereMet())
			_ = db.Close()
		}
	}
}
