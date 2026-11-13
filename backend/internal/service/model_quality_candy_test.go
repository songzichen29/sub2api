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
		{"nested final answer", "**最终答案：最少取 21 个。**", "normal"},
		{"parenthesized boxed conclusion", `综上，最少需要取 **(\boxed{21}) 个**。`, "normal"},
		{"Chinese parenthesized boxed conclusion", `综上，最少需要取 **（\boxed{21}） 个**。`, "normal"},
		{"nested wrong answer", "最终答案：最少取22个。", "degraded"},
		{"nested wrong boxed answer", `最终答案：最少取（\boxed{22}）个。`, "degraded"},
		{"nested conflicting conclusions", "最终答案：最少取21个。最终答案：最少取22个。", "unknown"},
		{"nested conflicting boxed conclusions", `最终答案：最少取21个。综上，最少取(\boxed{22})个。`, "unknown"},
		{"nested decimal", "最终答案：最少取21.5个。", "unknown"},
		{"nested boxed decimal", `最终答案：最少取(\boxed{21.5})个。`, "unknown"},
		{"nested negative declaration", "最终答案：最少不是21个。", "unknown"},
		{"parenthesized negative number", "最终答案：最少取(-21)个。", "unknown"},
		{"nested alternatives", "最终答案：最少取21，或22个。", "unknown"},
		{"mismatched parentheses", "最终答案：最少取(21）个。", "unknown"},
		{"parenthesized expression", "最终答案：最少取(20+1)个。", "unknown"},
		{"adjacent parenthesized digit", "最终答案：最少取2(1)个。", "unknown"},
		{"parenthesized derivation only", `证明中有 (\boxed{21})，尚未确认答案。`, "unknown"},
		{"shape lower bounds do not replace total", "最少需要取出21个。至少取5个五角星形，才能配对；至少取9个圆形，才能配对。", "normal"},
		{"shape lower bounds do not fix wrong total", "最少需要取出20个。至少取5个五角星形，才能配对。", "degraded"},
		{"shape lower bound alone", "至少取21个圆形，才能配对。", "unknown"},
		{"conflicting total lower bound", "最少取21个。至少取22个。", "unknown"},
		{"concluding circle count remains ambiguous", "最少取21个。综上，至少取22个圆形。", "unknown"},
		{"concluding star count remains ambiguous", "最少取21个。因此，至少取22个五角星形。", "unknown"},
		{"explicit total shape count remains ambiguous", "最少取21个。至少取出的总数为22个圆形。", "unknown"},
		{"explicit quantity shape count remains ambiguous", "最少取21个。至少取出的数量为22个五角星形。", "unknown"},
		{"explicit final shape count remains ambiguous", "最少取21个。最终答案：至少取5个五角星形。", "unknown"},
		{"shape counts do not hide invalid final", "最少取21个。至少取5个五角星形，才能配对。最终答案为22.5个。", "unknown"},
		{"inline proof sum preserves answer", "最终答案：最少取21个。因此至少需要16+5=21个。", "normal"},
		{"multiline proof sum preserves answer", "答案：最少取21个。因此至少要取\n9+12=21个。", "normal"},
		{"wrong proof sum does not grade proof", "最终答案：最少取21个。因此至少需要16+5=22个。", "normal"},
		{"proof sum does not fix wrong answer", "因此至少需要16+5=21个。最终答案是22个。", "degraded"},
		{"proof sum does not hide conflicting final", "最终答案是21个。因此至少需要16+5=21个。最终答案是22个。", "unknown"},
		{"inline proof sum alone", "因此至少需要16+5=21个。", "unknown"},
		{"multiline proof sum alone", "因此至少要取\n9+12=21个。", "unknown"},
		{"final sum remains ambiguous", "最终答案是21个。最终答案为16+5=21个。", "unknown"},
		{"nested final sum remains ambiguous", "最终答案是21个。最终答案：至少需要16+5=21个。", "unknown"},
		{"summary sum remains ambiguous", "最终答案是21个。综上，至少需要16+5=22个。", "unknown"},
		{"multiline summary sum remains ambiguous", "最终答案是21个。综上，至少要取\n16+5=22个。", "unknown"},
		{"proof sum exception preserves numeric conflict", "最终答案是21个。因此至少需要22个。", "unknown"},
		{"proof sum alternatives remain ambiguous", "最终答案是21个。因此至少需要16+5=21或22个。", "unknown"},
		{"explicit quantity sum remains ambiguous", "最终答案是21个。因此至少取出的数量为16+5=21个。", "unknown"},
		{"explicit total multiline sum remains ambiguous", "最终答案是21个。因此至少取出的总数为\n16+5=21个。", "unknown"},
		{"final header proof sum remains ambiguous", "最少取21个。最终答案：\n因此至少需要16+5=22个。综上，最少取21个。", "unknown"},
		{"final header multiline proof sum remains ambiguous", "最少取21个。最终答案：\n因此至少要取\n16+5=22个。综上，最少取21个。", "unknown"},
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

