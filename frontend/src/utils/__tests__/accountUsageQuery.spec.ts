import { describe, expect, it } from 'vitest'
import { buildUsageQueryConfig, normalizeUsageQueryProvider } from '../accountUsageQuery'

describe('accountUsageQuery', () => {
  it('builds sub2api config without duplicating account credentials', () => {
    expect(buildUsageQueryConfig('sub2api', {
      baseUrl: 'https://ignored.example/v1',
      accessToken: 'must-not-be-copied',
      userId: 'unused'
    })).toEqual({
      enabled: true,
      provider: 'sub2api'
    })
  })

  it('requires and trims all newapi fields', () => {
    expect(buildUsageQueryConfig('newapi', {
      baseUrl: ' https://newapi.example ',
      accessToken: ' token ',
      userId: ' 123 '
    })).toEqual({
      enabled: true,
      provider: 'newapi',
      base_url: 'https://newapi.example',
      access_token: 'token',
      user_id: '123'
    })
    expect(buildUsageQueryConfig('newapi', {
      baseUrl: '',
      accessToken: 'token',
      userId: '123'
    })).toBeNull()
  })

  it('normalizes unknown legacy providers to newapi', () => {
    expect(normalizeUsageQueryProvider('sub2api')).toBe('sub2api')
    expect(normalizeUsageQueryProvider('newapi')).toBe('newapi')
    expect(normalizeUsageQueryProvider('')).toBe('newapi')
  })
})
