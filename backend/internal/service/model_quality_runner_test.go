package service

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestModelQualityDefaultJudgeIsSol(t *testing.T) {
	for _, cfg := range []*config.Config{nil, {}} {
		s := NewModelQualityService(nil, nil, nil, nil, nil, nil, cfg)
		require.Equal(t, "gpt-6.1-sol", s.judgeModel)
	}
	s, repo, _, _ := qualityFixture()
	s.judgeModel = NewModelQualityService(nil, nil, nil, nil, nil, nil, nil).judgeModel
	runs, err := s.Queue(context.Background(), repo.plan.ID, false)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "gpt-6.1-sol", runs[0].Target.JudgeModel)
}

func TestModelQualityNativeAllModes(t *testing.T) {
	for _, mode := range []string{"candy", "pelican", "creative"} {
		t.Run(mode, func(t *testing.T) {
			s, _, c, r := qualityFixture()
			s.repo.(*qualityTestRepo).plan.Config.Benchmark, r.Target.Benchmark = mode, mode
			if mode != "candy" {
				c.result.Text = "```html\n<html><svg></svg></html>\n```"
			}
			s.step(context.Background(), r)
			require.Equal(t, qualityPrompt(mode), c.request.Input)
			s.step(context.Background(), r)
			require.Equal(t, "normal", r.Verdict)
			require.Len(t, r.Result.Assessment.Checks, len(qualityRubric(mode)))
			if mode != "candy" {
				require.Equal(t, "<html><svg></svg></html>", r.Result.HTML)
				require.Contains(t, r.Reason, "未运行浏览器")
			}
			s.step(context.Background(), r)
			require.Equal(t, 1, c.creates, "finished work must never regenerate")
			require.Equal(t, 1, c.gets, "finished work must never rejudge")
		})
	}
}

func TestModelQualityNativeBoundMonitor(t *testing.T) {
	s, repo, c, _ := qualityFixture()
	s.monitors = qualityTestMonitors{&ChannelMonitor{ID: 8, Name: "channel", Provider: MonitorProviderOpenAI, Enabled: true, Endpoint: "https://relay.example", APIKey: "channel-only-key"}}
	in := ModelQualityInput{Name: "native", GroupID: 12, IntervalMinutes: 3, Config: ModelQualityConfig{MonitorID: 8, Model: "custom-model", Benchmark: "pelican", ReasoningEffort: "max", BaseURL: "https://discard.example", APIKeyID: 100}}
	p, err := s.Save(context.Background(), "", 3, in)
	require.NoError(t, err)
	require.Empty(t, p.Config.BaseURL)
	require.Zero(t, p.Config.APIKeyID)
	runs, err := s.Queue(context.Background(), p.ID, false)
	require.NoError(t, err)
	repo.run = runs[0]
	c.result.Text = "<html><svg></svg></html>"
	s.step(context.Background(), repo.run)
	require.Equal(t, qualityCredentials{"https://relay.example/v1", "channel-only-key"}, c.credential)
	require.Equal(t, "max", c.request.Effort)
	s.step(context.Background(), repo.run)
	require.Equal(t, qualityCredentials{"http://127.0.0.1:8080/v1", "private-test-key"}, c.credential)
	require.NotContains(t, c.request.Input, "channel-only-key")
	stored, err := json.Marshal(p)
	require.NoError(t, err)
	require.NotContains(t, string(stored), "channel-only-key")
	wrongGroup := int64(99)
	s.keys.(qualityTestKeys).key.GroupID = &wrongGroup
	_, err = s.Queue(context.Background(), p.ID, false)
	require.ErrorContains(t, err, "分组不一致")
}

type qualityNamedKeys struct {
	qualityTestKeys
	listed []APIKey
}

func (k qualityNamedKeys) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return k.listed, &pagination.PaginationResult{Total: int64(len(k.listed))}, nil
}

