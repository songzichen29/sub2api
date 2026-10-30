package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type qualityTestRepo struct {
	ModelQualityRepository
	plan                      *ModelQualityPlan
	plans                     []*ModelQualityPlan
	run                       *ModelQualityRun
	saved                     []*ModelQualityRun
	next                      time.Time
	historyPlans              []string
	resultLimit, resultOffset int
	results                   []*ModelQualityRun
	queued                    []*ModelQualityRun
	dimensions                []ModelQualityDimension
	resultScope               []string
	deleted                   string
	resultBenchmark           string
	recentStats               map[string]ModelQualityStats
	statsPlan                 string
	statsNow                  time.Time
}

func (r *qualityTestRepo) GetPlan(context.Context, string) (*ModelQualityPlan, error) {
	return r.plan, nil
}
func (r *qualityTestRepo) ListPlans(context.Context) ([]*ModelQualityPlan, error) {
	return r.plans, nil
}
func (r *qualityTestRepo) GetRun(context.Context, string) (*ModelQualityRun, error) {
	return r.run, nil
}
func (r *qualityTestRepo) SaveRun(_ context.Context, run *ModelQualityRun, next time.Time) error {
	copy := *run
	r.saved = append(r.saved, &copy)
	r.next = next
	return nil
}
func (r *qualityTestRepo) History(context.Context, string, int, int) ([]*ModelQualityRun, error) {
	return []*ModelQualityRun{r.run}, nil
}
func (r *qualityTestRepo) Stats(context.Context, string) (ModelQualityStats, error) {
	return ModelQualityStats{}, nil
}
func (r *qualityTestRepo) HistoryForPlans(_ context.Context, ids []string, _, _ int) ([]*ModelQualityRun, error) {
	r.historyPlans = ids
	return []*ModelQualityRun{r.run}, nil
}
func (r *qualityTestRepo) ResultsForPlans(_ context.Context, ids []string, benchmark string, limit, offset int) ([]*ModelQualityRun, error) {
	r.resultBenchmark = benchmark
	r.historyPlans, r.resultLimit, r.resultOffset = ids, limit, offset
	return r.results, nil
}
func (r *qualityTestRepo) CreatePlan(_ context.Context, p *ModelQualityPlan) error {
	r.plan = p
	return nil
}

func (r *qualityTestRepo) UpdatePlan(_ context.Context, p *ModelQualityPlan) error {
	r.plan = p
	return nil
}
func (r *qualityTestRepo) DeletePlan(_ context.Context, id string) error { r.deleted = id; return nil }
func (r *qualityTestRepo) QueueBatch(_ context.Context, runs []*ModelQualityRun, _ bool) error {
	r.queued = runs
	return nil
}
func (r *qualityTestRepo) Batches(context.Context, string, int) ([]ModelQualityBatch, error) {
	return []ModelQualityBatch{}, nil
}
func (r *qualityTestRepo) ResultDimensions(_ context.Context, planID, batchID, excludeBatchID string) ([]ModelQualityDimension, error) {
	r.resultScope = []string{planID, batchID, excludeBatchID}
	return r.dimensions, nil
}
func (r *qualityTestRepo) PlanResults(_ context.Context, planID, batchID, excludeBatchID, model, benchmark string, limit, offset int) ([]*ModelQualityRun, error) {
	r.resultScope = []string{planID, batchID, excludeBatchID, model, benchmark}
	r.resultLimit, r.resultOffset = limit, offset
	return r.results, nil
}
func (r *qualityTestRepo) RecentStats(_ context.Context, planID string, now time.Time) (map[string]ModelQualityStats, error) {
	r.statsPlan, r.statsNow = planID, now
	return r.recentStats, nil
}

type qualityTestKeys struct {
	APIKeyRepository
	key *APIKey
}

func (k qualityTestKeys) GetByID(context.Context, int64) (*APIKey, error) { return k.key, nil }
func (k qualityTestKeys) GetByKey(context.Context, string) (*APIKey, error) {
	if k.key == nil {
		return nil, ErrAPIKeyNotFound
	}
	return k.key, nil
}

type qualityTestMonitors struct{ monitor *ChannelMonitor }

func (m qualityTestMonitors) Get(context.Context, int64) (*ChannelMonitor, error) {
	return m.monitor, nil
}
func (m qualityTestMonitors) List(context.Context, ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	return []*ChannelMonitor{m.monitor}, 1, nil
}

