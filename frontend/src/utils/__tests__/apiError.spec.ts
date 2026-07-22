import { beforeAll, describe, expect, it } from 'vitest'

import { i18n, loadLocaleMessages } from '@/i18n'
import { extractApiErrorMessage } from '@/utils/apiError'

describe('apiError', () => {
  beforeAll(async () => {
    await Promise.all([loadLocaleMessages('en'), loadLocaleMessages('zh')])
  })

  it('maps model_not_allowed to localized zh message', async () => {
    i18n.global.locale.value = 'zh'

    const message = extractApiErrorMessage({
      reason: 'model_not_allowed',
      message: 'fallback raw message'
    }, 'fallback')

    expect(message).toBe('当前分组未开放该模型')
  })

  it('maps model_not_configured to localized en message', async () => {
    i18n.global.locale.value = 'en'

    const message = extractApiErrorMessage({
      reason: 'model_not_configured',
      message: 'fallback raw message'
    }, 'fallback')

    expect(message).toBe('No account in the current group is configured to support this model')
  })
})
