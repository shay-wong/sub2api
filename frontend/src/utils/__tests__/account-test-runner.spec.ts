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

  it('forwards optional Grok media inputs and media output events', async () => {
    localStorage.setItem('sub2api_selected_project_id', '169')
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"audio","audio_url":"data:audio/wav;base64,T0s=","mime_type":"audio/wav"}\n',
        'data: {"type":"video","video_url":"https://example.test/video.mp4","mime_type":"video/mp4"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
    const events: Array<{ type: string; audio_url?: string; video_url?: string }> = []

    await runAccountConnectionTest({
      accountId: 13,
      authToken: 'admin-token',
      mode: 'stt',
      imageDataURL: 'data:image/png;base64,SU1H',
      audioDataURL: 'data:audio/wav;base64,QVVESU8=',
      onEvent: (event) => events.push(event)
    })

    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      mode: 'stt',
      image_data_url: 'data:image/png;base64,SU1H',
      audio_data_url: 'data:audio/wav;base64,QVVESU8='
    })
    expect(request.headers).toMatchObject({ 'X-Project-ID': '169' })
    expect(events).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'audio', audio_url: 'data:audio/wav;base64,T0s=' }),
      expect.objectContaining({ type: 'video', video_url: 'https://example.test/video.mp4' })
    ]))
  })
})
