import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import VersionBadge from '../VersionBadge.vue'

const { getRollbackVersions, performUpdate, restartService, rollback, copyToClipboard, appStore } = vi.hoisted(() => ({
  getRollbackVersions: vi.fn(),
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  rollback: vi.fn(),
  copyToClipboard: vi.fn(),
  appStore: {
    versionLoading: false,
    currentVersion: '0.1.149-fork.1',
    latestVersion: '0.1.149-fork.1',
    hasUpdate: false,
    releaseInfo: null,
    buildType: 'release',
    fetchVersion: vi.fn(),
    clearVersionCache: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => appStore,
}))

vi.mock('@/api/admin/system', () => ({
  getRollbackVersions,
  performUpdate,
  restartService,
  rollback,
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: false,
    copyToClipboard,
  }),
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

describe('VersionBadge rollback commands', () => {
  beforeEach(() => {
    getRollbackVersions.mockReset()
    performUpdate.mockReset()
    restartService.mockReset()
    rollback.mockReset()
    copyToClipboard.mockReset()
    appStore.fetchVersion.mockReset()
    appStore.clearVersionCache.mockReset()
    appStore.versionLoading = false
    appStore.currentVersion = '0.1.149-fork.1'
    appStore.latestVersion = '0.1.149-fork.1'
    appStore.hasUpdate = false
    appStore.releaseInfo = null
    appStore.buildType = 'release'
  })

  it('uses stable fork artifacts in manual rollback commands', async () => {
    getRollbackVersions.mockResolvedValue({
      versions: [
        {
          version: '0.1.148-fork.1',
          published_at: '2026-07-09T00:00:00Z',
          html_url: 'https://github.com/shay-wong/sub2api/releases/tag/v0.1.148-fork.1',
        },
      ],
    })

    const wrapper = mount(VersionBadge, {
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await wrapper.get('button').trigger('click')
    await wrapper.findAll('button').find((button) => button.text().includes('version.rollback'))!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('v0.1.148-fork.1'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(
      'https://raw.githubusercontent.com/shay-wong/sub2api/v0.1.148-fork.1/deploy/install.sh'
    )

    await wrapper.findAll('button').find((button) => button.text().includes('version.deployDocker'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('image: ghcr.io/shay-wong/sub2api:0.1.148-fork.1')
  })
})
