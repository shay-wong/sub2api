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
    expect(componentSource).toContain('class="flex h-10 w-auto min-w-0 max-w-48')
    expect(componentSource).toContain('max-w-[min(28rem,calc(100vw-2rem))]')
    expect(componentSource).toContain('role="listbox"')
    expect(componentSource).toContain('aria-haspopup="listbox"')
    expect(componentSource).not.toContain('class="hidden items-center gap-2 rounded-lg border border-primary-200')
    expect(componentSource).not.toContain('select-native select-native-compact')
  })
})
