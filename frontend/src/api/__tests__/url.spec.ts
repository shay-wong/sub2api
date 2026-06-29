import axios from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'

async function loadUrlModule(apiBaseUrl?: string) {
  vi.resetModules()
  vi.unstubAllEnvs()
  if (apiBaseUrl !== undefined) {
    vi.stubEnv('VITE_API_BASE_URL', apiBaseUrl)
  }
  return import('../url')
}

describe('api url helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('builds API URLs under the configured api base', async () => {
    const { buildApiUrl } = await loadUrlModule('https://api.example.com/prefix/api/v1')

    expect(buildApiUrl('/admin/accounts/1/test')).toBe('https://api.example.com/prefix/api/v1/admin/accounts/1/test')
    expect(buildApiUrl('/api/v1/admin/accounts/1/test')).toBe('https://api.example.com/prefix/api/v1/admin/accounts/1/test')
  })

  it('builds gateway URLs at the configured deployment root', async () => {
    const { buildGatewayUrl } = await loadUrlModule('https://api.example.com/prefix/api/v1')

    expect(buildGatewayUrl('/setup/status')).toBe('https://api.example.com/prefix/setup/status')
    expect(buildGatewayUrl('/v1/usage')).toBe('https://api.example.com/prefix/v1/usage')
    expect(buildGatewayUrl('/api/v1/admin/ops/ws/qps')).toBe('https://api.example.com/prefix/api/v1/admin/ops/ws/qps')
  })

  it('preserves relative deployment prefixes', async () => {
    const { buildGatewayUrl } = await loadUrlModule('/sub2api/api/v1')

    expect(buildGatewayUrl('/setup/status')).toBe('http://localhost:3000/sub2api/setup/status')
  })

  it('preserves relative deployment prefixes without a browser origin', async () => {
    const { buildGatewayUrl } = await loadUrlModule('/sub2api/api/v1')
    vi.stubGlobal('window', undefined)

    expect(buildGatewayUrl('/setup/status')).toBe('http://localhost/sub2api/setup/status')
  })

  it('builds an absolute setup client base URL without a browser origin', async () => {
    const { buildGatewayUrl } = await loadUrlModule('/sub2api/api/v1')
    vi.stubGlobal('window', undefined)

    const setupBaseURL = buildGatewayUrl('/').replace(/\/+$/, '')
    const setupClient = axios.create({ baseURL: setupBaseURL })

    expect(setupClient.getUri({ url: '/setup/status' })).toBe('http://localhost/sub2api/setup/status')
  })

  it('uses an explicit origin for no-window callers that know the request origin', async () => {
    const { buildGatewayUrl } = await loadUrlModule('/sub2api/api/v1')
    vi.stubGlobal('window', undefined)

    expect(buildGatewayUrl('/setup/status', { origin: 'https://tenant.example' })).toBe(
      'https://tenant.example/sub2api/setup/status'
    )
  })

  it('keeps the default same-origin root when no prefix is configured', async () => {
    const { buildGatewayUrl } = await loadUrlModule()

    expect(buildGatewayUrl('/setup/status')).toBe('http://localhost:3000/setup/status')
  })
})
