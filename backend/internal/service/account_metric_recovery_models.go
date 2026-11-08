package service

import (
	"context"
	"encoding/json"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrInvalidAccountMetricConfig    = infraerrors.BadRequest("ACCOUNT_METRIC_CONFIG_INVALID", "invalid account metric recovery configuration")
	ErrAccountMetricRevisionConflict = infraerrors.Conflict("ACCOUNT_METRIC_REVISION_CONFLICT", "account metric configuration changed; refresh and retry")
	ErrAccountMetricPauseConflict    = infraerrors.Conflict("ACCOUNT_METRIC_PAUSE_CONFLICT", "account metric pause state changed; refresh and retry")
)

// AccountMetricSnapshot contains valid rolling observations for one account.
// Rate values are ratios in [0, 1]; latency values are arithmetic means in ms.
type AccountMetricSnapshot struct {
	AccountID         int64     `json:"account_id"`
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
	SuccessRate       *float64  `json:"success_rate"`
	SuccessSamples    int64     `json:"success_samples"`
	ErrorSamples      int64     `json:"error_samples"`
	CacheRate         *float64  `json:"cache_rate"`
	CacheSamples      int64     `json:"cache_samples"`
	AverageTTFTMs     *float64  `json:"average_ttft_ms"`
	TTFTSamples       int64     `json:"ttft_samples"`
	AverageDurationMs *float64  `json:"average_duration_ms"`
	DurationSamples   int64     `json:"duration_samples"`
}

// AccountMetricRepository reads account-scoped request observations from Ops
// errors and successful usage rows.
type AccountMetricRepository interface {
	GetAccountMetricSnapshot(ctx context.Context, accountID int64, start, end time.Time) (*AccountMetricSnapshot, error)
}

type AccountPerformanceMonitor struct {
	AccountID       int64                       `json:"account_id"`
	Config          AccountMetricRecoveryConfig `json:"config"`
	Revision        int64                       `json:"revision"`
	Paused          bool                        `json:"paused"`
	PauseGeneration int64                       `json:"pause_generation"`
	PausedAt        *time.Time                  `json:"paused_at,omitempty"`
	Reason          string                      `json:"reason"`
	TrialStartedAt  *time.Time                  `json:"trial_started_at,omitempty"`
	TrialRequests   int64                       `json:"trial_requests"`
	NextTrialAt     *time.Time                  `json:"next_trial_at,omitempty"`
	MetricsSince    *time.Time                  `json:"metrics_since,omitempty"`
	CreatedAt       *time.Time                  `json:"created_at,omitempty"`
	UpdatedAt       *time.Time                  `json:"updated_at,omitempty"`
}

type AccountPerformanceStatus struct {
	Monitor          *AccountPerformanceMonitor `json:"monitor"`
	MetricsAvailable bool                       `json:"metrics_available"`
	Metrics          *AccountMetricSnapshot     `json:"metrics,omitempty"`
}

// Bind background evidence to the configuration, pause and trial that produced
// it. A different evaluator may have completed a cycle while SQL was running.
type AccountMetricEvaluationToken struct {
	Revision        int64
	PauseGeneration int64
	TrialStartedAt  *time.Time
}

func (m *AccountPerformanceMonitor) EvaluationToken() AccountMetricEvaluationToken {
	return AccountMetricEvaluationToken{Revision: m.Revision, PauseGeneration: m.PauseGeneration, TrialStartedAt: m.TrialStartedAt}
}

type AccountPerformanceEvent struct {
	ID              int64           `json:"id"`
	AccountID       int64           `json:"account_id"`
	Action          string          `json:"action"`
	ActorID         *int64          `json:"actor_id,omitempty"`
	Revision        int64           `json:"revision"`
	PauseGeneration int64           `json:"pause_generation"`
	Evidence        json.RawMessage `json:"evidence"`
	CreatedAt       time.Time       `json:"created_at"`
}

type AccountPerformanceAdmissionRepository interface {
	// AdmitAccountPerformanceRequest returns false while the account is paused
	// and outside its rate-limited recovery trial.
	AdmitAccountPerformanceRequest(ctx context.Context, accountID int64, now time.Time) (bool, error)
}

type AccountPerformanceRepository interface {
	GetAccountMetricSnapshots(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]*AccountMetricSnapshot, error)
	GetAccountPerformanceMonitors(ctx context.Context, accountIDs []int64) (map[int64]*AccountPerformanceMonitor, error)
	AccountMetricRepository
	AccountPerformanceAdmissionRepository
	GetAccountPerformanceMonitor(ctx context.Context, accountID int64) (*AccountPerformanceMonitor, error)
	ListAccountPerformanceMonitors(ctx context.Context) ([]*AccountPerformanceMonitor, error)
	SaveAccountMetricRecoveryConfig(ctx context.Context, accountID, expectedRevision int64, config AccountMetricRecoveryConfig, actorID *int64, now time.Time) (*AccountPerformanceMonitor, error)
	ReleaseAccountMetricPause(ctx context.Context, accountID, expectedRevision, expectedPauseGeneration int64, actorID *int64, now time.Time) (*AccountPerformanceMonitor, error)
	ListAccountPerformanceEvents(ctx context.Context, accountID, beforeID int64, limit int) ([]AccountPerformanceEvent, error)
	PauseAccountForMetricBreach(ctx context.Context, accountID int64, token AccountMetricEvaluationToken, reason string, evidence []AccountMetricRuleEvidence, now time.Time) (bool, error)
	RecoverAccountFromMetricTrial(ctx context.Context, accountID int64, token AccountMetricEvaluationToken, evidence []AccountMetricRuleEvidence, now time.Time) (bool, error)
	EndAccountMetricTrial(ctx context.Context, accountID int64, token AccountMetricEvaluationToken, outcome string, evidence []AccountMetricRuleEvidence, now time.Time) (bool, error)
}
