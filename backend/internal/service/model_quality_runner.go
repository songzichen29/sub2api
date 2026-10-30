package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

func (s *ModelQualityService) qualityJudgeKey(ctx context.Context, owner, id int64) (*APIKey, error) {
	if s.judgeModel != "gpt-6-luna" && s.judgeModel != "gpt-6.1-sol" {
		return nil, qualityInvalid("裁判模型须为 gpt-6-luna 或 gpt-6.1-sol")
	}
	if id == 0 {
		// Resolve by exact name within the plan owner's keys, never globally and
		// never choose arbitrarily when names are duplicated.
		for page := 1; ; page++ {
			keys, total, err := s.keys.ListByUserID(ctx, owner, pagination.PaginationParams{Page: page, PageSize: 100}, APIKeyListFilters{Search: s.judgeKeyName})
			if err != nil {
				return nil, qualityInvalid("无法读取裁判 Key")
			}
			for _, key := range keys {
				if key.Name != s.judgeKeyName {
					continue
				}
				if id != 0 {
					return nil, qualityInvalid("同名裁判 Key 不唯一，请保留唯一名称")
				}
				id = key.ID
			}
			if len(keys) == 0 || (total != nil && page*100 >= int(total.Total)) || len(keys) < 100 {
				break
			}
		}
		if id == 0 {
			return nil, qualityInvalid("请为计划创建者配置名为「" + s.judgeKeyName + "」的本站裁判 Key")
		}
	}
	key, err := s.keys.GetByID(ctx, id)
	if err != nil || key == nil || key.UserID != owner || key.GroupID == nil || key.Key == "" || !key.IsActive() || key.IsExpired() || key.IsQuotaExhausted() {
		return nil, qualityInvalid("裁判 Key 不存在、不属于计划创建者、未绑定分组或已失效")
	}
	return key, nil
}

func cleanQualityText(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return logredact.RedactText(text)
}

