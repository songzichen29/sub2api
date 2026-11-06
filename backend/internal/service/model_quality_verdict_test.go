package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func qualityScoredAssessment(t *testing.T, benchmark string, confidence float64, scores map[string]int) string {
	t.Helper()
	ids := make([]string, 0, len(qualityRubric(benchmark)))
	for id := range qualityRubric(benchmark) {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	checks := make([]ModelQualityCheck, 0, len(ids))
	for _, id := range ids {
		score := 2
		if override, ok := scores[id]; ok {
			score = override
		}
		checks = append(checks, ModelQualityCheck{ID: id, Score: score, Evidence: "具体评分依据：" + id})
	}
	b, err := json.Marshal(map[string]any{"confidence": confidence, "reason": "逐项核对作品", "checks": checks})
	require.NoError(t, err)
	return string(b)
}

func TestQualityAssessmentCoreRequirementsCannotBeAveragedAway(t *testing.T) {
	// These are task requirements, independent of the scoring-policy implementation.
	for _, task := range []struct {
		benchmark string
		core      map[string]string
	}{
		{"candy", map[string]string{"answer": "最终答案", "sufficiency": "充分性证明", "minimality": "最小性证明"}},
		{"pelican", map[string]string{"subject": "鹈鹕主体", "bicycle": "自行车结构", "riding_pose": "骑行动作", "motion": "运动关系", "completeness": "作品完整性"}},
		{"creative", map[string]string{"subject": "主题与主体", "motion": "连续运动", "interaction": "暂停/继续功能", "completeness": "作品完整性"}},
	} {
		for id, label := range task.core {
			for _, score := range []int{0, 1, 2} {
				t.Run(fmt.Sprintf("%s/%s/%d", task.benchmark, id, score), func(t *testing.T) {
					text := qualityScoredAssessment(t, task.benchmark, 0.95, map[string]int{id: score})
					assessment, verdict, reason, err := parseQualityAssessment(text, task.benchmark, "test-judge")
					require.NoError(t, err)
					require.Equal(t, map[int]string{0: "degraded", 1: "unknown", 2: "normal"}[score], verdict)
					require.NotNil(t, assessment)
					if score != 2 {
						require.Contains(t, reason, label)
					}
				})
			}
		}
	}
}

func TestQualityAssessmentConfidenceAndComposition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		confidence float64
		scores     map[string]int
		verdict    string
		reason     string
	}{
		{"all supported", 0.8, nil, "normal", "核心要求均有充分证据"},
		{"uncertain composition only", 0.95, map[string]int{"composition": 1}, "normal", "核心要求均有充分证据"},
		{"composition defect alone", 0.95, map[string]int{"composition": 0}, "unknown", "场景构图"},
		{"core defect with composition defect", 0.95, map[string]int{"motion": 0, "composition": 0}, "degraded", "连续运动"},
		{"core defect with another uncertain core", 0.95, map[string]int{"motion": 0, "interaction": 1}, "degraded", "连续运动"},
		{"multiple uncertain cores", 0.95, map[string]int{"motion": 1, "interaction": 1}, "unknown", "连续运动、暂停/继续功能"},
		{"confidence below boundary with perfect scores", 0.7999, nil, "unknown", "裁判置信度不足"},
		{"confidence below boundary with a core defect", 0.7999, map[string]int{"motion": 0}, "unknown", "裁判置信度不足"},
		{"confidence at boundary with a core defect", 0.8, map[string]int{"motion": 0}, "degraded", "连续运动"},
		{"no confidence", 0, nil, "unknown", "裁判置信度不足"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, verdict, reason, err := parseQualityAssessment(qualityScoredAssessment(t, "creative", tc.confidence, tc.scores), "creative", "test-judge")
			require.NoError(t, err)
			require.Equal(t, tc.verdict, verdict)
			require.Contains(t, reason, tc.reason)
			require.Contains(t, reason, "源码分析（未运行浏览器）")
			require.Contains(t, reason, "仅反映本次测试")
		})
	}
}

