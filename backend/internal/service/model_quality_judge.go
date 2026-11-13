package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

func validateQualityJudgeModel(model string) error {
	if model == "" || len(model) > 200 || strings.IndexFunc(model, unicode.IsControl) >= 0 {
		return qualityInvalid("评分模型名称不能为空、不能含控制字符且最多 200 字节")
	}
	return nil
}

func (s *ModelQualityService) planJudgeModel(p *ModelQualityPlan) string {
	if p != nil && strings.TrimSpace(p.JudgeModel) != "" {
		return strings.TrimSpace(p.JudgeModel)
	}
	if model := strings.TrimSpace(s.judgeModel); model != "" {
		return model
	}
	return "gpt-6.1-sol"
}

func qualityJudgeRequest(benchmark, model, artifact string) qualityCompletionRequest {
	data, _ := json.Marshal(map[string]any{"benchmark": benchmark, "task": qualityPrompt(benchmark), "rubric": qualityRubric(benchmark), "artifact": artifact})
	schema := qualityAssessmentSchema(benchmark)
	format, _ := json.Marshal(schema)
	// Repeat the exact contract in instructions for compatible upstreams which
	// do not enforce text.format. Probe and actual scoring use the same contract.
	instructions := qualityJudgeInstructions + "\n输出契约版本：" + qualityJudgeFormatVersion + "。只返回一个符合以下 JSON Schema 的对象，不加代码围栏或说明。checks 必须包含 rubric 的每个 ID 各一次；不要省略评分或证据。\n" + string(format)
	return qualityCompletionRequest{Model: model, Effort: "medium", Instructions: instructions, Input: string(data), Schema: schema, MaxOutputTokens: 6000}
}

func (s *ModelQualityService) probeQualityJudge(ctx context.Context, owner int64, model string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateQualityJudgeModel(model); err != nil {
		return err
	}
	// Prevent double-clicks/concurrent saves for this owner from starting
	// duplicate billable probes in this process. No account identity is pinned.
	s.judgeProbeMu.Lock()
	if s.judgeProbing[owner] {
		s.judgeProbeMu.Unlock()
		return qualityInvalid("评分模型正在测试，请等待当前测试结束")
	}
	if s.judgeProbing == nil {
		s.judgeProbing = map[int64]bool{}
	}
	s.judgeProbing[owner] = true
	s.judgeProbeMu.Unlock()
	defer func() {
		s.judgeProbeMu.Lock()
		delete(s.judgeProbing, owner)
		s.judgeProbeMu.Unlock()
	}()
	key, err := s.qualityJudgeKey(ctx, owner, 0)
	if err != nil {
		return err
	}
	keyID, keyText, groupID := key.ID, key.Key, *key.GroupID
	// A fixed public sample tests transport + the actual scoring output contract,
	// not model intelligence. It never generates an artwork or enters the queue.
	const sample = `<html><body><svg viewBox="0 0 400 300"><circle cx="120" cy="220" r="45"/><circle cx="280" cy="220" r="45"/><path d="M120 220L180 150L220 220L120 220L250 150L280 220"/><ellipse cx="180" cy="100" rx="45" ry="30"/><path d="M200 80L270 85L220 110Z"/></svg></body></html>`
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	completion, err := s.judge.Complete(probeCtx, qualityCredentials{s.judgeOrigin, keyText}, qualityJudgeRequest("pelican", model, sample))
	if err != nil {
		var statusErr *qualityHTTPStatusError
		if errors.As(err, &statusErr) {
			return qualityInvalid("评分模型测试失败：" + statusErr.Error() + "；配置未保存")
		}
		return qualityInvalid("评分模型测试失败、超时或响应不完整；配置未保存，未自动重试")
	}
	if completion == nil {
		return qualityInvalid("评分模型测试未返回正文；配置未保存")
	}
	if _, _, _, err := parseQualityAssessment(completion.Text, "pelican", model); err != nil {
		return qualityInvalid("评分模型测试返回的评分结构无效：" + err.Error() + "；配置未保存")
	}
	// Re-resolve the named key, but never the selected upstream account. A key
	// rotation/group change during the probe requires testing the new credential.
	fresh, err := s.qualityJudgeKey(probeCtx, owner, 0)
	if err != nil || fresh.ID != keyID || fresh.Key != keyText || *fresh.GroupID != groupID {
		return qualityInvalid("测试期间裁判 Key 配置已变化，请重新测试；配置未保存")
	}
	return probeCtx.Err()
}