// Each step makes at most one billable request. Persist intent and a lease
// longer than its timeout BEFORE sending it. A recovered in-flight intent is
// uncertain and must never be automatically sent again (including judging).
func (s *ModelQualityService) step(ctx context.Context, r *ModelQualityRun) {
	if r == nil || r.FinishedAt != nil {
		return
	}
	finish := func(status, verdict, reason string) {
		saveCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		s.finish(saveCtx, r, status, verdict, reason)
	}
	if r.CancelRequested {
		finish("cancelled", "unknown", "检测已取消；已发生的生成或裁判用量仍可能计费")
		return
	}
	if r.RemoteID != "" {
		finish("failed", "error", "旧 Manxue 在途任务未迁移，已停止查询和自动重发；历史结果不受影响")
		return
	}
	start := r.StartedAt
	if r.ExecutionStartedAt != nil {
		start = *r.ExecutionStartedAt
	}
	if time.Since(start) > 12*time.Minute {
		finish("failed", "error", "检测执行超时，不能据此判定降智")
		return
	}
	if r.Status == "submitting" {
		finish("failed", "error", "生成或裁判请求结果不确定，已停止自动重发以避免重复计费")
		return
	}
	if r.Target.BenchmarkVersion != "" && r.Target.BenchmarkVersion != qualityBenchmarkVersion {
		finish("failed", "error", "检测规则版本已变更，请重新发起检测")
		return
	}
	p, err := s.repo.GetPlan(ctx, r.PlanID)
	if err != nil || p == nil {
		finish("failed", "error", "无法读取检测配置")
		return
	}
	cfg, ok := qualityConfigForRun(p, r.ConfigID)
	if !ok {
		finish("failed", "error", "检测配置已被删除")
		return
	}
	if qualityPrompt(cfg.Benchmark) == "" || cfg.Benchmark != r.Target.Benchmark || cfg.Model != r.Target.Model {
		finish("failed", "error", "检测配置与运行快照不匹配")
		return
	}
	if _, err = s.target(ctx, p, cfg); err != nil {
		finish("failed", "error", "目标分组或渠道配置已失效")
		return
	}
	credential, err := s.credentials(ctx, p, cfg)
	if err != nil {
		finish("failed", "error", err.Error())
		return
	}
	judgeKey, err := s.qualityJudgeKey(ctx, p.OwnerID, r.JudgeKeyID)
	if err != nil {
		finish("failed", "error", err.Error())
		return
	}
	r.JudgeKeyID = judgeKey.ID
	if r.Target.JudgeModel == "" {
		r.Target.JudgeModel = s.judgeModel
	}
	if r.Target.JudgeModel != "gpt-6-luna" && r.Target.JudgeModel != "gpt-6.1-sol" {
		finish("failed", "error", "裁判模型配置无效")
		return
	}
	r.Target.BenchmarkVersion = qualityBenchmarkVersion
	if r.Result != nil && r.CredentialHash != credential.hash() {
		finish("failed", "error", "生成后渠道凭据已变更，已停止本次评分；请重新检测")
		return
	}
	secrets := []string{credential.key, credential.baseURL, judgeKey.Key, s.judgeOrigin}
	request := qualityCompletionRequest{Model: cfg.Model, Effort: cfg.ReasoningEffort, ServiceTier: cfg.ServiceTier, Input: qualityPrompt(cfg.Benchmark), MaxOutputTokens: 24000}
	client, endpoint, timeout := s.generator, credential, 8*time.Minute
	generating := r.Result == nil
	if generating {
		r.CredentialHash = credential.hash()
		r.Phase = "generating"
	} else {
		artifact := r.Result.HTML
		if cfg.Benchmark == "candy" {
			artifact = r.Result.Answer
		}
		if strings.TrimSpace(artifact) == "" {
			finish("failed", "error", "缺少待评分作品")
			return
		}
		data, _ := json.Marshal(map[string]any{"benchmark": cfg.Benchmark, "task": qualityPrompt(cfg.Benchmark), "rubric": qualityRubric(cfg.Benchmark), "artifact": artifact})
		request = qualityCompletionRequest{Model: r.Target.JudgeModel, Effort: "medium", Instructions: qualityJudgeInstructions, Input: string(data), Schema: qualityAssessmentSchema(cfg.Benchmark), MaxOutputTokens: 6000}
		client, endpoint, timeout = s.judge, qualityCredentials{s.judgeOrigin, judgeKey.Key}, 2*time.Minute
		r.Phase = "classifying"
	}
	r.Status = "submitting"
	r.Attempts++
	if !s.persist(ctx, r, time.Now().Add(timeout+time.Minute)) {
		return
	}
	completion, err, cancelled, lostLease := s.qualityOperation(ctx, r, timeout, client, endpoint, request)
	if lostLease {
		return
	}
	if cancelled {
		finish("cancelled", "unknown", "检测已取消；已发生的用量仍可能计费")
		return
	}
	if err != nil {
		phase := "生成"
		if !generating {
			phase = "裁判"
		}
		// Client errors are already sanitized; don't expose injected implementation
		// errors, upstream response bodies, keys, URLs or source text.
		reason := phase + "请求失败、超时或响应不完整；未自动重发，不能据此判定降智"
		var statusErr *qualityHTTPStatusError
		if errors.As(err, &statusErr) {
			reason = phase + "请求失败：" + statusErr.Error() + "；不能据此判定降智"
		}
		finish("failed", "error", reason)
		return
	}
	if completion == nil {
		finish("failed", "error", "缺少模型响应")
		return
	}
	if generating {
		text := cleanQualityText(strings.TrimSpace(completion.Text), secrets...)
		if text == "" || len(text) > qualityMaxArtifactBytes {
			finish("failed", "error", "生成正文为空或超过大小上限")
			return
		}
		r.Result = &ModelQualityResult{Source: qualityBenchmarkVersion + ":unassessed", DurationMS: completion.DurationMS, InputTokens: completion.InputTokens, OutputTokens: completion.OutputTokens, ReasoningTokens: completion.ReasoningTokens, ServiceTier: completion.ServiceTier}
		if cfg.Benchmark == "candy" {
			r.Result.Answer = text
		} else {
			r.Result.HTML = qualityStripFence(text)
		}
		r.Status, r.Phase, r.Reason = "running", "classifying", "作品已生成，等待独立裁判"
		if r.Result.ServiceTier != "default" && r.Result.ServiceTier != "priority" && r.Result.ServiceTier != "ultrafast" {
			r.Result.ServiceTier = ""
		}
		saveCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		s.persist(saveCtx, r, time.Now())
		return
	}
	assessment, verdict, reason, err := parseQualityAssessment(completion.Text, cfg.Benchmark, r.Target.JudgeModel)
	if err != nil {
		finish("failed", "error", "裁判返回的评分结构无效，未当成正常或降智")
		return
	}
	for i := range assessment.Checks {
		assessment.Checks[i].Evidence = qualityTextLimit(cleanQualityText(assessment.Checks[i].Evidence, secrets...), 2048)
	}
	r.Result.Assessment = assessment
	r.Result.JudgeInputTokens, r.Result.JudgeOutputTokens = completion.InputTokens, completion.OutputTokens
	r.Result.Source = qualityBenchmarkVersion + ":" + r.Target.JudgeModel + ":" + assessment.Basis
	finish("succeeded", verdict, qualityTextLimit(cleanQualityText(reason, secrets...), 8192))
}

