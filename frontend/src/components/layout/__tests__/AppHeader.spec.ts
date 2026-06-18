import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppHeader project switcher visibility', () => {
  it('shows project switcher for any user with multiple projects', () => {
    expect(componentSource).toContain('const userProjects = computed(() => user.value?.projects ?? [])')
    expect(componentSource).toContain('const showProjectSwitcher = computed(() => userProjects.value.length > 1)')
    expect(componentSource).not.toContain('showProjectSwitcher = computed(() => authStore.isAdmin')
  })

  it('keeps the project switcher visible on mobile layouts', () => {
    expect(componentSource).toContain('v-if="showProjectSwitcher"')
    expect(componentSource).toContain('class="flex min-w-0 max-w-36')
    expect(componentSource).not.toContain('class="hidden items-center gap-2 rounded-lg border border-primary-200')
  })
})
