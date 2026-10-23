package repository

import (
	"context"
	"encoding/json"
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
			r := &service.ModelQualityRun{ID: "run", PlanID: "plan", StartedAt: now}
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT active_run_id, enabled, next_run_at, interval_minutes .* FOR UPDATE`).WithArgs("plan").WillReturnRows(sqlmock.NewRows([]string{"active", "enabled", "due", "interval"}).AddRow(tc.active, tc.enabled, now.Add(-time.Minute), 60))
			if tc.blocked {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec(`INSERT INTO model_quality_runs`).WithArgs("run", "plan", sqlmock.AnyArg(), now, now).WillReturnResult(sqlmock.NewResult(0, 1))
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
			mock.ExpectExec(`UPDATE model_quality_runs SET .* WHERE id = \? AND lease_token = \? AND finished_at IS NULL`).WillReturnResult(sqlmock.NewResult(0, affected))
			if affected == 0 {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec(`UPDATE model_quality_plans SET active_run_id = NULL WHERE id = \? AND active_run_id = \?`).WithArgs("plan", "run").WillReturnResult(sqlmock.NewResult(0, 1))
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
	columns := []string{"id", "plan_id", "target_json", "status", "verdict", "phase", "reason", "result_json", "remote_id", "attempts", "cancel_requested", "lease_token", "started_at", "finished_at"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM model_quality_runs WHERE finished_at IS NULL AND lease_until <= \? .* FOR UPDATE SKIP LOCKED`).WithArgs(now).WillReturnRows(sqlmock.NewRows(columns).AddRow("run", "plan", string(target), "running", "pending", "generating", "", nil, "remote-id", 0, false, "old", now.Add(-time.Minute), nil))
	mock.ExpectExec(`UPDATE model_quality_runs SET lease_token`).WithArgs("new-lease", now.Add(time.Minute), "run").WillReturnResult(sqlmock.NewResult(0, 1))
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
	columns := []string{"id", "plan_id", "target_json", "status", "verdict", "phase", "reason", "result_json", "remote_id", "attempts", "cancel_requested", "lease_token", "started_at", "finished_at"}
	mock.ExpectQuery(`SELECT .* result_json, '', attempts, cancel_requested, '', started_at, finished_at FROM model_quality_runs WHERE plan_id IN \(\?,\?\) ORDER BY started_at DESC, id DESC LIMIT \? OFFSET \?`).
		WithArgs("plan1", "plan2", 13, 12).
		WillReturnRows(sqlmock.NewRows(columns).AddRow("run", "plan1", string(target), "succeeded", "normal", "complete", "", `{"html":"<svg></svg>","duration_ms":100}`, "", 0, false, "", now, now))
	rows, err := NewModelQualityRepository(db).ResultsForPlans(context.Background(), []string{"plan1", "plan2"}, 13, 12)
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
