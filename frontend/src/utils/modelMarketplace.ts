import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserSupportedModelPricing,
} from '@/api/channels'
import type { Channel as AdminChannel, ChannelModelPricing } from '@/api/admin/channels'
import type { AdminGroup } from '@/types'

export interface ModelMarketplaceChannelEntry {
  channel_name: string
  channel_description: string
  model_note?: string
  groups: UserAvailableGroup[]
  pricing: UserSupportedModelPricing | null
}

export interface ModelMarketplaceRow {
  model_name: string
  platform: string
  channels: ModelMarketplaceChannelEntry[]
}

export interface MarketplaceProviderFacet {
  provider: string
  model_count: number
}

export interface MarketplaceGroupFacet {
  id: number
  name: string
  description: string
  provider: string
  model_count: number
  rate_multiplier: number
  subscription_type: string
}

export interface MarketplaceModelItem {
  key: string
  model_name: string
  provider: string
  channel_count: number
  channels: ModelMarketplaceChannelEntry[]
}

function normalize(value: string): string {
  return value.trim().toLowerCase()
}

function hasPlatformMatch(platform: string, selectedPlatform: string): boolean {
  const filter = normalize(selectedPlatform)
  if (!filter) return true
  return normalize(platform) === filter
}

export function collectMarketplacePlatforms(channels: UserAvailableChannel[]): string[] {
  return Array.from(
    new Set(channels.flatMap((channel) => channel.platforms.map((section) => section.platform))),
  ).sort((left, right) => left.localeCompare(right))
}

export function buildModelMarketplaceRows(channels: UserAvailableChannel[]): ModelMarketplaceRow[] {
  const rows = new Map<string, ModelMarketplaceRow>()

  for (const channel of channels) {
    for (const section of channel.platforms) {
      for (const model of section.supported_models) {
        const key = `${section.platform}::${model.name}`
        const existing = rows.get(key)
        const channelEntry: ModelMarketplaceChannelEntry = {
          channel_name: channel.name,
          channel_description: channel.description,
          groups: section.groups,
          pricing: model.pricing,
        }

        if (!existing) {
          rows.set(key, {
            model_name: model.name,
            platform: section.platform,
            channels: [channelEntry],
          })
          continue
        }

        const duplicate = existing.channels.some(
          (entry) =>
            entry.channel_name === channelEntry.channel_name &&
            entry.channel_description === channelEntry.channel_description,
        )
        if (!duplicate) {
          existing.channels.push(channelEntry)
        }
      }
    }
  }

  return Array.from(rows.values())
    .map((row) => ({
      ...row,
      channels: [...row.channels].sort((left, right) => left.channel_name.localeCompare(right.channel_name)),
    }))
    .sort((left, right) => {
      const modelOrder = left.model_name.localeCompare(right.model_name)
      if (modelOrder !== 0) return modelOrder
      return left.platform.localeCompare(right.platform)
    })
}

export function filterAvailableChannels(
  channels: UserAvailableChannel[],
  searchQuery: string,
  selectedPlatform: string,
): UserAvailableChannel[] {
  const query = normalize(searchQuery)

  return channels
    .map((channel) => {
      const platformScopedSections = channel.platforms.filter((section) =>
        hasPlatformMatch(section.platform, selectedPlatform),
      )
      if (platformScopedSections.length === 0) return null

      if (!query) {
        return { ...channel, platforms: platformScopedSections }
      }

      const nameHit = normalize(channel.name).includes(query)
      const descriptionHit = normalize(channel.description || '').includes(query)
      if (nameHit || descriptionHit) {
        return { ...channel, platforms: platformScopedSections }
      }

      const matchingSections = platformScopedSections.filter(
        (section) =>
          normalize(section.platform).includes(query) ||
          section.groups.some((group) => normalize(group.name).includes(query)) ||
          section.supported_models.some((model) => normalize(model.name).includes(query)),
      )

      if (matchingSections.length === 0) return null

      return { ...channel, platforms: matchingSections }
    })
    .filter((channel): channel is UserAvailableChannel => channel !== null)
}

export function filterModelMarketplaceRows(
  rows: ModelMarketplaceRow[],
  searchQuery: string,
  selectedPlatform: string,
): ModelMarketplaceRow[] {
  const query = normalize(searchQuery)

  return rows
    .filter((row) => hasPlatformMatch(row.platform, selectedPlatform))
    .map((row) => {
      if (!query) return row

      const modelHit =
        normalize(row.model_name).includes(query) || normalize(row.platform).includes(query)
      if (modelHit) return row

      const matchingChannels = row.channels.filter(
        (channel) =>
          normalize(channel.channel_name).includes(query) ||
          normalize(channel.channel_description || '').includes(query) ||
          channel.groups.some((group) => normalize(group.name).includes(query)),
      )

      if (matchingChannels.length === 0) return null

      return {
        ...row,
        channels: matchingChannels,
      }
    })
    .filter((row): row is ModelMarketplaceRow => row !== null)
}

export function buildMarketplaceModelItems(channels: UserAvailableChannel[]): MarketplaceModelItem[] {
  return buildModelMarketplaceRows(channels).map((row) => ({
    key: `${row.platform}::${row.model_name}`,
    model_name: row.model_name,
    provider: row.platform,
    channel_count: row.channels.length,
    channels: row.channels,
  }))
}

