package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const qualityValidCandyAssessment = `{"confidence":0.95,"reason":"答案与两项证明完整","checks":[{"id":"answer","score":2,"evidence":"结论21"},{"id":"sufficiency","score":2,"evidence":"9圆和12星保证成功"},{"id":"minimality","score":2,"evidence":"20颗存在合法反例"}]}`

func TestQualityAssessmentLosslessFormatting(t *testing.T) {
	withMetadata := strings.Replace(qualityValidCandyAssessment, `"confidence":`, `"account_id":4589,"account_available":false,"verdict":"degraded","unused_secret":"do-not-persist","confidence":`, 1)
	withMetadata = strings.Replace(withMetadata, `"id":"answer"`, `"provider_note":"extra","id":"answer"`, 1)
	for _, text := range []string{
		qualityValidCandyAssessment,
		" \uFEFF\n" + qualityValidCandyAssessment + "\n",
		"```json\n" + qualityValidCandyAssessment + "\n```",
		"```JSON\r\n" + qualityValidCandyAssessment + "\r\n```",
		"```\n" + qualityValidCandyAssessment + "\n```",
		withMetadata,
	} {
		out, verdict, reason, err := parseQualityAssessment(text, "candy", "gpt-6.1-sol")
		require.NoError(t, err)
		require.Equal(t, "normal", verdict)
		require.Equal(t, "gpt-6.1-sol", out.Model)
		require.Equal(t, qualityBenchmarkVersion+":"+qualityJudgeFormatVersion+":"+qualityVerdictVersion, out.Version)
		stored, err := json.Marshal(out)
		require.NoError(t, err)
		for _, extra := range []string{"account_id", "account_available", "provider_note", "do-not-persist"} {
			require.NotContains(t, string(stored)+reason, extra)
		}
	}
	// Presentation compatibility never changes the original verdict rules.
	for _, tc := range []struct{ text, verdict string }{
		{strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":0`, 1), "degraded"},
		{strings.Replace(qualityValidCandyAssessment, `"confidence":0.95`, `"confidence":0.5`, 1), "unknown"},
	} {
		_, verdict, _, err := parseQualityAssessment("```json\n"+tc.text+"\n```", "candy", "gpt-6.1-sol")
		require.NoError(t, err)
		require.Equal(t, tc.verdict, verdict)
	}
}

func TestQualityAssessmentRejectsAmbiguousOrMissingScores(t *testing.T) {
	for _, tc := range []struct{ name, text, diagnostic string }{
		{"prose", "结果正常：" + qualityValidCandyAssessment, "单个完整 JSON"},
		{"trailing prose", qualityValidCandyAssessment + "这是评分", "单个完整 JSON"},
		{"two objects", qualityValidCandyAssessment + qualityValidCandyAssessment, "单个完整 JSON"},
		{"incomplete fence", "```json\n" + qualityValidCandyAssessment, "代码块不完整"},
		{"wrong fence", "```html\n" + qualityValidCandyAssessment + "\n```", "不是 JSON"},
		{"multiple fences", "```json\n" + qualityValidCandyAssessment + "\n```\n```json\n{}\n```", "单个完整 JSON"},
		{"duplicate score", strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":0,"score":2`, 1), "重复字段"},
		{"case duplicate score", strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":0,"Score":2`, 1), "重复字段"},
		{"escaped duplicate score", strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":0,"\u0073core":2`, 1), "重复字段"},
		{"missing score", strings.Replace(qualityValidCandyAssessment, `"score":2,`, "", 1), "score 缺失"},
		{"out of range", strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":3`, 1), "score 缺失"},
		{"string score", strings.Replace(qualityValidCandyAssessment, `"score":2`, `"score":"2"`, 1), "字段类型"},
		{"missing confidence", strings.Replace(qualityValidCandyAssessment, `"confidence":0.95,`, "", 1), "confidence 缺失"},
		{"missing reason", strings.Replace(qualityValidCandyAssessment, `"reason":"答案与两项证明完整",`, "", 1), "reason 缺失"},
		{"missing evidence", strings.Replace(qualityValidCandyAssessment, `"evidence":"结论21"`, `"evidence":""`, 1), "evidence 缺失"},
		{"duplicate check id", strings.Replace(qualityValidCandyAssessment, `"id":"minimality"`, `"id":"answer"`, 1), "评分项 ID"},
		{"missing checks", `{"confidence":1,"reason":"正确"}`, "评分项数量"},
		{"oversize", strings.Repeat(" ", 32769), "大小上限"},
		{"deep nesting", `{"extra":` + strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66) + `}`, "嵌套层级"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, verdict, _, err := parseQualityAssessment(tc.text, "candy", "gpt-6.1-sol")
			require.ErrorContains(t, err, tc.diagnostic)
			require.Nil(t, out)
			require.Empty(t, verdict)
		})
	}
}

func TestModelQualitySavedArtifactDoesNotRevalidateGenerator(t *testing.T) {
	for _, mode := range []string{"candy", "pelican"} {
		t.Run(mode, func(t *testing.T) {
			s, repo, client, run := qualityFixture()
			repo.plan.Config.Benchmark, run.Target.Benchmark = mode, mode
			s.step(context.Background(), run)
			require.NotNil(t, run.Result)
			// Generation is finished. Any attempt to inspect a now unavailable
			// generator/channel/group would fail; judging must not consult them.
			s.groups, s.channels, s.monitors = nil, nil, nil
			repo.plan.Config.BaseURL = "http://unavailable.invalid"
			repo.plan.Config.MonitorID = 999
			run.CredentialHash = "old-generator-credentials"
			s.step(context.Background(), run)
			require.Equal(t, 1, client.creates)
			if mode == "candy" {
				require.Zero(t, client.gets)
			} else {
				require.Equal(t, 1, client.gets)
			}
			require.Equal(t, "succeeded", run.Status)
			require.Equal(t, "normal", run.Verdict)
		})
	}
}

func TestModelQualityAccountIndependencePreservesAuthorization(t *testing.T) {
	t.Run("unavailable generator still blocks generation", func(t *testing.T) {
		s, repo, client, run := qualityJudgedFixture()
		repo.plan.Config.MonitorID = 8
		s.monitors = qualityTestMonitors{&ChannelMonitor{ID: 8, Enabled: false, Provider: MonitorProviderOpenAI, Endpoint: "https://relay.example", APIKey: "generator-key"}}
		s.step(context.Background(), run)
		require.Zero(t, client.creates+client.gets)
		require.Equal(t, "error", run.Verdict)
	})
	t.Run("revoked judge key still blocks judging", func(t *testing.T) {
		s, _, client, run := qualityJudgedFixture()
		s.step(context.Background(), run)
		s.keys.(qualityTestKeys).key.Status = "disabled"
		s.step(context.Background(), run)
		require.Equal(t, 1, client.creates)
		require.Zero(t, client.gets)
		require.Equal(t, "error", run.Verdict)
		require.NotEmpty(t, run.Result.HTML)
	})
}

func TestModelQualityJudgeAcceptsGatewayAccountChanges(t *testing.T) {
	for _, account := range []string{"4589", "4597"} {
		t.Run(account, func(t *testing.T) {
			s, _, generator, run := qualityJudgedFixture()
			s.judgeModel, run.Target.JudgeModel = "gpt-6.1-sol", "gpt-6.1-sol"
			s.step(context.Background(), run)
			calls := 0
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, "/v1/responses", r.URL.Path)
				require.Equal(t, "gpt-6.1-sol", payload["model"])
				require.Empty(t, r.Header.Get("X-Account-ID"))
				require.Empty(t, r.Header.Get("X-Sub2api-Account-ID"))
				require.NotContains(t, payload, "account_id")
				instructions := payload["instructions"].(string)
				require.Contains(t, instructions, qualityJudgeFormatVersion)
				for _, field := range []string{"checks", "confidence", "reason", "subject", "bicycle", "motion"} {
					require.Contains(t, instructions, `"`+field+`"`)
				}
				text := qualityScoredAssessment(t, "pelican", 0.95, nil)
				if account == "4589" {
					text = "```json\n" + text + "\n```"
				}
				// These are simulated account identities, not live upstream calls.
				w.Header().Set("X-Upstream-Account-ID", account)
				response, err := json.Marshal(map[string]any{"status": "completed", "output_text": text, "usage": map[string]any{"input_tokens": 100, "output_tokens": 50}})
				require.NoError(t, err)
				_, _ = fmt.Fprint(w, string(response))
			}))
			defer gateway.Close()
			s.judgeOrigin = gateway.URL + "/v1"
			s.judge = &qualityHTTPClient{client: gateway.Client()}
			s.step(context.Background(), run)
			require.Equal(t, 1, calls)
			require.Equal(t, 1, generator.creates)
			require.Equal(t, "normal", run.Verdict)
			require.EqualValues(t, 50, *run.Result.JudgeOutputTokens)
		})
	}
}

func TestModelQualityInvalidAssessmentDiagnosticDoesNotEchoResponse(t *testing.T) {
	s, _, _, run := qualityJudgedFixture()
	s.step(context.Background(), run)
	inputTokens, outputTokens := int64(100), int64(50)
	s.judge = qualityFixedCompletionClient{result: &qualityCompletion{
		Text:        `{"confidence":"private-test-key","secret-field":"private-test-key"}`,
		InputTokens: &inputTokens, OutputTokens: &outputTokens,
	}}
	s.step(context.Background(), run)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "error", run.Verdict)
	require.Contains(t, run.Reason, "字段类型")
	require.NotContains(t, run.Reason, "private-test-key")
	require.NotContains(t, run.Reason, "secret-field")
	require.NotEmpty(t, run.Result.HTML)
	require.EqualValues(t, 50, *run.Result.JudgeOutputTokens)
}

type qualityFixedCompletionClient struct{ result *qualityCompletion }

func (c qualityFixedCompletionClient) Complete(context.Context, qualityCredentials, qualityCompletionRequest) (*qualityCompletion, error) {
	return c.result, nil
}
