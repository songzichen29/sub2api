//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetAllGroupUsageSummaryUsesRollupTail(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")
	todayStart := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)
	tomorrowStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := time.Date(2026, 3, 7, 5, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT CAST\(closed_before AS CHAR\), retained_from, timezone_name.*usage_group_rollup_state`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("2026-03-08", time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC), "America/New_York"))
	mock.ExpectQuery(`(?s)WITH historical AS.*usage_group_daily_rollups.*ul\.created_at >= \?`).
		WithArgs("2026-03-07", true, "2026-03-01", "2026-03-08", todayStart, tomorrowStart, yesterdayStart, todayStart, todayStart).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}).
			AddRow(int64(7), 12.5, 1.25, 2.5))

	result, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
	require.NoError(t, err)
	require.Equal(t, int64(7), result[0].GroupID)
	require.InDelta(t, 12.5, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 1.25, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 2.5, result[0].YesterdayCost, 0.0000001)
	require.NoError(t, mock.ExpectationsWereMet())
}
