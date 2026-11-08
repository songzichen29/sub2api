package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountMetricSnapshotAggregatesValidObservationsAndDeduplicatesErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	start := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	repo := &accountMetricRepository{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders(accountMetricUsageAggregateSQL))).
		WithArgs(int64(17), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"success", "cached", "prompt", "cache_samples", "ttft_avg", "ttft_samples", "duration_avg", "duration_samples"}).
			AddRow(int64(3), float64(75), float64(100), int64(2), float64(850), int64(2), float64(4100), int64(3)))
	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders(accountMetricTerminalErrorsSQL))).
		WithArgs(int64(17), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"errors"}).AddRow(int64(1)))

	snapshot, err := repo.GetAccountMetricSnapshot(context.Background(), 17, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(3), snapshot.SuccessSamples)
	require.Equal(t, int64(1), snapshot.ErrorSamples)
	require.InDelta(t, 0.75, *snapshot.SuccessRate, 0.0001)
	require.Equal(t, int64(2), snapshot.CacheSamples)
	require.InDelta(t, 0.75, *snapshot.CacheRate, 0.0001)
	require.InDelta(t, 850.0, *snapshot.AverageTTFTMs, 0.001)
	require.Equal(t, int64(2), snapshot.TTFTSamples)
	require.InDelta(t, 4100.0, *snapshot.AverageDurationMs, 0.001)
	require.Equal(t, int64(3), snapshot.DurationSamples)
	require.NoError(t, mock.ExpectationsWereMet())

	query := accountMetricTerminalErrorsSQL
	for _, fragment := range []string{
		"e.error_owner = 'provider'",
		"COALESCE(e.is_business_limited, FALSE) = FALSE",
		"COALESCE(e.is_count_tokens, FALSE) = FALSE",
		"GROUP BY COALESCE(e.request_id, e.client_request_id), COALESCE(e.api_key_id, 0)",
		"NOT EXISTS",
		"COALESCE(ul.request_type, 0) NOT IN (4, 6)",
		usageLogSuccessFilterUL,
	} {
		require.Contains(t, query, fragment)
	}
}

