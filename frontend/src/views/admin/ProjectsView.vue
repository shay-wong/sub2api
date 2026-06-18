<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.projects.title') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
              {{ t('admin.projects.description') }}
            </p>
          </div>
          <button v-if="authStore.isAdmin" class="btn btn-primary" @click="openCreateDialog">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.projects.createProject') }}
          </button>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex h-full items-center justify-center py-16">
          <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        </div>
        <div v-else-if="projects.length === 0" class="flex h-full items-center justify-center">
          <EmptyState
            :title="t('admin.projects.noProjects')"
            :description="t('admin.projects.noProjectsDescription')"
            :action-text="authStore.isAdmin ? t('admin.projects.createProject') : undefined"
            @action="openCreateDialog"
          />
        </div>
        <div v-else class="grid h-full grid-cols-1 overflow-hidden lg:grid-cols-[minmax(320px,420px)_1fr]">
          <div class="border-b border-gray-200 dark:border-dark-700 lg:border-b-0 lg:border-r">
            <div class="table-wrapper max-h-full">
              <table>
                <thead>
                  <tr>
                    <th>{{ t('admin.projects.project') }}</th>
                    <th>{{ t('admin.projects.slug') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="project in projects"
                    :key="project.id"
                    class="cursor-pointer"
                    :class="selectedProject?.id === project.id ? 'bg-primary-50 dark:bg-primary-900/20' : 'hover:bg-gray-50 dark:hover:bg-dark-700/60'"
                    @click="selectProject(project)"
                  >
                    <td>
                      <div class="min-w-0">
                        <div class="flex items-center gap-2">
                          <span class="truncate font-medium text-gray-900 dark:text-white">{{ project.name }}</span>
                          <span v-if="project.role" class="badge badge-primary shrink-0">
                            {{ formatProjectRole(project.role) }}
                          </span>
                        </div>
                        <div class="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-dark-300">
                          {{ project.description || t('admin.projects.noDescription') }}
                        </div>
                      </div>
                    </td>
                    <td>
                      <span class="rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">
                        {{ project.slug }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="flex min-h-0 flex-col">
            <div class="border-b border-gray-200 p-4 dark:border-dark-700">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="min-w-0">
                  <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
                    {{ selectedProject?.name || t('admin.projects.projectConfig') }}
                  </h2>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
                    {{ activeTab === 'members' ? t('admin.projects.membersDescription') : t('admin.projects.profilesDescription') }}
                  </p>
                </div>
                <button class="btn btn-secondary" :disabled="!selectedProject || detailLoading" @click="reloadSelectedProject">
                  <Icon name="refresh" size="sm" :class="detailLoading ? 'animate-spin' : ''" />
                </button>
              </div>

              <div class="tabs mt-4">
                <button class="tab" :class="{ 'tab-active': activeTab === 'members' }" @click="activeTab = 'members'">
                  {{ t('admin.projects.members') }}
                </button>
                <button class="tab" :class="{ 'tab-active': activeTab === 'profiles' }" @click="activeTab = 'profiles'">
                  {{ t('admin.projects.appProfiles') }}
                </button>
              </div>
            </div>

            <div v-if="activeTab === 'members'" class="min-h-0 flex-1 overflow-auto">
              <div class="flex justify-end border-b border-gray-100 p-3 dark:border-dark-700">
                <button class="btn btn-primary" :disabled="!selectedProject" @click="openMemberDialog">
                  <Icon name="userPlus" size="md" class="mr-2" />
                  {{ t('admin.projects.addMember') }}
                </button>
              </div>
              <div class="table-wrapper min-h-0">
                <table>
                  <thead>
                    <tr>
                      <th>{{ t('admin.projects.member') }}</th>
                      <th>{{ t('admin.projects.role') }}</th>
                      <th>{{ t('admin.projects.owner') }}</th>
                      <th>{{ t('admin.projects.status') }}</th>
                      <th>{{ t('common.actions') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-if="membersLoading">
                      <td colspan="5" class="py-10 text-center text-gray-500 dark:text-dark-300">
                        {{ t('common.loading') }}
                      </td>
                    </tr>
                    <tr v-else-if="members.length === 0">
                      <td colspan="5" class="py-10 text-center text-gray-500 dark:text-dark-300">
                        {{ t('admin.projects.noMembers') }}
                      </td>
                    </tr>
                    <tr v-for="member in members" v-else :key="member.user_id">
                      <td>
                        <div class="font-medium text-gray-900 dark:text-white">{{ member.email }}</div>
                        <div class="text-xs text-gray-500 dark:text-dark-300">{{ member.username || `#${member.user_id}` }}</div>
                      </td>
                      <td>
                        <select
                          class="input min-w-28 py-1 text-sm"
                          :value="member.role"
                          @change="updateMemberRole(member, ($event.target as HTMLSelectElement).value as AssignableProjectRole)"
                        >
                          <option value="admin">{{ t('admin.users.roles.admin') }}</option>
                          <option value="user">{{ t('admin.users.roles.user') }}</option>
                        </select>
                      </td>
                      <td>
                        <span v-if="member.is_owner" class="badge badge-success">{{ t('admin.projects.owner') }}</span>
                        <button v-else class="btn btn-secondary btn-sm" @click="transferOwner(member)">
                          {{ t('admin.projects.transferOwner') }}
                        </button>
                      </td>
                      <td>
                        <select
                          class="input min-w-28 py-1 text-sm"
                          :value="member.status"
                          :disabled="member.is_owner"
                          @change="updateMemberStatus(member, ($event.target as HTMLSelectElement).value as ProjectMemberStatus)"
                        >
                          <option value="active">{{ t('common.active') }}</option>
                          <option value="disabled">{{ t('admin.users.disabled') }}</option>
                        </select>
                      </td>
                      <td>
                        <button class="btn btn-danger btn-sm" :disabled="member.is_owner" @click="removeMember(member)">
                          {{ t('common.delete') }}
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <div v-else class="min-h-0 flex-1 overflow-auto p-4">
              <div class="grid min-h-full grid-cols-1 gap-4 xl:grid-cols-[minmax(260px,340px)_1fr]">
                <div class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
                  <div class="flex items-center justify-between border-b border-gray-100 p-3 dark:border-dark-700">
                    <div class="font-medium text-gray-900 dark:text-white">{{ t('admin.projects.appProfiles') }}</div>
                    <button class="btn btn-primary btn-sm" :disabled="!selectedProject" @click="openProfileDialog()">
                      <Icon name="plus" size="sm" class="mr-1" />
                      {{ t('admin.projects.addProfile') }}
                    </button>
                  </div>
                  <div v-if="profilesLoading" class="p-6 text-center text-sm text-gray-500 dark:text-dark-300">
                    {{ t('common.loading') }}
                  </div>
                  <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
                    <button
                      v-for="profile in profiles"
                      :key="profile.id"
                      type="button"
                      class="flex w-full items-start justify-between gap-3 px-3 py-3 text-left hover:bg-gray-50 dark:hover:bg-dark-700/60"
                      :class="selectedProfile?.id === profile.id ? 'bg-primary-50 dark:bg-primary-900/20' : ''"
                      @click="selectProfile(profile)"
                    >
                      <span class="min-w-0">
                        <span class="flex items-center gap-2">
                          <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ profile.name }}</span>
                          <span v-if="profile.is_active" class="badge badge-success">{{ t('admin.projects.activeProfile') }}</span>
                        </span>
                        <span class="mt-1 block text-xs text-gray-500 dark:text-dark-300">
                          {{ formatProfileMode(profile.mode) }}
                        </span>
                      </span>
                      <Icon name="chevronRight" size="sm" class="mt-1 text-gray-400" />
                    </button>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
                  <div v-if="!selectedProfile" class="flex h-full min-h-80 items-center justify-center p-8 text-sm text-gray-500 dark:text-dark-300">
                    {{ t('admin.projects.selectProfile') }}
                  </div>
                  <div v-else class="flex min-h-full flex-col">
                    <div class="border-b border-gray-100 p-4 dark:border-dark-700">
                      <div class="flex flex-wrap items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="flex flex-wrap items-center gap-2">
                            <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ selectedProfile.name }}</h3>
                            <span v-if="selectedProfile.is_active" class="badge badge-success">{{ t('admin.projects.activeProfile') }}</span>
                          </div>
                          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
                            {{ selectedProfile.description || t('admin.projects.noDescription') }}
                          </p>
                        </div>
                        <div class="flex flex-wrap gap-2">
                          <button class="btn btn-secondary btn-sm" :disabled="isProfileManagementDisabled(selectedProfile)" @click="openProfileDialog(selectedProfile)">
                            <Icon name="edit" size="sm" class="mr-1" />
                            {{ t('common.edit') }}
                          </button>
                          <button class="btn btn-secondary btn-sm" :disabled="selectedProfile.is_active || isProfileManagementDisabled(selectedProfile)" @click="activateSelectedProfile">
                            <Icon name="check" size="sm" class="mr-1" />
                            {{ t('admin.projects.activateProfile') }}
                          </button>
                          <button class="btn btn-danger btn-sm" :disabled="selectedProfile.is_active || isProfileManagementDisabled(selectedProfile)" @click="deleteSelectedProfile">
                            <Icon name="trash" size="sm" class="mr-1" />
                            {{ t('common.delete') }}
                          </button>
                        </div>
                      </div>

                      <div class="mt-4 inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900">
                        <button
                          type="button"
                          class="rounded-md px-3 py-1.5 text-sm"
                          :class="selectedProfile.mode === 'restricted' ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-dark-300'"
                          :disabled="isProfileManagementDisabled(selectedProfile)"
                          @click="changeProfileMode('restricted')"
                        >
                          {{ t('admin.projects.restrictedMode') }}
                        </button>
                        <button
                          type="button"
                          class="rounded-md px-3 py-1.5 text-sm"
                          :class="selectedProfile.mode === 'unrestricted' ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-dark-300'"
                          :disabled="isProfileManagementDisabled(selectedProfile)"
                          @click="changeProfileMode('unrestricted')"
                        >
                          {{ t('admin.projects.unrestrictedMode') }}
                        </button>
                      </div>
                    </div>

                    <div v-if="selectedProfile.mode === 'unrestricted'" class="flex flex-1 items-center justify-center p-8">
                      <div class="max-w-md text-center">
                        <Icon name="globe" size="xl" class="mx-auto text-primary-500" />
                        <h4 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.projects.unrestrictedMode') }}</h4>
                        <p class="mt-2 text-sm text-gray-500 dark:text-dark-300">
                          {{ t('admin.projects.unrestrictedHint') }}
                        </p>
                      </div>
                    </div>

                    <div v-else class="flex flex-1 flex-col gap-4 p-4">
                      <div class="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(220px,280px)_1fr]">
                        <div>
                          <label class="input-label">{{ t('admin.projects.resourceType') }}</label>
                          <select v-model="resourceSearch.type" class="input">
                            <option v-for="type in resourceTypes" :key="type" :value="type">
                              {{ resourceTypeLabel(type) }}
                            </option>
                          </select>
                        </div>
                        <div>
                          <label class="input-label">{{ t('admin.projects.searchResources') }}</label>
                          <div class="flex gap-2">
                            <input
                              v-model.trim="resourceSearch.query"
                              class="input"
                              :placeholder="t('admin.projects.searchResourcesPlaceholder')"
                              @keydown.enter.prevent="searchResources"
                            />
                            <button class="btn btn-secondary" :disabled="resourceSearch.loading" @click="searchResources">
                              <Icon name="search" size="sm" :class="resourceSearch.loading ? 'animate-spin' : ''" />
                            </button>
                          </div>
                        </div>
                      </div>

                      <div v-if="currentCandidates.length > 0" class="rounded-lg border border-gray-200 dark:border-dark-700">
                        <div class="border-b border-gray-100 px-3 py-2 text-sm font-medium text-gray-900 dark:border-dark-700 dark:text-white">
                          {{ t('admin.projects.searchResults') }}
                        </div>
                        <div class="max-h-56 divide-y divide-gray-100 overflow-auto dark:divide-dark-700">
                          <div v-for="candidate in currentCandidates" :key="candidate.key" class="flex items-center justify-between gap-3 px-3 py-2">
                            <div class="min-w-0">
                              <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ candidate.title }}</div>
                              <div class="truncate text-xs text-gray-500 dark:text-dark-300">{{ candidate.subtitle }}</div>
                            </div>
                            <button class="btn btn-secondary btn-sm" :disabled="isResourceBound(candidate.type, candidate.id)" @click="addResourceBinding(candidate)">
                              {{ isResourceBound(candidate.type, candidate.id) ? t('admin.projects.bound') : t('admin.projects.bind') }}
                            </button>
                          </div>
                        </div>
                      </div>

                      <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
                        <div
                          v-for="type in resourceTypes"
                          :key="type"
                          class="rounded-lg border border-gray-200 dark:border-dark-700"
                        >
                          <div class="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-dark-700">
                            <div class="text-sm font-medium text-gray-900 dark:text-white">{{ resourceTypeLabel(type) }}</div>
                            <span class="text-xs text-gray-500 dark:text-dark-300">{{ boundIDs(type).length }}</span>
                          </div>
                          <div class="max-h-48 overflow-auto p-2">
                            <div v-if="boundIDs(type).length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-dark-300">
                              {{ t('admin.projects.noBindings') }}
                            </div>
                            <div
                              v-for="id in boundIDs(type)"
                              v-else
                              :key="`${type}-${id}`"
                              class="mb-2 flex items-center justify-between gap-2 rounded-md bg-gray-50 px-2 py-2 text-sm dark:bg-dark-700"
                            >
                              <div class="min-w-0">
                                <div class="truncate font-medium text-gray-900 dark:text-white">{{ bindingLabel(type, id) }}</div>
                                <div class="text-xs text-gray-500 dark:text-dark-300">#{{ id }}</div>
                              </div>
                              <button class="text-gray-400 hover:text-red-600 dark:hover:text-red-400" @click="removeResourceBinding(type, id)">
                                <Icon name="x" size="sm" />
                              </button>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showProjectDialog"
      :title="t('admin.projects.createProject')"
      width="normal"
      @close="showProjectDialog = false"
    >
      <form id="project-form" class="space-y-4" @submit.prevent="submitProject">
        <div>
          <label class="input-label">{{ t('admin.projects.name') }}</label>
          <input v-model.trim="projectForm.name" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.slug') }}</label>
          <input v-model.trim="projectForm.slug" class="input font-mono" required />
          <p class="input-hint">{{ t('admin.projects.slugHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.descriptionField') }}</label>
          <textarea v-model.trim="projectForm.description" class="input min-h-24 resize-y" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.profileMode') }}</label>
          <div class="grid grid-cols-2 gap-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
            <button
              type="button"
              class="rounded-md px-3 py-2 text-sm transition-colors"
              :class="projectForm.profileMode === 'restricted' ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'"
              @click="projectForm.profileMode = 'restricted'"
            >
              {{ t('admin.projects.restrictedMode') }}
            </button>
            <button
              type="button"
              class="rounded-md px-3 py-2 text-sm transition-colors"
              :class="projectForm.profileMode === 'unrestricted' ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'"
              @click="projectForm.profileMode = 'unrestricted'"
            >
              {{ t('admin.projects.unrestrictedMode') }}
            </button>
          </div>
          <p class="input-hint">
            {{ projectForm.profileMode === 'unrestricted' ? t('admin.projects.createUnrestrictedHint') : t('admin.projects.createRestrictedHint') }}
          </p>
        </div>
        <div class="space-y-3">
          <div>
            <label class="input-label">{{ t('admin.projects.initialBindings') }}</label>
            <p class="input-hint">{{ t('admin.projects.initialBindingsHint') }}</p>
          </div>
          <div v-if="projectForm.profileMode === 'unrestricted'" class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-4 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200">
            {{ t('admin.projects.unrestrictedHint') }}
          </div>
          <div v-else class="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(160px,220px)_1fr]">
            <div>
              <select v-model="projectResourceSearch.type" class="input">
                <option v-for="type in resourceTypes" :key="type" :value="type">
                  {{ resourceTypeLabel(type) }}
                </option>
              </select>
            </div>
            <div class="flex gap-2">
              <input
                v-model.trim="projectResourceSearch.query"
                class="input"
                :placeholder="t('admin.projects.searchResourcesPlaceholder')"
                @keydown.enter.prevent="searchProjectResources"
              />
              <button type="button" class="btn btn-secondary" :disabled="projectResourceSearch.loading" @click="searchProjectResources">
                <Icon name="search" size="sm" :class="projectResourceSearch.loading ? 'animate-spin' : ''" />
              </button>
            </div>
          </div>

          <div v-if="projectForm.profileMode === 'restricted' && projectCurrentCandidates.length > 0" class="max-h-48 overflow-auto rounded-lg border border-gray-200 dark:border-dark-700">
            <div v-for="candidate in projectCurrentCandidates" :key="candidate.key" class="flex items-center justify-between gap-3 px-3 py-2">
              <div class="min-w-0">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ candidate.title }}</div>
                <div class="truncate text-xs text-gray-500 dark:text-dark-300">{{ candidate.subtitle }}</div>
              </div>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="isProjectResourceBound(candidate.type, candidate.id)" @click="addProjectResourceBinding(candidate)">
                {{ isProjectResourceBound(candidate.type, candidate.id) ? t('admin.projects.bound') : t('admin.projects.bind') }}
              </button>
            </div>
          </div>

          <div v-if="projectForm.profileMode === 'restricted'" class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div v-for="type in resourceTypes" :key="type" class="rounded-lg border border-gray-200 p-2 dark:border-dark-700">
              <div class="mb-2 flex items-center justify-between text-xs font-medium text-gray-700 dark:text-dark-200">
                <span>{{ resourceTypeLabel(type) }}</span>
                <span>{{ projectBoundIDs(type).length }}</span>
              </div>
              <div v-if="projectBoundIDs(type).length === 0" class="py-3 text-center text-xs text-gray-500 dark:text-dark-300">
                {{ t('admin.projects.noBindings') }}
              </div>
              <div
                v-for="id in projectBoundIDs(type)"
                v-else
                :key="`project-${type}-${id}`"
                class="mb-1 flex items-center justify-between gap-2 rounded-md bg-gray-50 px-2 py-1 text-sm dark:bg-dark-700"
              >
                <span class="truncate text-gray-900 dark:text-white">{{ bindingLabel(type, id) }}</span>
                <button type="button" class="text-gray-400 hover:text-red-600 dark:hover:text-red-400" @click="removeProjectResourceBinding(type, id)">
                  <Icon name="x" size="sm" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showProjectDialog = false">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="project-form" type="submit" :disabled="savingProject">
          {{ savingProject ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showMemberDialog"
      :title="t('admin.projects.addMember')"
      width="normal"
      @close="showMemberDialog = false"
    >
      <form id="member-form" class="space-y-4" @submit.prevent="submitMember">
        <div>
          <label class="input-label">{{ t('admin.projects.searchMember') }}</label>
          <div class="flex gap-2">
            <input
              v-model.trim="memberSearch.query"
              class="input"
              :placeholder="t('admin.projects.searchMemberPlaceholder')"
              @keydown.enter.prevent="searchMembers"
            />
            <button type="button" class="btn btn-secondary" :disabled="memberSearch.loading" @click="searchMembers">
              <Icon name="search" size="sm" :class="memberSearch.loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
        <div v-if="memberCandidates.length > 0" class="max-h-48 overflow-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <button
            v-for="candidate in memberCandidates"
            :key="candidate.id"
            type="button"
            class="block w-full px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
            :class="memberForm.userId === candidate.id ? 'bg-primary-50 dark:bg-primary-900/20' : ''"
            @click="selectMemberCandidate(candidate)"
          >
            <div class="text-sm font-medium text-gray-900 dark:text-white">{{ candidate.email }}</div>
            <div class="text-xs text-gray-500 dark:text-dark-300">{{ candidate.username || candidate.notes || `#${candidate.id}` }}</div>
          </button>
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.userId') }}</label>
          <input v-model.number="memberForm.userId" type="number" min="1" class="input" required />
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.projects.role') }}</label>
            <select v-model="memberForm.role" class="input">
              <option value="admin">{{ t('admin.users.roles.admin') }}</option>
              <option value="user">{{ t('admin.users.roles.user') }}</option>
            </select>
          </div>
          <div>
            <label class="input-label">{{ t('admin.projects.status') }}</label>
            <select v-model="memberForm.status" class="input">
              <option value="active">{{ t('common.active') }}</option>
              <option value="disabled">{{ t('admin.users.disabled') }}</option>
            </select>
          </div>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showMemberDialog = false">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="member-form" type="submit" :disabled="savingMember">
          {{ savingMember ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showProfileDialog"
      :title="profileForm.id ? t('admin.projects.editProfile') : t('admin.projects.addProfile')"
      width="normal"
      @close="showProfileDialog = false"
    >
      <form id="profile-form" class="space-y-4" @submit.prevent="submitProfile">
        <div>
          <label class="input-label">{{ t('admin.projects.profileName') }}</label>
          <input v-model.trim="profileForm.name" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.descriptionField') }}</label>
          <textarea v-model.trim="profileForm.description" class="input min-h-20 resize-y" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.profileMode') }}</label>
          <select v-model="profileForm.mode" class="input">
            <option value="restricted">{{ t('admin.projects.restrictedMode') }}</option>
            <option value="unrestricted">
              {{ t('admin.projects.unrestrictedMode') }}
            </option>
          </select>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showProfileDialog = false">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="profile-form" type="submit" :disabled="savingProfile">
          {{ savingProfile ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type {
  AdminProject,
  AssignableProjectRole,
  ProjectMember,
  ProjectMemberStatus,
  ProjectProfile,
  ProjectProfileBindings,
  ProjectProfileMode,
  ProjectResourceSearchResult,
  ProjectResourceType,
  ProjectResourceUserCandidate
} from '@/api/admin/projects'
import { useAppStore, useAuthStore } from '@/stores'

type CandidateItem = {
  id: number
  key: string
  type: ProjectResourceType
  title: string
  subtitle: string
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const membersLoading = ref(false)
const profilesLoading = ref(false)
const savingProject = ref(false)
const savingMember = ref(false)
const savingProfile = ref(false)
const savingBindings = ref(false)
const projects = ref<AdminProject[]>([])
const members = ref<ProjectMember[]>([])
const profiles = ref<ProjectProfile[]>([])
const selectedProject = ref<AdminProject | null>(null)
const selectedProfile = ref<ProjectProfile | null>(null)
const bindings = ref<ProjectProfileBindings | null>(null)
const activeTab = ref<'members' | 'profiles'>('members')
const showProjectDialog = ref(false)
const showMemberDialog = ref(false)
const showProfileDialog = ref(false)
const searchResult = ref<ProjectResourceSearchResult | null>(null)
const bindingNames = reactive<Record<ProjectResourceType, Record<number, string>>>({
  users: {},
  groups: {},
  accounts: {},
  subscriptions: {},
  api_keys: {}
})

const resourceTypes: ProjectResourceType[] = ['users', 'groups', 'accounts', 'subscriptions', 'api_keys']

const projectForm = reactive({
  name: '',
  slug: '',
  description: '',
  profileMode: 'restricted' as ProjectProfileMode,
  bindings: {
    profile_id: 0,
    user_ids: [] as number[],
    group_ids: [] as number[],
    account_ids: [] as number[],
    subscription_ids: [] as number[],
    api_key_ids: [] as number[]
  } satisfies ProjectProfileBindings
})

const memberForm = reactive({
  userId: undefined as number | undefined,
  role: 'user' as AssignableProjectRole,
  status: 'active' as ProjectMemberStatus
})

const memberSearch = reactive({
  query: '',
  loading: false
})

const memberCandidates = ref<ProjectResourceUserCandidate[]>([])

const profileForm = reactive({
  id: undefined as number | undefined,
  name: '',
  description: '',
  mode: 'restricted' as ProjectProfileMode
})

const resourceSearch = reactive({
  type: 'users' as ProjectResourceType,
  query: '',
  loading: false
})

const projectResourceSearch = reactive({
  type: 'users' as ProjectResourceType,
  query: '',
  loading: false
})

const projectSearchResult = ref<ProjectResourceSearchResult | null>(null)

const detailLoading = computed(() => membersLoading.value || profilesLoading.value)

const currentCandidates = computed<CandidateItem[]>(() => {
  if (!searchResult.value) return []
  return candidatesForType(resourceSearch.type, searchResult.value)
})

const projectCurrentCandidates = computed<CandidateItem[]>(() => {
  if (!projectSearchResult.value) return []
  return candidatesForType(projectResourceSearch.type, projectSearchResult.value)
})

async function loadProjects() {
  loading.value = true
  try {
    projects.value = await adminAPI.projects.list()
    if (!selectedProject.value || !projects.value.some(project => project.id === selectedProject.value?.id)) {
      selectedProject.value = projects.value[0] ?? null
    }
    if (selectedProject.value) {
      await reloadSelectedProject()
    } else {
      members.value = []
      profiles.value = []
      selectedProfile.value = null
      bindings.value = null
    }
  } catch (error) {
    console.error('Failed to load projects:', error)
    appStore.showError(t('admin.projects.failedToLoad'))
  } finally {
    loading.value = false
  }
}

async function reloadSelectedProject() {
  if (!selectedProject.value) return
  await Promise.all([loadMembers(), loadProfiles()])
}

async function loadMembers() {
  if (!selectedProject.value) return
  membersLoading.value = true
  try {
    members.value = await adminAPI.projects.listMembers(selectedProject.value.id)
  } catch (error) {
    console.error('Failed to load project members:', error)
    appStore.showError(t('admin.projects.failedToLoadMembers'))
  } finally {
    membersLoading.value = false
  }
}

async function loadProfiles() {
  if (!selectedProject.value) return
  profilesLoading.value = true
  try {
    profiles.value = await adminAPI.projects.listProfiles(selectedProject.value.id)
    const next = selectedProfile.value
      ? profiles.value.find(profile => profile.id === selectedProfile.value?.id)
      : profiles.value.find(profile => profile.is_active) ?? profiles.value[0]
    selectedProfile.value = next ?? null
    if (selectedProfile.value) {
      await loadBindings(selectedProfile.value)
    } else {
      bindings.value = null
    }
  } catch (error) {
    console.error('Failed to load project profiles:', error)
    appStore.showError(t('admin.projects.failedToLoadProfiles'))
  } finally {
    profilesLoading.value = false
  }
}

async function loadBindings(profile: ProjectProfile) {
  if (!selectedProject.value) return
  try {
    bindings.value = await adminAPI.projects.getProfileBindings(selectedProject.value.id, profile.id)
  } catch (error) {
    console.error('Failed to load project profile bindings:', error)
    appStore.showError(t('admin.projects.failedToLoadBindings'))
  }
}

async function selectProject(project: AdminProject) {
  selectedProject.value = project
  selectedProfile.value = null
  searchResult.value = null
  await reloadSelectedProject()
}

async function selectProfile(profile: ProjectProfile) {
  selectedProfile.value = profile
  searchResult.value = null
  await loadBindings(profile)
}

function openCreateDialog() {
  projectForm.name = ''
  projectForm.slug = ''
  projectForm.description = ''
  projectForm.profileMode = 'restricted'
  projectForm.bindings = emptyProjectBindings()
  projectResourceSearch.type = 'users'
  projectResourceSearch.query = ''
  projectSearchResult.value = null
  showProjectDialog.value = true
}

async function submitProject() {
  savingProject.value = true
  try {
    const bindings = projectForm.profileMode === 'restricted' ? projectForm.bindings : emptyProjectBindings()
    const project = await adminAPI.projects.create({
      name: projectForm.name,
      slug: projectForm.slug,
      description: projectForm.description || null,
      profile_mode: projectForm.profileMode,
      user_ids: bindings.user_ids,
      group_ids: bindings.group_ids,
      account_ids: bindings.account_ids,
      subscription_ids: bindings.subscription_ids,
      api_key_ids: bindings.api_key_ids
    })
    appStore.showSuccess(t('admin.projects.projectCreated'))
    showProjectDialog.value = false
    await loadProjects()
    selectedProject.value = projects.value.find(item => item.id === project.id) ?? selectedProject.value
    await reloadSelectedProject()
  } catch (error) {
    console.error('Failed to create project:', error)
    appStore.showError(t('admin.projects.failedToCreate'))
  } finally {
    savingProject.value = false
  }
}

function openMemberDialog() {
  memberForm.userId = undefined
  memberForm.role = 'user'
  memberForm.status = 'active'
  memberSearch.query = ''
  memberCandidates.value = []
  showMemberDialog.value = true
}

async function searchMembers() {
  if (!selectedProject.value) return
  memberSearch.loading = true
  try {
    const result = authStore.isAdmin
      ? await adminAPI.projects.searchGlobalBindableResources(memberSearch.query, 20)
      : await adminAPI.projects.searchBindableResources(selectedProject.value.id, memberSearch.query, 20)
    memberCandidates.value = result.users
  } catch (error) {
    console.error('Failed to search project members:', error)
    appStore.showError(t('admin.projects.failedToSearchResources'))
  } finally {
    memberSearch.loading = false
  }
}

function selectMemberCandidate(candidate: ProjectResourceUserCandidate) {
  memberForm.userId = candidate.id
  bindingNames.users[candidate.id] = candidate.email
}

async function submitMember() {
  if (!selectedProject.value || !memberForm.userId) return
  savingMember.value = true
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, memberForm.userId, {
      role: memberForm.role,
      status: memberForm.status
    })
    appStore.showSuccess(t('admin.projects.memberSaved'))
    showMemberDialog.value = false
    await loadMembers()
  } catch (error) {
    console.error('Failed to save project member:', error)
    appStore.showError(t('admin.projects.failedToSaveMember'))
  } finally {
    savingMember.value = false
  }
}

async function updateMemberRole(member: ProjectMember, role: AssignableProjectRole) {
  if (!selectedProject.value || role === member.role) return
  await saveMember(member, { role })
}

async function updateMemberStatus(member: ProjectMember, status: ProjectMemberStatus) {
  if (!selectedProject.value || status === member.status) return
  await saveMember(member, { status })
}

async function transferOwner(member: ProjectMember) {
  if (!selectedProject.value || member.is_owner) return
  await saveMember(member, { is_owner: true, status: 'active' })
}

async function saveMember(member: ProjectMember, patch: { role?: AssignableProjectRole; is_owner?: boolean; status?: ProjectMemberStatus }) {
  if (!selectedProject.value) return
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, member.user_id, {
      role: patch.role ?? member.role,
      is_owner: patch.is_owner ?? member.is_owner,
      status: patch.status ?? member.status
    })
    appStore.showSuccess(t('admin.projects.memberSaved'))
    await loadMembers()
  } catch (error) {
    console.error('Failed to save project member:', error)
    appStore.showError(t('admin.projects.failedToSaveMember'))
    await loadMembers()
  }
}

async function removeMember(member: ProjectMember) {
  if (!selectedProject.value) return
  try {
    await adminAPI.projects.removeMember(selectedProject.value.id, member.user_id)
    appStore.showSuccess(t('admin.projects.memberRemoved'))
    await loadMembers()
  } catch (error) {
    console.error('Failed to remove project member:', error)
    appStore.showError(t('admin.projects.failedToRemoveMember'))
  }
}

function openProfileDialog(profile?: ProjectProfile) {
  profileForm.id = profile?.id
  profileForm.name = profile?.name ?? ''
  profileForm.description = profile?.description ?? ''
  profileForm.mode = profile?.mode ?? 'restricted'
  showProfileDialog.value = true
}

async function submitProfile() {
  if (!selectedProject.value) return
  savingProfile.value = true
  try {
    if (profileForm.id) {
      const payload: { name: string; description: string | null; mode?: ProjectProfileMode } = {
        name: profileForm.name,
        description: profileForm.description || null
      }
      payload.mode = profileForm.mode
      await adminAPI.projects.updateProfile(selectedProject.value.id, profileForm.id, payload)
      appStore.showSuccess(t('admin.projects.profileSaved'))
    } else {
      const profile = await adminAPI.projects.createProfile(selectedProject.value.id, {
        name: profileForm.name,
        description: profileForm.description || null,
        mode: profileForm.mode
      })
      selectedProfile.value = profile
      appStore.showSuccess(t('admin.projects.profileCreated'))
    }
    showProfileDialog.value = false
    await loadProfiles()
  } catch (error) {
    console.error('Failed to save project profile:', error)
    appStore.showError(t('admin.projects.failedToSaveProfile'))
  } finally {
    savingProfile.value = false
  }
}

async function changeProfileMode(mode: ProjectProfileMode) {
  if (!selectedProject.value || !selectedProfile.value || selectedProfile.value.mode === mode) return
  if (isProfileManagementDisabled(selectedProfile.value)) return
  try {
    const updated = await adminAPI.projects.updateProfile(selectedProject.value.id, selectedProfile.value.id, { mode })
    selectedProfile.value = updated
    profiles.value = profiles.value.map(profile => profile.id === updated.id ? updated : profile)
    await loadBindings(updated)
    appStore.showSuccess(t('admin.projects.profileSaved'))
  } catch (error) {
    console.error('Failed to change project profile mode:', error)
    appStore.showError(t('admin.projects.failedToSaveProfile'))
  }
}

async function activateSelectedProfile() {
  if (!selectedProject.value || !selectedProfile.value || selectedProfile.value.is_active) return
  try {
    const updated = await adminAPI.projects.activateProfile(selectedProject.value.id, selectedProfile.value.id)
    selectedProfile.value = updated
    await loadProfiles()
    appStore.showSuccess(t('admin.projects.profileActivated'))
  } catch (error) {
    console.error('Failed to activate project profile:', error)
    appStore.showError(t('admin.projects.failedToActivateProfile'))
  }
}

function isProfileManagementDisabled(_profile: ProjectProfile): boolean {
  return false
}

async function deleteSelectedProfile() {
  if (!selectedProject.value || !selectedProfile.value || selectedProfile.value.is_active) return
  try {
    await adminAPI.projects.deleteProfile(selectedProject.value.id, selectedProfile.value.id)
    selectedProfile.value = null
    appStore.showSuccess(t('admin.projects.profileDeleted'))
    await loadProfiles()
  } catch (error) {
    console.error('Failed to delete project profile:', error)
    appStore.showError(t('admin.projects.failedToDeleteProfile'))
  }
}

async function searchResources() {
  if (!selectedProject.value) return
  resourceSearch.loading = true
  try {
    searchResult.value = await adminAPI.projects.searchBindableResources(selectedProject.value.id, resourceSearch.query, 30)
    indexSearchResultNames(searchResult.value)
  } catch (error) {
    console.error('Failed to search bindable resources:', error)
    appStore.showError(t('admin.projects.failedToSearchResources'))
  } finally {
    resourceSearch.loading = false
  }
}

async function searchProjectResources() {
  projectResourceSearch.loading = true
  try {
    projectSearchResult.value = await adminAPI.projects.searchGlobalBindableResources(projectResourceSearch.query, 30)
    indexSearchResultNames(projectSearchResult.value)
  } catch (error) {
    console.error('Failed to search bindable resources:', error)
    appStore.showError(t('admin.projects.failedToSearchResources'))
  } finally {
    projectResourceSearch.loading = false
  }
}

function addProjectResourceBinding(candidate: CandidateItem) {
  const ids = bindingIDsRef(projectForm.bindings, candidate.type)
  if (!ids.includes(candidate.id)) {
    ids.push(candidate.id)
  }
  bindingNames[candidate.type][candidate.id] = candidate.title
}

function removeProjectResourceBinding(type: ProjectResourceType, id: number) {
  const ids = bindingIDsRef(projectForm.bindings, type)
  const index = ids.indexOf(id)
  if (index >= 0) ids.splice(index, 1)
}

function projectBoundIDs(type: ProjectResourceType): number[] {
  return bindingIDsRef(projectForm.bindings, type)
}

function isProjectResourceBound(type: ProjectResourceType, id: number): boolean {
  return projectBoundIDs(type).includes(id)
}

async function addResourceBinding(candidate: CandidateItem) {
  if (!bindings.value) return
  const next = cloneBindings(bindings.value)
  const ids = bindingIDsRef(next, candidate.type)
  if (!ids.includes(candidate.id)) {
    ids.push(candidate.id)
  }
  bindingNames[candidate.type][candidate.id] = candidate.title
  await saveBindings(next)
}

async function removeResourceBinding(type: ProjectResourceType, id: number) {
  if (!bindings.value) return
  const next = cloneBindings(bindings.value)
  const ids = bindingIDsRef(next, type)
  const index = ids.indexOf(id)
  if (index >= 0) ids.splice(index, 1)
  await saveBindings(next)
}

async function saveBindings(next: ProjectProfileBindings) {
  if (!selectedProject.value || !selectedProfile.value || savingBindings.value) return
  savingBindings.value = true
  try {
    bindings.value = await adminAPI.projects.setProfileBindings(selectedProject.value.id, selectedProfile.value.id, normalizeBindings(next))
    appStore.showSuccess(t('admin.projects.bindingsSaved'))
  } catch (error) {
    console.error('Failed to save project profile bindings:', error)
    appStore.showError(t('admin.projects.failedToSaveBindings'))
  } finally {
    savingBindings.value = false
  }
}

function cloneBindings(value: ProjectProfileBindings): ProjectProfileBindings {
  return {
    profile_id: value.profile_id,
    user_ids: [...(value.user_ids ?? [])],
    group_ids: [...(value.group_ids ?? [])],
    account_ids: [...(value.account_ids ?? [])],
    subscription_ids: [...(value.subscription_ids ?? [])],
    api_key_ids: [...(value.api_key_ids ?? [])]
  }
}

function emptyProjectBindings(): ProjectProfileBindings {
  return {
    profile_id: 0,
    user_ids: [],
    group_ids: [],
    account_ids: [],
    subscription_ids: [],
    api_key_ids: []
  }
}

function normalizeBindings(value: ProjectProfileBindings): ProjectProfileBindings {
  return {
    profile_id: value.profile_id,
    user_ids: uniqueSorted(value.user_ids),
    group_ids: uniqueSorted(value.group_ids),
    account_ids: uniqueSorted(value.account_ids),
    subscription_ids: uniqueSorted(value.subscription_ids),
    api_key_ids: uniqueSorted(value.api_key_ids)
  }
}

function uniqueSorted(ids: number[]): number[] {
  return Array.from(new Set(ids.filter(id => Number.isSafeInteger(id) && id > 0))).sort((a, b) => a - b)
}

function bindingIDsRef(value: ProjectProfileBindings, type: ProjectResourceType): number[] {
  switch (type) {
    case 'users': return value.user_ids
    case 'groups': return value.group_ids
    case 'accounts': return value.account_ids
    case 'subscriptions': return value.subscription_ids
    case 'api_keys': return value.api_key_ids
  }
}

function boundIDs(type: ProjectResourceType): number[] {
  if (!bindings.value) return []
  return bindingIDsRef(bindings.value, type)
}

function isResourceBound(type: ProjectResourceType, id: number): boolean {
  return boundIDs(type).includes(id)
}

function bindingLabel(type: ProjectResourceType, id: number): string {
  return bindingNames[type][id] || `${resourceTypeLabel(type)} #${id}`
}

function candidatesForType(type: ProjectResourceType, result: ProjectResourceSearchResult): CandidateItem[] {
  switch (type) {
    case 'users':
      return result.users.map(item => ({
        id: item.id,
        key: `users-${item.id}`,
        type,
        title: item.email,
        subtitle: item.username || item.notes || item.status
      }))
    case 'groups':
      return result.groups.map(item => ({
        id: item.id,
        key: `groups-${item.id}`,
        type,
        title: item.name,
        subtitle: [item.platform, item.status, `project #${item.project_id}`].filter(Boolean).join(' · ')
      }))
    case 'accounts':
      return result.accounts.map(item => ({
        id: item.id,
        key: `accounts-${item.id}`,
        type,
        title: item.name || item.email || `#${item.id}`,
        subtitle: [item.platform, item.type, item.status, item.notes].filter(Boolean).join(' · ')
      }))
    case 'subscriptions':
      return result.subscriptions.map(item => ({
        id: item.id,
        key: `subscriptions-${item.id}`,
        type,
        title: `${item.user_email} / ${item.group_name}`,
        subtitle: [item.status, item.notes].filter(Boolean).join(' · ')
      }))
    case 'api_keys':
      return result.api_keys.map(item => ({
        id: item.id,
        key: `api_keys-${item.id}`,
        type,
        title: item.name || item.key_prefix || `#${item.id}`,
        subtitle: [item.user_email, item.key_prefix, item.status].filter(Boolean).join(' · ')
      }))
  }
}

function indexSearchResultNames(result: ProjectResourceSearchResult) {
  for (const type of resourceTypes) {
    for (const candidate of candidatesForType(type, result)) {
      bindingNames[type][candidate.id] = candidate.title
    }
  }
}

function resourceTypeLabel(type: ProjectResourceType): string {
  return t(`admin.projects.resourceTypes.${type}`)
}

function formatProjectRole(role: string): string {
  if (role === 'super_admin') return t('admin.users.roles.super_admin')
  if (role === 'admin') return t('admin.users.roles.admin')
  return t('admin.users.roles.user')
}

function formatProfileMode(mode: ProjectProfileMode): string {
  return mode === 'unrestricted' ? t('admin.projects.unrestrictedMode') : t('admin.projects.restrictedMode')
}

onMounted(() => {
  loadProjects()
})
</script>
