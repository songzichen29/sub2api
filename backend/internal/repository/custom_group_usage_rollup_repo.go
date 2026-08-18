package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) getAllGroupUsageSummaryFromRollups(ctx context.Context, todayStart time.Time) (results []usagestats.GroupUsageSummary, err error) {
	todayStart = service.GroupUsageTodayStart(todayStart)
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	todayDate := service.GroupUsageDate(todayStart)
	yesterdayDate := service.GroupUsageDate(yesterdayStart)
	todayDateStart, parseErr := service.ParseGroupUsageDate(todayDate)
	if parseErr != nil {
		return nil, fmt.Errorf("解析分组用量汇总日期 %q: %w", todayDate, parseErr)
	}
	tomorrowStart := todayDateStart.AddDate(0, 0, 1).UTC()

	var closedBefore string
	var retainedFrom time.Time
	var stateTimezoneName string
	stateErr := scanSingleRow(ctx, r.sql, `
		SELECT CAST(closed_before AS CHAR), retained_from, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &closedBefore, &retainedFrom, &stateTimezoneName)
	stateValid := stateErr == nil && stateTimezoneName == timezoneName && closedBefore <= todayDate
	tailStart := time.Unix(0, 0).UTC()
	retainedDate := service.GroupUsageDate(tailStart)
	if stateValid {
		parsed, parseErr := service.ParseGroupUsageDate(closedBefore)
		if parseErr != nil {
			return nil, fmt.Errorf("解析分组用量汇总水位 %q: %w", closedBefore, parseErr)
		}
		tailStart = parsed.UTC()
		retainedDate = service.GroupUsageDate(retainedFrom)
	} else {
		closedBefore = "1970-01-01"
	}

	query := `
		WITH historical AS (
			SELECT
				rollup.group_id,
				COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(CASE WHEN rollup.bucket_date = DATE(?) THEN rollup.actual_cost ELSE 0 END), 0) AS yesterday_cost
			FROM usage_group_daily_rollups rollup
			WHERE ?
				AND rollup.bucket_date >= DATE(?)
				AND rollup.bucket_date < DATE(?)
			GROUP BY rollup.group_id
		),
		tail AS (
			SELECT
				ul.group_id,
				COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(CASE WHEN ul.created_at >= ? AND ul.created_at < ? THEN ul.actual_cost ELSE 0 END), 0) AS today_cost,
				COALESCE(SUM(CASE WHEN ul.created_at >= ? AND ul.created_at < ? THEN ul.actual_cost ELSE 0 END), 0) AS yesterday_cost
			FROM usage_logs ul
			WHERE ul.created_at >= ?
			GROUP BY ul.group_id
		)
		SELECT
			g.id AS group_id,
			COALESCE(historical.actual_cost, 0) + COALESCE(tail.actual_cost, 0) AS total_cost,
			COALESCE(tail.today_cost, 0) AS today_cost,
			COALESCE(historical.yesterday_cost, 0) + COALESCE(tail.yesterday_cost, 0) AS yesterday_cost
		FROM ` + quotedGroupsTable + ` g
		LEFT JOIN historical ON historical.group_id = g.id
		LEFT JOIN tail ON tail.group_id = g.id
		ORDER BY g.id
	`

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		yesterdayDate,
		stateValid,
		retainedDate,
		closedBefore,
		todayStart,
		tomorrowStart,
		yesterdayStart,
		todayStart,
		tailStart,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.GroupUsageSummary, 0)
	for rows.Next() {
		var row usagestats.GroupUsageSummary
		if err := rows.Scan(&row.GroupID, &row.TotalCost, &row.TodayCost, &row.YesterdayCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// SyncGroupUsageRollups 将服务端配置时区今日以前的用量发布为分组日桶。
func (r *dashboardAggregationRepository) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	todayStart = service.GroupUsageTodayStart(todayStart)
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.syncGroupUsageRollupsInTx(ctx, todayStart); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.syncGroupUsageRollupsInTx(ctx, todayStart)
}

func (r *dashboardAggregationRepository) syncGroupUsageRollupsInTx(ctx context.Context, todayStart time.Time) error {
	var closedBefore string
	var previousRetainedFrom time.Time
	var stateTimezoneName string
	if err := scanSingleRow(ctx, r.sql, `
		SELECT CAST(closed_before AS CHAR), retained_from, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`, nil, &closedBefore, &previousRetainedFrom, &stateTimezoneName); err != nil {
		return fmt.Errorf("读取分组用量汇总水位: %w", err)
	}

	todayDate := service.GroupUsageDate(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	timezoneChanged := stateTimezoneName != timezoneName
	var closedTime time.Time
	if !timezoneChanged {
		var err error
		closedTime, err = service.ParseGroupUsageDate(closedBefore)
		if err != nil {
			return fmt.Errorf("解析分组用量汇总水位 %q: %w", closedBefore, err)
		}
		todayDateTime, err := service.ParseGroupUsageDate(todayDate)
		if err != nil {
			return err
		}
		if closedTime.After(todayDateTime) {
			return fmt.Errorf("分组用量汇总水位位于未来: %s", closedBefore)
		}
		if closedBefore == todayDate {
			return nil
		}
	}

	var earliest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, "SELECT MIN(created_at) FROM usage_logs", nil, &earliest); err != nil {
		return fmt.Errorf("读取最早用量记录: %w", err)
	}
	retainedFrom := todayStart
	if earliest.Valid {
		retainedFrom = earliest.Time.UTC()
	}
	retainedDate := service.GroupUsageDate(retainedFrom)
	retainedDateTime, err := service.ParseGroupUsageDate(retainedDate)
	if err != nil {
		return err
	}
	rebuildStartDate := retainedDate
	if !timezoneChanged && closedTime.After(retainedDateTime) {
		rebuildStartDate = closedBefore
	}
	rebuildStart, err := service.ParseGroupUsageDate(rebuildStartDate)
	if err != nil {
		return err
	}

	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_group_daily_rollups
		WHERE bucket_date < DATE(?)
			OR (bucket_date >= DATE(?) AND bucket_date < DATE(?))
			OR bucket_date >= DATE(?)
	`, retainedDate, rebuildStartDate, todayDate, todayDate); err != nil {
		return fmt.Errorf("清理分组用量日桶: %w", err)
	}

	todayDateTime, err := service.ParseGroupUsageDate(todayDate)
	if err != nil {
		return err
	}
	for bucketStart := rebuildStart; bucketStart.Before(todayDateTime); bucketStart = bucketStart.AddDate(0, 0, 1) {
		bucketEnd := bucketStart.AddDate(0, 0, 1)
		bucketDate := service.GroupUsageDate(bucketStart)
		if _, err := r.sql.ExecContext(ctx, `
			INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
			SELECT DATE(?), group_id, COALESCE(SUM(actual_cost), 0), NOW(6)
			FROM usage_logs
			WHERE group_id IS NOT NULL AND created_at >= ? AND created_at < ?
			GROUP BY group_id
			ON DUPLICATE KEY UPDATE
				actual_cost = VALUES(actual_cost),
				computed_at = VALUES(computed_at)
		`, bucketDate, bucketStart.UTC(), bucketEnd.UTC()); err != nil {
			return fmt.Errorf("重建分组用量日桶 %s: %w", bucketDate, err)
		}
	}

	if _, err := r.sql.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = DATE(?),
			retained_from = ?,
			timezone_name = ?,
			updated_at = NOW(6)
		WHERE id = 1
	`, todayDate, retainedFrom, timezoneName); err != nil {
		return fmt.Errorf("更新分组用量汇总水位: %w", err)
	}
	return nil
}

func lockGroupUsageRollupState(ctx context.Context, tx *sql.Tx) error {
	var id int16
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`).Scan(&id); err != nil {
		return fmt.Errorf("锁定分组用量汇总水位: %w", err)
	}
	return nil
}

func invalidateGroupUsageRollupsAt(ctx context.Context, tx *sql.Tx, affectedAt time.Time) error {
	affectedDate := service.GroupUsageDate(affectedAt)
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = LEAST(closed_before, DATE(?)),
			updated_at = NOW(6)
		WHERE id = 1
	`, affectedDate)
	return err
}
