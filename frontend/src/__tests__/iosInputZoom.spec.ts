import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const indexSource = readFileSync(resolve(frontendRoot, 'index.html'), 'utf8')
const mainSource = readFileSync(resolve(frontendRoot, 'src/main.ts'), 'utf8')
const styleSource = readFileSync(resolve(frontendRoot, 'src/style.css'), 'utf8')

describe('iOS input focus zoom', () => {
  it('keeps page zoom available and raises touch form controls to 16px', () => {
    expect(indexSource).not.toMatch(/maximum-scale|user-scalable/i)
    expect(mainSource).not.toMatch(/maximum-scale|user-scalable|meta\[name=["']viewport["']\]/i)
    expect(styleSource).toContain('@supports (-webkit-touch-callout: none)')
    expect(styleSource).toContain('@media (pointer: coarse)')
    expect(styleSource).toMatch(/input,\s*select,\s*textarea\s*\{\s*font-size:\s*16px;/)
  })
})
