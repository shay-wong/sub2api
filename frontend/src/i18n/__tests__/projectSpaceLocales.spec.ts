import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const requiredKeys = [
  'nav.projects',
  'common.apply',
  'admin.projectSwitcher.label',
  'admin.users.roles.super_admin',
  'admin.users.roles.admin',
  'admin.users.roles.user',
  'admin.projects.title',
  'admin.projects.description',
  'admin.projects.defaultProjectDescription',
  'admin.projects.owner',
  'admin.projects.createProject',
  'admin.projects.noProjects',
  'admin.projects.noProjectsDescription',
  'admin.projects.members',
  'admin.projects.membersDescription',
  'admin.projects.appProfiles',
  'admin.projects.profilesDescription',
  'admin.projects.addMember',
  'admin.projects.addProfile',
  'admin.projects.activeScope',
  'admin.projects.unrestrictedMode',
  'admin.projects.restrictedMode',
  'admin.projects.activeProfile',
  'admin.projects.noDescription',
  'admin.projects.noBindings',
  'admin.projects.apiKeys',
  'admin.projects.accountDisabled',
  'admin.projects.name',
  'admin.projects.slug',
  'admin.projects.slugHint',
  'admin.projects.descriptionField',
  'admin.projects.profileMode',
  'admin.projects.createUnrestrictedHint',
  'admin.projects.createRestrictedHint',
  'admin.projects.initialBindings',
  'admin.projects.initialBindingsHint',
  'admin.projects.addResource',
  'admin.projects.searchMember',
  'admin.projects.searchMemberPlaceholder',
  'admin.projects.searchResourcesPlaceholder',
  'admin.projects.userId',
  'admin.projects.role',
  'admin.projects.status',
  'admin.projects.adminPermissions',
  'admin.projects.editMember',
  'admin.projects.transferOwnerHint',
  'admin.projects.transferOwner',
  'admin.projects.editProfile',
  'admin.projects.profileName',
  'admin.projects.resourceBindings',
  'admin.projects.resourceBindingsHint',
  'admin.projects.resourceBindingsLoadFailedHint',
  'admin.projects.applySelection',
  'admin.projects.member',
  'admin.projects.noMembers',
  'admin.projects.noMemberApiKeys',
  'admin.projects.memberApiKeys',
  'admin.projects.selectedCount',
  'admin.projects.clearSelection',
  'admin.projects.selectAll',
  'admin.projects.targetProject',
  'admin.projects.selectTargetProject',
  'admin.projects.apiKeyTransferSuperAdminOnly',
  'admin.projects.apiKeyTransferHint',
  'admin.projects.apiKeyTransferDisabledMemberOption',
  'admin.projects.removeMemberConfirmTitle',
  'admin.projects.removeMemberConfirmMessage',
  'admin.projects.deleteProfileConfirmTitle',
  'admin.projects.deleteProfileConfirmMessage',
  'admin.projects.projectCreated',
  'admin.projects.profileSaved',
  'admin.projects.profileCreated',
  'admin.projects.profileActivated',
  'admin.projects.profileDeleted',
  'admin.projects.memberSaved',
  'admin.projects.memberRemoved',
  'admin.projects.apiKeyTransferred',
  'admin.projects.failedToLoad',
  'admin.projects.failedToCreate',
  'admin.projects.failedToLoadMembers',
  'admin.projects.failedToSaveMember',
  'admin.projects.failedToRemoveMember',
  'admin.projects.failedToLoadProfiles',
  'admin.projects.failedToSaveProfile',
  'admin.projects.failedToActivateProfile',
  'admin.projects.failedToDeleteProfile',
  'admin.projects.failedToLoadBindings',
  'admin.projects.failedToSearchResources',
  'admin.projects.failedToLoadMemberApiKeys',
  'admin.projects.failedToTransferApiKey',
  'admin.projects.noAdminPermissions',
  'admin.projects.permissions.dashboard',
  'admin.projects.permissions.ops',
  'admin.projects.permissions.users',
  'admin.projects.permissions.groups',
  'admin.projects.permissions.proxies',
  'admin.projects.permissions.subscriptions',
  'admin.projects.permissions.accounts',
  'admin.projects.permissions.usage',
  'admin.projects.resourceTypes.groups',
  'admin.projects.resourceTypes.accounts',
  'admin.projects.resourceTypes.proxies',
  'admin.projects.resourceTypes.subscriptions'
] as const

function valueAt(source: Record<string, unknown>, key: string): unknown {
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[part]
  }, source)
}

describe.each([
  ['zh', zh],
  ['en', en]
] as const)('project space locale keys for %s', (_locale, messages) => {
  for (const key of requiredKeys) {
    it(`defines ${key}`, () => {
      const value = valueAt(messages, key)
      expect(value, key).toEqual(expect.any(String))
      expect(value).not.toBe(key)
    })
  }
})

describe('zh project space locale copy', () => {
  it('does not leave project owner labels in English', () => {
    expect(valueAt(zh, 'admin.projects.owner')).toBe('所有者')
    expect(valueAt(zh, 'admin.projects.transferOwner')).toBe('转移所有者')
  })
})
