package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestModelQualityBoundMonitorCredentials(t *testing.T) {
	s, repo, client, run := qualityFixture()
	m := &ChannelMonitor{ID: 8, Name: "probe", Provider: MonitorProviderOpenAI, Enabled: true, Endpoint: "https://relay.example", APIKey: "bound-secret"}
	s.monitors = qualityTestMonitors{m}
	repo.plan.Config.MonitorID = m.ID
	repo.plan.Config.BaseURL, repo.plan.Config.APIKeyID = "", 0
	s.step(context.Background(), run)
	require.Equal(t, "https://relay.example/v1", client.request.BaseURL)
	require.Equal(t, "bound-secret", client.request.APIKey)
	require.NotEmpty(t, run.CredentialHash)
	client.task = &ManxueTask{Status: "succeeded", Candy: &ManxueCandy{Status: "passed", Answer: "bound-secret"}}
	m.APIKey = "rotated-secret"
	s.step(context.Background(), run)
	require.Equal(t, "normal", run.Verdict)
	require.Empty(t, run.Result.Answer, "rotated credentials must not allow the original secret to be echoed")

	t.Run("wrong group rejected before submit", func(t *testing.T) {
		s, repo, client, run := qualityFixture()
		s.monitors = qualityTestMonitors{m}
		repo.plan.Config.MonitorID = m.ID
		groupID := int64(99)
		s.keys = qualityTestKeys{key: &APIKey{GroupID: &groupID, Status: StatusActive}}
		s.step(context.Background(), run)
		require.Zero(t, client.creates)
		require.Equal(t, "error", run.Verdict)
	})
	t.Run("new plan stores a reference only", func(t *testing.T) {
		s, repo, _, _ := qualityFixture()
		s.monitors = qualityTestMonitors{m}
		p, err := s.Save(context.Background(), "", 3, ModelQualityInput{Name: "monitor test", GroupID: 12, IntervalMinutes: 60, Config: ModelQualityConfig{MonitorID: 8, BaseURL: "https://ignored.example", APIKeyID: 9, Benchmark: "candy", Model: "our-model"}})
		require.NoError(t, err)
		require.Equal(t, int64(8), p.Config.MonitorID)
		require.Empty(t, repo.plan.Config.BaseURL)
		require.Zero(t, repo.plan.Config.APIKeyID)
	})
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
	creates, gets, cancels int
	task                   *ManxueTask
	err                    error
	request                ManxueCreateRequest
}

func (c *qualityTestClient) Create(_ context.Context, req ManxueCreateRequest) (*ManxueTask, error) {
	c.creates++
	c.request = req
	return c.task, c.err
}
func (c *qualityTestClient) Get(context.Context, string) (*ManxueTask, error) {
	c.gets++
	return c.task, c.err
}
func (c *qualityTestClient) Cancel(context.Context, string) error { c.cancels++; return c.err }

func qualityFixture() (*ModelQualityService, *qualityTestRepo, *qualityTestClient, *ModelQualityRun) {
	groupID := int64(12)
	p := &ModelQualityPlan{ID: "plan", GroupID: groupID, OwnerID: 3, PublicVisible: true, Config: ModelQualityConfig{APIKeyID: 9, BaseURL: "https://our.example/custom/v1", Model: "our-model", Benchmark: "candy", ReasoningEffort: "high"}}
	r := &ModelQualityRun{ID: "run", PlanID: p.ID, Status: "queued", Verdict: "pending", StartedAt: time.Now(), Target: ModelQualityTarget{GroupID: groupID, Model: "our-model", Benchmark: "candy"}}
	repo := &qualityTestRepo{plan: p, plans: []*ModelQualityPlan{p}, run: r}
	c := &qualityTestClient{task: &ManxueTask{ID: "remote-capability", Status: "running"}}
	s := &ModelQualityService{repo: repo, keys: qualityTestKeys{key: &APIKey{ID: 9, UserID: 3, GroupID: &groupID, Key: "private-test-key", Status: StatusActive}}, groups: qualityTestGroups{}, channels: qualityTestChannels{}, visibility: qualityTestVisibility{groups: []Group{{ID: groupID}}}, client: c}
	return s, repo, c, r
}

