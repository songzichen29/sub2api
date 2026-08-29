package service

import (
	"context"
	"strings"
)

const (
	defaultOpenAIMessagesDispatchOpusMappedModel   = "gpt-5.4"
	defaultOpenAIMessagesDispatchSonnetMappedModel = "gpt-5.3-codex"
	defaultOpenAIMessagesDispatchHaikuMappedModel  = "gpt-5.4-mini"
)

func normalizeOpenAIMessagesDispatchMappedModel(model string) string {
	model = NormalizeOpenAICompatRequestedModel(strings.TrimSpace(model))
	return strings.TrimSpace(model)
}

func normalizeOpenAIMessagesDispatchModelConfig(cfg OpenAIMessagesDispatchModelConfig) OpenAIMessagesDispatchModelConfig {
	out := OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   normalizeOpenAIMessagesDispatchMappedModel(cfg.OpusMappedModel),
		SonnetMappedModel: normalizeOpenAIMessagesDispatchMappedModel(cfg.SonnetMappedModel),
		HaikuMappedModel:  normalizeOpenAIMessagesDispatchMappedModel(cfg.HaikuMappedModel),
	}

	if len(cfg.ExactModelMappings) > 0 {
		out.ExactModelMappings = make(map[string]string, len(cfg.ExactModelMappings))
		for requestedModel, mappedModel := range cfg.ExactModelMappings {
			requestedModel = strings.TrimSpace(requestedModel)
			mappedModel = normalizeOpenAIMessagesDispatchMappedModel(mappedModel)
			if requestedModel == "" || mappedModel == "" {
				continue
			}
			out.ExactModelMappings[requestedModel] = mappedModel
		}
		if len(out.ExactModelMappings) == 0 {
			out.ExactModelMappings = nil
		}
	}

	return out
}

func claudeMessagesDispatchFamily(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(normalized, "claude") {
		return ""
	}
	switch {
	case strings.Contains(normalized, "opus"):
		return "opus"
	case strings.Contains(normalized, "sonnet"):
		return "sonnet"
	case strings.Contains(normalized, "haiku"):
		return "haiku"
	default:
		return ""
	}
}

func appendUniqueMessagesDispatchCandidate(candidates []string, seen map[string]struct{}, model string) []string {
	model = normalizeOpenAIMessagesDispatchMappedModel(model)
	if model == "" {
		return candidates
	}
	if _, exists := seen[model]; exists {
		return candidates
	}
	seen[model] = struct{}{}
	return append(candidates, model)
}

func messagesDispatchFallbackCandidatesByFamily(family string) []string {
	switch strings.TrimSpace(family) {
	case "opus":
		return []string{"gpt-5.4", "gpt-5.5", "gpt-5.3-codex", "gpt-5.2", "gpt-5.4-mini"}
	case "sonnet":
		return []string{"gpt-5.3-codex", "gpt-5.4", "gpt-5.5", "gpt-5.2", "gpt-5.4-mini"}
	case "haiku":
		return []string{"gpt-5.4-mini", "gpt-5.4", "gpt-5.3-codex", "gpt-5.2", "gpt-5.5"}
	default:
		return nil
	}
}

func isOpenAIMessagesDispatchEligibleModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	return !strings.HasPrefix(model, "gpt-image")
}

func buildMessagesDispatchModelCandidates(g *Group, requestedModel string) []string {
	if g == nil {
		return nil
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil
	}

	// 国产供应商分组:调度级模型映射不适用(其配置被 sanitize 置空,且下方的
	// gpt-5.x 默认值是 openai 专属,发给 CN 上游必错)。模型改写完全交给账号级
	// model_mapping;anthropic 协议上游本身接受 claude-* 模型名。
	if IsCNProvider(g.Platform) {
		return ""
	}

	cfg := normalizeOpenAIMessagesDispatchModelConfig(g.MessagesDispatchModelConfig)
	family := claudeMessagesDispatchFamily(requestedModel)
	// 精确映射优先于 family 判定：非 opus/sonnet/haiku 的 Claude 模型（如 claude-fable-*）
	// 仍可走 ExactModelMappings（与上游 0.1.155 行为一致）。
	if family == "" {
		if mapped := strings.TrimSpace(cfg.ExactModelMappings[requestedModel]); mapped != "" {
			return []string{mapped}
		}
		return nil
	}

	candidates := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, cfg.ExactModelMappings[requestedModel])

	switch family {
	case "opus":
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, cfg.OpusMappedModel)
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, defaultOpenAIMessagesDispatchOpusMappedModel)
	case "sonnet":
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, cfg.SonnetMappedModel)
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, defaultOpenAIMessagesDispatchSonnetMappedModel)
	case "haiku":
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, cfg.HaikuMappedModel)
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, defaultOpenAIMessagesDispatchHaikuMappedModel)
	}

	for _, candidate := range messagesDispatchFallbackCandidatesByFamily(family) {
		candidates = appendUniqueMessagesDispatchCandidate(candidates, seen, candidate)
	}
	return candidates
}

func (g *Group) ResolveMessagesDispatchModel(requestedModel string) string {
	candidates := buildMessagesDispatchModelCandidates(g, requestedModel)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func (s *OpenAIGatewayService) ResolveMessagesDispatchMappedModel(ctx context.Context, groupID *int64, group *Group, requestedModel string) string {
	candidates := buildMessagesDispatchModelCandidates(group, requestedModel)
	if len(candidates) == 0 {
		return ""
	}
	if s == nil || groupID == nil {
		return candidates[0]
	}
	if ctx == nil {
		ctx = context.Background()
	}

	snapshot, found := s.PeekAvailableModelsSnapshot(groupID)
	if !found {
		s.GetAvailableModels(ctx, groupID)
		snapshot, _ = s.PeekAvailableModelsSnapshot(groupID)
	}

	isAllowedCandidate := func(candidate string) bool {
		candidate = normalizeOpenAIMessagesDispatchMappedModel(candidate)
		if !isOpenAIMessagesDispatchEligibleModel(candidate) {
			return false
		}
		if s.checkChannelPricingRestriction(ctx, groupID, candidate) {
			return false
		}
		if snapshot.Restrictive && len(snapshot.Models) > 0 {
			return SnapshotSupportsRequestedModel(snapshot, PlatformOpenAI, candidate)
		}
		return true
	}

	for _, candidate := range candidates {
		if isAllowedCandidate(candidate) {
			return candidate
		}
	}
	for _, candidate := range snapshot.Models {
		if isAllowedCandidate(candidate) {
			return normalizeOpenAIMessagesDispatchMappedModel(candidate)
		}
	}
	return candidates[0]
}

func sanitizeGroupMessagesDispatchFields(g *Group) {
	if g == nil || g.Platform == PlatformOpenAI {
		return
	}
	if g.Platform != PlatformComposite {
		g.AllowMessagesDispatch = false
	}
	g.DefaultMappedModel = ""
	g.MessagesDispatchModelConfig = OpenAIMessagesDispatchModelConfig{}
}
