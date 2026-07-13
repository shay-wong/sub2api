import type { OpenAIEndpointCapability } from '@/types'

const defaultCapabilities: OpenAIEndpointCapability[] = ['chat_completions', 'embeddings']

export function normalizeOpenAIEndpointCapabilities(
  values: OpenAIEndpointCapability[]
): OpenAIEndpointCapability[] {
  const allowed: OpenAIEndpointCapability[] = [...defaultCapabilities, 'alpha_search']
  const selected = allowed.filter((value) => values.includes(value))
  return selected.length > 0 ? selected : [...defaultCapabilities]
}

export function inferOpenAIEndpointCapabilities(baseURL: unknown): OpenAIEndpointCapability[] {
  const raw = typeof baseURL === 'string' ? baseURL.trim() : ''
  const effectiveURL = raw || 'https://api.openai.com'
  try {
    const parsed = new URL(effectiveURL)
    if (parsed.hostname.toLowerCase() === 'api.openai.com') {
      return [...defaultCapabilities, 'alpha_search']
    }
  } catch {
    // Invalid custom URLs are not assumed to support optional endpoints.
  }
  return [...defaultCapabilities]
}

export function hasExplicitOpenAIEndpointCapabilities(
  credentials?: Record<string, unknown>
): boolean {
  return credentials?.openai_capabilities !== undefined && credentials.openai_capabilities !== null
}

export function readOpenAIEndpointCapabilities(
  credentials: Record<string, unknown> | undefined,
  baseURL: unknown
): OpenAIEndpointCapability[] {
  if (!hasExplicitOpenAIEndpointCapabilities(credentials)) {
    return inferOpenAIEndpointCapabilities(baseURL)
  }

  const raw = credentials?.openai_capabilities
  if (Array.isArray(raw)) {
    return normalizeOpenAIEndpointCapabilities(
      raw.filter((value): value is OpenAIEndpointCapability =>
        value === 'chat_completions' || value === 'embeddings' || value === 'alpha_search'
      )
    )
  }
  if (raw !== null && typeof raw === 'object') {
    const capabilityMap = raw as Record<string, unknown>
    return normalizeOpenAIEndpointCapabilities(
      (['chat_completions', 'embeddings', 'alpha_search'] as OpenAIEndpointCapability[]).filter(
        (value) => capabilityMap[value] === true
      )
    )
  }
  return [...defaultCapabilities]
}
