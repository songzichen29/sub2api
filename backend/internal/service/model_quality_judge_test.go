package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type qualityProbeFunc func(context.Context, qualityCredentials, qualityCompletionRequest) (*qualityCompletion, error)

func (f qualityProbeFunc) Complete(ctx context.Context, cred qualityCredentials, req qualityCompletionRequest) (*qualityCompletion, error) {
	return f(ctx, cred, req)
}

func qualityJudgeSaveFixture() (*ModelQualityService, *qualityTestRepo, *qualityTestClient, ModelQualityInput) {
	s, repo, client, _ := qualityFixture()
	s.monitors = qualityTestMonitors{&ChannelMonitor{ID: 8, Name: "channel", Provider: MonitorProviderOpenAI, Enabled: true, Endpoint: "https://relay.example", APIKey: "channel-only-key"}}
	in := ModelQualityInput{Name: "plan", GroupID: 12, JudgeModel: "custom-judge", IntervalMinutes: 3, Config: ModelQualityConfig{MonitorID: 8, Model: "test-model", Benchmark: "pelican"}}
	return s, repo, client, in
}

func TestModelQualitySaveTestsSelectedJudgeAndFreezesItForRun(t *testing.T) {
	s, repo, c, in := qualityJudgeSaveFixture()
	p, err := s.Save(context.Background(), "", 3, in)
	require.NoError(t, err)
	require.Same(t, p, repo.plan)
	require.Equal(t, "custom-judge", p.JudgeModel)
	require.Equal(t, 1, c.gets, "one probe, not a generated test or a retry")
	require.Zero(t, c.creates)
	require.Equal(t, "custom-judge", c.request.Model)
	require.Equal(t, "medium", c.request.Effort)
	require.Equal(t, qualityAssessmentSchema("candy"), c.request.Schema)
	require.Contains(t, c.request.Input, `"benchmark":"candy"`)
	require.NotContains(t, c.request.Input, "channel-only-key")
	require.Equal(t, qualityCredentials{s.judgeOrigin, "private-test-key"}, c.credential)

	s.judgeModel = "changed-global-default"
	runs, err := s.Queue(context.Background(), p.ID, false)
	require.NoError(t, err)
	require.Equal(t, "custom-judge", runs[0].Target.JudgeModel)
	repo.run = runs[0]
	c.result.Text = "<svg></svg>"
	s.step(context.Background(), repo.run)
	s.step(context.Background(), repo.run)
	require.Equal(t, "succeeded", repo.run.Status)
	require.Equal(t, "custom-judge", c.request.Model)
	require.Equal(t, "custom-judge", repo.run.Result.Assessment.Model)
}

func TestModelQualitySaveProbeFailureDoesNotPersist(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, failure := range []string{"http", "invalid-json", "nil", "network"} {
			t.Run(failure+map[bool]string{true: "/existing", false: "/new"}[existing], func(t *testing.T) {
				s, repo, _, in := qualityJudgeSaveFixture()
				old := repo.plan
				old.JudgeModel = "previous-judge"
				id := ""
				if existing {
					id = old.ID
				}
				calls := 0
				s.judge = qualityProbeFunc(func(context.Context, qualityCredentials, qualityCompletionRequest) (*qualityCompletion, error) {
					calls++
					switch failure {
					case "http":
						return nil, &qualityHTTPStatusError{StatusCode: 503}
					case "invalid-json":
						return &qualityCompletion{Text: `{"private-test-key":true}`}, nil
					case "network":
						return nil, errors.New("private-test-key upstream error")
					default:
						return nil, nil
					}
				})
				p, err := s.Save(context.Background(), id, 3, in)
				require.ErrorIs(t, err, ErrQualityInvalid)
				require.NotContains(t, err.Error(), "private-test-key")
				require.Nil(t, p)
				require.Same(t, old, repo.plan)
				require.Equal(t, "previous-judge", repo.plan.JudgeModel)
				require.Equal(t, 1, calls)
			})
		}
	}
}

func TestModelQualitySaveRejectsInvalidConfigurationBeforeProbe(t *testing.T) {
	for _, scenario := range []string{"ownership", "active", "invalid-model", "group", "generator"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, c, in := qualityJudgeSaveFixture()
			owner := int64(3)
			switch scenario {
			case "ownership":
				owner = 999
			case "active":
				repo.plan.ActiveRunID = "running"
			case "invalid-model":
				in.JudgeModel = "bad\x00model"
			case "group":
				in.GroupID = 99
			case "generator":
				s.monitors.(qualityTestMonitors).monitor.Enabled = false
			}
			_, err := s.Save(context.Background(), repo.plan.ID, owner, in)
			require.Error(t, err)
			require.Zero(t, c.gets)
		})
	}
}

func TestModelQualitySaveDisableDoesNotDependOnBrokenJudge(t *testing.T) {
	for _, changeModel := range []bool{false, true} {
		s, repo, c, in := qualityJudgeSaveFixture()
		repo.plan.Enabled = true
		repo.plan.Config = in.Config
		repo.plan.JudgeModel = in.JudgeModel
		s.keys.(qualityTestKeys).key.Status = "disabled"
		if changeModel {
			in.JudgeModel = "another-judge"
		}
		p, err := s.Save(context.Background(), repo.plan.ID, 3, in)
		if changeModel {
			require.Error(t, err)
			require.True(t, repo.plan.Enabled)
		} else {
			require.NoError(t, err)
			require.False(t, p.Enabled)
		}
		require.Zero(t, c.gets)
	}
}

