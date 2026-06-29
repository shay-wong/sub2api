const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

function normalizePath(path: string): string {
  return path.startsWith('/') ? path : `/${path}`
}

export function getAPIBaseURL(): string {
  return String(API_BASE_URL || '/api/v1')
}

export function buildApiUrl(path: string): string {
  const base = getAPIBaseURL().replace(/\/+$/, '')
  let suffix = normalizePath(path)
  if (suffix === '/api/v1') {
    suffix = ''
  } else if (suffix.startsWith('/api/v1/')) {
    suffix = suffix.slice('/api/v1'.length)
  }
  return `${base}${suffix}`
}

interface BuildGatewayUrlOptions {
  origin?: string
}

export function buildGatewayUrl(path: string, options: BuildGatewayUrlOptions = {}): string {
  const suffix = normalizePath(path)
  const apiBaseURL = getAPIBaseURL()
  const baseOrigin =
    typeof window === 'undefined'
      ? options.origin || 'http://localhost'
      : window.location.origin

  try {
    const apiBase = new URL(apiBaseURL, baseOrigin)
    const gatewayBasePath = apiBase.pathname.replace(/\/api\/v1\/?$/, '').replace(/\/+$/, '')
    return `${apiBase.origin}${gatewayBasePath}${suffix}`
  } catch {
    return suffix
  }
}
