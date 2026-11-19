//go:build unit

package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const (
	groupRollupPrecheckQuery = `SELECT CAST\(closed_before AS CHAR\), timezone_name, building_until IS NULL`
	groupRollupLockQuery     = `SELECT GET_LOCK\('sub2api:usage_group_daily_rollups:build', 0\)`
	groupRollupUnlockQuery   = `SELECT RELEASE_LOCK\('sub2api:usage_group_daily_rollups:build'\)`
	groupRollupBeginQuery    = `(?s)SELECT CAST\(closed_before AS CHAR\), retained_from, timezone_name, expired_before, CAST\(building_until AS CHAR\).*FOR UPDATE`
	groupRollupBuildQuery    = `(?s)SELECT CAST\(building_until AS CHAR\), timezone_name\s+FROM usage_group_rollup_state\s+WHERE id = 1\s*$`
	groupRollupPublishQuery  = `(?s)SELECT CAST\(closed_before AS CHAR\), CAST\(building_until AS CHAR\), CAST\(build_dirty_from AS CHAR\).*FOR UPDATE`
	groupRollupRegisterExec  = `(?s)UPDATE usage_group_rollup_state.*building_until = DATE\(\?\)`
	groupRollupPublishExec   = `(?s)UPDATE usage_group_rollup_state.*SET closed_before = DATE\(\?\),\s+building_until = NULL`
	groupRollupExpireExec    = `(?s)UPDATE usage_group_rollup_state.*expired_before = GREATEST\(expired_before, \?\)`
	groupRollupInvalidateSQL = `(?s)UPDATE usage_group_rollup_state.*SET build_dirty_from = CASE.*closed_before = LEAST\(closed_before, DATE\(\?\)\).*WHERE id = 1 AND \? >= expired_before`
)

var groupRollupEpoch = time.Unix(0, 0).UTC()

func expectGroupRollupPrecheck(mock sqlmock.Sqlmock, closedBefore, timezoneName string, idle bool) {
	mock.ExpectQuery(groupRollupPrecheckQuery).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "timezone_name", "idle"}).
			AddRow(closedBefore, timezoneName, idle))
}

func expectGroupRollupLock(mock sqlmock.Sqlmock, locked bool) {
	mock.ExpectQuery(groupRollupLockQuery).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(locked))
}

func expectGroupRollupUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(groupRollupUnlockQuery).
		WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
}

func groupRollupBeginRows(closedBefore string, retainedFrom time.Time, timezoneName string, expiredBefore time.Time, buildingUntil any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name", "expired_before", "building_until"}).
		AddRow(closedBefore, retainedFrom, timezoneName, expiredBefore, buildingUntil)
}

// expectGroupRollupRound 期望一轮完整的登记、计算、发布。
func expectGroupRollupRound(
	t *testing.T,
	mock sqlmock.Sqlmock,
	begin *sqlmock.Rows,
	minCreatedAt *sqlmock.Rows,
	minCreatedAfter time.Time,
	register []any,
	deleteArgs []any,
	insertArgs []any,
	publishRows *sqlmock.Rows,
	publishBefore string,
) {
	t.Helper()
	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBeginQuery).WillReturnRows(begin)
	if minCreatedAt != nil {
		mock.ExpectQuery(`(?s)SELECT MIN\(created_at\)\s+FROM usage_logs\s+WHERE created_at >= \?`).
			WithArgs(minCreatedAfter).
			WillReturnRows(minCreatedAt)
	}
	mock.ExpectExec(groupRollupRegisterExec).
		WithArgs(toDriverValues(register)...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBuildQuery).
		WillReturnRows(sqlmock.NewRows([]string{"building_until", "timezone_name"}).AddRow(register[3], register[2]))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs(toDriverValues(deleteArgs)...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	bucketStart, err := service.ParseGroupUsageDate(register[0].(string))
	require.NoError(t, err)
	for bucketStart.Before(insertArgs[1].(time.Time)) {
		bucketEnd := bucketStart.AddDate(0, 0, 1)
		rowsFrom := bucketStart.UTC()
		if insertArgs[0].(time.Time).After(rowsFrom) {
			rowsFrom = insertArgs[0].(time.Time)
		}
		mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
			WithArgs(toDriverValues([]any{service.GroupUsageDate(bucketStart), rowsFrom, bucketEnd.UTC()})...).
			WillReturnResult(sqlmock.NewResult(0, 1))
		bucketStart = bucketEnd
	}
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupPublishQuery).WillReturnRows(publishRows)
	mock.ExpectExec(groupRollupPublishExec).
		WithArgs(publishBefore).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func toDriverValues(values []any) []driver.Value {
	args := make([]driver.Value, 0, len(values))
	for _, value := range values {
		args = append(args, sqlmockValue{value: value})
	}
	return args
}