func TestModelQualityLifecycleAndPrivateResult(t *testing.T) {
	s, repo, client, run := qualityFixture()
	s.step(context.Background(), run)
	require.Equal(t, 1, client.creates)
	require.Equal(t, "https://our.example/custom/v1", client.request.BaseURL)
	require.Equal(t, "private-test-key", client.request.APIKey)
	require.Equal(t, "our-model", client.request.Model)
	require.Equal(t, run.ID, client.request.IdempotencyKey)
	require.Equal(t, "submitting", repo.saved[0].Status, "submission intent must be persisted before POST")
	require.Equal(t, "running", run.Status)
	client.task = &ManxueTask{ID: run.RemoteID, Status: "succeeded", Benchmark: "candy", Candy: &ManxueCandy{Status: "incorrect", Answer: "private-test-key remote-capability https://our.example/custom/v1"}}
	s.step(context.Background(), run)
	require.Equal(t, 1, client.creates)
	require.Equal(t, 1, client.gets)
	require.Equal(t, "succeeded", run.Status)
	require.Equal(t, "degraded", run.Verdict)
	require.NotNil(t, run.FinishedAt)
	b, err := json.Marshal(run)
	require.NoError(t, err)
	for _, secret := range []string{"private-test-key", "remote-capability", "https://our.example/custom/v1", "lease_token"} {
		require.NotContains(t, string(b), secret)
	}
}

func TestModelQualityRestartAndSubmissionUncertainty(t *testing.T) {
	t.Run("persisted remote task is queried", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.RemoteID = "remote-capability"
		r.Status = "running"
		c.task = &ManxueTask{Status: "succeeded", Candy: &ManxueCandy{Status: "passed"}}
		s.step(context.Background(), r)
		require.Zero(t, c.creates)
		require.Equal(t, 1, c.gets)
		require.Equal(t, "normal", r.Verdict)
	})
	t.Run("interrupted POST is not repeated", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.Status = "submitting"
		s.step(context.Background(), r)
		require.Zero(t, c.creates)
		require.Equal(t, "error", r.Verdict)
		require.NotNil(t, r.FinishedAt)
	})
	t.Run("ambiguous network error is not retried", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		c.err = errors.New("connection lost with secret body")
		s.step(context.Background(), r)
		require.Equal(t, 1, c.creates)
		require.Equal(t, "error", r.Verdict)
		require.NotContains(t, r.Reason, "secret body")
	})
	t.Run("rate limit waits and reuses idempotency key", func(t *testing.T) {
		s, repo, c, r := qualityFixture()
		c.err = &manxueHTTPError{Status: 429, RetryAfter: 45 * time.Second}
		s.step(context.Background(), r)
		require.Equal(t, "queued", r.Status)
		require.Nil(t, r.FinishedAt)
		require.True(t, time.Until(repo.next) > 44*time.Second)
		key := c.request.IdempotencyKey
		c.err = nil
		s.step(context.Background(), r)
		require.Equal(t, key, c.request.IdempotencyKey)
		require.Equal(t, 2, c.creates)
	})
	t.Run("expired remote task is an error", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.RemoteID = "remote-capability"
		c.err = &manxueHTTPError{Status: 404}
		s.step(context.Background(), r)
		require.Zero(t, c.creates)
		require.Equal(t, "error", r.Verdict)
	})
}

func TestModelQualityValidationCancellationAndVisibility(t *testing.T) {
	t.Run("wrong key owner and group never submit", func(t *testing.T) {
		for _, mutate := range []func(*APIKey){func(k *APIKey) { k.UserID = 999 }, func(k *APIKey) { other := int64(900); k.GroupID = &other }, func(k *APIKey) { k.Status = StatusAPIKeyDisabled }} {
			s, _, c, r := qualityFixture()
			mutate(s.keys.(qualityTestKeys).key)
			s.step(context.Background(), r)
			require.Zero(t, c.creates)
			require.Equal(t, "error", r.Verdict)
		}
	})
	t.Run("cancel queued and running", func(t *testing.T) {
		for _, remote := range []string{"", "remote-capability"} {
			s, _, c, r := qualityFixture()
			r.RemoteID = remote
			r.CancelRequested = true
			s.step(context.Background(), r)
			require.Zero(t, c.creates)
			require.Equal(t, "cancelled", r.Status)
			require.Equal(t, "unknown", r.Verdict)
			if remote != "" {
				require.Equal(t, 1, c.cancels)
			}
		}
	})
	t.Run("deleted key cannot leak through result text", func(t *testing.T) {
		s, _, c, r := qualityFixture()
		r.RemoteID = "remote-capability"
		s.keys = qualityTestKeys{}
		c.task = &ManxueTask{Status: "succeeded", Candy: &ManxueCandy{Status: "passed", Answer: "previously-deleted-secret"}}
		s.step(context.Background(), r)
		require.Equal(t, "normal", r.Verdict)
		require.Empty(t, r.Result.Answer)
	})
	t.Run("user list and detail respect group and publication", func(t *testing.T) {
		s, repo, _, run := qualityFixture()
		repo.recentStats = map[string]ModelQualityStats{
			"high": {Total: 10, Passed: 6, Failed: 1, Errors: 2, Unknown: 1},
			"low":  {Total: 2, Passed: 1, Errors: 1},
		}
		list, err := s.List(context.Background(), 42, false)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, ModelQualityStats{Total: 12, Passed: 7, Failed: 1, Errors: 3, Unknown: 1}, list[0].Stats24H)
		b, err := json.Marshal(list)
		require.NoError(t, err)
		require.NotContains(t, string(b), "base_url")
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
	})
}