export function collectMarketplaceProviderFacets(items: MarketplaceModelItem[]): MarketplaceProviderFacet[] {
  const counts = new Map<string, number>()
  for (const item of items) {
    counts.set(item.provider, (counts.get(item.provider) ?? 0) + 1)
  }

  return Array.from(counts.entries())
    .map(([provider, model_count]) => ({ provider, model_count }))
    .sort((left, right) => left.provider.localeCompare(right.provider))
}

export function collectMarketplaceGroupFacets(items: MarketplaceModelItem[]): MarketplaceGroupFacet[] {
  const facets = new Map<number, MarketplaceGroupFacet & { keys: Set<string> }>()

  for (const item of items) {
    for (const channel of item.channels) {
      for (const group of channel.groups) {
        const existing = facets.get(group.id)
        if (existing) {
          existing.keys.add(item.key)
          continue
        }

        facets.set(group.id, {
          id: group.id,
          name: group.name,
          description: group.description || '',
          provider: group.platform,
          model_count: 0,
          rate_multiplier: group.rate_multiplier,
          subscription_type: group.subscription_type,
          keys: new Set([item.key]),
        })
      }
    }
  }

  return Array.from(facets.values())
    .map(({ keys, ...facet }) => ({
      ...facet,
      model_count: keys.size,
    }))
    .sort((left, right) => {
      const providerOrder = left.provider.localeCompare(right.provider)
      if (providerOrder !== 0) return providerOrder
      return left.name.localeCompare(right.name)
    })
}

export function filterMarketplaceModelItems(
  items: MarketplaceModelItem[],
  searchQuery: string,
  selectedProvider: string,
  selectedGroupID?: number | null,
): MarketplaceModelItem[] {
  const query = normalize(searchQuery)
  const providerFilter = normalize(selectedProvider)
  const groupFilter = selectedGroupID ?? null

  return items
    .filter((item) => !providerFilter || normalize(item.provider) === providerFilter)
    .map((item) => {
      const groupScopedChannels =
        groupFilter == null
          ? item.channels
          : item.channels.filter((channel) =>
              channel.groups.some((group) => group.id === groupFilter),
            )

      if (groupScopedChannels.length === 0) return null
      if (query && !normalize(item.model_name).includes(query)) return null

      return {
        ...item,
        channel_count: groupScopedChannels.length,
        channels: groupScopedChannels,
      }
    })
    .filter((item): item is MarketplaceModelItem => item !== null)
}

function toUserSupportedModelPricing(pricing: ChannelModelPricing): UserSupportedModelPricing {
  return {
    billing_mode: pricing.billing_mode,
    input_price: pricing.input_price,
    output_price: pricing.output_price,
    cache_write_price: pricing.cache_write_price,
    cache_read_price: pricing.cache_read_price,
    image_input_price: pricing.image_input_price,
    image_output_price: pricing.image_output_price,
    per_request_price: pricing.per_request_price,
    intervals: pricing.intervals.map((interval) => ({
      min_tokens: interval.min_tokens,
      max_tokens: interval.max_tokens,
      tier_label: interval.tier_label,
      input_price: interval.input_price,
      output_price: interval.output_price,
      cache_write_price: interval.cache_write_price,
      cache_read_price: interval.cache_read_price,
      per_request_price: interval.per_request_price,
    })),
  }
}

export function transformAdminChannelsToAvailableChannels(
  channels: AdminChannel[],
  groups: AdminGroup[],
): UserAvailableChannel[] {
  const groupsById = new Map(groups.map((group) => [group.id, group]))

  return channels
    .map((channel) => {
      const groupedByPlatform = new Map<
        string,
        {
          groups: UserAvailableGroup[]
          supported_models: { name: string; platform: string; pricing: UserSupportedModelPricing | null }[]
        }
      >()

      for (const groupID of channel.group_ids || []) {
        const group = groupsById.get(groupID)
        if (!group) continue

        const existing = groupedByPlatform.get(group.platform) ?? {
          groups: [],
          supported_models: [],
        }

        existing.groups.push({
          id: group.id,
          name: group.name,
          description: group.description || '',
          platform: group.platform,
          subscription_type: group.subscription_type,
          rate_multiplier: group.rate_multiplier,
          peak_rate_enabled: group.peak_rate_enabled === true,
          peak_start: group.peak_start || '',
          peak_end: group.peak_end || '',
          peak_rate_multiplier: group.peak_rate_multiplier ?? group.rate_multiplier,
          is_exclusive: group.is_exclusive,
        })
        groupedByPlatform.set(group.platform, existing)
      }

      for (const pricing of channel.model_pricing || []) {
        const existing = groupedByPlatform.get(pricing.platform) ?? {
          groups: [],
          supported_models: [],
        }
        for (const modelName of pricing.models || []) {
          existing.supported_models.push({
            name: modelName,
            platform: pricing.platform,
            pricing: toUserSupportedModelPricing(pricing),
          })
        }
        groupedByPlatform.set(pricing.platform, existing)
      }

      const platforms = Array.from(groupedByPlatform.entries())
        .map(([platform, section]) => ({
          platform,
          groups: section.groups.sort((left, right) => left.name.localeCompare(right.name)),
          supported_models: section.supported_models.sort((left, right) => left.name.localeCompare(right.name)),
        }))
        .sort((left, right) => left.platform.localeCompare(right.platform))

      return {
        name: channel.name,
        description: channel.description,
        platforms,
      }
    })
    .filter((channel) => channel.platforms.length > 0)
    .sort((left, right) => left.name.localeCompare(right.name))
}
