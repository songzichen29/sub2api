package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelQualityQueueSerializesAndHonorsDisabledSchedule(t *testing.T) {
	for _, tc := range []struct {
		name               string
		active             any
		enabled, scheduled bool
		blocked            bool
	}{
		{"duplicate manual", "active-run", true, false, true},
		{"disabled schedule", nil, false, true, true},
		{"manual on disabled plan", nil, false, false, false},
		{"scheduled due", nil, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			now := time.Now()
			r := &service.ModelQualityRun{ID: "run", PlanID: "plan", BatchID: "batch", StartedAt: now}
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT active_run_id, enabled, next_run_at, interval_minutes.* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active", "enabled", "due", "interval", "config"}).AddRow(tc.active, tc.enabled, now.Add(-time.Minute), 60, "{}"))
			if tc.blocked {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec(`INSERT INTO model_quality_runs`).WithArgs("run", "plan", "", "batch", "", "", sqlmock.AnyArg(), now, now).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE model_quality_plans SET active_run_id`).WithArgs("run", now.Add(time.Hour), "plan").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = NewModelQualityRepository(db).Queue(context.Background(), r, tc.scheduled)
			if tc.blocked {
				require.ErrorIs(t, err, service.ErrQualityBusy)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModelQualityFencesOldWorkerAndReleasesPlanOnCompletion(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		t.Run(string(rune('0'+affected)), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			now := time.Now()
			r := &service.ModelQualityRun{ID: "run", PlanID: "plan", Status: "failed", Verdict: "error", Phase: "complete", LeaseToken: "lease", FinishedAt: &now}
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT active_run_id FROM model_quality_plans .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow("run"))
			mock.ExpectExec(`UPDATE model_quality_runs SET .* WHERE id = \? AND lease_token = \? AND finished_at IS NULL`).WillReturnResult(sqlmock.NewResult(0, affected))
			if affected == 0 {
				mock.ExpectRollback()
			} else {
				mock.ExpectQuery(`SELECT id FROM model_quality_runs .* FOR UPDATE`).WithArgs("plan").WillReturnError(sql.ErrNoRows)
				mock.ExpectExec(`UPDATE model_quality_plans SET active_run_id = \? WHERE id = \?`).WithArgs(nil, "plan").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = NewModelQualityRepository(db).SaveRun(context.Background(), r, now)
			if affected == 0 {
				require.ErrorIs(t, err, service.ErrQualityBusy)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestModelQualityClaimRecoversPersistedRemoteTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	target, err := json.Marshal(service.ModelQualityTarget{GroupID: 12})
	require.NoError(t, err)
	columns := strings.Split(qualityRunColumns, ", ")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM model_quality_runs WHERE finished_at IS NULL AND lease_until <= \? .* FOR UPDATE SKIP LOCKED`).WithArgs(now).WillReturnRows(sqlmock.NewRows(columns).AddRow("run", "plan", "", "batch", string(target), "running", "pending", "generating", "", nil, "remote-id", 0, false, "old", now.Add(-time.Minute), now.Add(-time.Minute), nil))
	mock.ExpectExec(`UPDATE model_quality_runs SET lease_token`).WithArgs("new-lease", now.Add(time.Minute), now, "run").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	run, err := NewModelQualityRepository(db).Claim(context.Background(), now, "new-lease")
	require.NoError(t, err)
	require.Equal(t, "remote-id", run.RemoteID)
	require.Equal(t, "new-lease", run.LeaseToken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityResultsPageRestoresHTMLWithoutInternalFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	target, err := encodeQualityTarget(&service.ModelQualityRun{Target: service.ModelQualityTarget{GroupID: 12, MonitorID: 8}, CredentialHash: "private-fingerprint"})
	require.NoError(t, err)
	columns := strings.Split(qualityRunColumns, ", ")
	mock.ExpectQuery(`SELECT .* FROM model_quality_runs WHERE plan_id IN \(\?,\?\) AND finished_at IS NOT NULL AND benchmark = \? ORDER BY started_at DESC, id DESC LIMIT \? OFFSET \?`).
		WithArgs("plan1", "plan2", "pelican", 13, 12).
		WillReturnRows(sqlmock.NewRows(columns).AddRow("run", "plan1", "", "batch", string(target), "succeeded", "normal", "complete", "", `{"html":"<svg></svg>","duration_ms":100}`, "", 0, false, "", now, now, now))
	rows, err := NewModelQualityRepository(db).ResultsForPlans(context.Background(), []string{"plan1", "plan2"}, "pelican", 13, 12)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "<svg></svg>", rows[0].Result.HTML)
	require.Equal(t, "private-fingerprint", rows[0].CredentialHash)
	public, err := json.Marshal(rows)
	require.NoError(t, err)
	require.NotContains(t, string(public), "private-fingerprint")
	require.NotContains(t, string(public), "remote_id")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityBatchQueueRollsBackAllOnPartialFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	runs := []*service.ModelQualityRun{{ID: "one", PlanID: "plan", BatchID: "batch", StartedAt: now}, {ID: "two", PlanID: "plan", BatchID: "batch", StartedAt: now}}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT active_run_id, enabled, next_run_at, interval_minutes.* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active", "enabled", "due", "interval", "config"}).AddRow(nil, true, now.Add(-time.Minute), 3, `{"configs":[{},{}]}`))
	mock.ExpectExec(`INSERT INTO model_quality_runs`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO model_quality_runs`).WillReturnError(errors.New("disk failure"))
	mock.ExpectRollback()
	err = NewModelQualityRepository(db).QueueBatch(context.Background(), runs, false)
	require.ErrorContains(t, err, "disk failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityCompletedChildKeepsPlanBusyForSiblings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	run := &service.ModelQualityRun{ID: "child-one", PlanID: "plan", FinishedAt: &now, LeaseToken: "lease"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT active_run_id FROM model_quality_plans .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow("child-one"))
	mock.ExpectExec(`UPDATE model_quality_runs .* lease_token = \? AND finished_at IS NULL`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id FROM model_quality_runs .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("child-two"))
	mock.ExpectExec(`UPDATE model_quality_plans SET active_run_id = \? WHERE id = \?`).WithArgs("child-two", "plan").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewModelQualityRepository(db).SaveRun(context.Background(), run, now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityDeleteIsAtomicAndRejectsActivePlan(t *testing.T) {
	for _, active := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		var value any
		if active {
			value = "run"
		}
		mock.ExpectQuery(`SELECT active_run_id .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow(value))
		if active {
			mock.ExpectRollback()
		} else {
			mock.ExpectExec(`DELETE FROM model_quality_runs WHERE plan_id = \?`).WithArgs("plan").WillReturnResult(sqlmock.NewResult(0, 5))
			mock.ExpectExec(`DELETE FROM model_quality_plans WHERE id = \?`).WithArgs("plan").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		err = NewModelQualityRepository(db).DeletePlan(context.Background(), "plan")
		if active {
			require.ErrorIs(t, err, service.ErrQualityBusy)
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestModelQualityResultsSQLFiltersBeforePagination(t *testing.T) {
	for _, current := range []bool{true, false} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		batch, exclude, clause := "batch", "", "batch_id = ?"
		if !current {
			batch, exclude, clause = "", "batch", "batch_id <> ?"
		}
		query := "SELECT " + qualityRunColumns + " FROM model_quality_runs WHERE plan_id = ? AND finished_at IS NOT NULL AND " + clause + " AND model = ? AND benchmark = ? ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?"
		mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("plan", "batch", "model-a", "creative", 13, 12).WillReturnRows(sqlmock.NewRows(strings.Split(qualityRunColumns, ", ")))
		rows, err := NewModelQualityRepository(db).PlanResults(context.Background(), "plan", batch, exclude, "model-a", "creative", 13, 12)
		require.NoError(t, err)
		require.Empty(t, rows)
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestModelQualityRecentStatsUse24HoursAndSeparateConfigurations(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	high := service.QualityConfigID("model-a", "pelican", "high", "")
	low := service.QualityConfigID("model-a", "pelican", "low", "")
	// The completed-time window applies before aggregation. No batch filter,
	// page limit or current model filter can narrow these rolling statistics.
	mock.ExpectQuery(`SELECT config_id, target_json, COUNT\(\*\), .* FROM model_quality_runs WHERE plan_id = \? AND finished_at >= \? AND finished_at <= \? GROUP BY config_id, target_json$`).
		WithArgs("plan", now.Add(-24*time.Hour), now).
		WillReturnRows(sqlmock.NewRows([]string{"config", "target", "total", "normal", "degraded", "error", "unknown"}).
			AddRow(high, `{}`, 10, 9, 1, 0, 0).
			AddRow(high, `{"monitor_name":"renamed"}`, 10, 8, 0, 1, 1).
			AddRow("", `{"model":"model-a","benchmark":"pelican","reasoning_effort":"high"}`, 2, 1, 0, 1, 0).
			AddRow("", `{"model":"model-a","benchmark":"pelican","reasoning_effort":"low"}`, 3, 1, 1, 1, 0))
	stats, err := NewModelQualityRepository(db).RecentStats(context.Background(), "plan", now)
	require.NoError(t, err)
	require.Len(t, stats, 2)
	require.Equal(t, service.ModelQualityStats{Total: 22, Passed: 18, Failed: 1, Errors: 2, Unknown: 1}, stats[high])
	require.Equal(t, service.ModelQualityStats{Total: 3, Passed: 1, Failed: 1, Errors: 1}, stats[low])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityBatchesExposeCountsForMixedResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`SELECT batch_id, .* HAVING COUNT\(\*\) = COUNT\(finished_at\).* LIMIT \?`).WithArgs("plan", 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "started", "finished", "count", "verdict", "normal", "degraded", "error", "unknown"}).
			AddRow("mixed", now, now, 3, "degraded", 1, 1, 1, 0).
			AddRow("error", now, now, 2, "error", 1, 0, 1, 0))
	batches, err := NewModelQualityRepository(db).Batches(context.Background(), "plan", 60)
	require.NoError(t, err)
	require.Len(t, batches, 2)
	require.Equal(t, service.ModelQualityStats{Total: 3, Passed: 1, Failed: 1, Errors: 1}, batches[0].Stats)
	require.Equal(t, service.ModelQualityStats{Total: 2, Passed: 1, Errors: 1}, batches[1].Stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityLegacyPlanAndRunIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	config := `{"model":"legacy","benchmark":"candy","reasoning_effort":"high"}`
	mock.ExpectQuery(`SELECT .* FROM model_quality_plans`).WithArgs("plan").WillReturnRows(sqlmock.NewRows(strings.Split(qualityPlanColumns, ", ")).AddRow("plan", 3, 12, "name", config, false, true, 60, now, now, nil))
	repo := NewModelQualityRepository(db)
	plan, err := repo.GetPlan(context.Background(), "plan")
	require.NoError(t, err)
	require.Len(t, plan.Configs, 1)
	mock.ExpectQuery(`SELECT .* FROM model_quality_runs`).WithArgs("run").WillReturnRows(sqlmock.NewRows(strings.Split(qualityRunColumns, ", ")).AddRow("run", "plan", "", "run", config, "succeeded", "normal", "complete", "", nil, "", 0, false, "", now, now, now))
	run, err := repo.GetRun(context.Background(), "run")
	require.NoError(t, err)
	require.Equal(t, plan.Configs[0].ID, run.ConfigID)
	require.Equal(t, "run", run.BatchID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityCancelTargetsEntireRound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`SELECT .* FROM model_quality_runs WHERE id = \?`).WithArgs("child").WillReturnRows(sqlmock.NewRows(strings.Split(qualityRunColumns, ", ")).AddRow("child", "plan", "cfg", "round", "{}", "running", "pending", "", "", nil, "", 0, false, "", now, now, nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE model_quality_runs SET cancel_requested = TRUE WHERE plan_id = ? AND batch_id = ? AND finished_at IS NULL`)).WithArgs("plan", "round").WillReturnResult(sqlmock.NewResult(0, 3))
	require.NoError(t, NewModelQualityRepository(db).RequestCancel(context.Background(), "child"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelQualityRejectsRoundBuiltBeforePlanEdit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT active_run_id, enabled, next_run_at, interval_minutes, config_json .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active", "enabled", "due", "interval", "config"}).AddRow(nil, true, now, 3, `{"model":"new-model","benchmark":"candy"}`))
	mock.ExpectRollback()
	err = NewModelQualityRepository(db).Queue(context.Background(), &service.ModelQualityRun{ID: "run", PlanID: "plan", StartedAt: now, Target: service.ModelQualityTarget{Model: "old-model", Benchmark: "candy"}}, false)
	require.ErrorIs(t, err, service.ErrQualityBusy)
	require.NoError(t, mock.ExpectationsWereMet())
}
