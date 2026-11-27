package qqcommunity

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestLotteryLastPrizeBecomesThanksWithoutRedistribution(t *testing.T) {
	limit := int64(1)
	cfg := defaultLotteryConfig()
	cfg.Prizes = []LotteryPrize{
		{ID: "limited", Name: "Last prize", Amount: 5, ProbabilityBPS: 3000, StockLimit: &limit},
		{ID: "unlimited", Name: "Other prize", Amount: 1, ProbabilityBPS: 2000},
		{Name: "Thanks", ProbabilityBPS: 5000},
	}
	firstPool := availableLotteryPool(cfg.Prizes)
	winner := lotteryPrizeAt(firstPool, 2999)
	require.Equal(t, "Last prize", winner.Name)
	cfg.reservePrize(winner)
	secondPool := availableLotteryPool(cfg.Prizes)
	require.Equal(t, "谢谢参与", secondPool[0].Name)
	require.Zero(t, secondPool[0].Amount)
	require.Equal(t, 3000, secondPool[0].ProbabilityBPS)
	require.Equal(t, "Other prize", lotteryPrizeAt(secondPool, 3000).Name)
	require.Equal(t, winner, firstPool[0], "最后一份奖的开奖快照须保留原奖项")
	for range 10 {
		cfg.reservePrize(lotteryPrizeAt(secondPool, 0))
	}
	require.EqualValues(t, 1, cfg.Prizes[0].Allocated)
	require.EqualValues(t, 1, cfg.GuaranteePrizes[0].Amount)
}

func TestLotteryStockSaveKeepsIdentityAndCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	raw, err := db.Begin()
	require.NoError(t, err)
	defer raw.Rollback()
	tx := &transaction{raw: raw}
	ctx := context.Background()
	current := LotteryConfig{Prizes: []LotteryPrize{{ID: "stable"}}}
	limit := int64(3)
	next := LotteryConfig{Prizes: []LotteryPrize{{Name: "Added"}, {ID: "stable", Name: "Renamed", StockLimit: &limit, Allocated: 0}}}
	mock.ExpectQuery("SELECT COUNT").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT").WithArgs("stable").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	require.NoError(t, prepareLotteryStock(ctx, tx, current, &next))
	require.NotEmpty(t, next.Prizes[0].ID)
	require.Equal(t, "stable", next.Prizes[1].ID)
	limit = 2
	mock.ExpectQuery("SELECT COUNT").WithArgs(next.Prizes[0].ID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT").WithArgs("stable").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	require.ErrorContains(t, prepareLotteryStock(ctx, tx, next, &next), "不能小于已发")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLotteryVersionIgnoresStockConsumption(t *testing.T) {
	c := defaultLotteryConfig()
	version := ConfigVersion(c)
	c.Prizes[0].Allocated = 123
	require.Equal(t, version, ConfigVersion(c))
	c.Prizes[0].Amount = 2
	require.NotEqual(t, version, ConfigVersion(c))
	c.GuaranteePrizes[0].StockLimit = new(int64)
	require.ErrorContains(t, validateLottery(c), "保底奖池不限量")
}
