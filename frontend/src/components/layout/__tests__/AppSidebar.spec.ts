import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar admin/user hierarchy', () => {
  it('renders admin-console navigation only for admin-console roles', () => {
    expect(componentSource).toContain('<template v-if="canAccessAdminConsole">')
    expect(componentSource).toContain('const canAccessAdminConsole = computed(() => authStore.canAccessAdminConsole)')
  })

  it('keeps user navigation available to admin-console roles', () => {
    expect(componentSource).toContain('authStore.hasUserAccess')
    expect(componentSource).not.toContain('!authStore.isSimpleMode && authStore.isAdmin')
  })

  it('does not fetch admin-only settings for operator accounts', () => {
    const adminSettingsWatchMatch = componentSource.match(/watch\(\s*\n\s*\(\) => authStore\.(\w+)/)
    expect(adminSettingsWatchMatch?.[1]).toBe('isAdmin')
  })

  it('does not keep a legacy operator-only admin navigation list', () => {
    expect(componentSource).not.toContain('operatorItems')
  })

  it('keeps project admins on scoped management pages only', () => {
    const projectItemsMatch = componentSource.match(/const projectItems: NavItem\[\] = \[[\s\S]*?\n {2}\]/)
    const projectItemsSource = projectItemsMatch?.[0] ?? ''

    expect(projectItemsSource).toContain("path: '/admin/dashboard'")
    expect(projectItemsSource).toContain("path: '/admin/ops'")
    expect(projectItemsSource).not.toContain("path: '/admin/projects'")
    expect(projectItemsSource).toContain("path: '/admin/users'")
    expect(projectItemsSource).toContain("path: '/admin/groups'")
    expect(projectItemsSource).toContain("path: '/admin/subscriptions'")
    expect(projectItemsSource).toContain("path: '/admin/accounts'")

    const projectAdminBranchMatch = componentSource.match(/if \(!authStore\.isAdmin\) \{[\s\S]*?\n {2}\}/)
    const projectAdminBranchSource = projectAdminBranchMatch?.[0] ?? ''
    expect(projectAdminBranchSource).toContain('return applyFeatureFlags(projectItems)')
    expect(projectAdminBranchSource).not.toContain('/admin/settings')
    expect(projectAdminBranchSource).not.toContain('/admin/channels')
    expect(projectAdminBranchSource).not.toContain('/admin/projects')
    expect(projectAdminBranchSource).not.toContain('/admin/redeem')
  })

  it('keeps project-space management on the super-admin navigation only', () => {
    expect(componentSource).toContain("{ path: '/admin/projects', label: t('nav.projects'), icon: FolderIcon, hideInSimpleMode: true }")
  })
})
