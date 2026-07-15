import { createRequire } from 'node:module'
import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const { createI18n } = createRequire(import.meta.url)('vue-i18n/dist/vue-i18n.cjs') as typeof import('vue-i18n')

const compiler = createI18n({
  legacy: false,
  locale: 'probe',
  missingWarn: false,
  fallbackWarn: false,
  messages: { probe: { value: '' } }
})

// vue-i18n 在运行时才编译消息：文案里未转义的花括号（如内嵌 JSON 示例
// "{\"user-agent\": ...}"）会在渲染时抛 "Invalid token in placeholder"，
// 直接炸掉整个组件树，且构建期完全无感。本测试把全部文案预编译一遍，
// 将该类问题固化为显式失败。字面量花括号请用 {'{'} / {'}'} 转义，
// 或将语言中立的示例文本（如 JSON）移出 i18n。
function collectCompileErrors(node: unknown, path: string, out: string[]): void {
  if (typeof node === 'string') {
    const reported: string[] = []
    const originalConsoleError = console.error
    const originalConsoleWarn = console.warn
    console.error = (...args: unknown[]) => reported.push(args.map(String).join(' '))
    console.warn = () => {}
    try {
      compiler.global.setLocaleMessage('probe', { value: node })
      compiler.global.t('value')
    } catch (err) {
      reported.push(err instanceof Error ? err.message : String(err))
    } finally {
      console.error = originalConsoleError
      console.warn = originalConsoleWarn
    }
    if (reported.length > 0) {
      out.push(`${path}: ${reported.join('; ')}`)
    }
    return
  }
  if (Array.isArray(node)) {
    node.forEach((item, index) => collectCompileErrors(item, `${path}[${index}]`, out))
    return
  }
  if (node && typeof node === 'object') {
    for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
      collectCompileErrors(value, path ? `${path}.${key}` : key, out)
    }
  }
}

describe('locale messages compile', () => {
  it('detects invalid placeholder syntax', () => {
    const errors: string[] = []
    collectCompileErrors('{"invalid": true}', 'probe', errors)
    expect(errors).not.toEqual([])
  })

  it.each([
    ['zh', zh],
    ['en', en]
  ] as const)('%s messages all compile without placeholder errors', (locale, messages) => {
    const errors: string[] = []
    collectCompileErrors(messages, locale, errors)
    expect(errors).toEqual([])
  })
})