func TestQualityCandyReportedNestedAnswer(t *testing.T) {
	result := &ModelQualityResult{Answer: `**最终答案：最少取 21 个。**

### 一、取 21 个足够

选择取 **12 个五角星形、9 个圆形**。

- 五角星形中，非苹果味只有 \(6+4=10\) 个，非桃子味只有 \(7+4=11\) 个。因此，取12个五角星形，**必定同时有苹果味和桃子味**。
- 圆形中，西瓜味只有8个。因此，取9个圆形，**必定有苹果味或桃子味**。

若圆形中有苹果味，就与五角星形桃子味配对；若圆形中有桃子味，就与五角星形苹果味配对。因此21个一定满足要求。

### 二、取 20 个不能保证

考虑一种可能的取出顺序：每次选定形状后，该形状的糖果依次按以下顺序出现：

- **圆形：**8个西瓜味 → 7个苹果味 → 9个桃子味；
- **五角星形：**4个西瓜味 → 7个苹果味 → 6个桃子味。

这完全符合袋中各类糖果的数量。在这种情况下：

- 要得到“圆形苹果味＋五角星形桃子味”，至少须取到第9个圆形和第12个五角星形，共需
  \[
  9+12=21\text{ 个}。
  \]
- 要得到“圆形桃子味＋五角星形苹果味”，至少须取到第16个圆形和第5个五角星形，共需
  \[
  16+5=21\text{ 个}。
  \]

所以，无论怎样分配两种形状的取出数量，**即使允许根据已取出的口味随时调整下一次选择的形状，取20个仍可能无法满足要求**。

综上，最少需要取 **\(\boxed{21}\) 个**。`}
	verdict, _ := assessQualityCandy(result)
	require.Equal(t, "normal", verdict)
	require.Equal(t, "answer_exact", result.Assessment.Basis)
	require.Empty(t, result.Assessment.Model)
}

func TestQualityCandyReportedShapeCounts(t *testing.T) {
	result := &ModelQualityResult{Answer: `**最少需要取出 \(21\) 个。**

**一、取 \(21\) 个足够**

选择取出 **\(12\) 个五角星形糖果和 \(9\) 个圆形糖果**。

- 五角星形中，非苹果味只有 \(6+4=10\) 个，所以取出 \(12\) 个一定有苹果味。
- 五角星形中，非桃子味只有 \(7+4=11\) 个，所以取出 \(12\) 个一定有桃子味。
- 圆形中，西瓜味只有 \(8\) 个，所以取出 \(9\) 个一定有苹果味或桃子味。

因此，若圆形中有苹果味，就与五角星形中的桃子味配对；若圆形中有桃子味，就与五角星形中的苹果味配对。无论哪种情况，都符合要求。

**二、取 \(20\) 个不能保证**

考虑一种合法的最坏取出顺序：每种形状内部，都先取到西瓜味，再取到苹果味，最后才取到桃子味。具体为：

| 形状 | 先取到 | 接着取到 | 最后取到 |
|---|---|---|---|
| 圆形 | \(8\) 个西瓜味 | \(7\) 个苹果味 | \(9\) 个桃子味 |
| 五角星形 | \(4\) 个西瓜味 | \(7\) 个苹果味 | \(6\) 个桃子味 |

由于不能通过手感辨别口味，这种情况是可能发生的。即使允许根据已经取出的口味随时调整下一颗所选的形状，也无法排除它。

在这种情况下，要得到符合要求的一对，只有两种可能：

- **圆形桃子味＋五角星形苹果味**：至少取 \(16\) 个圆形，才能出现圆形桃子味；至少取 \(5\) 个五角星形，才能出现五角星形苹果味。合计至少 \(16+5=21\) 个。
- **五角星形桃子味＋圆形苹果味**：至少取 \(12\) 个五角星形，才能出现五角星形桃子味；至少取 \(9\) 个圆形，才能出现圆形苹果味。合计至少 \(12+9=21\) 个。

所以，无论怎样选择形状，取出 \(20\) 个都可能无法得到目标组合。结合上述取法，最少数量为 **\(\boxed{21}\) 个**。`}
	verdict, _ := assessQualityCandy(result)
	require.Equal(t, "normal", verdict)
	require.Equal(t, "answer_exact", result.Assessment.Basis)
	require.Empty(t, result.Assessment.Model)
}