func TestModelQualityGroupHistoryFiltersPrivateAndOtherGroups(t *testing.T) {
	s, repo, _, _ := qualityFixture()
	repo.plans = append(repo.plans, &ModelQualityPlan{ID: "same-group-private", GroupID: 12}, &ModelQualityPlan{ID: "other-group", GroupID: 99, PublicVisible: true})
	_, err := s.GroupHistory(context.Background(), 12, 42, false, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"plan"}, repo.historyPlans)
	_, err = s.GroupHistory(context.Background(), 99, 42, false, 1)
	require.ErrorIs(t, err, ErrQualityNotFound)
	_, err = s.GroupHistory(context.Background(), 12, 42, true, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "same-group-private"}, repo.historyPlans)
}

func TestModelQualityGroupResultsFiltersBeforePaging(t *testing.T) {
	s, repo, _, _ := qualityFixture()
	repo.plans = append(repo.plans,
		&ModelQualityPlan{ID: "pelican", GroupID: 12, PublicVisible: true, Config: ModelQualityConfig{Benchmark: "pelican"}},
		&ModelQualityPlan{ID: "private", GroupID: 12, Config: ModelQualityConfig{Benchmark: "creative"}},
		&ModelQualityPlan{ID: "other-group", GroupID: 99, PublicVisible: true, Config: ModelQualityConfig{Benchmark: "pelican"}},
	)
	for i := 0; i < 13; i++ {
		repo.results = append(repo.results, &ModelQualityRun{Result: &ModelQualityResult{HTML: "<svg></svg>"}})
	}
	result, err := s.GroupResults(context.Background(), 12, 42, false, "pelican", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"pelican"}, repo.historyPlans)
	require.Equal(t, []string{"candy", "pelican"}, result.Benchmarks)
	require.Equal(t, 12, repo.resultOffset)
	require.Equal(t, 13, repo.resultLimit)
	require.Len(t, result.Items, 12)
	require.True(t, result.HasMore)
	require.Equal(t, "<svg></svg>", result.Items[0].Result.HTML)
	_, err = s.GroupResults(context.Background(), 99, 42, false, "pelican", 1)
	require.ErrorIs(t, err, ErrQualityNotFound)
	_, err = s.GroupResults(context.Background(), 12, 42, false, "invalid", 1)
	require.ErrorIs(t, err, ErrQualityInvalid)
	_, err = s.GroupResults(context.Background(), 12, 42, false, "candy", 0)
	require.ErrorIs(t, err, ErrQualityInvalid)
}

func TestModelQualityGroupResultsModesComeFromVisiblePlans(t *testing.T) {
	s, repo, _, _ := qualityFixture()
	repo.plan.Config.Benchmark = "creative"
	repo.plans = append(repo.plans,
		&ModelQualityPlan{ID: "future", GroupID: 12, PublicVisible: true, Config: ModelQualityConfig{Benchmark: "new-mode"}},
		&ModelQualityPlan{ID: "duplicate", GroupID: 12, PublicVisible: true, Config: ModelQualityConfig{Benchmark: "creative"}},
		&ModelQualityPlan{ID: "private", GroupID: 12, Config: ModelQualityConfig{Benchmark: "pelican"}},
		&ModelQualityPlan{ID: "other-group", GroupID: 99, PublicVisible: true, Config: ModelQualityConfig{Benchmark: "candy"}},
	)
	result, err := s.GroupResults(context.Background(), 12, 42, false, "", 1)
	require.NoError(t, err)
	require.Equal(t, []string{"creative", "new-mode"}, result.Benchmarks)
	require.Equal(t, "creative", result.Benchmark)
	require.Equal(t, []string{"plan", "duplicate"}, repo.historyPlans)
	result, err = s.GroupResults(context.Background(), 12, 42, false, "new-mode", 1)
	require.NoError(t, err)
	require.Equal(t, "new-mode", result.Benchmark)
	require.Equal(t, []string{"future"}, repo.historyPlans)
	_, err = s.GroupResults(context.Background(), 12, 42, false, "pelican", 1)
	require.ErrorIs(t, err, ErrQualityInvalid)
}

type qualityTestGroups struct{ GroupRepository }

func (qualityTestGroups) GetByID(context.Context, int64) (*Group, error) {
	return &Group{ID: 12, Name: "Test group", Status: StatusActive}, nil
}

type qualityTestChannels struct{}

func (qualityTestChannels) GetChannelForGroup(context.Context, int64) (*Channel, error) {
	return &Channel{ID: 4, Name: "Test channel"}, nil
}

type qualityTestVisibility struct{ groups []Group }

func (v qualityTestVisibility) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return v.groups, nil
}

type qualityTestClient struct {
	creates, gets int
	result        *qualityCompletion
	err           error
	request       qualityCompletionRequest
	credential    qualityCredentials
	judgment      string
}

