import { readFileSync, readdirSync } from 'node:fs'
import { extname, join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const sourceRoot = join(process.cwd(), 'src')
const localeRoot = join(sourceRoot, 'i18n', 'locales')

function leafKeys(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object') return prefix ? [prefix] : []

  return Object.entries(value).flatMap(([key, child]) =>
    leafKeys(child, prefix ? `${prefix}.${key}` : key)
  )
}

function valueAt(source: unknown, key: string): unknown {
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[part]
  }, source)
}

function placeholders(value: unknown): string[] {
  if (typeof value !== 'string') return []
  return [...new Set([...value.matchAll(/\{([\w]+)\}/g)].map((match) => match[1]))].sort()
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (path === localeRoot || entry.name === '__tests__') return []
      return sourceFiles(path)
    }
    if (!['.ts', '.vue'].includes(extname(entry.name))) return []
    if (/\.(?:spec|test)\.ts$/.test(entry.name)) return []
    return [path]
  })
}

function staticLocaleReferences(): Map<string, string[]> {
  const references = new Map<string, string[]>()
  const patterns = [
    /(?:^|[^\w$])(?:i18n\.global\.t|\$t|t)\(\s*(['"])([^'"]+)\1/gm,
    /(?:^|[^\w$])(?:i18n\.global\.t|\$t|t)\(\s*(`)([^`$]+)\1/gm,
    /\b(?:titleKey|descriptionKey|messageKey|hintKey|labelKey)\s*:\s*(['"])([^'"]+\.[^'"]+)\1/gm
  ]

  for (const file of sourceFiles(sourceRoot)) {
    const source = readFileSync(file, 'utf8')
    for (const pattern of patterns) {
      for (const match of source.matchAll(pattern)) {
        const key = match[2]
        const locations = references.get(key) ?? []
        locations.push(relative(process.cwd(), file))
        references.set(key, locations)
      }
    }
  }
  return references
}

function dynamicLocalePrefixes(): Map<string, string[]> {
  const references = new Map<string, string[]>()
  const patterns = [
    /(?:^|[^\w$])(?:i18n\.global\.t|\$t|t)\(\s*`([^`$]*)\$\{/gm,
    /(?:^|[^\w$])(?:i18n\.global\.t|\$t|t)\(\s*(['"])([^'"]*\.)\1\s*\+/gm
  ]

  for (const file of sourceFiles(sourceRoot)) {
    const source = readFileSync(file, 'utf8')
    for (const [index, pattern] of patterns.entries()) {
      for (const match of source.matchAll(pattern)) {
        const prefix = match[index === 0 ? 1 : 2]
        const locations = references.get(prefix) ?? []
        locations.push(relative(process.cwd(), file))
        references.set(prefix, locations)
      }
    }
  }
  return references
}

describe('locale completeness', () => {
  const enKeys = new Set(leafKeys(en))
  const zhKeys = new Set(leafKeys(zh))

  it('keeps English and Chinese locale keys in sync', () => {
    const difference = {
      missingInEnglish: [...zhKeys].filter((key) => !enKeys.has(key)).sort(),
      missingInChinese: [...enKeys].filter((key) => !zhKeys.has(key)).sort()
    }
    expect(difference, JSON.stringify(difference, null, 2)).toEqual({
      missingInEnglish: [],
      missingInChinese: []
    })
  })

  it('defines every static locale key referenced by application code', () => {
    const missing = [...staticLocaleReferences()]
      .filter(([key]) => !key.endsWith('.') && (!enKeys.has(key) || !zhKeys.has(key)))
      .map(([key, files]) => ({ key, files: [...new Set(files)].sort() }))
      .sort((a, b) => a.key.localeCompare(b.key))

    expect(missing, JSON.stringify(missing, null, 2)).toEqual([])
  })

  it('defines both locale namespaces used by dynamic keys', () => {
    const missing = [...dynamicLocalePrefixes()]
      .filter(([prefix]) =>
        ![...enKeys].some((key) => key.startsWith(prefix)) ||
        ![...zhKeys].some((key) => key.startsWith(prefix))
      )
      .map(([prefix, files]) => ({ prefix, files: [...new Set(files)].sort() }))
      .sort((a, b) => a.prefix.localeCompare(b.prefix))

    expect(missing, JSON.stringify(missing, null, 2)).toEqual([])
  })

  it('keeps interpolation placeholders in sync', () => {
    const mismatched = [...enKeys]
      .map((key) => ({
        key,
        English: placeholders(valueAt(en, key)),
        Chinese: placeholders(valueAt(zh, key))
      }))
      .filter(({ English, Chinese }) => English.join() !== Chinese.join())

    expect(mismatched, JSON.stringify(mismatched, null, 2)).toEqual([])
  })
})
