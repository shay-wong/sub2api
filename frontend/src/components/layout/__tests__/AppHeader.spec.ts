import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppHeader project removal', () => {
  it('does not keep project switching state or UI', () => {
    expect(componentSource).not.toContain('sub2api_selected_project_id')
    expect(componentSource).not.toContain('showProjectSwitcher')
    expect(componentSource).not.toContain('user.value?.projects')
  })
})

describe('AppHeader model plaza visibility', () => {
  it('hides the entry from non-global admins in backend mode', () => {
    expect(componentSource).toContain(
      'v-if="user && modelPlazaEnabled && (!appStore.backendModeEnabled || authStore.isAdmin)"',
    )
  })
})
