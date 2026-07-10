package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// AccountImportApplyTemplate is the saved preset used by admin and standalone
// account import flows.
type AccountImportApplyTemplate struct {
	ID                     string                      `json:"id"`
	Name                   string                      `json:"name"`
	EnableTags             bool                        `json:"enableTags"`
	EnableGroups           bool                        `json:"enableGroups"`
	EnableProxy            bool                        `json:"enableProxy"`
	EnableConcurrency      bool                        `json:"enableConcurrency"`
	EnablePriority         bool                        `json:"enablePriority"`
	EnableModelRestriction bool                        `json:"enableModelRestriction"`
	ApplyTags              []string                    `json:"applyTags"`
	ApplyGroupIDs          []int64                     `json:"applyGroupIds"`
	ApplyProxyID           *int64                      `json:"applyProxyId"`
	ApplyConcurrency       int                         `json:"applyConcurrency"`
	ApplyPriority          int                         `json:"applyPriority"`
	ModelRestrictionMode   string                      `json:"modelRestrictionMode"`
	AllowedModels          []string                    `json:"allowedModels"`
	ModelMappings          []AccountImportModelMapItem `json:"modelMappings"`
}

type AccountImportModelMapItem struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *SettingService) GetAccountImportApplyTemplates(ctx context.Context) ([]AccountImportApplyTemplate, error) {
	if s == nil || s.settingRepo == nil {
		return []AccountImportApplyTemplate{}, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountImportApplyTemplates)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []AccountImportApplyTemplate{}, nil
		}
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []AccountImportApplyTemplate{}, nil
	}
	var templates []AccountImportApplyTemplate
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return nil, err
	}
	return normalizeAccountImportApplyTemplates(templates)
}

func (s *SettingService) UpdateAccountImportApplyTemplates(ctx context.Context, templates []AccountImportApplyTemplate) ([]AccountImportApplyTemplate, error) {
	normalized, err := normalizeAccountImportApplyTemplates(templates)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if s == nil || s.settingRepo == nil {
		return normalized, nil
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAccountImportApplyTemplates, string(data)); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeAccountImportApplyTemplates(input []AccountImportApplyTemplate) ([]AccountImportApplyTemplate, error) {
	if len(input) == 0 {
		return []AccountImportApplyTemplate{}, nil
	}
	out := make([]AccountImportApplyTemplate, 0, len(input))
	for _, tpl := range input {
		tpl.ID = strings.TrimSpace(tpl.ID)
		tpl.Name = strings.TrimSpace(tpl.Name)
		if tpl.ModelRestrictionMode != "mapping" {
			tpl.ModelRestrictionMode = "whitelist"
		}
		tags, err := NormalizeAccountTags(tpl.ApplyTags)
		if err != nil {
			return nil, err
		}
		tpl.ApplyTags = tags
		tpl.AllowedModels = normalizeStringList(tpl.AllowedModels)
		mappings := make([]AccountImportModelMapItem, 0, len(tpl.ModelMappings))
		for _, item := range tpl.ModelMappings {
			item.From = strings.TrimSpace(item.From)
			item.To = strings.TrimSpace(item.To)
			if item.From == "" || item.To == "" {
				continue
			}
			mappings = append(mappings, item)
		}
		tpl.ModelMappings = mappings
		if tpl.ApplyGroupIDs == nil {
			tpl.ApplyGroupIDs = []int64{}
		}
		out = append(out, tpl)
	}
	return out, nil
}

func normalizeStringList(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
