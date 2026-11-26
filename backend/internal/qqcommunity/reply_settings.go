package qqcommunity

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
)

// 省略字段表示继承；空 prompt 表示明确使用空人设。
type ReplyRule struct {
	Mode   *string `json:"mode,omitempty"`
	Voice  *string `json:"voice,omitempty"`
	Prompt *string `json:"prompt,omitempty"`
}

type ReplySettings struct {
	GroupDefaults ReplyRule            `json:"group_defaults"`
	Groups        map[string]ReplyRule `json:"groups,omitempty"`
	PrivateUsers  map[string]ReplyRule `json:"private_users,omitempty"`
	Revision      uint64               `json:"revision,omitempty"`
	LastOperation *ReplyOperation      `json:"last_operation,omitempty"`
}

// 只保留最后一次操作。更早请求由单调递增的版本拒绝，避免恢复旧配置后重复追加。
type ReplyOperation struct {
	Actor       string `json:"actor"`
	OperationID string `json:"operation_id"`
	RequestHash string `json:"request_hash"`
}

type ReplySettingsUpdate struct {
	Scope        string   `json:"scope"`
	GroupIDs     []string `json:"group_ids,omitempty"`
	Mode         *string  `json:"mode,omitempty"`
	Voice        *string  `json:"voice,omitempty"`
	Prompt       *string  `json:"prompt,omitempty"`
	PromptAction string   `json:"prompt_action,omitempty"`
	Reset        []string `json:"reset,omitempty"`
}

type replySettingsView struct {
	GroupDefaults ReplyRule            `json:"group_defaults"`
	Groups        map[string]ReplyRule `json:"groups"`
	PrivateUsers  map[string]ReplyRule `json:"private_users"`
}

type replySettingsResult struct {
	Status   string            `json:"status"`
	Version  string            `json:"version"`
	Settings replySettingsView `json:"settings"`
}

type replyGroup struct {
	GroupID string `json:"group_id"`
	Label   string `json:"label"`
}

func replyGroups(c Config) []replyGroup {
	groups := make([]replyGroup, 0, len(c.AllowedGroups))
	for _, id := range c.AllowedGroups {
		groups = append(groups, replyGroup{GroupID: id, Label: c.groupPolicy(id).Label})
	}
	return groups
}

func replySettingsForUser(settings ReplySettings, sender string) replySettingsView {
	private := map[string]ReplyRule{}
	if rule, exists := settings.PrivateUsers[sender]; exists {
		private[sender] = rule
	}
	groups := maps.Clone(settings.Groups)
	if groups == nil {
		groups = map[string]ReplyRule{}
	}
	return replySettingsView{GroupDefaults: settings.GroupDefaults, Groups: groups, PrivateUsers: private}
}

func validateReplyRule(rule ReplyRule) error {
	if rule.Mode != nil && *rule.Mode != "text" && *rule.Mode != "voice" {
		return errors.New("回复方式须为 text 或 voice")
	}
	// 音色目录由可信运行时的语音适配器校验，后端不复制第二份目录。
	if rule.Voice != nil && (strings.TrimSpace(*rule.Voice) != *rule.Voice || *rule.Voice == "" || len([]rune(*rule.Voice)) > 80 || strings.ContainsAny(*rule.Voice, "\r\n")) {
		return errors.New("音色名称须为有效目录名称")
	}
	if rule.Prompt != nil && len(*rule.Prompt) > 64000 {
		return errors.New("回复人设超过现有 64000 字节上限")
	}
	return nil
}

func validateReplySettings(settings ReplySettings) error {
	if err := validateReplyRule(settings.GroupDefaults); err != nil {
		return err
	}
	for id, rule := range settings.Groups {
		if !runtimeNumericID.MatchString(id) {
			return errors.New("回复设置中的群号无效")
		}
		if err := validateReplyRule(rule); err != nil {
			return err
		}
	}
	for id, rule := range settings.PrivateUsers {
		if !runtimeNumericID.MatchString(id) {
			return errors.New("回复设置中的私聊 QQ 号无效")
		}
		if err := validateReplyRule(rule); err != nil {
			return err
		}
	}
	return nil
}

func patchReplyRule(rule ReplyRule, update ReplySettingsUpdate, inheritedPrompt string) (ReplyRule, error) {
	seen := map[string]bool{}
	for _, field := range update.Reset {
		if seen[field] {
			return rule, errors.New("恢复字段不能重复")
		}
		seen[field] = true
		switch field {
		case "mode":
			rule.Mode = nil
		case "voice":
			rule.Voice = nil
		case "prompt":
			rule.Prompt = nil
		default:
			return rule, errors.New("只能恢复 mode、voice 或 prompt")
		}
	}
	if (seen["mode"] && update.Mode != nil) || (seen["voice"] && update.Voice != nil) || (seen["prompt"] && update.Prompt != nil) {
		return rule, errors.New("同一字段不能同时修改和恢复")
	}
	if update.Mode == nil && update.Voice == nil && update.Prompt == nil && len(update.Reset) == 0 {
		return rule, errors.New("请指定需要修改或恢复的字段")
	}
	if update.PromptAction != "" && update.PromptAction != "replace" && update.PromptAction != "append" {
		return rule, errors.New("prompt_action 须为 replace 或 append")
	}
	if update.PromptAction != "" && update.Prompt == nil {
		return rule, errors.New("修改人设时需要提供 prompt")
	}
	if update.Mode != nil {
		rule.Mode = update.Mode
	}
	if update.Voice != nil {
		rule.Voice = update.Voice
	}
	if update.Prompt != nil {
		text := *update.Prompt
		if update.PromptAction == "append" {
			base := inheritedPrompt
			if rule.Prompt != nil {
				base = *rule.Prompt
			}
			if base != "" && text != "" {
				base += "\n"
			}
			text = base + text
		}
		rule.Prompt = &text
	}
	return rule, validateReplyRule(rule)
}

