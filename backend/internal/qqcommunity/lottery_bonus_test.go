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
	state, err := calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 1, 1, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, state.Remaining)
	require.EqualValues(t, 1, state.Used)
	state, err = calculateLotteryState(defaultLotteryConfig(), 42, decimal.Zero, 2, 1, 1)
	require.NoError(t, err)
	require.Zero(t, state.Remaining)
}

func TestLotteryBonusReplayFailsClosedOnLedgerError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT user_id,chance_delta FROM").WillReturnRows(sqlmock.NewRows([]string{"user_id", "chance_delta"}).AddRow(42, 1).AddRow(42, -1).RowError(1, errors.New("ledger unavailable")))
	_, err = lotteryBonusUsage(context.Background(), New(db, nil, false, nil).db, 42)
	require.ErrorContains(t, err, "ledger unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}
