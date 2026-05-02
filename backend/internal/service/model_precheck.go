package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

type availableModelsSnapshot struct {
	Models      []string
	Restrictive bool
}

var (
	ErrModelBlockedByGroup            = errors.New("model blocked by group restriction")
	ErrModelUnsupportedByGroupAccount = errors.New("model unsupported by group accounts")
)

func buildAvailableModelsSnapshot(accounts []Account) availableModelsSnapshot {
	modelSet := make(map[string]struct{})
	hasAnyMapping := false
	hasUnrestrictedAccount := false

	for i := range accounts {
		mapping := accounts[i].GetModelMapping()
		if len(mapping) == 0 {
			hasUnrestrictedAccount = true
			continue
		}
		hasAnyMapping = true
		for model := range mapping {
			modelSet[model] = struct{}{}
		}
	}

	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)

	return availableModelsSnapshot{
		Models:      models,
		Restrictive: hasAnyMapping && !hasUnrestrictedAccount,
	}
}

func cloneAvailableModelsSnapshot(snapshot availableModelsSnapshot) availableModelsSnapshot {
	return availableModelsSnapshot{
		Models:      cloneStringSlice(snapshot.Models),
		Restrictive: snapshot.Restrictive,
	}
}

func storeAvailableModelsSnapshot(cache *gocache.Cache, ttl time.Duration, groupID *int64, platform string, snapshot availableModelsSnapshot) {
	if cache == nil {
		return
	}
	cache.Set(modelsListCacheKey(groupID, platform), cloneAvailableModelsSnapshot(snapshot), ttl)
	modelsListCacheStoreTotal.Add(1)
}

func peekAvailableModelsSnapshot(cache *gocache.Cache, groupID *int64, platform string) (availableModelsSnapshot, bool) {
	if cache == nil {
		return availableModelsSnapshot{}, false
	}
	cached, found := cache.Get(modelsListCacheKey(groupID, platform))
	if !found {
		return availableModelsSnapshot{}, false
	}
	switch value := cached.(type) {
	case availableModelsSnapshot:
		modelsListCacheHitTotal.Add(1)
		return cloneAvailableModelsSnapshot(value), true
	case []string:
		modelsListCacheHitTotal.Add(1)
		return availableModelsSnapshot{Models: cloneStringSlice(value)}, true
	default:
		return availableModelsSnapshot{}, false
	}
}

func SnapshotSupportsRequestedModel(snapshot availableModelsSnapshot, platform, requestedModel string) bool {
	if requestedModel = strings.TrimSpace(requestedModel); requestedModel == "" {
		return true
	}
	if !snapshot.Restrictive || len(snapshot.Models) == 0 {
		return true
	}

	mapping := make(map[string]string, len(snapshot.Models))
	for _, model := range snapshot.Models {
		mapping[model] = model
	}
	if mappingSupportsRequestedModel(mapping, requestedModel) {
		return true
	}
	normalized := normalizeRequestedModelForLookup(platform, requestedModel)
	return normalized != requestedModel && mappingSupportsRequestedModel(mapping, normalized)
}

func ModelPrecheckUnavailableMessage(requestedModel string) string {
	return fmt.Sprintf("当前分组未配置支持模型 %q 的账号", requestedModel)
}

func ModelBlockedByGroupMessage(requestedModel string) string {
	return fmt.Sprintf("当前分组未开放模型 %q", requestedModel)
}