func TestModelQualityJudgeKeyResolution(t *testing.T) {
	s, _, _, _ := qualityFixture()
	keys := s.keys.(qualityTestKeys)
	s.keys = qualityNamedKeys{qualityTestKeys: keys, listed: []APIKey{{ID: 1, Name: "测试降智副本"}}}
	_, err := s.qualityJudgeKey(context.Background(), 3, 0)
	require.ErrorContains(t, err, "配置名为")
	s.keys = qualityNamedKeys{qualityTestKeys: keys, listed: []APIKey{{ID: 9, Name: "测试降智"}, {ID: 10, Name: "测试降智"}}}
	_, err = s.qualityJudgeKey(context.Background(), 3, 0)
	require.ErrorContains(t, err, "不唯一")
	s.keys = keys
	_, err = s.qualityJudgeKey(context.Background(), 999, 9)
	require.Error(t, err)
	expired := time.Now().Add(-time.Hour)
	keys.key.ExpiresAt = &expired
	_, err = s.qualityJudgeKey(context.Background(), 3, 9)
	require.Error(t, err)
	keys.key.ExpiresAt = nil
	s.judgeModel = "unapproved-model"
	_, err = s.qualityJudgeKey(context.Background(), 3, 9)
	require.Error(t, err)
	s.judgeModel = "gpt-6.1-sol"
	_, err = s.qualityJudgeKey(context.Background(), 3, 9)
	require.NoError(t, err)
}

func TestModelQualityJudgeHTTPFailureIsNotDegradation(t *testing.T) {
	s, _, c, r := qualityFixture()
	s.step(context.Background(), r)
	c.err = &qualityHTTPStatusError{StatusCode: 502}
	s.step(context.Background(), r)
	require.Equal(t, "error", r.Verdict)
	require.Contains(t, r.Reason, "裁判请求失败")
	require.Contains(t, r.Reason, "HTTP 502")
	require.NotEmpty(t, r.Result.Answer)
}

func TestModelQualityJudgeEvidenceIsRedacted(t *testing.T) {
	s, _, c, r := qualityFixture()
	s.step(context.Background(), r)
	c.judgment = `{"confidence":0.95,"reason":"private-test-key","checks":[{"id":"answer","score":2,"evidence":"private-test-key"},{"id":"sufficiency","score":2,"evidence":"https://our.example/custom/v1"},{"id":"minimality","score":2,"evidence":"http://127.0.0.1:8080/v1"}]}`
	s.step(context.Background(), r)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "normal", r.Verdict)
	for _, secret := range []string{"private-test-key", "https://our.example", "http://127.0.0.1"} {
		require.NotContains(t, string(b), secret)
	}
}

type qualityCancellationRepo struct {
	*qualityTestRepo
	cancelRequested atomic.Bool
	lostLease       bool
}

func (r *qualityCancellationRepo) GetRun(context.Context, string) (*ModelQualityRun, error) {
	token := ""
	if r.lostLease {
		token = "other-worker"
	}
	return &ModelQualityRun{ID: "run", CancelRequested: r.cancelRequested.Load(), LeaseToken: token}, nil
}

type qualityBlockingClient struct{ started func() }

func (c qualityBlockingClient) Complete(ctx context.Context, _ qualityCredentials, _ qualityCompletionRequest) (*qualityCompletion, error) {
	c.started()
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestModelQualityInFlightCancellationAndLeaseLoss(t *testing.T) {
	for _, lostLease := range []bool{false, true} {
		s, repo, _, r := qualityFixture()
		guard := &qualityCancellationRepo{qualityTestRepo: repo, lostLease: lostLease}
		s.repo = guard
		s.generator = qualityBlockingClient{started: func() { guard.cancelRequested.Store(!lostLease) }}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.step(ctx, r)
		cancel()
		if lostLease {
			require.Nil(t, r.FinishedAt, "a fenced worker must not overwrite another worker")
			require.Len(t, repo.saved, 1)
		} else {
			require.Equal(t, "cancelled", r.Status)
			require.Equal(t, "unknown", r.Verdict)
		}
	}
}
