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

func TestAccountMetricSnapshotBatchUsesOneAggregateQuery(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		repo := &accountMetricRepository{db: db, mysql: mysql}
		start := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
		end := start.Add(time.Hour)
		mock.ExpectQuery(regexp.QuoteMeta(repo.query(accountMetricSnapshotsSQL(2, mysql)))).
			WithArgs(int64(17), int64(18), start, end, int64(17), int64(18), start, end).
			WillReturnRows(sqlmock.NewRows([]string{"account", "success", "errors", "cache", "cache_samples", "ttft", "ttft_samples", "duration", "duration_samples"}).
				AddRow(17, 3, 1, 0.75, 2, 850.0, 2, 4100.0, 3))
		snapshots, err := repo.GetAccountMetricSnapshots(context.Background(), []int64{17, 18}, start, end)
		require.NoError(t, err)
		require.Len(t, snapshots, 2)
		require.InDelta(t, 0.75, *snapshots[17].SuccessRate, 0.0001)
		require.InDelta(t, 0.75, *snapshots[17].CacheRate, 0.0001)
		require.Equal(t, int64(2), snapshots[17].TTFTSamples)
		require.Nil(t, snapshots[18].SuccessRate)
		require.Nil(t, snapshots[18].CacheRate)
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestAccountMetricSnapshotRejectsInvalidBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountMetricRepository{db: db}
	now := time.Now()
	for _, ids := range [][]int64{{0}, {-1}, make([]int64, 101)} {
		_, err = repo.GetAccountMetricSnapshots(context.Background(), ids, now.Add(-time.Hour), now)
		require.Error(t, err)
	}
	_, err = repo.GetAccountMetricSnapshot(context.Background(), 1, now, now)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountMetricAdmissionRequiresAutoRecover(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountMetricRepository{db: db}
	now := time.Now()
	row := func() *sqlmock.Rows {
		return accountPerformanceMonitorTestRow(17, `{"enabled":true,"auto_recover":false,"success_rate":{"pause_threshold":0.8}}`, 1, true, 1, &now, nil, nil, nil)
	}
	query := "SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ?"
	mock.ExpectQuery(regexp.QuoteMeta(repo.query(query))).WithArgs(int64(17)).WillReturnRows(row())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(repo.query(query + " FOR UPDATE"))).WithArgs(int64(17)).WillReturnRows(row())
	mock.ExpectRollback()
	allowed, err := repo.AdmitAccountPerformanceRequest(context.Background(), 17, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, mock.ExpectationsWereMet())
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

	changed, err := repo.PauseAccountForMetricBreach(context.Background(), 17, service.AccountMetricEvaluationToken{Revision: 4, PauseGeneration: 2}, "success_rate 0.4 below 0.8", nil, now)
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

func TestAccountMetricEvaluationRejectsPreviousPauseAndTrial(t *testing.T) {
	now := time.Now().UTC()
	oldTrial := now.Add(-time.Minute)
	for _, action := range []string{"pause", "recover", "end"} {
		for _, staleGeneration := range []bool{true, false} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			repo := &accountMetricRepository{db: db}
			token := service.AccountMetricEvaluationToken{Revision: 4, PauseGeneration: 2, TrialStartedAt: &oldTrial}
			if staleGeneration {
				token.PauseGeneration = 1
				token.TrialStartedAt = &now
			}
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(repo.query("SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ? FOR UPDATE"))).WithArgs(int64(17)).
				WillReturnRows(accountPerformanceMonitorTestRow(17, `{"enabled":true}`, 4, action != "pause", 2, &now, &now, nil, nil))
			mock.ExpectRollback()
			var changed bool
			switch action {
			case "pause":
				changed, err = repo.PauseAccountForMetricBreach(context.Background(), 17, token, "stale", nil, now)
			case "recover":
				changed, err = repo.RecoverAccountFromMetricTrial(context.Background(), 17, token, nil, now)
			case "end":
				changed, err = repo.EndAccountMetricTrial(context.Background(), 17, token, "stale", nil, now)
			}
			require.NoError(t, err)
			require.False(t, changed)
			require.NoError(t, mock.ExpectationsWereMet())
			db.Close()
		}
	}
}

func metricTestTimePtr(value time.Time) *time.Time { return &value }
