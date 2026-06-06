export interface AccountTestEvent {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
  image_url?: string
  mime_type?: string
}

export interface RunAccountConnectionTestOptions {
  accountId: number
  authToken?: string | null
  modelId?: string
  prompt?: string
  mode?: string
  signal?: AbortSignal
  onEvent?: (event: AccountTestEvent) => void
}

export interface AccountConnectionTestResult {
  success: boolean
  error?: string
}

const buildRequestBody = (options: RunAccountConnectionTestOptions) => {
  const body: Record<string, string> = {}
  if (options.modelId) body.model_id = options.modelId
  if (options.prompt) body.prompt = options.prompt
  if (options.mode) body.mode = options.mode
  return JSON.stringify(body)
}

export async function runAccountConnectionTest(
  options: RunAccountConnectionTestOptions
): Promise<AccountConnectionTestResult> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  }
  if (options.authToken) {
    headers.Authorization = `Bearer ${options.authToken}`
  }

  const response = await fetch(`/api/v1/admin/accounts/${options.accountId}/test`, {
    method: 'POST',
    headers,
    body: buildRequestBody(options),
    signal: options.signal
  })

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const reader = response.body?.getReader()
  if (!reader) {
    throw new Error('No response body')
  }

  const decoder = new TextDecoder()
  let buffer = ''
  let completed = false
  let result: AccountConnectionTestResult = { success: false, error: 'Test did not complete' }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      const jsonStr = line.slice(6).trim()
      if (!jsonStr) continue
      try {
        const event = JSON.parse(jsonStr) as AccountTestEvent
        options.onEvent?.(event)
        if (event.type === 'test_complete') {
          completed = true
          result = event.success
            ? { success: true }
            : { success: false, error: event.error || 'Test failed' }
        } else if (event.type === 'error') {
          completed = true
          result = { success: false, error: event.error || 'Unknown error' }
        }
      } catch (error) {
        console.error('Failed to parse SSE event:', error)
      }
    }
  }

  if (!completed) {
    return { success: false, error: result.error }
  }
  return result
}
