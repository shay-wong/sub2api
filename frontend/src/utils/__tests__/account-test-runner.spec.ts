import { afterEach, describe, expect, it, vi } from 'vitest'
import { runAccountConnectionTest } from '../accountTestRunner'

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

describe('runAccountConnectionTest', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('posts to the account test SSE endpoint and reports completion', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"claude-sonnet"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const events: Array<{ type: string; text?: string }> = []
    const result = await runAccountConnectionTest({
      accountId: 12,
      authToken: 'admin-token',
      onEvent: (event) => events.push(event)
    })

    expect(result).toEqual({ success: true })
    expect(global.fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/12/test', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({
        Authorization: 'Bearer admin-token',
        'Content-Type': 'application/json',
        'X-Admin-UI-Request': '1'
      }),
      body: '{}'
    }))
    expect(events.map(event => event.type)).toEqual(['test_start', 'content', 'test_complete'])
  })

  it('returns a failure when the stream emits an error event', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"error","error":"token expired"}\n'
      ])
    ) as any

    const result = await runAccountConnectionTest({
      accountId: 3,
      authToken: 'admin-token'
    })

    expect(result).toEqual({ success: false, error: 'token expired' })
  })

  it('preserves the selected project context for raw fetch streaming requests', async () => {
    localStorage.setItem('sub2api_selected_project_id', '169')
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    await runAccountConnectionTest({
      accountId: 5837,
      authToken: 'admin-token'
    })

    expect(global.fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/5837/test', expect.objectContaining({
      headers: expect.objectContaining({
        'X-Project-ID': '169'
      })
    }))
  })
})
