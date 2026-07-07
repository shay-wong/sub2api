import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

const sidebarTestState = vi.hoisted(() => ({
  appStore: {
    sidebarCollapsed: false,
    mobileOpen: false,
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: 'test',
    publicSettingsLoaded: true,
    backendModeEnabled: false,
    cachedPublicSettings: { custom_menu_items: [] },
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  authStore: {
    isAdmin: false,
    canAccessAdminConsole: true,
    isSimpleMode: false,
    hasUserAccess: true,
    hasAdminPermission: vi.fn(() => true),
  },
  adminSettingsStore: {
    customMenuItems: [],
    opsMonitoringEnabled: true,
    paymentEnabled: true,
    fetch: vi.fn(),
  },
  onboardingStore: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
  route: {
    path: '/admin/accounts',
  },
  router: {
    push: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => sidebarTestState.adminSettingsStore,
  useAppStore: () => sidebarTestState.appStore,
  useAuthStore: () => sidebarTestState.authStore,
  useOnboardingStore: () => sidebarTestState.onboardingStore,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => sidebarTestState.route,
    useRouter: () => sidebarTestState.router,
  }
})

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    affiliate: 'affiliate',
    availableChannels: 'availableChannels',
    channelMonitor: 'channelMonitor',
    payment: 'payment',
    riskControl: 'riskControl',
  },
  makeSidebarFlag: () => () => true,
}))

beforeEach(() => {
  sidebarTestState.appStore.sidebarCollapsed = false
  sidebarTestState.appStore.mobileOpen = false
  sidebarTestState.appStore.siteName = 'Sub2API'
  sidebarTestState.appStore.siteLogo = ''
  sidebarTestState.appStore.siteVersion = 'test'
  sidebarTestState.appStore.publicSettingsLoaded = true
  sidebarTestState.appStore.backendModeEnabled = false
  sidebarTestState.appStore.cachedPublicSettings = { custom_menu_items: [] }
  sidebarTestState.authStore.isAdmin = false
  sidebarTestState.authStore.canAccessAdminConsole = true
  sidebarTestState.authStore.isSimpleMode = false
  sidebarTestState.authStore.hasUserAccess = true
  sidebarTestState.authStore.hasAdminPermission.mockReturnValue(true)
  sidebarTestState.adminSettingsStore.customMenuItems = []
  sidebarTestState.adminSettingsStore.opsMonitoringEnabled = true
  sidebarTestState.adminSettingsStore.paymentEnabled = true
  sidebarTestState.route.path = '/admin/accounts'
  localStorage.setItem('theme', 'light')
  vi.clearAllMocks()
})

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

  it('routes project-admin brand links to the admin dashboard at runtime', async () => {
    const { default: AppSidebar } = await import('../AppSidebar.vue')

    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          VersionBadge: true,
          transition: false,
        },
      },
    })

    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links[0].props('to')).toBe('/admin/dashboard')
    expect(links[1].props('to')).toBe('/admin/dashboard')
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
    expect(projectItemsSource).toContain("path: '/admin/proxies'")
    expect(projectItemsSource).toContain('adminUsageItem')
    expect(componentSource).toContain("const adminUsageItem: NavItem = { path: '/admin/usage'")

    const projectAdminBranchMatch = componentSource.match(/if \(!authStore\.isAdmin\) \{[\s\S]*?\n {2}\}/)
    const projectAdminBranchSource = projectAdminBranchMatch?.[0] ?? ''
    expect(projectAdminBranchSource).toContain('return applyAdminPermissions(applyFeatureFlags(projectItems))')
    expect(componentSource).toContain('function applyAdminPermissions')
    expect(componentSource).toContain('authStore.hasAdminPermission(item.adminPermission)')
    expect(componentSource).toContain('AdminPermissions.proxies')
    expect(projectAdminBranchSource).not.toContain('/admin/settings')
    expect(projectAdminBranchSource).not.toContain('/admin/channels')
    expect(projectAdminBranchSource).not.toContain('/admin/projects')
    expect(projectAdminBranchSource).not.toContain('/admin/redeem')
  })

  it('keeps project-space management on the super-admin navigation only', () => {
    expect(componentSource).toContain("{ path: '/admin/projects', label: t('nav.projects'), icon: FolderIcon, hideInSimpleMode: true }")
  })
})