func applyReplySettings(c Config, sender string, updates []ReplySettingsUpdate) (ReplySettings, error) {
	if len(updates) == 0 {
		return c.ReplySettings, errors.New("没有要保存的回复设置")
	}
	settings := c.ReplySettings
	settings.Groups = maps.Clone(settings.Groups)
	settings.PrivateUsers = maps.Clone(settings.PrivateUsers)
	if settings.Groups == nil {
		settings.Groups = map[string]ReplyRule{}
	}
	if settings.PrivateUsers == nil {
		settings.PrivateUsers = map[string]ReplyRule{}
	}
	for _, update := range updates {
		if update.Scope != "group" && len(update.GroupIDs) != 0 {
			return c.ReplySettings, errors.New("只有群设置可以指定 group_ids")
		}
		switch update.Scope {
		case "group_defaults":
			rule, err := patchReplyRule(settings.GroupDefaults, update, c.SystemPrompt)
			if err != nil {
				return c.ReplySettings, err
			}
			settings.GroupDefaults = rule
		case "private":
			rule, err := patchReplyRule(settings.PrivateUsers[sender], update, c.SystemPrompt)
			if err != nil {
				return c.ReplySettings, err
			}
			settings.PrivateUsers[sender] = rule
		case "group":
			if len(update.GroupIDs) == 0 {
				return c.ReplySettings, errors.New("请指定要修改的已开放群")
			}
			seen := map[string]bool{}
			for _, id := range update.GroupIDs {
				if !c.allows(id) || seen[id] {
					return c.ReplySettings, errors.New("目标群须为不重复的已开放群")
				}
				seen[id] = true
				inherited := c.SystemPrompt
				if settings.GroupDefaults.Prompt != nil {
					inherited = *settings.GroupDefaults.Prompt
				}
				rule, err := patchReplyRule(settings.Groups[id], update, inherited)
				if err != nil {
					return c.ReplySettings, err
				}
				settings.Groups[id] = rule
			}
		default:
			return c.ReplySettings, errors.New("设置范围须为 group_defaults、group 或 private")
		}
	}
	return settings, nil
}

var errReplyOperationApplied = errors.New("回复设置操作已执行")

func (s *Service) updateReplySettings(ctx context.Context, bot string, in runtimeToolRequest) (replySettingsResult, error) {
	if in.OperationID == "" || len(in.OperationID) > 256 || !strings.HasPrefix(in.OperationID, in.MessageID+":") {
		return replySettingsResult{}, errors.New("回复设置操作编号无效")
	}
	raw, err := json.Marshal(struct {
		Version string                `json:"version"`
		Updates []ReplySettingsUpdate `json:"updates"`
	}{in.Version, in.Updates})
	if err != nil {
		return replySettingsResult{}, err
	}
	requestHash := codeHash(string(raw))
	var duplicate Config
	c, err := s.saveConfigChecked(ctx, func(old Config) (Config, error) {
		if old.AppID != bot || in.GroupID != "" || !old.allowsPrivateChat(in.MemberID) {
			return old, errors.New("私聊管理权限已关闭")
		}
		last := old.ReplySettings.LastOperation
		if last != nil && last.Actor == in.MemberID && last.OperationID == in.OperationID {
			if last.RequestHash != requestHash {
				return old, errors.New("同一操作编号不能提交不同的修改")
			}
			duplicate = old
			return old, errReplyOperationApplied
		}
		if in.Version == "" || in.Version != sectionVersion(old, "reply-settings") {
			return old, ErrConfigConflict
		}
		settings, applyErr := applyReplySettings(old, in.MemberID, in.Updates)
		if applyErr != nil {
			return old, applyErr
		}
		settings.LastOperation = &ReplyOperation{Actor: in.MemberID, OperationID: in.OperationID, RequestHash: requestHash}
		old.ReplySettings = settings
		return old, nil
	}, "reply-settings")
	if errors.Is(err, errReplyOperationApplied) {
		return replySettingsResult{Status: "unchanged", Version: sectionVersion(duplicate, "reply-settings"), Settings: replySettingsForUser(duplicate.ReplySettings, in.MemberID)}, nil
	}
	if err != nil {
		return replySettingsResult{}, err
	}
	return replySettingsResult{Status: "saved", Version: sectionVersion(c, "reply-settings"), Settings: replySettingsForUser(c.ReplySettings, in.MemberID)}, nil
}