// sqlmockValue 按时刻比较 time.Time（忽略 Location），其余值按相等比较。
type sqlmockValue struct{ value any }

func (v sqlmockValue) Match(actual driver.Value) bool {
	if expectedTime, ok := v.value.(time.Time); ok {
		actualTime, ok := actual.(time.Time)
		return ok && actualTime.Equal(expectedTime)
	}
	return actual == v.value
}

func publishRows(closedBefore, buildingUntil string, dirtyFrom any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"closed_before", "building_until", "build_dirty_from"}).
		AddRow(closedBefore, buildingUntil, dirtyFrom)
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsNoopsWithoutLockingAtCurrentDate(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-14", "Asia/Shanghai", true)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsSkipsWhenAnotherBuildHoldsLock(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-13", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, false)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsBuildsAndPublishesInSeparateTransactions(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)
	rebuildStart := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-13", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-13", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
		nil, time.Time{},
		[]any{"2026-08-13", retainedFrom, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-01", "2026-08-13"},
		[]any{rebuildStart, todayStart, "Asia/Shanghai"},
		publishRows("2026-08-13", "2026-08-14", nil),
		"2026-08-14",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsRebuildsDirtyRangeBeforeFullPublish(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-11", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	// 第一轮计算期间有 08-12 的迟到写入：只发布到 08-12。
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-11", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
		nil, time.Time{},
		[]any{"2026-08-11", retainedFrom, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-01", "2026-08-11"},
		[]any{time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
		publishRows("2026-08-11", "2026-08-14", "2026-08-12"),
		"2026-08-12",
	)
	// 第二轮只重算脏日期之后的区间。
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-12", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
		nil, time.Time{},
		[]any{"2026-08-12", retainedFrom, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-01", "2026-08-12"},
		[]any{time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
		publishRows("2026-08-12", "2026-08-14", nil),
		"2026-08-14",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsStopsAfterMaxRoundsWithoutOverPublishing(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-12", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	for range groupUsageRollupBuildMaxRounds {
		expectGroupRollupRound(t, mock,
			groupRollupBeginRows("2026-08-12", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
			nil, time.Time{},
			[]any{"2026-08-12", retainedFrom, "Asia/Shanghai", "2026-08-14"},
			[]any{"2026-05-01", "2026-08-12"},
			[]any{time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
			publishRows("2026-08-12", "2026-08-14", "2026-08-12"),
			"2026-08-12",
		)
	}
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsPublishKeepsUnmarkedInvalidation(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-13", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	// 计算期间水位被后退到 08-10 而没有记脏：发布不越过 08-10。
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-13", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
		nil, time.Time{},
		[]any{"2026-08-13", retainedFrom, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-01", "2026-08-13"},
		[]any{time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
		publishRows("2026-08-10", "2026-08-14", nil),
		"2026-08-10",
	)
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-10", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil),
		nil, time.Time{},
		[]any{"2026-08-10", retainedFrom, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-01", "2026-08-10"},
		[]any{time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
		publishRows("2026-08-10", "2026-08-14", nil),
		"2026-08-14",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsRebuildsWhenTimezoneChanges(t *testing.T) {
	useGroupUsageRepositoryTestTimezone(t, "America/New_York")
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-03-09", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-03-09", groupRollupEpoch, "Asia/Shanghai", groupRollupEpoch, nil),
		sqlmock.NewRows([]string{"min"}).AddRow(retainedFrom), groupRollupEpoch,
		[]any{"2026-03-01", retainedFrom, "America/New_York", "2026-03-09"},
		[]any{"2026-03-01", "2026-03-01"},
		[]any{retainedFrom, todayStart, "America/New_York"},
		publishRows("2026-03-01", "2026-03-09", nil),
		"2026-03-09",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsFullRebuildExcludesExpiredRows(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	// 保留下界不在 Asia/Shanghai 的日界上（例如在其他时区下推进）：重建只计入下界之后的记录。
	expiredBefore := time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC)
	earliestRetained := time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "1970-01-01", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("1970-01-01", groupRollupEpoch, "Asia/Shanghai", expiredBefore, nil),
		sqlmock.NewRows([]string{"min"}).AddRow(earliestRetained), expiredBefore,
		[]any{"2026-05-02", earliestRetained, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-02", "2026-05-02"},
		[]any{expiredBefore, todayStart, "Asia/Shanghai"},
		publishRows("2026-05-02", "2026-08-14", nil),
		"2026-08-14",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsKeepsRetainedFromAtExpiredFloor(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	staleRetainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)
	expiredBefore := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-13", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	// retained_from 早于保留下界（例如被其他版本的同步改写）：登记与清理都以保留下界为起点。
	expectGroupRollupRound(t, mock,
		groupRollupBeginRows("2026-08-13", staleRetainedFrom, "Asia/Shanghai", expiredBefore, nil),
		nil, time.Time{},
		[]any{"2026-08-13", expiredBefore, "Asia/Shanghai", "2026-08-14"},
		[]any{"2026-05-15", "2026-08-13"},
		[]any{time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC), todayStart, "Asia/Shanghai"},
		publishRows("2026-08-13", "2026-08-14", nil),
		"2026-08-14",
	)
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsClearsStaleBuildMarkWhenPublished(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-14", "Asia/Shanghai", false)
	expectGroupRollupLock(mock, true)
	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBeginQuery).
		WillReturnRows(groupRollupBeginRows("2026-08-14", groupRollupEpoch, "Asia/Shanghai", groupRollupEpoch, "2026-08-14"))
	mock.ExpectExec(`(?s)UPDATE usage_group_rollup_state\s+SET building_until = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectGroupRollupUnlock(mock)

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsRejectsFutureWatermark(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-15", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBeginQuery).
		WillReturnRows(groupRollupBeginRows("2026-08-15", groupRollupEpoch, "Asia/Shanghai", groupRollupEpoch, nil))
	mock.ExpectRollback()
	expectGroupRollupUnlock(mock)

	err := repo.SyncGroupUsageRollups(context.Background(), todayStart)
	require.ErrorContains(t, err, "未来")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsAbortsWhenBuildMarkReplaced(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
	retainedFrom := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)

	expectGroupRollupPrecheck(mock, "2026-08-13", "Asia/Shanghai", true)
	expectGroupRollupLock(mock, true)
	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBeginQuery).
		WillReturnRows(groupRollupBeginRows("2026-08-13", retainedFrom, "Asia/Shanghai", groupRollupEpoch, nil))
	mock.ExpectExec(groupRollupRegisterExec).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(groupRollupBuildQuery).
		WillReturnRows(sqlmock.NewRows([]string{"building_until", "timezone_name"}).AddRow(nil, "Asia/Shanghai"))
	mock.ExpectRollback()
	expectGroupRollupUnlock(mock)

	err := repo.SyncGroupUsageRollups(context.Background(), todayStart)
	require.ErrorIs(t, err, errGroupUsageRollupBuildPreempted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryRecomputeRangeInvalidatesGroupRollupsBeforeDashboardRebuild(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	start := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectExec(groupRollupInvalidateSQL).
		WithArgs(service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), start).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM usage_dashboard_hourly`).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err := repo.RecomputeRange(context.Background(), start, end)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryRecomputeRangeLeavesGroupRollupRebuildToSync(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	start := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectExec(groupRollupInvalidateSQL).
		WithArgs(service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), start).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	for _, query := range []string{
		`DELETE FROM usage_dashboard_hourly WHERE`,
		`DELETE FROM usage_dashboard_hourly_users WHERE`,
		`DELETE FROM usage_dashboard_daily WHERE`,
		`DELETE FROM usage_dashboard_daily_users WHERE`,
		`INSERT INTO usage_dashboard_hourly_users`,
		`INSERT INTO usage_dashboard_daily_users`,
		`INSERT INTO usage_dashboard_hourly`,
		`INSERT INTO usage_dashboard_daily`,
	} {
		mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	require.NoError(t, repo.RecomputeRange(context.Background(), start, end))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryRecomputeRangeStopsWhenInvalidationFails(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	start := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)

	mock.ExpectExec(groupRollupInvalidateSQL).
		WithArgs(service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), service.GroupUsageDate(start), start).
		WillReturnError(sql.ErrConnDone)

	err := repo.RecomputeRange(context.Background(), start, start.Add(time.Hour))
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsExpiresWholeDaysBeforeDeleting(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	// 2026-07-01 13:00 +08 对齐到 2026-07-01 00:00 +08。
	cutoff := time.Date(2026, 7, 1, 5, 0, 0, 0, time.UTC)
	alignedCutoff := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	fixedNow := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	repo.clock = func() time.Time { return fixedNow }

	mock.ExpectExec(groupRollupExpireExec).
		WithArgs(alignedCutoff, alignedCutoff).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM usage_logs.*ORDER BY created_at ASC, id ASC.*LIMIT \?`).
		WithArgs(alignedCutoff, usageLogsCleanupBatchSize).
		WillReturnResult(sqlmock.NewResult(0, usageLogsCleanupBatchSize))
	mock.ExpectExec(`(?s)DELETE FROM usage_logs.*ORDER BY created_at ASC, id ASC.*LIMIT \?`).
		WithArgs(alignedCutoff, usageLogsCleanupBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 3))
	expectGroupRollupPrecheck(mock, service.GroupUsageDate(service.GroupUsageTodayStart(fixedNow)), "Asia/Shanghai", true)

	require.NoError(t, repo.CleanupUsageLogs(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsStopsWhenExpireFails(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)

	mock.ExpectExec(groupRollupExpireExec).
		WithArgs(cutoff, cutoff).
		WillReturnError(sql.ErrConnDone)

	err := repo.CleanupUsageLogs(context.Background(), cutoff)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsBatchFailureStopsWithoutSync(t *testing.T) {
	setGroupUsageRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)

	mock.ExpectExec(groupRollupExpireExec).
		WithArgs(cutoff, cutoff).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM usage_logs.*ORDER BY created_at ASC, id ASC.*LIMIT \?`).
		WithArgs(cutoff, usageLogsCleanupBatchSize).
		WillReturnError(sql.ErrConnDone)

	err := repo.CleanupUsageLogs(context.Background(), cutoff)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func setGroupUsageRollupTestTimezone(t *testing.T) {
	t.Helper()
	useGroupUsageRepositoryTestTimezone(t, "Asia/Shanghai")
}

func TestDashboardAggregationTxOptionsUseReadCommitted(t *testing.T) {
	opts := dashboardAggregationTxOptions()
	require.NotNil(t, opts)
	require.Equal(t, sql.LevelReadCommitted, opts.Isolation)
	require.False(t, opts.ReadOnly)
}

func TestDashboardAggregationRepositoryMySQLUsageLogsAreNotPartitioned(t *testing.T) {
	repo := newDashboardAggregationRepositoryWithSQL(nil)
	partitioned, err := repo.isUsageLogsPartitioned(context.Background())
	require.NoError(t, err)
	require.False(t, partitioned)
}
