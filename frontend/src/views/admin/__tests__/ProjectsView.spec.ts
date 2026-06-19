import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ProjectsView from '../ProjectsView.vue'

const {
  authState,
  listProjects,
  listMembers,
  listProfiles,
  getProfileBindings,
  createProject,
  createProfile,
  deleteProfile,
  removeMember,
  setProfileBindings,
  updateProfile,
  activateProfile,
  activateUnrestrictedScope,
  searchBindableResources,
  searchGlobalBindableResources,
  setMember,
  getUserApiKeys,
  transferApiKeyProject,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  authState: { isAdmin: false },
  listProjects: vi.fn(),
  listMembers: vi.fn(),
  listProfiles: vi.fn(),
  getProfileBindings: vi.fn(),
  createProject: vi.fn(),
  createProfile: vi.fn(),
  deleteProfile: vi.fn(),
  removeMember: vi.fn(),
  setProfileBindings: vi.fn(),
  updateProfile: vi.fn(),
  activateProfile: vi.fn(),
  activateUnrestrictedScope: vi.fn(),
  searchBindableResources: vi.fn(),
  searchGlobalBindableResources: vi.fn(),
  setMember: vi.fn(),
  getUserApiKeys: vi.fn(),
  transferApiKeyProject: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    projects: {
      list: listProjects,
      create: createProject,
      update: vi.fn(),
      listMembers,
      setMember,
      removeMember,
      listProfiles,
      createProfile,
      updateProfile,
      deleteProfile,
      activateProfile,
      activateUnrestrictedScope,
      getProfileBindings,
      setProfileBindings,
      searchBindableResources,
      searchGlobalBindableResources
    },
    users: {
      getUserApiKeys
    },
    apiKeys: {
      transferApiKeyProject
    }
  }
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authState,
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key} ${JSON.stringify(params)}`
      }
    })
  }
})

const mountProjectsView = () => mount(ProjectsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /></div>' },
      DataTable: {
        props: ['columns', 'data', 'loading'],
        template: `
          <div>
            <div v-if="loading">loading</div>
            <div v-else-if="!data || data.length === 0"><slot name="empty" /></div>
            <table v-else>
              <tbody>
                <tr v-for="row in data" :key="row.id || row.user_id">
                  <td v-for="column in columns" :key="column.key">
                    <slot :name="'cell-' + column.key" :row="row" :value="row[column.key]" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        `
      },
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show" data-test="dialog"><slot /><slot name="footer" /></div>'
      },
      ConfirmDialog: {
        props: ['show', 'title', 'message'],
        template: `
          <div v-if="show" data-test="confirm-dialog">
            <div data-test="confirm-title">{{ title }}</div>
            <div data-test="confirm-message">{{ message }}</div>
            <slot />
            <button data-test="confirm-dialog-confirm" @click="$emit('confirm')">confirm</button>
            <button data-test="confirm-dialog-cancel" @click="$emit('cancel')">cancel</button>
          </div>
        `
      },
      EmptyState: true,
      Icon: true,
      Teleport: true
    }
  }
})

const emptySearchResult = () => ({
  users: [],
  groups: [],
  accounts: [],
  subscriptions: [],
  api_keys: []
})

const waitForPickerDebounce = async () => {
  await new Promise(resolve => setTimeout(resolve, 400))
  await flushPromises()
}

const waitForMemberSearchDebounce = waitForPickerDebounce

describe('admin ProjectsView project resource scope', () => {
  beforeEach(() => {
    authState.isAdmin = false
    vi.clearAllMocks()

    listProjects.mockResolvedValue([
      { id: 1, name: 'Project One', slug: 'project-one', role: 'admin', is_owner: false }
    ])
    listMembers.mockResolvedValue([])
    listProfiles.mockResolvedValue([
      {
        id: 2,
        project_id: 1,
        name: 'Restricted',
        mode: 'restricted',
        is_active: true
      }
    ])
    getProfileBindings.mockResolvedValue({
      profile_id: 2,
      group_ids: [],
      account_ids: [],
      subscription_ids: []
    })
    createProject.mockResolvedValue({
      id: 9,
      name: 'Open Project',
      slug: 'open-project',
      role: 'super_admin',
      is_owner: true
    })
    updateProfile.mockResolvedValue({
      id: 2,
      project_id: 1,
      name: 'Restricted',
      mode: 'restricted',
      is_active: true
    })
    createProfile.mockResolvedValue({
      id: 4,
      project_id: 1,
      name: 'Created',
      mode: 'restricted',
      is_active: false
    })
    activateProfile.mockResolvedValue({
      id: 2,
      project_id: 1,
      name: 'Restricted',
      mode: 'restricted',
      is_active: true
    })
    activateUnrestrictedScope.mockResolvedValue({
      id: 99,
      project_id: 1,
      name: '__unrestricted__',
      mode: 'unrestricted',
      is_active: true
    })
    searchBindableResources.mockResolvedValue(emptySearchResult())
    searchGlobalBindableResources.mockResolvedValue(emptySearchResult())
    getUserApiKeys.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    transferApiKeyProject.mockResolvedValue({ api_key: { id: 11, project_id: 2 }, auto_granted_group_access: false })
    removeMember.mockResolvedValue({ message: 'ok' })
    deleteProfile.mockResolvedValue({ message: 'ok' })
    setMember.mockResolvedValue({
      project_id: 1,
      user_id: 42,
      email: 'new@example.test',
      username: '',
      role: 'user',
      is_owner: false,
      status: 'active'
    })
  })

  it('does not expose unrestricted as a profile mode', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('#profile-form select').exists()).toBe(false)
    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateProfile).toHaveBeenCalledWith(1, 2, {
      name: 'Restricted',
      description: null
    })
    expect(updateProfile).not.toHaveBeenCalledWith(1, 2, expect.objectContaining({ mode: 'unrestricted' }))
  })

  it('activates unrestricted as a project-level scope only after applying the selection', async () => {
    authState.isAdmin = true
    listProfiles
      .mockResolvedValueOnce([
        { id: 2, project_id: 1, name: 'Restricted', mode: 'restricted', is_active: true }
      ])
      .mockResolvedValueOnce([
        { id: 99, project_id: 1, name: '__unrestricted__', mode: 'unrestricted', is_active: true },
        { id: 2, project_id: 1, name: 'Restricted', mode: 'restricted', is_active: false }
      ])

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get<HTMLSelectElement>('[data-test="profile-scope-select"]').setValue('unrestricted')
    await flushPromises()
    expect(activateUnrestrictedScope).not.toHaveBeenCalled()

    await wrapper.get('[data-test="apply-profile-scope"]').trigger('click')
    await flushPromises()

    expect(activateUnrestrictedScope).toHaveBeenCalledWith(1)
    expect(updateProfile).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-test="profile-list-item"]').map(item => item.text()).join(' ')).not.toContain('admin.projects.unrestrictedMode')
  })

  it('requires applying the selected project profile scope before activation', async () => {
    authState.isAdmin = true
    listProfiles
      .mockResolvedValueOnce([
        { id: 2, project_id: 1, name: 'Restricted', mode: 'restricted', is_active: true },
        { id: 3, project_id: 1, name: 'Secondary', mode: 'restricted', is_active: false }
      ])
      .mockResolvedValueOnce([
        { id: 2, project_id: 1, name: 'Restricted', mode: 'restricted', is_active: false },
        { id: 3, project_id: 1, name: 'Secondary', mode: 'restricted', is_active: true }
      ])
    activateProfile.mockResolvedValue({
      id: 3,
      project_id: 1,
      name: 'Secondary',
      mode: 'restricted',
      is_active: true
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    const scopeSelect = wrapper.get<HTMLSelectElement>('[data-test="profile-scope-select"]')
    const options = scopeSelect.findAll('option').map(option => option.text())
    expect(options).toContain('admin.projects.unrestrictedMode')
    expect(options).toContain('Secondary')
    expect(wrapper.findAll('[data-test="profile-list-item"]').map(item => item.text()).join(' ')).not.toContain('admin.projects.unrestrictedMode')

    await scopeSelect.setValue('profile:3')
    await flushPromises()
    expect(activateProfile).not.toHaveBeenCalled()
    expect(activateUnrestrictedScope).not.toHaveBeenCalled()

    await wrapper.get('[data-test="apply-profile-scope"]').trigger('click')
    await flushPromises()

    expect(activateProfile).toHaveBeenCalledWith(1, 3)
    expect(activateUnrestrictedScope).not.toHaveBeenCalled()
  })

  it('requires applying unrestricted scope selection before activation', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get<HTMLSelectElement>('[data-test="profile-scope-select"]').setValue('unrestricted')
    await flushPromises()
    expect(activateUnrestrictedScope).not.toHaveBeenCalled()

    await wrapper.get('[data-test="apply-profile-scope"]').trigger('click')
    await flushPromises()

    expect(activateUnrestrictedScope).toHaveBeenCalledWith(1)
    expect(activateProfile).not.toHaveBeenCalled()
  })

  it('keeps default profile bindings when creating an unrestricted project', async () => {
    authState.isAdmin = true
    searchGlobalBindableResources.mockResolvedValue({
      ...emptySearchResult(),
      groups: [{ id: 7, project_id: 1, name: 'Shared Group', description: '', platform: 'openai', status: 'active' }]
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.createProject')!.trigger('click')
    await flushPromises()

    const form = wrapper.get('#project-form')
    const inputs = form.findAll('input')
    await inputs[0].setValue('Open Project')
    await inputs[1].setValue('open-project')
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.unrestrictedMode')!.trigger('click')
    await wrapper.get('[data-test="project-add-groups"]').trigger('click')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 50)
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.applySelection')!.trigger('click')
    await flushPromises()

    await form.trigger('submit.prevent')
    await flushPromises()

    expect(createProject).toHaveBeenCalledWith(expect.objectContaining({
      profile_mode: 'unrestricted',
      group_ids: [7],
      account_ids: [],
      subscription_ids: []
    }))
  })

  it('opens a resource picker, searches as the user types, and saves selected profile resources after submitting', async () => {
    authState.isAdmin = true
    searchGlobalBindableResources
      .mockResolvedValueOnce({
        ...emptySearchResult(),
        groups: [{ id: 7, project_id: 1, name: 'Initial Group', description: '', platform: 'openai', status: 'active' }]
      })
      .mockResolvedValueOnce({
        ...emptySearchResult(),
        groups: [{ id: 8, project_id: 1, name: 'External Group', description: '', platform: 'anthropic', status: 'active' }]
      })
    setProfileBindings.mockResolvedValue({
      profile_id: 2,
      group_ids: [8],
      account_ids: [],
      subscription_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="edit-profile-2"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="profile-add-groups"]').trigger('click')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 50)
    const pickerInput = wrapper.find('input[placeholder="admin.projects.searchResourcesPlaceholder"]')
    await pickerInput.setValue('External')
    await waitForPickerDebounce()

    expect(searchGlobalBindableResources).toHaveBeenLastCalledWith('External', 50)
    expect(wrapper.text()).toContain('External Group')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.applySelection')!.trigger('click')
    await flushPromises()

    expect(setProfileBindings).not.toHaveBeenCalled()

    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(setProfileBindings).toHaveBeenCalledWith(1, 2, expect.objectContaining({
      group_ids: [8],
      account_ids: [],
      subscription_ids: []
    }))
  })

  it('renders existing profile bindings with names returned by the bindings endpoint', async () => {
    authState.isAdmin = true
    getProfileBindings.mockResolvedValue({
      profile_id: 2,
      group_ids: [7],
      account_ids: [8],
      subscription_ids: [9],
      groups: [{ id: 7, project_id: 1, name: '共享分组', description: '', platform: 'openai', status: 'active' }],
      accounts: [{ id: 8, project_id: 1, name: '主账号', notes: '', platform: 'openai', type: 'api_key', status: 'active', email: 'owner@example.test' }],
      subscriptions: [{ id: 9, user_id: 42, group_id: 7, user_email: 'user@example.test', group_name: '共享分组', status: 'active', notes: '包月' }]
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="edit-profile-2"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('共享分组')
    expect(wrapper.text()).toContain('主账号')
    expect(wrapper.text()).toContain('user@example.test / 共享分组')
    expect(wrapper.text()).not.toContain('admin.projects.resourceTypes.groups #7')
    expect(wrapper.text()).not.toContain('admin.projects.resourceTypes.accounts #8')
    expect(wrapper.text()).not.toContain('admin.projects.resourceTypes.subscriptions #9')
  })

  it('keeps existing profile resource removals as a draft until the profile form is saved', async () => {
    authState.isAdmin = true
    getProfileBindings.mockResolvedValue({
      profile_id: 2,
      group_ids: [7],
      account_ids: [],
      subscription_ids: [],
      groups: [{ id: 7, project_id: 1, name: '共享分组', description: '', platform: 'openai', status: 'active' }]
    })
    searchGlobalBindableResources.mockResolvedValue({
      ...emptySearchResult(),
      groups: [{ id: 7, project_id: 1, name: '共享分组', description: '', platform: 'openai', status: 'active' }]
    })
    setProfileBindings.mockResolvedValue({
      profile_id: 2,
      group_ids: [],
      account_ids: [],
      subscription_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="edit-profile-2"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('共享分组')

    await wrapper.get('[data-test="profile-add-groups"]').trigger('click')
    await flushPromises()

    const checkbox = wrapper.get('input[type="checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(true)

    await checkbox.setValue(false)
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.applySelection')!.trigger('click')
    await flushPromises()

    expect(setProfileBindings).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('共享分组')
    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(setProfileBindings).toHaveBeenCalledWith(1, 2, expect.objectContaining({
      group_ids: [],
      account_ids: [],
      subscription_ids: []
    }))
  })

  it('allows editing resource bindings before creating a profile', async () => {
    authState.isAdmin = true
    createProfile.mockResolvedValue({
      id: 7,
      project_id: 1,
      name: 'Created Profile',
      mode: 'restricted',
      is_active: false
    })
    searchGlobalBindableResources.mockResolvedValue({
      ...emptySearchResult(),
      accounts: [{ id: 88, project_id: 1, name: '主账号', notes: '', platform: 'openai', type: 'api_key', status: 'active', email: 'owner@example.test' }]
    })
    setProfileBindings.mockResolvedValue({
      profile_id: 7,
      group_ids: [],
      account_ids: [88],
      subscription_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addProfile')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.projects.resourceBindings')

    await wrapper.get('[data-test="profile-add-accounts"]').trigger('click')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 50)
    expect(wrapper.text()).toContain('主账号')

    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.applySelection')!.trigger('click')
    await flushPromises()

    const form = wrapper.get('#profile-form')
    await form.find('input').setValue('Created Profile')
    await form.trigger('submit.prevent')
    await flushPromises()

    expect(createProfile).toHaveBeenCalledWith(1, {
      name: 'Created Profile',
      description: null
    })
    expect(setProfileBindings).toHaveBeenCalledWith(1, 7, expect.objectContaining({
      group_ids: [],
      account_ids: [88],
      subscription_ids: []
    }))
  })

  it('keeps the app profile list open when profile bindings fail to load', async () => {
    authState.isAdmin = true
    getProfileBindings.mockRejectedValue(new Error('bindings unavailable'))

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Restricted')
    expect(showError).toHaveBeenCalledWith('admin.projects.failedToLoadBindings')
    expect(showError).not.toHaveBeenCalledWith('admin.projects.failedToLoadProfiles')
  })

  it('does not overwrite existing profile bindings when they fail to load before editing', async () => {
    authState.isAdmin = true
    getProfileBindings
      .mockResolvedValueOnce({
        profile_id: 2,
        group_ids: [7],
        account_ids: [],
        subscription_ids: [],
        groups: [{ id: 7, project_id: 1, name: '共享分组', description: '', platform: 'openai', status: 'active' }]
      })
      .mockRejectedValueOnce(new Error('bindings unavailable'))

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="edit-profile-2"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.projects.resourceBindingsLoadFailedHint')
    expect(wrapper.get('[data-test="profile-add-groups"]').attributes('disabled')).toBeDefined()

    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateProfile).toHaveBeenCalledWith(1, 2, {
      name: 'Restricted',
      description: null
    })
    expect(setProfileBindings).not.toHaveBeenCalled()
  })

  it('uses global member search for super admins when adding project members', async () => {
    authState.isAdmin = true
    searchGlobalBindableResources.mockResolvedValue({
      ...emptySearchResult(),
      users: [{ id: 42, email: 'new@example.test', username: '', notes: '', status: 'active' }]
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addMember')!.trigger('click')
    await flushPromises()

    await wrapper.get('input[placeholder="admin.projects.searchMemberPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 20)
    expect(searchBindableResources).not.toHaveBeenCalled()

    await wrapper.get('input[type="number"]').setValue(42)
    await wrapper.get('#member-form').trigger('submit.prevent')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({ role: 'user', status: 'active' }))
  })

  it('searches member candidates automatically when opening and typing in the add member dialog', async () => {
    authState.isAdmin = true
    searchGlobalBindableResources
      .mockResolvedValueOnce({
        ...emptySearchResult(),
        users: [{ id: 42, email: 'initial@example.test', username: '', notes: '', status: 'active' }]
      })
      .mockResolvedValueOnce({
        ...emptySearchResult(),
        users: [{ id: 43, email: 'alice@example.test', username: 'alice', notes: '', status: 'active' }]
      })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addMember')!.trigger('click')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 20)
    expect(wrapper.text()).toContain('initial@example.test')

    await wrapper.get('input[placeholder="admin.projects.searchMemberPlaceholder"]').setValue('alice')
    await waitForMemberSearchDebounce()

    expect(searchGlobalBindableResources).toHaveBeenLastCalledWith('alice', 20)
    expect(wrapper.text()).toContain('alice@example.test')
  })

  it('requires applying member role and status drafts before saving', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'user',
        is_owner: false,
        status: 'active'
      }
    ])
    setMember.mockResolvedValue({
      project_id: 1,
      user_id: 42,
      email: 'member@example.test',
      username: '',
      role: 'admin',
      is_owner: false,
      status: 'disabled'
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="edit-member-42"]').trigger('click')
    await flushPromises()

    const selects = wrapper.findAll('#member-edit-form select')
    await selects[0].setValue('admin')
    await selects[1].setValue('disabled')
    await flushPromises()

    expect(setMember).not.toHaveBeenCalled()

    await wrapper.get('#member-edit-form').trigger('submit.prevent')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({
      role: 'admin',
      status: 'disabled',
      permissions: expect.arrayContaining([
        'admin.dashboard.read',
        'admin.ops.read',
        'admin.users.manage',
        'admin.groups.manage',
        'admin.subscriptions.manage',
        'admin.accounts.write'
      ])
    }))
  })

  it('shows the global super admin role for project members', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'admin@example.test',
        username: 'root',
        role: 'admin',
        user_role: 'super_admin',
        is_owner: true,
        status: 'active',
        permissions: ['admin.dashboard.read']
      }
    ])

    const wrapper = mountProjectsView()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.users.roles.super_admin')
    expect(wrapper.text()).toContain('admin.projects.permissions.dashboard')
  })

  it('toggles project member status from the row action without opening the edit dialog', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'user',
        is_owner: false,
        status: 'active'
      }
    ])
    setMember.mockResolvedValue({
      project_id: 1,
      user_id: 42,
      email: 'member@example.test',
      username: '',
      role: 'user',
      is_owner: false,
      status: 'disabled'
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    const toggleButton = wrapper.get('[data-test="toggle-member-status-42"]')
    expect(toggleButton.classes()).toContain('flex-col')
    await toggleButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('#member-edit-form').exists()).toBe(false)
    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({
      role: 'user',
      status: 'disabled',
      permissions: []
    }))
  })

  it('saves only checked project admin permissions from the member edit dialog', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'admin',
        is_owner: false,
        status: 'active',
        permissions: ['admin.dashboard.read', 'admin.accounts.write']
      }
    ])

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="edit-member-42"]').trigger('click')
    await flushPromises()

    const accountsCheckbox = wrapper.findAll('input[type="checkbox"]').find((input) => (input.element as HTMLInputElement).value === 'admin.accounts.write')
    expect(accountsCheckbox).toBeTruthy()
    await accountsCheckbox!.setValue(false)

    await wrapper.get('#member-edit-form').trigger('submit.prevent')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({
      role: 'admin',
      permissions: ['admin.dashboard.read']
    }))
  })

  it('promotes transferred owners to project admins with default permissions', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'user',
        is_owner: false,
        status: 'active'
      }
    ])

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="edit-member-42"]').trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.transferOwner')!.trigger('click')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({
      role: 'admin',
      is_owner: true,
      status: 'active',
      permissions: expect.arrayContaining([
        'admin.dashboard.read',
        'admin.ops.read',
        'admin.users.manage',
        'admin.groups.manage',
        'admin.subscriptions.manage',
        'admin.accounts.write'
      ])
    }))
  })

  it('requires confirmation before removing a project member', async () => {
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'user',
        is_owner: false,
        status: 'active'
      }
    ])

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="member-more-42"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="remove-member-42"]').trigger('click')
    await flushPromises()

    expect(removeMember).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.projects.removeMemberConfirmTitle')
    expect(wrapper.text()).toContain('member@example.test')

    await wrapper.get('[data-test="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    expect(removeMember).toHaveBeenCalledWith(1, 42)
  })

  it('requires confirmation before deleting an application profile', async () => {
    listProfiles.mockResolvedValue([
      {
        id: 2,
        project_id: 1,
        name: 'Active Profile',
        mode: 'restricted',
        is_active: true
      },
      {
        id: 3,
        project_id: 1,
        name: 'Unused Profile',
        mode: 'restricted',
        is_active: false
      }
    ])

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="delete-profile-3"]').trigger('click')
    await flushPromises()

    expect(deleteProfile).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.projects.deleteProfileConfirmTitle')
    expect(wrapper.text()).toContain('Unused Profile')

    await wrapper.get('[data-test="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    expect(deleteProfile).toHaveBeenCalledWith(1, 3)
  })

  it('moves selected member API keys to another project only after applying once', async () => {
    authState.isAdmin = true
    listProjects.mockResolvedValue([
      { id: 1, name: 'Project One', slug: 'project-one', role: 'super_admin', is_owner: true },
      { id: 2, name: 'Project Two', slug: 'project-two', role: 'super_admin', is_owner: true },
      { id: 3, name: 'Project Three', slug: 'project-three', role: 'super_admin', is_owner: true }
    ])
    listMembers.mockImplementation((projectId: number) => {
      if (projectId === 1) {
        return Promise.resolve([
          {
            project_id: 1,
            user_id: 42,
            email: 'member@example.test',
            username: '',
            role: 'user',
            is_owner: false,
            status: 'active'
          }
        ])
      }
      if (projectId === 2) {
        return Promise.resolve([
          {
            project_id: 2,
            user_id: 42,
            email: 'member@example.test',
            username: '',
            role: 'user',
            is_owner: false,
            status: 'disabled'
          }
        ])
      }
      return Promise.resolve([
        {
          project_id: 3,
          user_id: 99,
          email: 'other@example.test',
          username: '',
          role: 'user',
          is_owner: false,
          status: 'active'
        }
      ])
    })
    getUserApiKeys
      .mockResolvedValueOnce({
        items: [
          {
            id: 11,
            user_id: 42,
            project_id: 1,
            key: 'sk-project-one-secret',
            name: 'Project One Key',
            group_id: null,
            status: 'active',
            ip_whitelist: [],
            ip_blacklist: [],
            last_used_at: null,
            quota: 0,
            quota_used: 0,
            expires_at: null,
            created_at: '2026-06-19T00:00:00Z',
            updated_at: '2026-06-19T00:00:00Z',
            rate_limit_5h: 0,
            rate_limit_1d: 0,
            rate_limit_7d: 0,
            usage_5h: 0,
            usage_1d: 0,
            usage_7d: 0,
            window_5h_start: null,
            window_1d_start: null,
            window_7d_start: null,
            reset_5h_at: null,
            reset_1d_at: null,
            reset_7d_at: null
          },
          {
            id: 12,
            user_id: 42,
            project_id: 1,
            key: 'sk-project-two-secret',
            name: 'Project Two Key',
            group_id: null,
            status: 'active',
            ip_whitelist: [],
            ip_blacklist: [],
            last_used_at: null,
            quota: 0,
            quota_used: 0,
            expires_at: null,
            created_at: '2026-06-19T00:00:00Z',
            updated_at: '2026-06-19T00:00:00Z',
            rate_limit_5h: 0,
            rate_limit_1d: 0,
            rate_limit_7d: 0,
            usage_5h: 0,
            usage_1d: 0,
            usage_7d: 0,
            window_5h_start: null,
            window_1d_start: null,
            window_7d_start: null,
            reset_5h_at: null,
            reset_1d_at: null,
            reset_7d_at: null
          }
        ],
        total: 2,
        page: 1,
        page_size: 20,
        pages: 1
      })
      .mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="member-more-42"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="member-api-keys-42"]').trigger('click')
    await flushPromises()

    expect(getUserApiKeys).toHaveBeenCalledWith(42, 1)
    expect(wrapper.text()).toContain('Project One Key')
    expect(wrapper.text()).toContain('Project Two Key')
    const targetSelect = wrapper.get<HTMLSelectElement>('[data-test="api-key-bulk-target"]')
    const targetOptions = targetSelect.findAll('option').map(option => option.text())
    expect(targetOptions).toContain('Project Two (admin.projects.apiKeyTransferDisabledMemberOption)')
    expect(targetOptions).not.toContain('Project Three')

    await wrapper.get('[data-test="select-api-key-11"]').setValue(true)
    await wrapper.get('[data-test="select-api-key-12"]').setValue(true)
    await flushPromises()

    await targetSelect.setValue('2')
    await flushPromises()

    expect(transferApiKeyProject).not.toHaveBeenCalled()

    await wrapper.get('[data-test="transfer-selected-api-keys"]').trigger('click')
    await flushPromises()

    expect(transferApiKeyProject).toHaveBeenCalledWith(11, 2)
    expect(transferApiKeyProject).toHaveBeenCalledWith(12, 2)
    expect(transferApiKeyProject).toHaveBeenCalledTimes(2)
    expect(getUserApiKeys).toHaveBeenLastCalledWith(42, 1)
    expect(showSuccess).toHaveBeenCalledWith('admin.projects.apiKeyTransferred')
  })

  it('hides member API key project transfer controls from project admins', async () => {
    authState.isAdmin = false
    listProjects.mockResolvedValue([
      { id: 1, name: 'Project One', slug: 'project-one', role: 'admin', is_owner: false },
      { id: 2, name: 'Project Two', slug: 'project-two', role: 'user', is_owner: false }
    ])
    listMembers.mockResolvedValue([
      {
        project_id: 1,
        user_id: 42,
        email: 'member@example.test',
        username: '',
        role: 'user',
        is_owner: false,
        status: 'active'
      }
    ])
    getUserApiKeys.mockResolvedValueOnce({
      items: [
        {
          id: 11,
          user_id: 42,
          project_id: 1,
          key: 'sk-project-one-secret',
          name: 'Project One Key',
          group_id: null,
          status: 'active',
          ip_whitelist: [],
          ip_blacklist: [],
          last_used_at: null,
          quota: 0,
          quota_used: 0,
          expires_at: null,
          created_at: '2026-06-19T00:00:00Z',
          updated_at: '2026-06-19T00:00:00Z',
          rate_limit_5h: 0,
          rate_limit_1d: 0,
          rate_limit_7d: 0,
          usage_5h: 0,
          usage_1d: 0,
          usage_7d: 0,
          window_5h_start: null,
          window_1d_start: null,
          window_7d_start: null,
          reset_5h_at: null,
          reset_1d_at: null,
          reset_7d_at: null
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.get('[data-test="member-more-42"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="member-api-keys-42"]').trigger('click')
    await flushPromises()

    expect(getUserApiKeys).toHaveBeenCalledWith(42, 1)
    expect(listMembers).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Project One Key')
    expect(wrapper.text()).toContain('admin.projects.apiKeyTransferSuperAdminOnly')
    expect(wrapper.find('[data-test="api-key-bulk-target"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="transfer-selected-api-keys"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="select-api-key-11"]').exists()).toBe(false)
    expect(transferApiKeyProject).not.toHaveBeenCalled()
  })
})
