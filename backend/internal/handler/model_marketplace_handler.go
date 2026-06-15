package handler

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelMarketplaceHandler struct {
	channelService *service.ChannelService
}

func NewModelMarketplaceHandler(channelService *service.ChannelService) *ModelMarketplaceHandler {
	return &ModelMarketplaceHandler{channelService: channelService}
}

type marketplaceChannelEntry struct {
	ChannelName        string                     `json:"channel_name"`
	ChannelDescription string                     `json:"channel_description"`
	ModelNote          string                     `json:"model_note,omitempty"`
	Groups             []userAvailableGroup       `json:"groups"`
	Pricing            *userSupportedModelPricing `json:"pricing"`
}

type marketplaceModelItem struct {
	Key          string                    `json:"key"`
	ModelName    string                    `json:"model_name"`
	Provider     string                    `json:"provider"`
	ChannelCount int                       `json:"channel_count"`
	Channels     []marketplaceChannelEntry `json:"channels"`
}

type marketplaceProviderFacet struct {
	Provider   string `json:"provider"`
	ModelCount int    `json:"model_count"`
}

type marketplaceGroupFacet struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Provider         string  `json:"provider"`
	ModelCount       int     `json:"model_count"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	SubscriptionType string  `json:"subscription_type"`
}

// List handles dedicated marketplace listing
// GET /api/v1/model-marketplace
func (h *ModelMarketplaceHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	search := strings.TrimSpace(c.Query("search"))
	provider := strings.TrimSpace(c.Query("provider"))
	groupIDStr := strings.TrimSpace(c.Query("group_id"))

	var groupID *int64
	if groupIDStr != "" {
		parsed, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "invalid group_id")
			return
		}
		groupID = &parsed
	}

	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	allItems := buildMarketplaceModelItems(channels)
	searchFiltered := filterMarketplaceItemsByModelName(allItems, search)
	providerFacets := collectMarketplaceProviderFacets(searchFiltered)
	providerFiltered := filterMarketplaceItemsByProvider(searchFiltered, provider)
	groupFacets := collectMarketplaceGroupFacets(providerFiltered)
	finalItems := filterMarketplaceItemsByGroup(providerFiltered, groupID)

	total := len(finalItems)
	pages := 0
	if pageSize > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	response.Success(c, gin.H{
		"items":           finalItems[start:end],
		"provider_facets": providerFacets,
		"group_facets":    groupFacets,
		"total":           total,
		"page":            page,
		"page_size":       pageSize,
		"pages":           pages,
	})
}

func buildMarketplaceModelItems(channels []service.AvailableChannel) []marketplaceModelItem {
	rows := make(map[string]*marketplaceModelItem)

	for _, ch := range channels {
		groupsByPlatform := make(map[string][]userAvailableGroup, 4)
		for _, g := range ch.Groups {
			if !isMarketplaceVisibleGroup(g) {
				continue
			}
			groupsByPlatform[g.Platform] = append(groupsByPlatform[g.Platform], userAvailableGroup{
				ID:               g.ID,
				Name:             g.Name,
				Description:      g.Description,
				Platform:         g.Platform,
				SubscriptionType: g.SubscriptionType,
				RateMultiplier:   g.RateMultiplier,
				IsExclusive:      g.IsExclusive,
			})
		}

		for _, model := range ch.SupportedModels {
			platformGroups := groupsByPlatform[model.Platform]
			if len(platformGroups) == 0 {
				continue
			}

			key := model.Platform + "::" + model.Name
			entry := marketplaceChannelEntry{
				ChannelName:        ch.Name,
				ChannelDescription: ch.Description,
				ModelNote:          model.MappingNote,
				Groups:             platformGroups,
				Pricing:            toUserPricing(model.Pricing),
			}

			if existing, ok := rows[key]; ok {
				duplicate := false
				for _, current := range existing.Channels {
					if current.ChannelName == entry.ChannelName && current.ChannelDescription == entry.ChannelDescription {
						duplicate = true
						break
					}
				}
				if !duplicate {
					existing.Channels = append(existing.Channels, entry)
				}
				continue
			}

			rows[key] = &marketplaceModelItem{
				Key:          key,
				ModelName:    model.Name,
				Provider:     model.Platform,
				ChannelCount: 1,
				Channels:     []marketplaceChannelEntry{entry},
			}
		}
	}

	out := make([]marketplaceModelItem, 0, len(rows))
	for _, item := range rows {
		sort.SliceStable(item.Channels, func(i, j int) bool {
			return strings.ToLower(item.Channels[i].ChannelName) < strings.ToLower(item.Channels[j].ChannelName)
		})
		item.ChannelCount = len(item.Channels)
		out = append(out, *item)
	}

	sort.SliceStable(out, func(i, j int) bool {
		left := strings.ToLower(out[i].ModelName)
		right := strings.ToLower(out[j].ModelName)
		if left != right {
			return left < right
		}
		return strings.ToLower(out[i].Provider) < strings.ToLower(out[j].Provider)
	})
	return out
}

// isMarketplaceVisibleGroup 决定一个分组是否能进入模型广场聚合视图。
//
// 模型广场是面向所有访问者的公共探索页，不做 per-user 过滤，因此必须排除两类
// 不属于"公开可申请"语义的分组：
//   - 订阅类型分组：通过订阅入口分发，不在通用模型展示口中暴露；
//   - 专属分组（IsExclusive）：定义上仅授权用户可见，不应在公共聚合中泄露。
func isMarketplaceVisibleGroup(group service.AvailableGroupRef) bool {
	if group.SubscriptionType == service.SubscriptionTypeSubscription {
		return false
	}
	if group.IsExclusive {
		return false
	}
	return true
}

func filterMarketplaceItemsByModelName(items []marketplaceModelItem, query string) []marketplaceModelItem {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}

	out := make([]marketplaceModelItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ModelName), query) {
			out = append(out, item)
		}
	}
	return out
}

func filterMarketplaceItemsByProvider(items []marketplaceModelItem, provider string) []marketplaceModelItem {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return items
	}

	out := make([]marketplaceModelItem, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.Provider, provider) {
			out = append(out, item)
		}
	}
	return out
}

func filterMarketplaceItemsByGroup(items []marketplaceModelItem, groupID *int64) []marketplaceModelItem {
	if groupID == nil {
		return items
	}

	out := make([]marketplaceModelItem, 0, len(items))
	for _, item := range items {
		channels := make([]marketplaceChannelEntry, 0, len(item.Channels))
		for _, ch := range item.Channels {
			include := false
			for _, group := range ch.Groups {
				if group.ID == *groupID {
					include = true
					break
				}
			}
			if include {
				channels = append(channels, ch)
			}
		}
		if len(channels) == 0 {
			continue
		}
		item.Channels = channels
		item.ChannelCount = len(channels)
		out = append(out, item)
	}
	return out
}

func collectMarketplaceProviderFacets(items []marketplaceModelItem) []marketplaceProviderFacet {
	counts := make(map[string]int, 8)
	for _, item := range items {
		counts[item.Provider]++
	}

	out := make([]marketplaceProviderFacet, 0, len(counts))
	for provider, count := range counts {
		out = append(out, marketplaceProviderFacet{
			Provider:   provider,
			ModelCount: count,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Provider) < strings.ToLower(out[j].Provider)
	})
	return out
}

func collectMarketplaceGroupFacets(items []marketplaceModelItem) []marketplaceGroupFacet {
	type aggregate struct {
		facet marketplaceGroupFacet
		keys  map[string]struct{}
	}

	aggregates := make(map[int64]*aggregate, 16)
	for _, item := range items {
		for _, ch := range item.Channels {
			for _, group := range ch.Groups {
				entry, ok := aggregates[group.ID]
				if !ok {
					entry = &aggregate{
						facet: marketplaceGroupFacet{
							ID:               group.ID,
							Name:             group.Name,
							Description:      group.Description,
							Provider:         group.Platform,
							RateMultiplier:   group.RateMultiplier,
							SubscriptionType: group.SubscriptionType,
						},
						keys: make(map[string]struct{}),
					}
					aggregates[group.ID] = entry
				}
				entry.keys[item.Key] = struct{}{}
			}
		}
	}

	out := make([]marketplaceGroupFacet, 0, len(aggregates))
	for _, entry := range aggregates {
		entry.facet.ModelCount = len(entry.keys)
		out = append(out, entry.facet)
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftProvider := strings.ToLower(out[i].Provider)
		rightProvider := strings.ToLower(out[j].Provider)
		if leftProvider != rightProvider {
			return leftProvider < rightProvider
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