func (c *qualityTestClient) Complete(_ context.Context, cred qualityCredentials, req qualityCompletionRequest) (*qualityCompletion, error) {
	c.request, c.credential = req, cred
	if req.Schema == nil {
		c.creates++
		return c.result, c.err
	}
	c.gets++
	if c.err != nil {
		return nil, c.err
	}
	if c.judgment != "" {
		return &qualityCompletion{Text: c.judgment}, nil
	}
	ids := req.Schema["properties"].(map[string]any)["checks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["id"].(map[string]any)["enum"].([]string)
	checks := []ModelQualityCheck{}
	for _, id := range ids {
		checks = append(checks, ModelQualityCheck{ID: id, Score: 2, Evidence: "explicit evidence for " + id})
	}
	b, _ := json.Marshal(map[string]any{"confidence": 0.95, "reason": "根据具体实现完成了任务", "checks": checks})
	return &qualityCompletion{Text: string(b)}, nil
}
func (k qualityTestKeys) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	if k.key == nil {
		return nil, &pagination.PaginationResult{}, nil
	}
	return []APIKey{*k.key}, &pagination.PaginationResult{Total: 1}, nil
}
func qualityFixture() (*ModelQualityService, *qualityTestRepo, *qualityTestClient, *ModelQualityRun) {
	groupID := int64(12)
	p := &ModelQualityPlan{ID: "plan", GroupID: groupID, OwnerID: 3, PublicVisible: true, Config: ModelQualityConfig{APIKeyID: 9, BaseURL: "https://our.example/custom/v1", Model: "our-model", Benchmark: "candy", ReasoningEffort: "high"}}
	r := &ModelQualityRun{ID: "run", PlanID: p.ID, Status: "queued", Verdict: "pending", StartedAt: time.Now(), Target: ModelQualityTarget{GroupID: groupID, Model: "our-model", Benchmark: "candy", BenchmarkVersion: qualityBenchmarkVersion, JudgeModel: "gpt-6-luna"}}
	repo := &qualityTestRepo{plan: p, plans: []*ModelQualityPlan{p}, run: r}
	c := &qualityTestClient{result: &qualityCompletion{Text: "最终答案21，9圆+12星。", DurationMS: 123}}
	s := &ModelQualityService{repo: repo, keys: qualityTestKeys{key: &APIKey{ID: 9, UserID: 3, Name: "测试降智", GroupID: &groupID, Key: "private-test-key", Status: StatusActive}}, groups: qualityTestGroups{}, channels: qualityTestChannels{}, visibility: qualityTestVisibility{groups: []Group{{ID: groupID}}}, generator: c, judge: c, judgeOrigin: "http://127.0.0.1:8080/v1", judgeKeyName: "测试降智", judgeModel: "gpt-6-luna"}
	return s, repo, c, r
}
func TestModelQualityLifecycleAndPrivateResult(t *testing.T) {
	s, repo, c, r := qualityFixture()
	c.result.Text = "21 private-test-key https://our.example/custom/v1"
	s.step(context.Background(), r)
	require.Equal(t, 1, c.creates)
	require.Equal(t, "our-model", c.request.Model)
	require.Equal(t, qualityCandyPrompt, c.request.Input)
	require.Equal(t, "submitting", repo.saved[0].Status)
	require.Equal(t, "running", r.Status)
	require.Equal(t, "classifying", r.Phase)
	require.Contains(t, r.Result.Answer, "[REDACTED]")
	s.step(context.Background(), r)
	require.Equal(t, 1, c.creates)
	require.Equal(t, 1, c.gets)
	require.Equal(t, "gpt-6-luna", c.request.Model)
	require.Equal(t, "http://127.0.0.1:8080/v1", c.credential.baseURL)
	require.Equal(t, "succeeded", r.Status)
	require.Equal(t, "normal", r.Verdict)
	require.NotNil(t, r.Result.Assessment)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	for _, secret := range []string{"private-test-key", "https://our.example/custom/v1", "lease_token", "_judge_key_id"} {
		require.NotContains(t, string(b), secret)
	}
}
func TestModelQualityRestartAndFailure(t *testing.T) {
	for _, phase := range []string{"generating", "classifying"} {
		t.Run("uncertain "+phase, func(t *testing.T) {
			s, _, c, r := qualityFixture()
			r.Status = "submitting"
			r.Phase = phase
			s.step(context.Background(), r)
			require.Zero(t, c.creates+c.gets)
			require.Equal(t, "error", r.Verdict)
		})
	}
	t.Run("legacy tasks never contact Manxue", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.RemoteID = "legacy"
		s.step(context.Background(), r)
		require.Zero(t, c.creates+c.gets)
		require.Equal(t, "error", r.Verdict)
	})
	t.Run("request failure never resubmits", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		c.err = errors.New("secret-echo")
		s.step(context.Background(), r)
		require.Equal(t, 1, c.creates)
		require.Equal(t, "error", r.Verdict)
		require.NotContains(t, r.Reason, "secret-echo")
	})
	t.Run("saved artifact resumes judging only", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		s.step(context.Background(), r)
		s.step(context.Background(), r)
		require.Equal(t, 1, c.creates)
		require.Equal(t, 1, c.gets)
	})
	t.Run("judge failure keeps artifact", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		s.step(context.Background(), r)
		c.err = errors.New("oops")
		s.step(context.Background(), r)
		require.Equal(t, "error", r.Verdict)
		require.NotEmpty(t, r.Result.Answer)
	})
	t.Run("cancelled", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.CancelRequested = true
		s.step(context.Background(), r)
		require.Zero(t, c.creates+c.gets)
		require.Equal(t, "cancelled", r.Status)
	})
	t.Run("rotated generator key", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		s.step(context.Background(), r)
		s.keys.(qualityTestKeys).key.Key = "new"
		s.step(context.Background(), r)
		require.Zero(t, c.gets)
		require.Equal(t, "error", r.Verdict)
	})
	t.Run("wrong owner", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		s.keys.(qualityTestKeys).key.UserID = 999
		s.step(context.Background(), r)
		require.Zero(t, c.creates+c.gets)
		require.Equal(t, "error", r.Verdict)
	})
}
func TestModelQualityVisibility(t *testing.T) {
	s, repo, _, run := qualityFixture()
	repo.recentStats = map[string]ModelQualityStats{"a": {Total: 10, Passed: 6, Failed: 1, Errors: 2, Unknown: 1}, "b": {Total: 2, Passed: 1, Errors: 1}}
	list, err := s.List(context.Background(), 42, false)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, ModelQualityStats{Total: 12, Passed: 7, Failed: 1, Errors: 3, Unknown: 1}, list[0].Stats24H)
	b, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(b), "api_key_id")
	s.visibility = qualityTestVisibility{}
	list, err = s.List(context.Background(), 42, false)
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = s.Detail(context.Background(), run.ID, 42, false)
	require.ErrorIs(t, err, ErrQualityNotFound)
	s.visibility = qualityTestVisibility{groups: []Group{{ID: 12}}}
	repo.plan.PublicVisible = false
	_, err = s.Detail(context.Background(), run.ID, 42, false)
	require.ErrorIs(t, err, ErrQualityNotFound)
}
func TestModelQualityConfig(t *testing.T) {
	for _, base := range []string{"http://example.com/v1", "https://localhost/v1", "https://127.0.0.1/v1", "https://10.0.0.1/v1", "https://user:password@example.com/v1", "https://example.com/v1?key=secret"} {
		cfg := ModelQualityConfig{BaseURL: base, Benchmark: "candy", Model: "model", ReasoningEffort: "high"}
		require.Error(t, validateQualityConfig(&cfg))
	}
	for _, mode := range []string{"candy", "pelican", "creative"} {
		cfg := ModelQualityConfig{BaseURL: "https://our.example/v1", Benchmark: mode, Model: "custom-model", ReasoningEffort: "max"}
		require.NoError(t, validateQualityConfig(&cfg))
	}
}

