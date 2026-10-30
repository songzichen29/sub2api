package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQualityResponsesContract(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			var payload map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/custom/v1/responses", r.URL.Path)
				require.Equal(t, "Bearer private-key", r.Header.Get("Authorization"))
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				data := `{"status":"completed","output":[{"type":"reasoning","summary":[]},{"type":"message","content":[{"type":"output_text","text":"21"}]}],"usage":{"input_tokens":12,"output_tokens":34,"output_tokens_details":{"reasoning_tokens":5}},"service_tier":"default"}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", data)
				} else {
					fmt.Fprint(w, data)
				}
			}))
			defer server.Close()
			client := &qualityHTTPClient{client: server.Client()}
			out, err := client.Complete(context.Background(), qualityCredentials{server.URL + "/custom/v1", "private-key"}, qualityCompletionRequest{Model: "gpt-6-luna", Effort: "medium", Input: "test", Schema: qualityAssessmentSchema("candy"), MaxOutputTokens: 6000})
			require.NoError(t, err)
			require.Equal(t, "21", out.Text)
			require.EqualValues(t, 34, *out.OutputTokens)
			require.Equal(t, "gpt-6-luna", payload["model"])
			require.Equal(t, false, payload["store"])
			require.Equal(t, true, payload["stream"])
			require.Contains(t, payload, "text")
			require.NotContains(t, payload, "api_key")
			require.NotContains(t, payload, "service_tier")
		})
	}
}

func TestQualityRejectsIncompleteAndOversizeResponses(t *testing.T) {
	for _, tc := range []struct{ name, body, contentType string }{
		{"incomplete", `{"status":"incomplete","output_text":"21"}`, "application/json"},
		{"no status", `{"output_text":"21"}`, "application/json"},
		{"refusal", `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"no"}]}]}`, "application/json"},
		{"missing complete", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: [DONE]\n\n", "text/event-stream"},
		{"stream error", "data: {\"type\":\"error\",\"message\":\"private-key\"}\n\n", "text/event-stream"},
		{"oversize", strings.Repeat("x", qualityMaxResponseBytes+2), "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := (&qualityHTTPClient{server.Client()}).Complete(context.Background(), qualityCredentials{server.URL, "private-key"}, qualityCompletionRequest{Model: "model"})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-key")
		})
	}
}

func TestQualityHTTPPrivacyAndRedirects(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(307)
		fmt.Fprint(w, "private-key")
	}))
	defer source.Close()
	c := newQualityHTTPClient(true)
	_, err := c.Complete(context.Background(), qualityCredentials{source.URL, "private-key"}, qualityCompletionRequest{})
	require.Error(t, err)
	require.False(t, called)
	require.NotContains(t, err.Error(), "private-key")
	c = newQualityHTTPClient(false)
	_, err = c.Complete(context.Background(), qualityCredentials{source.URL, "private-key"}, qualityCompletionRequest{})
	require.Error(t, err, "public generator must block loopback")
}

func TestQualityAssessmentValidationAndVerdicts(t *testing.T) {
	makeJSON := func(conf float64, scores []int) string {
		checks := []ModelQualityCheck{}
		for i, id := range []string{"answer", "sufficiency", "minimality"} {
			checks = append(checks, ModelQualityCheck{ID: id, Score: scores[i], Evidence: "specific evidence"})
		}
		b, _ := json.Marshal(map[string]any{"confidence": conf, "reason": "具体推理依据", "checks": checks})
		return string(b)
	}
	for _, tc := range []struct {
		conf   float64
		scores []int
		want   string
	}{{.95, []int{2, 2, 2}, "normal"}, {.95, []int{0, 2, 2}, "degraded"}, {.7, []int{0, 0, 0}, "unknown"}, {.95, []int{2, 1, 2}, "unknown"}} {
		_, v, _, err := parseQualityAssessment(makeJSON(tc.conf, tc.scores), "candy", "gpt-6-luna")
		require.NoError(t, err)
		require.Equal(t, tc.want, v)
	}
	good := makeJSON(.95, []int{2, 2, 2})
	for _, bad := range []string{"{}", good + "{}", strings.Replace(good, `"score":2`, `"score":3`, 1), strings.Replace(good, `"id":"minimality"`, `"id":"answer"`, 1), strings.Replace(good, `"confidence":0.95,`, "", 1), strings.Replace(good, `"score":2,`, "", 1), strings.Replace(good, `"specific evidence"`, `""`, 1)} {
		_, _, _, err := parseQualityAssessment(bad, "candy", "gpt-6-luna")
		require.Error(t, err, bad)
	}
}

func TestQualityJudgeFailureAndCancellationKeepsArtifact(t *testing.T) {
	s, _, c, r := qualityFixture()
	s.step(context.Background(), r)
	c.judgment = `{"confidence":1,"reason":"pretend normal"}`
	s.step(context.Background(), r)
	require.Equal(t, "error", r.Verdict)
	require.NotEmpty(t, r.Result.Answer)
}

func TestQualityOperationLeaseExceedsTimeout(t *testing.T) {
	s, repo, _, r := qualityFixture()
	// The first saved next-run deadline is the generation lease, not the short
	// scheduler polling delay. Capture it from a wrapped repository.
	wrapped := &qualityLeaseTestRepo{qualityTestRepo: repo}
	s.repo = wrapped
	s.step(context.Background(), r)
	require.Greater(t, time.Until(wrapped.first).Minutes(), 8.0)
}

type qualityLeaseTestRepo struct {
	*qualityTestRepo
	first time.Time
}

func (r *qualityLeaseTestRepo) SaveRun(ctx context.Context, run *ModelQualityRun, next time.Time) error {
	if r.first.IsZero() {
		r.first = next
	}
	return r.qualityTestRepo.SaveRun(ctx, run, next)
}