func TestAccountMetricSnapshotLeavesMissingMetricsNil(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	start := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	repo := &accountMetricRepository{db: db}
	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders(accountMetricUsageAggregateSQL))).
		WithArgs(int64(17), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"success", "cached", "prompt", "cache_samples", "ttft_avg", "ttft_samples", "duration_avg", "duration_samples"}).
			AddRow(int64(0), float64(0), float64(0), int64(0), nil, int64(0), nil, int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders(accountMetricTerminalErrorsSQL))).
		WithArgs(int64(17), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"errors"}).AddRow(int64(0)))

	snapshot, err := repo.GetAccountMetricSnapshot(context.Background(), 17, start, end)
	require.NoError(t, err)
	require.Nil(t, snapshot.SuccessRate)
	require.Nil(t, snapshot.CacheRate)
	require.Nil(t, snapshot.AverageTTFTMs)
	require.Nil(t, snapshot.AverageDurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountMetricQueriesUsePortablePlaceholderRebinding(t *testing.T) {
	require.Contains(t, opsRebindQuestionPlaceholders(accountMetricTerminalErrorsSQL), "e.account_id = $1")
	require.Contains(t, opsRebindQuestionPlaceholders(accountMetricTerminalErrorsSQL), "ul.account_id = e.account_id")
	require.Contains(t, (&accountMetricRepository{mysql: true}).query(accountMetricTerminalErrorsSQL), "e.account_id = ?")
	require.NotContains(t, (&accountMetricRepository{mysql: true}).query(accountMetricTerminalErrorsSQL), "$1")
}

func TestSaveAccountMetricRecoveryConfigRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountMetricRepository{db: db}
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ? FOR UPDATE"))).
		WithArgs(int64(17)).
		WillReturnRows(accountPerformanceMonitorTestRow(17, `{"enabled":false}`, 2, false, 0, nil, nil, nil, nil))
	mock.ExpectRollback()

	_, err = repo.SaveAccountMetricRecoveryConfig(context.Background(), 17, 1, service.AccountMetricRecoveryConfig{}, nil, now)
	require.ErrorIs(t, err, service.ErrAccountMetricRevisionConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdmitAccountPerformanceRequestAllowsUnconfiguredAccountAndDeniesCooldown(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountMetricRepository{db: db}
		mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ?"))).
			WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "config", "revision", "paused", "pause_generation", "paused_at", "reason", "trial_started_at", "trial_requests", "next_trial_at", "metrics_since", "created_at", "updated_at",
		}))

		allowed, err := repo.AdmitAccountPerformanceRequest(context.Background(), 17, time.Now().UTC())
		require.NoError(t, err)
		require.True(t, allowed)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("cooldown", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountMetricRepository{db: db}
		now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
		mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ?"))).
			WithArgs(int64(17)).WillReturnRows(accountPerformanceMonitorTestRow(
			17,
			`{"enabled":true,"cooldown_seconds":600,"recovery_samples":3,"success_rate":{"pause_threshold":0.8,"recovery_threshold":0.9,"min_samples":10}}`,
			1,
			true,
			1,
			&now,
			nil,
			metricTestTimePtr(now.Add(time.Minute)),
			nil,
		))
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ? FOR UPDATE"))).
			WithArgs(int64(17)).WillReturnRows(accountPerformanceMonitorTestRow(
			17,
			`{"enabled":true,"cooldown_seconds":600,"recovery_samples":3,"success_rate":{"pause_threshold":0.8,"recovery_threshold":0.9,"min_samples":10}}`,
			1,
			true,
			1,
			&now,
			nil,
			metricTestTimePtr(now.Add(time.Minute)),
			nil,
		))
		mock.ExpectRollback()

		allowed, err := repo.AdmitAccountPerformanceRequest(context.Background(), 17, now)
		require.NoError(t, err)
		require.False(t, allowed)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPauseAccountForMetricBreachPersistsStateAndAuditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountMetricRepository{db: db}
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	config := `{"enabled":true,"cooldown_seconds":600,"recovery_samples":3,"success_rate":{"pause_threshold":0.8,"recovery_threshold":0.9,"min_samples":10}}`

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(opsRebindQuestionPlaceholders("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ? FOR UPDATE"))).
		WithArgs(int64(17)).WillReturnRows(accountPerformanceMonitorTestRow(17, config, 4, false, 2, nil, nil, nil, nil))
	mock.ExpectExec(regexp.QuoteMeta(opsRebindQuestionPlaceholders(`
UPDATE account_performance_monitors SET paused = TRUE, pause_generation = ?, paused_at = ?,
  reason = ?, trial_started_at = NULL, trial_requests = 0, next_trial_at = ?, metrics_since = NULL,
  updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = FALSE`))).
		WithArgs(int64(3), now, "success_rate 0.4 below 0.8", now.Add(10*time.Minute), now, int64(17), int64(4), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(opsRebindQuestionPlaceholders(`
INSERT INTO account_performance_events (
  account_id, action, actor_id, revision, pause_generation, evidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`))).
		WithArgs(int64(17), "pause", nil, int64(4), int64(3), sqlmock.AnyArg(), now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	changed, err := repo.PauseAccountForMetricBreach(context.Background(), 17, 4, "success_rate 0.4 below 0.8", nil, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func accountPerformanceMonitorTestRow(accountID int64, config string, revision int64, paused bool, generation int64, pausedAt, trialStartedAt, nextTrialAt, metricsSince *time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"account_id", "config", "revision", "paused", "pause_generation", "paused_at", "reason",
		"trial_started_at", "trial_requests", "next_trial_at", "metrics_since", "created_at", "updated_at",
	}).AddRow(accountID, []byte(config), revision, paused, generation, pausedAt, "", trialStartedAt, int64(0), nextTrialAt, metricsSince, time.Now(), time.Now())
}

func metricTestTimePtr(value time.Time) *time.Time { return &value }