func TestModelQualityVerdicts(t *testing.T) {
	for _, tc := range []struct{ name, benchmark, status, candy, quality, verdict string }{
		{"pass", "candy", "succeeded", "passed", "", "normal"},
		{"wrong answer", "candy", "succeeded", "incorrect", "", "degraded"},
		{"candy error", "candy", "succeeded", "error", "", "error"},
		{"future candy status", "candy", "succeeded", "future", "", "unknown"},
		{"task failure", "candy", "failed", "passed", "", "error"},
		{"task unknown", "candy", "future", "passed", "", "error"},
		{"normal art", "pelican", "succeeded", "", "normal", "normal"},
		{"degraded art", "pelican", "succeeded", "", "degraded", "degraded"},
		{"legacy art", "pelican", "succeeded", "", "suspicious", "degraded"},
		{"unknown art", "creative", "succeeded", "", "unknown", "unknown"},
		{"future assessment", "creative", "succeeded", "", "new-value", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, verdict, _, _ := interpretManxue(&ManxueTask{Status: tc.status, Candy: &ManxueCandy{Status: tc.candy}, Assessment: &ManxueAssessment{Quality: tc.quality}}, tc.benchmark)
			require.Equal(t, tc.verdict, verdict)
		})
	}
}

func TestModelQualityConfig(t *testing.T) {
	for _, base := range []string{"http://example.com/v1", "https://localhost/v1", "https://127.0.0.1/v1", "https://10.0.0.1/v1", "https://user:password@example.com/v1", "https://example.com/v1?key=secret"} {
		cfg := ModelQualityConfig{BaseURL: base, Benchmark: "candy", Model: "model", ReasoningEffort: "high"}
		require.Error(t, validateQualityConfig(&cfg), base)
	}
	cfg := ModelQualityConfig{BaseURL: "https://our.example/v1/", Benchmark: "pelican", Model: "other-model", ReasoningEffort: "high"}
	require.Error(t, validateQualityConfig(&cfg))
	cfg.Model = "gpt-6-astra"
	require.NoError(t, validateQualityConfig(&cfg))
	cfg.ReasoningEffort = "ultra"
	require.Error(t, validateQualityConfig(&cfg))
	cfg.Benchmark = "candy"
	require.NoError(t, validateQualityConfig(&cfg))
}

func TestManxueHTTPContract(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/tests", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "local-run", r.Header.Get("Idempotency-Key"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"id":"private-id","benchmark":"candy","status":"running"}`))
	}))
	defer server.Close()
	c := newManxueClient(server.URL, server.Client())
	task, err := c.Create(context.Background(), ManxueCreateRequest{Benchmark: "candy", BaseURL: "https://our.example/custom/v1", APIKey: "secret", Model: "our-model", IdempotencyKey: "local-run"})
	require.NoError(t, err)
	require.Equal(t, "private-id", task.ID)
	require.Equal(t, "https://our.example/custom/v1", received["base_url"])
	require.Equal(t, "secret", received["api_key"])
	require.NotContains(t, received, "protocol")
	require.NotContains(t, received, "IdempotencyKey")
}

func TestManxueHTTPPrivacyAndLimits(t *testing.T) {
	t.Run("redirect is never followed", func(t *testing.T) {
		calls := 0
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(202) }))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(307)
		}))
		defer source.Close()
		_, err := newManxueClient(source.URL, source.Client()).Create(context.Background(), ManxueCreateRequest{Benchmark: "candy", BaseURL: "https://our.example/v1", APIKey: "secret"})
		require.Error(t, err)
		require.Zero(t, calls)
	})
	t.Run("retry-after honored and error body hidden", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "45")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":"secret-credential"}`))
		}))
		defer server.Close()
		_, err := newManxueClient(server.URL, server.Client()).Get(context.Background(), "id")
		var e *manxueHTTPError
		require.ErrorAs(t, err, &e)
		require.Equal(t, 45*time.Second, e.RetryAfter)
		require.NotContains(t, err.Error(), "secret-credential")
	})
	t.Run("response size is bounded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 1025))) }))
		defer server.Close()
		c := newManxueClient(server.URL, server.Client()).(*manxueClient)
		c.maxBody = 1024
		_, err := c.Get(context.Background(), "id")
		require.ErrorContains(t, err, "too large")
	})
	t.Run("structured remote error still records a task failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"id","status":"failed","error":{"message":"upstream unavailable"}}`))
		}))
		defer server.Close()
		task, err := newManxueClient(server.URL, server.Client()).Get(context.Background(), "id")
		require.NoError(t, err)
		_, verdict, _, _ := interpretManxue(task, "candy")
		require.Equal(t, "error", verdict)
	})
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
		require.Equal(t, p.Configs[i].Benchmark, client.request.Benchmark)
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