func TestModelQualityLegacyJudgeFallbackAndValidation(t *testing.T) {
	s, repo, c, in := qualityJudgeSaveFixture()
	in.JudgeModel = ""
	s.judgeModel = "gpt-6.1-sol"
	p, err := s.Save(context.Background(), repo.plan.ID, 3, in)
	require.NoError(t, err)
	require.Equal(t, "gpt-6.1-sol", p.JudgeModel)
	require.Equal(t, p.JudgeModel, c.request.Model)
	p.JudgeModel = "saved-model"
	p, err = s.Save(context.Background(), p.ID, 3, in)
	require.NoError(t, err)
	require.Equal(t, "saved-model", p.JudgeModel, "old API clients must preserve a saved model")
	s.judgeModel = ""
	require.Equal(t, "gpt-6.1-sol", s.planJudgeModel(nil))
	for _, model := range []string{"", "a\nb", "a\x00b", strings.Repeat("a", 201)} {
		require.Error(t, validateQualityJudgeModel(model))
	}
	for _, model := range []string{"gpt-6.1-sol", "custom/model-v2", strings.Repeat("a", 200)} {
		require.NoError(t, validateQualityJudgeModel(model))
	}
}

func TestModelQualityProbeGuardsConcurrentSavesAndPropagatesCancellation(t *testing.T) {
	s, _, c, _ := qualityJudgeSaveFixture()
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.judge = qualityProbeFunc(func(ctx context.Context, _ qualityCredentials, _ qualityCompletionRequest) (*qualityCompletion, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Minute {
			return nil, errors.New("missing bounded deadline")
		}
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	done := make(chan error, 1)
	go func() { done <- s.probeQualityJudge(ctx, 3, "selected-model") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	err := s.probeQualityJudge(context.Background(), 3, "selected-model")
	require.ErrorContains(t, err, "正在测试")
	cancel()
	require.Error(t, <-done)
	s.judge = c
	require.NoError(t, s.probeQualityJudge(context.Background(), 3, "selected-model"), "guard released after canceled request")
}

func TestModelQualityProbeRejectsCredentialRotationBeforeSave(t *testing.T) {
	for _, field := range []string{"key", "group", "id", "status"} {
		t.Run(field, func(t *testing.T) {
			s, repo, c, in := qualityJudgeSaveFixture()
			old := repo.plan
			s.judge = qualityProbeFunc(func(ctx context.Context, cred qualityCredentials, req qualityCompletionRequest) (*qualityCompletion, error) {
				key := s.keys.(qualityTestKeys).key
				switch field {
				case "key":
					key.Key = "rotated-key"
				case "group":
					*key.GroupID = 99
				case "id":
					key.ID = 77
				case "status":
					key.Status = "disabled"
				}
				return c.Complete(ctx, cred, req)
			})
			_, err := s.Save(context.Background(), "", 3, in)
			require.ErrorContains(t, err, "测试期间裁判 Key 配置已变化")
			require.Same(t, old, repo.plan)
		})
	}
}

func TestModelQualityCanceledSaveDoesNotCallJudge(t *testing.T) {
	s, repo, c, in := qualityJudgeSaveFixture()
	old := repo.plan
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Save(ctx, "", 3, in)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, c.gets)
	require.Same(t, old, repo.plan)
}

type qualityOptionGroups struct{ qualityTestGroups }

func (qualityOptionGroups) ListActive(context.Context) ([]Group, error) {
	return []Group{{ID: 12, Name: "group", Status: StatusActive}}, nil
}

type qualitySuggestionClient struct {
	*qualityTestClient
	listCalls  int
	credential qualityCredentials
	deadline   time.Time
	listErr    error
}

func (c *qualitySuggestionClient) Models(ctx context.Context, cred qualityCredentials) ([]string, error) {
	c.listCalls++
	c.credential = cred
	c.deadline, _ = ctx.Deadline()
	return []string{"suggested-model"}, c.listErr
}

func TestModelQualitySuggestionsNeverReplacePaidProbe(t *testing.T) {
	for _, scenario := range []string{"listed", "list-failed", "missing-key"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, c, in := qualityJudgeSaveFixture()
			s.groups = qualityOptionGroups{}
			s.judgeModel = "gpt-6.1-sol"
			client := &qualitySuggestionClient{qualityTestClient: c}
			s.judge = client
			if scenario == "list-failed" {
				client.listErr = errors.New("private-test-key")
			} else if scenario == "missing-key" {
				s.keys.(qualityTestKeys).key.Name = "another-key"
			}
			options, err := s.Options(context.Background(), 3)
			require.NoError(t, err)
			require.Equal(t, "gpt-6.1-sol", options.DefaultJudgeModel)
			require.Zero(t, c.gets, "listing is read-only, never a probe")
			require.NotContains(t, options.JudgeModelsWarning, "private-test-key")
			if scenario == "missing-key" {
				require.Zero(t, client.listCalls)
				require.NotEmpty(t, options.JudgeModelsWarning)
				return
			}
			require.Equal(t, 1, client.listCalls)
			require.Equal(t, qualityCredentials{s.judgeOrigin, "private-test-key"}, client.credential)
			require.WithinDuration(t, time.Now().Add(8*time.Second), client.deadline, time.Second)
			if scenario == "listed" {
				require.Equal(t, []string{"suggested-model"}, options.JudgeModels)
			} else {
				require.NotEmpty(t, options.JudgeModelsWarning)
			}
			// A custom model not listed by /models can still be usable.
			p, err := s.Save(context.Background(), "", 3, in)
			require.NoError(t, err)
			require.Equal(t, "custom-judge", p.JudgeModel)
			require.Equal(t, 1, c.gets)
		})
	}
}