func (s *ModelQualityService) qualityOperation(ctx context.Context, r *ModelQualityRun, timeout time.Duration, client qualityCompletionClient, credential qualityCredentials, in qualityCompletionRequest) (*qualityCompletion, error, bool, bool) {
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-opCtx.Done():
				return
			case <-ticker.C:
				readCtx, stop := context.WithTimeout(opCtx, time.Second)
				fresh, err := s.repo.GetRun(readCtx, r.ID)
				stop()
				if err == nil && fresh != nil && (fresh.CancelRequested || fresh.FinishedAt != nil || fresh.LeaseToken != r.LeaseToken) {
					cancel()
					return
				}
			}
		}
	}()
	result, err := client.Complete(opCtx, credential, in)
	cancel()
	<-done
	readCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	fresh, readErr := s.repo.GetRun(readCtx, r.ID)
	if readErr != nil || fresh == nil {
		return nil, errors.New("无法确认运行状态"), false, false
	}
	return result, err, fresh.CancelRequested, fresh.FinishedAt != nil || fresh.LeaseToken != r.LeaseToken
}

func qualityStripFence(text string) string {
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") {
		if pos := strings.IndexByte(text, '\n'); pos >= 0 {
			return strings.TrimSpace(text[pos+1 : len(text)-3])
		}
	}
	return text
}

func qualityAssessmentSchema(benchmark string) map[string]any {
	ids := make([]string, 0, len(qualityRubric(benchmark)))
	for id := range qualityRubric(benchmark) {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"confidence", "reason", "checks"}, "properties": map[string]any{
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"reason":     map[string]any{"type": "string"},
		"checks": map[string]any{"type": "array", "minItems": len(ids), "maxItems": len(ids), "items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"id", "score", "evidence"}, "properties": map[string]any{
				"id": map[string]any{"type": "string", "enum": ids}, "score": map[string]any{"type": "integer", "enum": []int{0, 1, 2}}, "evidence": map[string]any{"type": "string"},
			},
		}},
	}}
}

func parseQualityAssessment(text, benchmark, model string) (*ModelQualityAssessment, string, string, error) {
	var raw struct {
		Confidence *float64 `json:"confidence"`
		Reason     string   `json:"reason"`
		Checks     []struct {
			ID       string `json:"id"`
			Score    *int   `json:"score"`
			Evidence string `json:"evidence"`
		} `json:"checks"`
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	invalid := errors.New("invalid quality assessment")
	if len(text) > 32768 || decoder.Decode(&raw) != nil || raw.Confidence == nil || math.IsNaN(*raw.Confidence) || *raw.Confidence < 0 || *raw.Confidence > 1 || strings.TrimSpace(raw.Reason) == "" || len(raw.Reason) > 8192 {
		return nil, "", "", invalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, "", "", invalid
	}
	rubric := qualityRubric(benchmark)
	if len(raw.Checks) != len(rubric) {
		return nil, "", "", invalid
	}
	out := &ModelQualityAssessment{Version: qualityBenchmarkVersion, Basis: "html_source", Model: model, Confidence: *raw.Confidence}
	if benchmark == "candy" {
		out.Basis = "answer_reasoning"
	}
	seen := map[string]bool{}
	sum, failures := 0, 0
	for _, c := range raw.Checks {
		if _, ok := rubric[c.ID]; !ok || seen[c.ID] || c.Score == nil || *c.Score < 0 || *c.Score > 2 || strings.TrimSpace(c.Evidence) == "" || len(c.Evidence) > 4096 {
			return nil, "", "", invalid
		}
		seen[c.ID] = true
		sum += *c.Score
		if *c.Score == 0 {
			failures++
		}
		out.Checks = append(out.Checks, ModelQualityCheck{ID: c.ID, Score: *c.Score, Evidence: c.Evidence})
	}
	verdict := "unknown"
	if out.Confidence >= 0.8 {
		if failures > 0 {
			verdict = "degraded"
		} else if sum*10 >= len(rubric)*16 {
			verdict = "normal"
		}
		// A correct number without both proofs is not a confirmed candy pass.
		if benchmark == "candy" && failures == 0 && sum != 6 {
			verdict = "unknown"
		}
	}
	reason := raw.Reason + "；仅反映本次测试，不能证明模型身份或整体能力"
	if verdict == "unknown" {
		reason = "证据不足，无法判定：" + reason
	}
	if benchmark != "candy" {
		reason = "源码分析（未运行浏览器）：" + reason
	}
	return out, verdict, reason, nil
}