func TestQualityAssessmentRejectsUnsupportedBenchmark(t *testing.T) {
	text := qualityScoredAssessment(t, "creative", 0.95, nil)
	for _, benchmark := range []string{"", "unknown", "Candy"} {
		assessment, verdict, _, err := parseQualityAssessment(text, benchmark, "test-judge")
		require.ErrorContains(t, err, "不支持的检测模式")
		require.Nil(t, assessment)
		require.Empty(t, verdict)
	}
}

func TestQualityAssessmentDecisionIsIndependentOfCheckOrder(t *testing.T) {
	text := qualityScoredAssessment(t, "creative", 0.95, map[string]int{"motion": 1, "interaction": 1})
	var raw struct {
		Confidence float64             `json:"confidence"`
		Reason     string              `json:"reason"`
		Checks     []ModelQualityCheck `json:"checks"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &raw))
	_, verdict, reason, err := parseQualityAssessment(text, "creative", "test-judge")
	require.NoError(t, err)
	for i, j := 0, len(raw.Checks)-1; i < j; i, j = i+1, j-1 {
		raw.Checks[i], raw.Checks[j] = raw.Checks[j], raw.Checks[i]
	}
	reversed, err := json.Marshal(raw)
	require.NoError(t, err)
	_, gotVerdict, gotReason, err := parseQualityAssessment(string(reversed), "creative", "test-judge")
	require.NoError(t, err)
	require.Equal(t, verdict, gotVerdict)
	require.Equal(t, reason, gotReason)
}

func TestQualityVerdictPolicyCoversRubric(t *testing.T) {
	for _, benchmark := range []string{"candy", "pelican", "creative"} {
		rubric := qualityRubric(benchmark)
		criteria := qualityVerdictCriteria(benchmark)
		require.Len(t, criteria, len(rubric))
		seen := make(map[string]bool, len(criteria))
		for _, criterion := range criteria {
			require.Contains(t, rubric, criterion.id)
			require.NotEmpty(t, criterion.label)
			require.False(t, seen[criterion.id], "duplicate policy criterion")
			seen[criterion.id] = true
		}
	}
}

func TestModelQualityVerdictUpgradePreservesArtifactsAndHistory(t *testing.T) {
	t.Run("saved artifact uses new policy without regenerating", func(t *testing.T) {
		s, repo, client, run := qualityFixture()
		repo.plan.Config.Benchmark, run.Target.Benchmark = "creative", "creative"
		run.Status = "running"
		run.Target.BenchmarkVersion = "native-v1"
		run.Result = &ModelQualityResult{HTML: "<html><body>saved work</body></html>", Source: "native-v1:unassessed", DurationMS: 123}
		client.judgment = qualityScoredAssessment(t, "creative", 0.95, map[string]int{"interaction": 1})
		s.step(context.Background(), run)
		require.Equal(t, "succeeded", run.Status)
		require.Equal(t, "unknown", run.Verdict)
		require.Contains(t, run.Reason, "暂停/继续功能")
		require.Equal(t, "native-v1:json-v2:verdict-v2", run.Result.Assessment.Version)
		require.Equal(t, "<html><body>saved work</body></html>", run.Result.HTML)
		require.EqualValues(t, 123, run.Result.DurationMS)
		require.Zero(t, client.creates)
		require.Equal(t, 1, client.gets)
		s.step(context.Background(), run)
		require.Equal(t, 1, client.gets, "an inconclusive result must not trigger another paid request")
	})
	t.Run("finished legacy result is never rescored", func(t *testing.T) {
		s, repo, client, run := qualityFixture()
		finished := time.Now()
		run.Status, run.Verdict, run.Reason = "succeeded", "normal", "historical verdict"
		run.FinishedAt = &finished
		run.Result = &ModelQualityResult{Assessment: &ModelQualityAssessment{Version: "native-v1:json-v2", Checks: []ModelQualityCheck{{ID: "motion", Score: 1}}}}
		before, err := json.Marshal(run)
		require.NoError(t, err)
		s.step(context.Background(), run)
		after, err := json.Marshal(run)
		require.NoError(t, err)
		require.Equal(t, string(before), string(after))
		require.Zero(t, client.creates+client.gets)
		require.Empty(t, repo.saved)
	})
}
