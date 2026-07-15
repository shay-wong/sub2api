import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('group rate limit locale keys', () => {
  it('exposes the user window copy at the runtime zh path', () => {
    expect(zh.admin.users).toMatchObject({
      resetsAt: '重置于 {time}',
      groupRateLimits: '分组 5 小时窗口',
      groupRateLimitUsage: '{usage} / {limit}',
      groupRateLimitReset: '重置',
      groupRateLimitResetSuccess: '分组 5 小时窗口已重置',
      groupRateLimitResetFailed: '重置分组 5 小时窗口失败',
      groupRateLimitLoadFailed: '加载分组 5 小时窗口失败',
      groupRateLimitNoActiveWindow: '暂无活跃窗口',
      groupRateLimitUnlimited: '不限制'
    })
  })

  it('exposes the user window copy at the runtime en path', () => {
    expect(en.admin.users).toMatchObject({
      resetsAt: 'Resets at {time}',
      groupRateLimits: 'Group 5h Windows',
      groupRateLimitUsage: '{usage} / {limit}',
      groupRateLimitReset: 'Reset',
      groupRateLimitResetSuccess: 'Group 5h window reset',
      groupRateLimitResetFailed: 'Failed to reset group 5h window',
      groupRateLimitLoadFailed: 'Failed to load group 5h windows',
      groupRateLimitNoActiveWindow: 'No active window',
      groupRateLimitUnlimited: 'Unlimited'
    })
  })
})
