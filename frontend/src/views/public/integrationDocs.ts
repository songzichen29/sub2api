export interface DocsEndpoints {
  root: string
  openai: string
  gemini: string
}

/** Accept a root URL or a versioned endpoint, preserving gateway path prefixes. */
export function resolveDocsEndpoints(configured: string, origin: string): DocsEndpoints {
  let url: URL
  try {
    url = new URL(configured.trim() || origin, origin)
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) {
      throw new Error('Invalid gateway URL')
    }
  } catch {
    url = new URL(origin)
  }
  url.search = ''
  url.hash = ''
  url.pathname = url.pathname.replace(/\/+$/, '').replace(/\/v1(?:beta)?$/, '')
  const root = url.toString().replace(/\/+$/, '')
  return { root, openai: root + '/v1', gemini: root + '/v1beta' }
}

export interface DocsExample {
  label: string
  code: string
  hint?: string
}

export interface DocsSection {
  id: string
  title: string
  body: string
  keywords: string
  steps?: string[]
  note?: string
  links?: { label: string; to: string }[]
  rows?: { method: string; path: string; purpose: string }[]
  examples?: DocsExample[]
  questions?: { title: string; body: string }[]
}

export function createIntegrationDocs(t: (key: string) => string, endpoints: DocsEndpoints): DocsSection[] {
  const text = (key: string) => t('docs.' + key)
  // Encode URLs as data in each example instead of interpolating unescaped settings into source.
  const quote = (value: string) => JSON.stringify(value)
  const curl = (url: string, body?: object, headers: string[] = []) => [
    'curl ' + quote(url),
    '  -H "Authorization: Bearer YOUR_API_KEY"',
    ...headers.map(header => '  -H ' + quote(header)),
    ...(body ? [
      '  -H "Content-Type: application/json"',
      "  -d '" + JSON.stringify(body, null, 2) + "'",
    ] : []),
  ].join(' \\\n')
  const modelList = curl(endpoints.openai + '/models')
  const chat = { model: 'YOUR_MODEL', messages: [{ role: 'user', content: 'Hello' }] }

  return [
    {
      id: 'overview', title: text('overview'), body: text('overviewText'), keywords: 'quick start 快速开始',
      steps: [text('startKey'), text('startModel'), text('startClient')],
      note: text('capabilitiesNote'),
      links: [{ label: text('openKeys'), to: '/keys' }, { label: text('modelList'), to: '#models' }],
      examples: [{ label: text('firstRequest'), code: modelList, hint: text('curlHint') }],
    },
    {
      id: 'keys', title: text('keysSection'), body: text('keyIntro'), keywords: 'api key token group 密钥 分组 创建 复制 修改 停用 删除',
      steps: [text('keyStepCreate'), text('keyStepConfigure'), text('keyStepSave'), text('keyStepManage')],
      note: text('keyGroupNote'),
      links: [{ label: text('openKeys'), to: '/keys' }],
    },
    {
      id: 'endpoints', title: text('endpoints'), body: text('endpointsIntro'), keywords: 'base url bearer authentication 接口 地址 认证 v1',
      note: text('baseUrlNote'),
      rows: [
        { method: 'GET', path: '/v1/models', purpose: text('listModels') },
        { method: 'POST', path: '/v1/chat/completions', purpose: 'Chat Completions' },
        { method: 'POST', path: '/v1/responses', purpose: 'Responses / Codex' },
        { method: 'POST', path: '/v1/messages', purpose: 'Anthropic Messages' },
        { method: 'POST', path: '/v1beta/models/:model:generateContent', purpose: 'Gemini generateContent' },
      ],
    },
    {
      id: 'openai', title: text('openaiProtocol'), body: text('openaiIntro'), keywords: 'openai python node javascript sdk responses chat completions',
      examples: [
        { label: 'Chat Completions · cURL', code: curl(endpoints.openai + '/chat/completions', chat), hint: text('curlHint') },
        { label: 'Responses · cURL', code: curl(endpoints.openai + '/responses', { model: 'YOUR_MODEL', input: 'Hello' }) },
        { label: 'Python', code: [
          '# pip install openai',
          'import os',
          'from openai import OpenAI',
          '',
          'client = OpenAI(',
          '    api_key=os.environ["SUB2API_API_KEY"],',
          '    base_url=' + quote(endpoints.openai) + ',',
          ')',
          '',
          'response = client.chat.completions.create(',
          '    model="YOUR_MODEL",',
          '    messages=[{"role": "user", "content": "Hello"}],',
          ')',
          'print(response.choices[0].message.content)',
        ].join('\n'), hint: text('envHint') },
        { label: 'Node.js', code: [
          '// npm install openai',
          'import OpenAI from "openai";',
          '',
          'const client = new OpenAI({',
          '  apiKey: process.env.SUB2API_API_KEY,',
          '  baseURL: ' + quote(endpoints.openai) + ',',
          '});',
          '',
          'const response = await client.chat.completions.create({',
          '  model: "YOUR_MODEL",',
          '  messages: [{ role: "user", content: "Hello" }],',
          '});',
          'console.log(response.choices[0].message.content);',
        ].join('\n'), hint: text('envHint') },
      ],
    },
    {
      id: 'claude', title: text('claudeProtocol'), body: text('claudeIntro'), keywords: 'anthropic claude code messages',
      note: text('claudeNote'),
      examples: [
        { label: 'Messages · cURL', code: curl(endpoints.openai + '/messages', { ...chat, max_tokens: 256 }, ['anthropic-version: 2023-06-01']) },
        { label: 'Claude Code · settings.json', code: JSON.stringify({
          env: { ANTHROPIC_BASE_URL: endpoints.root, ANTHROPIC_AUTH_TOKEN: 'YOUR_API_KEY', ANTHROPIC_MODEL: 'YOUR_MODEL' },
        }, null, 2), hint: text('claudeConfigHint') },
      ],
    },
    {
      id: 'gemini', title: text('geminiProtocol'), body: text('geminiIntro'), keywords: 'google gemini native generatecontent 原生',
      examples: [{ label: 'generateContent · cURL', code: curl(endpoints.gemini + '/models/YOUR_MODEL:generateContent', {
        contents: [{ role: 'user', parts: [{ text: 'Hello' }] }],
      }) }],
    },
    {
      id: 'clients', title: text('clients'), body: text('clientsIntro'), keywords: 'codex opencode continue chatbox lobechat 客户端 配置',
      note: text('clientConfigNote'),
      examples: [
        { label: 'Codex · config.toml', hint: text('codexHint'), code: [
          'model_provider = "sub2api"',
          'model = "YOUR_MODEL"',
          '',
          '[model_providers.sub2api]',
          'name = "Sub2API"',
          'base_url = ' + quote(endpoints.openai),
          'env_key = "SUB2API_API_KEY"',
          'wire_api = "responses"',
        ].join('\n') },
        { label: 'OpenCode · opencode.json', hint: text('envHint'), code: JSON.stringify({
          $schema: 'https://opencode.ai/config.json',
          providers: {
            sub2api: {
              package: '@opencode/ai/providers/openai-compatible', name: 'Sub2API',
              env: ['SUB2API_API_KEY'],
              settings: { baseURL: endpoints.openai },
              models: { YOUR_MODEL: { name: 'YOUR_MODEL' } },
            },
          },
          model: 'sub2api/YOUR_MODEL',
        }, null, 2) },
        { label: 'Continue · config.yaml', hint: text('continueHint'), code: [
          'name: Sub2API',
          'version: 1.0.0',
          'schema: v1',
          'models:',
          '  - name: Sub2API',
          '    provider: openai',
          '    model: YOUR_MODEL',
          '    apiBase: ' + quote(endpoints.openai),
          '    apiKey: YOUR_API_KEY',
        ].join('\n') },
      ],
    },
    {
      id: 'models', title: text('modelList'), body: text('availableModelsText'), keywords: 'models model id 模型 列表 能力 权限',
      note: text('modelNotFoundNote'),
      examples: [{ label: 'GET /v1/models', code: modelList }],
      links: [{ label: text('channelStatus'), to: '/monitor' }, { label: text('usageRecords'), to: '/usage' }],
    },
    {
      id: 'troubleshooting', title: text('troubleshooting'), body: text('troubleshootingIntro'), keywords: 'error 400 401 402 403 404 429 500 502 503 timeout 错误 排查 超时 余额',
      questions: ['badRequest', 'apiKeyAuth', 'permission', 'model404', 'quota429', 'timeout'].map(key => ({
        title: text(key + 'Error'), body: text(key + 'Help'),
      })),
    },
    {
      id: 'security', title: text('security'), body: text('securityIntro'), keywords: 'security secret env 安全 泄露 环境变量',
      steps: [text('securityServerOnly'), text('securityNeverShare'), text('securityRotate'), text('securitySeparate')],
    },
  ]
}

export function filterDocsSections(sections: DocsSection[], query: string): DocsSection[] {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return sections.filter(section => {
    const searchable = JSON.stringify(section).toLocaleLowerCase()
    return terms.every(term => searchable.includes(term))
  })
}
