package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrQualityInvalid  = errors.New("invalid model quality configuration")
	ErrQualityBusy     = errors.New("a detection is already in progress")
	ErrQualityNotFound = errors.New("model quality record not found")
)

// Credentials are referenced, never copied into plans or result JSON.
type ModelQualityConfig struct {
	MonitorID       int64  `json:"monitor_id,omitempty"`
	BaseURL         string `json:"base_url,omitempty"`   // Legacy plans only.
	APIKeyID        int64  `json:"api_key_id,omitempty"` // Legacy plans only.
	Benchmark       string `json:"benchmark"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	ServiceTier     string `json:"service_tier"`
}

type ModelQualityPlan struct {
	ID              string             `json:"id"`
	OwnerID         int64              `json:"-"`
	GroupID         int64              `json:"group_id"`
	Name            string             `json:"name"`
	Config          ModelQualityConfig `json:"-"`
	Enabled         bool               `json:"enabled"`
	PublicVisible   bool               `json:"public_visible"`
	IntervalMinutes int                `json:"interval_minutes"`
	NextRunAt       time.Time          `json:"next_run_at"`
	CreatedAt       time.Time          `json:"created_at"`
}

type ModelQualityTarget struct {
	MonitorID       int64  `json:"monitor_id,omitempty"`
	MonitorName     string `json:"monitor_name,omitempty"`
	GroupID         int64  `json:"group_id"`
	GroupName       string `json:"group_name"`
	ChannelID       int64  `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	Benchmark       string `json:"benchmark"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	ServiceTier     string `json:"service_tier"`
}

type ModelQualityResult struct {
	Answer          string `json:"answer,omitempty"`
	HTML            string `json:"html,omitempty"`
	Source          string `json:"source,omitempty"`
	DurationMS      int64  `json:"duration_ms"`
	InputTokens     *int64 `json:"input_tokens,omitempty"`
	OutputTokens    *int64 `json:"output_tokens,omitempty"`
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
}

type ModelQualityRun struct {
	ID              string              `json:"id"`
	PlanID          string              `json:"plan_id"`
	Target          ModelQualityTarget  `json:"target"`
	Status          string              `json:"status"`
	Verdict         string              `json:"verdict"`
	Phase           string              `json:"phase"`
	Reason          string              `json:"reason"`
	StartedAt       time.Time           `json:"started_at"`
	FinishedAt      *time.Time          `json:"finished_at"`
	Result          *ModelQualityResult `json:"result,omitempty"`
	CancelRequested bool                `json:"cancel_requested"`
	RemoteID        string              `json:"-"`
	LeaseToken      string              `json:"-"`
	Attempts        int                 `json:"-"`
	CredentialHash  string              `json:"-"`
}

type ModelQualityStats struct {
	Total   int64 `json:"total"`
	Passed  int64 `json:"passed"`
	Failed  int64 `json:"failed"`
	Errors  int64 `json:"errors"`
	Unknown int64 `json:"unknown"`
}

type ModelQualityView struct {
	*ModelQualityPlan
	CanEdit bool               `json:"can_edit"`
	Target  ModelQualityTarget `json:"target"`
	// Admin-only; the API key itself is never included.
	AdminConfig *ModelQualityConfig `json:"config,omitempty"`
	Stats       ModelQualityStats   `json:"stats"`
	Recent      []*ModelQualityRun  `json:"recent"`
	MonitorIDs  []int64             `json:"monitor_ids"`
}

type ModelQualityResults struct {
	Benchmark  string             `json:"benchmark"`
	Benchmarks []string           `json:"benchmarks"`
	Items      []*ModelQualityRun `json:"items"`
	HasMore    bool               `json:"has_more"`
}

type ModelQualityRepository interface {
	CreatePlan(context.Context, *ModelQualityPlan) error
	GetPlan(context.Context, string) (*ModelQualityPlan, error)
	ListPlans(context.Context) ([]*ModelQualityPlan, error)
	UpdatePlan(context.Context, *ModelQualityPlan) error
	// Queue locks the plan row, serializing manual and scheduled runs across instances.
	Queue(context.Context, *ModelQualityRun, bool) error
	Claim(context.Context, time.Time, string) (*ModelQualityRun, error)
	SaveRun(context.Context, *ModelQualityRun, time.Time) error
	RequestCancel(context.Context, string) error
	GetRun(context.Context, string) (*ModelQualityRun, error)
	History(context.Context, string, int, int) ([]*ModelQualityRun, error)
	HistoryForPlans(context.Context, []string, int, int) ([]*ModelQualityRun, error)
	ResultsForPlans(context.Context, []string, int, int) ([]*ModelQualityRun, error)
	Stats(context.Context, string) (ModelQualityStats, error)
}