func TestModelQualityMultipleModelsAndModesInOneRound(t *testing.T) {
	s, repo, client, _ := qualityFixture()
	s.monitors = qualityTestMonitors{&ChannelMonitor{ID: 8, Name: "channel", Provider: MonitorProviderOpenAI, Enabled: true, Endpoint: "https://relay.example", APIKey: "test-key"}}
	in := ModelQualityInput{Name: "multi", GroupID: 12, IntervalMinutes: 3, Configs: []ModelQualityConfig{
		{MonitorID: 8, Model: "gpt-6-astra", Benchmark: "candy", ReasoningEffort: "high"},
		{MonitorID: 8, Model: "gpt-6-astra", Benchmark: "pelican", ReasoningEffort: "high"},
		{MonitorID: 8, Model: "another-model", Benchmark: "candy", ReasoningEffort: "low"},
	}}
	p, err := s.Save(context.Background(), "", 3, in)
	require.NoError(t, err)
	require.Len(t, p.Configs, 3)
	require.Equal(t, 3, p.IntervalMinutes)
	runs, err := s.Queue(context.Background(), p.ID, false)
	require.NoError(t, err)
	require.Len(t, runs, 3)
	require.Len(t, repo.queued, 3)
	ids := map[string]bool{}
	for i, run := range runs {
		require.NotEmpty(t, run.BatchID)
		require.Equal(t, runs[0].BatchID, run.BatchID)
		require.Equal(t, p.Configs[i].ID, run.ConfigID)
		require.False(t, ids[run.ID])
		ids[run.ID] = true
		s.step(context.Background(), run)
		require.Equal(t, p.Configs[i].Model, client.request.Model)
		require.Equal(t, qualityPrompt(p.Configs[i].Benchmark), client.request.Input)
	}
	// Existing plans can add models without deleting previous snapshots.
	in.Configs = append(in.Configs, ModelQualityConfig{MonitorID: 8, Model: "new-model", Benchmark: "candy"})
	_, err = s.Save(context.Background(), p.ID, 3, in)
	require.NoError(t, err)
	require.Len(t, repo.plan.Configs, 4)
	in.IntervalMinutes = 2
	_, err = s.Save(context.Background(), "", 3, in)
	require.ErrorIs(t, err, ErrQualityInvalid)
	in.IntervalMinutes = 3
	in.Configs[1].MonitorID = 99
	_, err = s.Save(context.Background(), "", 3, in)
	require.ErrorIs(t, err, ErrQualityInvalid)
	in.Configs[1] = in.Configs[0]
	_, err = s.Save(context.Background(), "", 3, in)
	require.ErrorIs(t, err, ErrQualityInvalid)
}

