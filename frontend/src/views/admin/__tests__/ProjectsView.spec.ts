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
  setProfileBindings,
  updateProfile,
  activateProfile,
  searchBindableResources,
  searchGlobalBindableResources,
  setMember,
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
  setProfileBindings: vi.fn(),
  updateProfile: vi.fn(),
  activateProfile: vi.fn(),
  searchBindableResources: vi.fn(),
  searchGlobalBindableResources: vi.fn(),
  setMember: vi.fn(),
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
      removeMember: vi.fn(),
      listProfiles,
      createProfile,
      updateProfile,
      deleteProfile,
      activateProfile,
      getProfileBindings,
      setProfileBindings,
      searchBindableResources,
      searchGlobalBindableResources
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
      t: (key: string) => key
    })
  }
})

const mountProjectsView = () => mount(ProjectsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="actions" /><slot name="table" /></div>' },
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show" data-test="dialog"><slot /><slot name="footer" /></div>'
      },
      EmptyState: true,
      Icon: true,
      Teleport: true
    }
  }
})

describe('admin ProjectsView profile permissions', () => {
  beforeEach(() => {
    authState.isAdmin = false
    listProjects.mockReset()
    listMembers.mockReset()
    listProfiles.mockReset()
    getProfileBindings.mockReset()
    createProject.mockReset()
    createProfile.mockReset()
    deleteProfile.mockReset()
    setProfileBindings.mockReset()
    updateProfile.mockReset()
    activateProfile.mockReset()
    searchBindableResources.mockReset()
    searchGlobalBindableResources.mockReset()
    setMember.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

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
      user_ids: [],
      group_ids: [],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
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
    searchBindableResources.mockResolvedValue({
      users: [],
      groups: [],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
    searchGlobalBindableResources.mockResolvedValue({
      users: [],
      groups: [],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
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

  it('lets super admins choose unrestricted profile mode for current project profiles', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()

    const options = wrapper.findAll('option').map(option => option.attributes('value'))
    expect(options).toContain('unrestricted')

    const modeSelect = wrapper.get<HTMLSelectElement>('#profile-form select')
    expect(modeSelect.attributes('disabled')).toBeUndefined()
    await modeSelect.setValue('unrestricted')
    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateProfile).toHaveBeenCalledWith(
      1,
      2,
      expect.objectContaining({ mode: 'unrestricted' })
    )
  })

  it('allows super admins to choose unrestricted profile mode', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()

    const options = wrapper.findAll('option').map(option => option.attributes('value'))
    expect(options).toContain('unrestricted')
  })

  it('reloads profile bindings after switching to unrestricted mode', async () => {
    authState.isAdmin = true
    getProfileBindings
      .mockResolvedValueOnce({
        profile_id: 2,
        user_ids: [],
        group_ids: [7],
        account_ids: [],
        subscription_ids: [],
        api_key_ids: []
      })
      .mockResolvedValueOnce({
        profile_id: 2,
        user_ids: [],
        group_ids: [],
        account_ids: [],
        subscription_ids: [],
        api_key_ids: []
      })
    updateProfile.mockResolvedValue({
      id: 2,
      project_id: 1,
      name: 'Restricted',
      mode: 'unrestricted',
      is_active: true
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    expect(getProfileBindings).toHaveBeenCalledTimes(1)
    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.unrestrictedMode')!.trigger('click')
    await flushPromises()

    expect(updateProfile).toHaveBeenCalledWith(1, 2, { mode: 'unrestricted' })
    expect(getProfileBindings).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.projects.unrestrictedHint')
  })

  it('lets super admins create unrestricted profiles in the current project', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addProfile')!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('Global Profile')
    const modeSelect = wrapper.get<HTMLSelectElement>('#profile-form select')
    expect(modeSelect.attributes('disabled')).toBeUndefined()
    await modeSelect.setValue('unrestricted')

    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()

    expect(createProfile).toHaveBeenCalledWith(1, expect.objectContaining({ mode: 'unrestricted' }))
  })

  it('lets super admins create projects with unrestricted default profile', async () => {
    authState.isAdmin = true

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.createProject')!.trigger('click')
    await flushPromises()

    const form = wrapper.get('#project-form')
    const inputs = form.findAll('input')
    await inputs[0].setValue('Open Project')
    await inputs[1].setValue('open-project')

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.unrestrictedMode')!.trigger('click')
    await form.trigger('submit.prevent')
    await flushPromises()

    expect(createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Open Project',
      slug: 'open-project',
      profile_mode: 'unrestricted',
      user_ids: [],
      group_ids: [],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    }))
  })

  it('lets super admins edit and activate existing unrestricted profiles in the current project', async () => {
    authState.isAdmin = true

    listProfiles.mockResolvedValue([
      {
        id: 2,
        project_id: 1,
        name: 'Active Restricted',
        mode: 'restricted',
        is_active: true
      },
      {
        id: 3,
        project_id: 1,
        name: 'Global Profile',
        mode: 'unrestricted',
        is_active: false
      }
    ])
    getProfileBindings.mockResolvedValue({
      profile_id: 3,
      user_ids: [],
      group_ids: [],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    })
    updateProfile
      .mockResolvedValueOnce({
        id: 3,
        project_id: 1,
        name: 'Global Profile',
        mode: 'unrestricted',
        is_active: false
      })
      .mockResolvedValueOnce({
        id: 3,
        project_id: 1,
        name: 'Global Profile',
        mode: 'restricted',
        is_active: false
      })
    activateProfile.mockResolvedValue({
      id: 3,
      project_id: 1,
      name: 'Global Profile',
      mode: 'restricted',
      is_active: true
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text().includes('Global Profile'))!.trigger('click')
    await flushPromises()

    const activateButton = wrapper.findAll('button').find(button => button.text() === 'admin.projects.activateProfile')!
    expect(activateButton.attributes('disabled')).toBeUndefined()

    const editButton = wrapper.findAll('button').find(button => button.text() === 'common.edit')!
    expect(editButton.attributes('disabled')).toBeUndefined()
    const restrictedModeButton = wrapper.findAll('button').find(button => button.text() === 'admin.projects.restrictedMode')!
    expect(restrictedModeButton.attributes('disabled')).toBeUndefined()

    await editButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('#profile-form').exists()).toBe(true)
    await wrapper.get('#profile-form').trigger('submit.prevent')
    await flushPromises()
    expect(updateProfile).toHaveBeenCalledWith(1, 3, expect.objectContaining({ mode: 'unrestricted' }))
    expect(setProfileBindings).not.toHaveBeenCalled()

    await restrictedModeButton.trigger('click')
    await flushPromises()
    expect(updateProfile).toHaveBeenCalledWith(1, 3, { mode: 'restricted' })

    await activateButton.trigger('click')
    await flushPromises()
    expect(activateProfile).toHaveBeenCalledWith(1, 3)
  })

  it('lets super admins delete inactive unrestricted profiles in the current project', async () => {
    authState.isAdmin = true

    listProfiles.mockResolvedValue([
      {
        id: 2,
        project_id: 1,
        name: 'Active Restricted',
        mode: 'restricted',
        is_active: true
      },
      {
        id: 3,
        project_id: 1,
        name: 'Global Profile',
        mode: 'unrestricted',
        is_active: false
      }
    ])
    getProfileBindings.mockResolvedValue({
      profile_id: 3,
      user_ids: [],
      group_ids: [],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text().includes('Global Profile'))!.trigger('click')
    await flushPromises()

    const deleteButton = wrapper.findAll('button').find(button => button.text() === 'common.delete')!
    expect(deleteButton.attributes('disabled')).toBeUndefined()

    await deleteButton.trigger('click')
    await flushPromises()

    expect(deleteProfile).toHaveBeenCalledWith(1, 3)
  })

  it('uses project resource search for super admins when binding existing profiles', async () => {
    authState.isAdmin = true
    searchBindableResources.mockResolvedValue({
      users: [],
      groups: [{ id: 7, project_id: 99, name: 'Shared Group', description: '', platform: 'openai', status: 'active' }],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
    setProfileBindings.mockResolvedValue({
      profile_id: 2,
      user_ids: [],
      group_ids: [7],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    const resourceSelects = wrapper.findAll('select')
    await resourceSelects[0].setValue('groups')
    await wrapper.get('input[placeholder="admin.projects.searchResourcesPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    expect(searchBindableResources).toHaveBeenCalledWith(1, '', 30)
    expect(searchGlobalBindableResources).not.toHaveBeenCalled()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.bind')!.trigger('click')
    await flushPromises()

    expect(setProfileBindings).toHaveBeenCalledWith(1, 2, expect.objectContaining({ group_ids: [7] }))
  })

  it('uses project resource search when configuring profile bindings', async () => {
    authState.isAdmin = true

    searchBindableResources.mockResolvedValue({
      users: [],
      groups: [{ id: 8, project_id: 77, name: 'External Group', description: '', platform: 'anthropic', status: 'active' }],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
    setProfileBindings.mockResolvedValue({
      profile_id: 2,
      user_ids: [],
      group_ids: [8],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    const resourceSelects = wrapper.findAll('select')
    await resourceSelects[0].setValue('groups')
    await wrapper.get('input[placeholder="admin.projects.searchResourcesPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    expect(searchBindableResources).toHaveBeenCalledWith(1, '', 30)
    expect(searchGlobalBindableResources).not.toHaveBeenCalled()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.bind')!.trigger('click')
    await flushPromises()

    expect(setProfileBindings).toHaveBeenCalledWith(1, 2, expect.objectContaining({ group_ids: [8] }))
  })

  it('keeps profile bindings usable when the API returns null ID arrays', async () => {
    authState.isAdmin = true
    getProfileBindings.mockResolvedValue({
      profile_id: 2,
      user_ids: null,
      group_ids: null,
      account_ids: null,
      subscription_ids: null,
      api_key_ids: null
    })
    searchBindableResources.mockResolvedValue({
      users: [],
      groups: [{ id: 8, project_id: 77, name: 'External Group', description: '', platform: 'anthropic', status: 'active' }],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
    setProfileBindings.mockResolvedValue({
      profile_id: 2,
      user_ids: [],
      group_ids: [8],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.appProfiles')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.projects.noBindings')
    const resourceSelects = wrapper.findAll('select')
    await resourceSelects[0].setValue('groups')
    await wrapper.get('input[placeholder="admin.projects.searchResourcesPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.bind')!.trigger('click')
    await flushPromises()

    expect(setProfileBindings).toHaveBeenCalledWith(1, 2, expect.objectContaining({
      user_ids: [],
      group_ids: [8],
      account_ids: [],
      subscription_ids: [],
      api_key_ids: []
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

  it('uses global member search for super admins when adding project members', async () => {
    authState.isAdmin = true
    searchGlobalBindableResources.mockResolvedValue({
      users: [{ id: 42, email: 'new@example.test', username: '', notes: '', status: 'active' }],
      groups: [],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addMember')!.trigger('click')
    await flushPromises()

    await wrapper.get('input[placeholder="admin.projects.searchMemberPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 20)
    expect(searchBindableResources).not.toHaveBeenCalled()

    await wrapper.findAll('button').find(button => button.text().includes('new@example.test'))!.trigger('click')
    await wrapper.get('#member-form').trigger('submit.prevent')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 42, expect.objectContaining({ role: 'user', status: 'active' }))
  })

  it('uses global member search when adding project members as super admin', async () => {
    authState.isAdmin = true
    searchBindableResources.mockResolvedValue({
      users: [{ id: 43, email: 'project-member@example.test', username: '', notes: '', status: 'active' }],
      groups: [],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })
    searchGlobalBindableResources.mockResolvedValue({
      users: [{ id: 43, email: 'project-member@example.test', username: '', notes: '', status: 'active' }],
      groups: [],
      accounts: [],
      subscriptions: [],
      api_keys: []
    })

    const wrapper = mountProjectsView()
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'admin.projects.addMember')!.trigger('click')
    await flushPromises()

    await wrapper.get('input[placeholder="admin.projects.searchMemberPlaceholder"]').trigger('keydown.enter')
    await flushPromises()

    expect(searchGlobalBindableResources).toHaveBeenCalledWith('', 20)
    expect(searchBindableResources).not.toHaveBeenCalled()

    await wrapper.findAll('button').find(button => button.text().includes('project-member@example.test'))!.trigger('click')
    await wrapper.get('#member-form').trigger('submit.prevent')
    await flushPromises()

    expect(setMember).toHaveBeenCalledWith(1, 43, expect.objectContaining({ role: 'user', status: 'active' }))
  })
})
