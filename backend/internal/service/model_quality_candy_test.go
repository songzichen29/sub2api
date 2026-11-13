package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQualityCandyAnswerOnly(t *testing.T) {
	for _, tc := range []struct {
		name, answer, verdict string
	}{
		{"bare answer", "21", "normal"},
		{"no proof", "最终答案是21。", "normal"},
		{"markdown", "**最少取21个。**", "normal"},
		{"inline math", `**最少取 \(21\) 个。**`, "normal"},
		{"display math", "最终答案为\n\\[\n\\boxed{21\\text{ 个}}。\n\\]", "normal"},
		{"boxed", `\(\boxed{21}\) 个。`, "normal"},
		{"Chinese number", "答案：二十一个糖果。", "normal"},
		{"full width", "答案为２１颗糖果。", "normal"},
		{"English", "The final answer is 21.", "normal"},
		{"wrong final", "推导中试过20和21。最终答案为22。", "degraded"},
		{"wrong Chinese", "最少取二十个。", "degraded"},
		{"larger integer", "答案是121个。", "degraded"},
		{"negative statement", "答案不是21。", "unknown"},
		{"intermediate number", "推导过程\n21\n继续分析", "unknown"},
		{"reasoning only", "考虑21颗的情形，但尚未确定最终答案。", "unknown"},
		{"wrong quoted answer", "有人说答案是21，但我没有确认。", "unknown"},
		{"conflicting conclusions", "答案是21。答案是22。", "unknown"},
		{"final overrides attempt", "至少取20个。最终答案是21。", "normal"},
		{"decimal", "答案为21.5。", "unknown"},
		{"invalid final overrides earlier answer", "至少取21个。最终答案为22.5。", "unknown"},
		{"ambiguous final overrides earlier answer", "答案是21。最终答案为21或22。", "unknown"},
		{"comma alternatives", "最终答案是21，或22。", "unknown"},
		{"negative final overrides earlier answer", "答案是21。最终答案不是21。", "unknown"},
		{"unfinished final header", "最终答案是21。最终答案为", "unknown"},
		{"invalid continued final", "最终答案是21。最终答案为\n21或22。", "unknown"},
		{"multiline final overrides attempt", "至少取20个。最终答案为\n\\[\n\\boxed{21\\text{ 个}}。\n\\]", "normal"},
		{"alternatives", "答案为21或22。", "unknown"},
		{"minimality proof heading", "答案：21个。\n最少性证明\n20个不能保证。", "normal"},
		{"answer proof heading", "最终答案是21。\n最终答案的证明：\n略。", "normal"},
		{"quantity conclusion", "最少取21个。故最少取出的数量为22个。", "degraded"},
		{"total conclusion", "最少取21个。所以最少需要取出的总数是22个。", "degraded"},
		{"minimum quantity", "最少取21个。因此，最少数量是22个。", "degraded"},
		{"requested minimum quantity", "最少取21个。因此，所求最小数量为22个。", "degraded"},
		{"quantity multiline", "最少取21个。故最少取出的数量为\n\\[\n\\boxed{22}\n\\]", "degraded"},
		{"escaped math", `**最少取 \\(21\\) 个。**`, "normal"},
		{"empty", "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &ModelQualityResult{Answer: tc.answer}
			verdict, reason := assessQualityCandy(result)
			require.Equal(t, tc.verdict, verdict)
			require.NotEmpty(t, reason)
			require.Equal(t, "answer_exact", result.Assessment.Basis)
			require.Empty(t, result.Assessment.Model)
			require.Len(t, result.Assessment.Checks, 1)
			require.Equal(t, "answer", result.Assessment.Checks[0].ID)
		})
	}
}

func TestModelQualityCandyNeverUsesJudge(t *testing.T) {
	s, repo, client, input := qualityJudgeSaveFixture()
	input.Config.Benchmark = "candy"
	input.JudgeModel = "invalid\x00judge"
	s.keys = qualityTestKeys{}
	s.judge = qualityProbeFunc(func(context.Context, qualityCredentials, qualityCompletionRequest) (*qualityCompletion, error) {
		t.Fatal("candy must not probe or call a judge")
		return nil, nil
	})
	plan, err := s.Save(context.Background(), "", 3, input)
	require.NoError(t, err)
	require.Empty(t, plan.JudgeModel)
	runs, err := s.Queue(context.Background(), plan.ID, false)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	run := runs[0]
	repo.run = run
	require.Zero(t, run.JudgeKeyID)
	require.Empty(t, run.Target.JudgeModel)
	client.result.Text = "答案是21。证明省略。"
	s.step(context.Background(), run)
	require.Equal(t, "succeeded", run.Status)
	require.Equal(t, "normal", run.Verdict)
	require.Equal(t, 1, client.creates)
	require.Zero(t, client.gets)
	require.Nil(t, run.Result.JudgeInputTokens)
	require.Nil(t, run.Result.JudgeOutputTokens)
	s.step(context.Background(), run)
	require.Equal(t, 1, client.creates)
}

func TestModelQualityCandySavedAnswerAndHistory(t *testing.T) {
	for _, finished := range []bool{false, true} {
		s, _, client, run := qualityFixture()
		s.keys, s.groups, s.channels, s.monitors = nil, nil, nil, nil
		run.Result = &ModelQualityResult{Answer: "最终答案21。", DurationMS: 123}
		run.Status, run.Phase = "running", "classifying"
		run.Target.JudgeModel = "invalid\x00old-judge"
		if finished {
			now := time.Now()
			run.FinishedAt = &now
			run.Status, run.Verdict, run.Reason = "failed", "error", "historical judge error"
		}
		s.step(context.Background(), run)
		require.Zero(t, client.creates+client.gets)
		require.EqualValues(t, 123, run.Result.DurationMS)
		if finished {
			require.Equal(t, "error", run.Verdict)
			require.Equal(t, "historical judge error", run.Reason)
		} else {
			require.Equal(t, "normal", run.Verdict)
			require.Equal(t, "succeeded", run.Status)
		}
	}
}

func TestModelQualityMixedPlanKeepsJudgeOnlyForArtwork(t *testing.T) {
	s, _, client, input := qualityJudgeSaveFixture()
	candy := input.Config
	candy.Benchmark = "candy"
	input.Configs = []ModelQualityConfig{candy, input.Config}
	plan, err := s.Save(context.Background(), "", 3, input)
	require.NoError(t, err)
	require.Equal(t, 1, client.gets)
	require.Contains(t, client.request.Input, `"benchmark":"pelican"`)
	runs, err := s.Queue(context.Background(), plan.ID, false)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Zero(t, runs[0].JudgeKeyID)
	require.Empty(t, runs[0].Target.JudgeModel)
	require.NotZero(t, runs[1].JudgeKeyID)
	require.Equal(t, "custom-judge", runs[1].Target.JudgeModel)
	// 画作仍保留既有裁判授权限制。
	s.keys.(qualityTestKeys).key.Status = "disabled"
	_, err = s.Queue(context.Background(), plan.ID, false)
	require.Error(t, err)
}