func TestModelQualityPlanResultsIsolateRoundAndHistory(t *testing.T) {
	s, repo, _, _ := qualityFixture()
	repo.recentStats = map[string]ModelQualityStats{
		"low":  {Total: 30, Passed: 10, Failed: 5, Errors: 10, Unknown: 5},
		"high": {Total: 10, Passed: 9, Failed: 1},
	}
	repo.dimensions = []ModelQualityDimension{{Model: "old-model", Benchmark: "creative"}, {Model: "model-b", Benchmark: "candy"}, {Model: "old-model", Benchmark: "future-mode"}}
	for i := 0; i < 13; i++ {
		repo.results = append(repo.results, &ModelQualityRun{ConfigID: "low"})
	}
	repo.results[1].ConfigID = "high"
	result, err := s.PlanResults(context.Background(), "plan", "batch-1", "", "old-model", "creative", 42, false, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"model-b", "old-model"}, result.Models)
	require.Equal(t, []string{"creative", "future-mode"}, result.Benchmarks)
	require.Equal(t, []string{"plan", "batch-1", "", "old-model", "creative"}, repo.resultScope)
	require.Len(t, result.Items, 12)
	require.True(t, result.HasMore)
	require.Equal(t, repo.recentStats["low"], *result.Items[0].Stats24H)
	require.Equal(t, repo.recentStats["high"], *result.Items[1].Stats24H)
	require.Equal(t, "plan", repo.statsPlan)
	require.WithinDuration(t, time.Now(), repo.statsNow, time.Second)
	result, err = s.PlanResults(context.Background(), "plan", "", "batch-1", "old-model", "future-mode", 42, false, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "", "batch-1", "old-model", "future-mode"}, repo.resultScope)
	require.Equal(t, 12, repo.resultOffset)
	require.Equal(t, repo.recentStats["low"], *result.Items[0].Stats24H)
	require.Equal(t, "plan", repo.statsPlan)
	_, err = s.PlanResults(context.Background(), "other-plan", "batch-1", "", "", "", 42, false, 1)
	require.ErrorIs(t, err, ErrQualityNotFound)
	repo.plan.PublicVisible = false
	_, err = s.PlanResults(context.Background(), "plan", "batch-1", "", "", "", 42, false, 1)
	require.ErrorIs(t, err, ErrQualityNotFound)
}

func TestModelQualityQueueWaitDoesNotConsumeExecutionTimeout(t *testing.T) {
	s, _, client, run := qualityFixture()
	run.StartedAt = time.Now().Add(-time.Hour)
	now := time.Now()
	run.ExecutionStartedAt = &now
	s.step(context.Background(), run)
	require.Equal(t, 1, client.creates)
	require.Nil(t, run.FinishedAt)
}

func TestModelQualityDeleteAuthorization(t *testing.T) {
	s, repo, _, _ := qualityFixture()
	err := s.Delete(context.Background(), "plan", 42, false)
	require.ErrorIs(t, err, ErrQualityInvalid)
	require.Empty(t, repo.deleted)
	require.NoError(t, s.Delete(context.Background(), "plan", 42, true))
	require.Equal(t, "plan", repo.deleted)
}
