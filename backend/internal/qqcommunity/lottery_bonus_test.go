package qqcommunity

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestLotteryBonusCannotPayForEarlierDraws(t *testing.T) {
	for _, tc := range []struct {
		name             string
		events           []int64
		remaining, spent int64
	}{
		{"historical debt then redemption", []int64{-1, 1}, 1, 0},
		{"new draw spends redeemed chance", []int64{-1, 1, -1}, 0, 1},
		{"later gift is not eaten by earlier deficit", []int64{1, -1, -1, 2}, 2, 1},
		{"interleaved gifts and draws", []int64{2, -1, 1, -1, -1, -1, 2}, 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b lotteryBonusBalance
			for _, delta := range tc.events {
				b.apply(delta)
			}
			require.Equal(t, tc.remaining, b.remaining)
			require.Equal(t, tc.spent, b.used)
		})
	}
	state, err := calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 1, 1, lotterySourceUsage{})
	require.NoError(t, err)
	require.EqualValues(t, 1, state.Remaining)
	require.EqualValues(t, 1, state.Used)
	state, err = calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 2, 1, lotterySourceUsage{bonusUsed: 1})
	require.NoError(t, err)
	require.Zero(t, state.Remaining)
}

func TestLotteryBonusReplayFailsClosedOnLedgerError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT user_id,chance_delta,config_snapshot,legacy_credit_only FROM").WillReturnRows(lotteryUsageRows().AddRow(42, 1, nil, false).AddRow(42, -1, `{}`, false).RowError(1, errors.New("ledger unavailable")))
	_, err = lotteryUsageBySource(context.Background(), New(db, nil, false, nil).db, 42)
	require.ErrorContains(t, err, "ledger unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func lotteryUsageRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"user_id", "chance_delta", "config_snapshot", "legacy_credit_only"})
}

func TestLotteryChanceSourcesStaySeparate(t *testing.T) {
	legacy := []byte(`{"recharge_step":10,"chances_per_step":1}`)
	activity := []byte(`{"chance_source":"activity","recharge_currency":"CNY"}`)
	recharge := []byte(`{"chance_source":"recharge","recharge_currency":"CNY"}`)
	var b lotteryBonusBalance
	require.NoError(t, b.draw(legacy, true))
	b.apply(1)
	require.NoError(t, b.draw(activity, false))
	state, err := calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 2, 1, lotterySourceUsage{b.used, b.legacy})
	require.NoError(t, err)
	require.EqualValues(t, 1, state.BonusUsed)
	require.EqualValues(t, 1, state.LegacyUsed)
	require.Zero(t, state.RechargeUsed)
	require.Zero(t, state.Remaining)
	require.Equal(t, float64(10), state.NextRecharge)
	// A real payment now creates a spendable chance, not a display-only promise.
	state, err = calculateLotteryState(defaultLotteryConfig(), 42, decimal.NewFromInt(10), 2, 1, lotterySourceUsage{b.used, b.legacy})
	require.NoError(t, err)
	require.EqualValues(t, 1, state.RechargeRemaining)
	require.EqualValues(t, 1, state.Remaining)
	// A gift arriving after that draw cannot reclassify its stored source.
	b.apply(2)
	require.NoError(t, b.draw(recharge, false))
	require.EqualValues(t, 2, b.remaining)
	require.EqualValues(t, 1, b.used)
}

func TestLotteryLegacyCompatibilityDoesNotForgivePaidDraws(t *testing.T) {
	for _, tc := range []struct {
		snapshot    string
		noPaidOrder bool
	}{
		{`{"recharge_step":10,"chances_per_step":1}`, false},
		{`{"recharge_step":10,"chances_per_step":1,"recharge_currency":"CNY"}`, true},
		{`{}`, true},
		{`{"chance_source":"recharge"}`, true},
	} {
		var b lotteryBonusBalance
		require.NoError(t, b.draw([]byte(tc.snapshot), tc.noPaidOrder))
		require.Zero(t, b.legacy)
		state, err := calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 1, 0, lotterySourceUsage{b.used, b.legacy})
		require.NoError(t, err)
		require.Equal(t, float64(20), state.NextRecharge, "a refunded paid draw must still be covered")
	}
}

func TestLotterySourceReplayRejectsCorruptSnapshots(t *testing.T) {
	for _, raw := range []string{`{`, `{"chance_source":"other"}`} {
		var b lotteryBonusBalance
		require.Error(t, b.draw([]byte(raw), false))
	}
}

func TestLotteryStoredSourceSurvivesTransactionTimestampOrdering(t *testing.T) {
	for _, granted := range []bool{false, true} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		rows := lotteryUsageRows().AddRow(42, -1, `{"chance_source":"activity"}`, false)
		if granted {
			rows.AddRow(42, 1, nil, false)
		}
		m.ExpectQuery("SELECT user_id,chance_delta,config_snapshot,legacy_credit_only FROM").WillReturnRows(rows)
		usage, err := lotteryUsageBySource(context.Background(), New(db, nil, false, nil).db, 42)
		if granted {
			require.NoError(t, err)
			require.EqualValues(t, 1, usage[42].bonusUsed)
		} else {
			require.ErrorContains(t, err, "inconsistent")
		}
		require.NoError(t, m.ExpectationsWereMet())
		db.Close()
	}
}
