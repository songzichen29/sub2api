import { describe, expect, it } from 'vitest'
import type { UserAvailableChannel } from '@/api/channels'
import type { Channel as AdminChannel } from '@/api/admin/channels'
import type { AdminGroup } from '@/types'
import {
  buildMarketplaceModelItems,
  buildModelMarketplaceRows,
  collectMarketplaceProviderFacets,
  collectMarketplacePlatforms,
  filterAvailableChannels,
  filterMarketplaceModelItems,
  filterModelMarketplaceRows,
  transformAdminChannelsToAvailableChannels,
} from '@/utils/modelMarketplace'

const sampleChannels: UserAvailableChannel[] = [
  {
    name: 'Alpha',
    description: 'Primary OpenAI channel',
    platforms: [
      {
        platform: 'openai',
        groups: [
          {
            id: 1,
            name: 'default-openai',
            platform: 'openai',
            subscription_type: 'standard',
            rate_multiplier: 1,
            is_exclusive: false,
          },
        ],
        supported_models: [
          {
            name: 'gpt-5.4',
            platform: 'openai',
            pricing: {
              billing_mode: 'token',
              input_price: 0.000001,
              output_price: 0.000002,
              cache_write_price: null,
              cache_read_price: null,
              image_output_price: null,
              per_request_price: null,
              intervals: [],
            },
          },
        ],
      },
    ],
  },
  {
    name: 'Beta',
    description: 'Backup OpenAI channel',
    platforms: [
      {
        platform: 'openai',
        groups: [
          {
            id: 2,
            name: 'vip-openai',
            platform: 'openai',
            subscription_type: 'subscription',
            rate_multiplier: 1.2,
            is_exclusive: true,
          },
        ],
        supported_models: [
          {
            name: 'gpt-5.4',
            platform: 'openai',
            pricing: {
              billing_mode: 'per_request',
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              image_output_price: null,
              per_request_price: 0.5,
              intervals: [],
            },
          },
        ],
      },
      {
        platform: 'anthropic',
        groups: [
          {
            id: 3,
            name: 'claude',
            platform: 'anthropic',
            subscription_type: 'standard',
            rate_multiplier: 1,
            is_exclusive: false,
          },
        ],
        supported_models: [
          {
            name: 'gpt-5.4',
            platform: 'anthropic',
            pricing: null,
          },
        ],
      },
    ],
  },
]

const adminGroups: AdminGroup[] = [
  {
    id: 10,
    name: 'openai-public',
    description: null,
    platform: 'openai',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '',
    updated_at: '',
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: false,
    sort_order: 1,
  },
]

const adminChannels: AdminChannel[] = [
  {
    id: 1,
    name: 'Admin Alpha',
    description: 'Admin visible channel',
    status: 'active',
    billing_model_source: 'requested',
    restrict_models: false,
    features_config: {},
    group_ids: [10],
    model_pricing: [
      {
        platform: 'openai',
        models: ['gpt-5.4'],
        billing_mode: 'token',
        input_price: 0.000001,
        output_price: 0.000002,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [],
      },
    ],
    model_mapping: {},
    apply_pricing_to_account_stats: false,
    account_stats_pricing_rules: [],
    created_at: '',
    updated_at: '',
  },
]

describe('modelMarketplace utils', () => {
  it('collectMarketplacePlatforms returns sorted unique platform names', () => {
    expect(collectMarketplacePlatforms(sampleChannels)).toEqual(['anthropic', 'openai'])
  })

  it('buildModelMarketplaceRows aggregates same platform/model across channels only', () => {
    const rows = buildModelMarketplaceRows(sampleChannels)

    expect(rows).toHaveLength(2)
    expect(rows.find((row) => row.platform === 'openai' && row.model_name === 'gpt-5.4')?.channels).toHaveLength(2)
    expect(rows.find((row) => row.platform === 'anthropic' && row.model_name === 'gpt-5.4')?.channels).toHaveLength(1)
  })

  it('filterAvailableChannels keeps full channel on channel-name hit and respects platform filter', () => {
    const byChannelName = filterAvailableChannels(sampleChannels, 'alpha', '')
    expect(byChannelName).toHaveLength(1)
    expect(byChannelName[0].platforms).toHaveLength(1)

    const byPlatform = filterAvailableChannels(sampleChannels, '', 'anthropic')
    expect(byPlatform).toHaveLength(1)
    expect(byPlatform[0].name).toBe('Beta')
    expect(byPlatform[0].platforms).toHaveLength(1)
    expect(byPlatform[0].platforms[0].platform).toBe('anthropic')
  })

  it('filterModelMarketplaceRows can match channel/group search and trim non-matching channels', () => {
    const rows = buildModelMarketplaceRows(sampleChannels)
    const filtered = filterModelMarketplaceRows(rows, 'vip-openai', '')

    expect(filtered).toHaveLength(1)
    expect(filtered[0].platform).toBe('openai')
    expect(filtered[0].channels).toHaveLength(1)
    expect(filtered[0].channels[0].channel_name).toBe('Beta')
  })

  it('transformAdminChannelsToAvailableChannels converts admin channel data into marketplace rows', () => {
    const transformed = transformAdminChannelsToAvailableChannels(adminChannels, adminGroups)

    expect(transformed).toHaveLength(1)
    expect(transformed[0].platforms).toHaveLength(1)
    expect(transformed[0].platforms[0].platform).toBe('openai')
    expect(transformed[0].platforms[0].groups[0].name).toBe('openai-public')
    expect(transformed[0].platforms[0].supported_models[0].name).toBe('gpt-5.4')
  })

  it('buildMarketplaceModelItems and facets preserve grouped channels by provider', () => {
    const items = buildMarketplaceModelItems(sampleChannels)
    const facets = collectMarketplaceProviderFacets(items)

    expect(items).toHaveLength(2)
    expect(items.find((item) => item.provider === 'openai')?.channel_count).toBe(2)
    expect(facets).toEqual([
      { provider: 'anthropic', model_count: 1 },
      { provider: 'openai', model_count: 1 },
    ])
  })

  it('filterMarketplaceModelItems can filter by provider and model keyword', () => {
    const items = buildMarketplaceModelItems(sampleChannels)

    expect(filterMarketplaceModelItems(items, '', 'anthropic')).toHaveLength(1)

    const byModelKeyword = filterMarketplaceModelItems(items, 'gpt-5.4', '')
    expect(byModelKeyword).toHaveLength(2)
    expect(byModelKeyword.find((item) => item.provider === 'openai')?.channels).toHaveLength(2)
  })
})