func TestQualityCandyReportedProofSums(t *testing.T) {
	for _, tc := range []struct{ name, answer string }{
		{"cde59e4e-44b4-476d-81db-b4b9913abd14", `**答案：最少取 21 个。**

### 一、21 个足够

取 **9 个圆形糖果、12 个五角星形糖果**。

- 圆形西瓜味只有 8 个，所以 9 个圆形中，必有苹果味或桃子味。
- 五角星形中，非苹果味只有 \(6+4=10\) 个，非桃子味只有 \(7+4=11\) 个。因此，12 个五角星形中，**苹果味、桃子味一定都有**。

所以，无论圆形中取到的是苹果味还是桃子味，都能与五角星形中另一种口味的糖果配成所需的一对。

### 二、20 个不能保证

考虑一种最不利但完全符合数量条件的取出顺序：

| 形状 | 该形状糖果的取出顺序 |
|---|---|
| 圆形 | 先 8 个西瓜味，再 7 个苹果味，最后 9 个桃子味 |
| 五角星形 | 先 4 个西瓜味，再 7 个苹果味，最后 6 个桃子味 |

所需的搭配只有两种：

1. **圆形苹果味＋五角星形桃子味**` + "  " + `
   圆形苹果味最早出现在第 9 个圆形，五角星形桃子味最早出现在第 12 个五角星形。因此至少要取
   \[
   9+12=21\text{ 个}.
   \]

2. **圆形桃子味＋五角星形苹果味**` + "  " + `
   圆形桃子味最早出现在第 16 个圆形，五角星形苹果味最早出现在第 5 个五角星形。因此至少要取
   \[
   16+5=21\text{ 个}.
   \]

在这种可能的情况下，**总共只取 20 个，无论怎样选择形状，都无法得到所需搭配**。即使允许看到已取糖果的口味后再调整形状选择，也不能保证成功。

综上，最少需要 **\(\boxed{21}\) 个**。`},
		{"e595b204-d6cc-4748-97e5-0ba1a1795161", `**最终答案：最少取 21 个。**

### 一、21 个足够

选择取 **9 个圆形糖果、12 个五角星形糖果**。

- 五角星形中，非苹果味只有 \(6+4=10\) 个，所以取 12 个必有苹果味；非桃子味只有 \(7+4=11\) 个，所以也必有桃子味。
- 圆形中，西瓜味只有 8 个，所以取 9 个，必有至少一个苹果味或桃子味。

因此：
- 若圆形中有苹果味，就与五角星形中的桃子味配对；
- 若圆形中有桃子味，就与五角星形中的苹果味配对。

两颗口味分别为苹果、桃子，且形状不同，故 **21 个一定足够**。

### 二、20 个不能保证

考虑一种最不利但完全符合数量的取出顺序：每种形状都先取到西瓜味，再取到苹果味，最后才取到桃子味。

| 形状 | 该形状的取出顺序 |
|---|---|
| 圆形 | 8 个西瓜 → 7 个苹果 → 9 个桃子 |
| 五角星形 | 4 个西瓜 → 7 个苹果 → 6 个桃子 |

在这种情况下，要出现符合要求的一对，只有两种可能：

1. **圆形桃子＋五角星形苹果**` + "  " + `
   圆形桃子最早是第 \(8+7+1=16\) 个圆形糖果；五角星形苹果最早是第 \(4+1=5\) 个五角星形糖果。` + "  " + `
   因此至少需要 \(16+5=21\) 个。

2. **圆形苹果＋五角星形桃子**` + "  " + `
   圆形苹果最早是第 \(8+1=9\) 个圆形糖果；五角星形桃子最早是第 \(4+7+1=12\) 个五角星形糖果。` + "  " + `
   因此至少需要 \(9+12=21\) 个。

所以在这种可能的取出顺序下，**无论怎样选择形状，甚至根据已取出的口味调整选择，取 20 个都无法成功**。

综上，最少需要 **\(\boxed{21}\) 个**。`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &ModelQualityResult{Answer: tc.answer}
			verdict, _ := assessQualityCandy(result)
			require.Equal(t, "normal", verdict)
			require.Equal(t, "answer_exact", result.Assessment.Basis)
			require.Empty(t, result.Assessment.Model)
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
