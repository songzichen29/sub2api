package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type quotaJSONMatcher struct{ wantDaily, wantWeekly float64 }

func (m quotaJSONMatcher) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	var extra map[string]any
	if json.Unmarshal([]byte(s), &extra) != nil {
		return false
	}
	return extra["quota_daily_used"] == m.wantDaily && extra["quota_weekly_used"] == m.wantWeekly && extra["custom_setting"] == "preserve"
}

func TestMySQLQuotaUsesLockedLimitsDespiteStaleHints(t *testing.T) {
	for _, hints := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		ctx := context.Background()
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		extra, err := json.Marshal(map[string]any{
			"quota_limit": 100, "quota_used": 3,
			"quota_daily_limit": 10, "quota_daily_used": 1, "quota_daily_start": time.Now().UTC().Format(time.RFC3339Nano),
			"quota_weekly_limit": 50, "quota_weekly_used": 20, "quota_weekly_start": time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano),
			"custom_setting": "preserve",
		})
		require.NoError(t, err)
		mock.ExpectQuery(`(?s)SELECT extra FROM accounts.*FOR UPDATE`).WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(string(extra)))
		mock.ExpectExec(`UPDATE accounts SET extra = \?`).WithArgs(quotaJSONMatcher{3.5, 2.5}, int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		state, err := incrementUsageBillingAccountQuota(ctx, tx, 9, 2.5, hints[0], hints[1])
		require.NoError(t, err)
		require.InDelta(t, 5.5, state.TotalUsed, 1e-6)
		require.InDelta(t, 3.5, state.DailyUsed, 1e-6)
		require.InDelta(t, 2.5, state.WeeklyUsed, 1e-6)
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}

func TestMySQLQuotaMissingAccountDoesNotWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectQuery(`(?s)SELECT extra FROM accounts.*FOR UPDATE`).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	state, err := incrementUsageBillingAccountQuota(context.Background(), tx, 9, 2.5, false, false)
	require.ErrorIs(t, err, service.ErrAccountNotFound)
	require.Nil(t, state)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
