<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <h1 class="truncate text-xl font-semibold text-gray-900 dark:text-white">
                {{ selectedProject?.name || t('admin.projects.title') }}
              </h1>
              <span v-if="selectedProject" class="rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                {{ selectedProject.slug }}
              </span>
              <span v-if="selectedProject?.role" class="badge badge-primary shrink-0">
                {{ formatProjectRole(selectedProject.role) }}
              </span>
              <span v-if="selectedProject?.is_owner" class="badge badge-success shrink-0">
                {{ t('admin.projects.owner') }}
              </span>
            </div>
            <p class="mt-1 line-clamp-1 text-sm text-gray-500 dark:text-dark-300">
              {{ selectedProject?.description || t('admin.projects.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              v-if="projects.length > 0"
              class="btn btn-secondary"
              :disabled="!selectedProject || detailLoading"
              @click="reloadSelectedProject"
            >
              <Icon name="refresh" size="sm" :class="detailLoading ? 'animate-spin' : ''" />
            </button>
            <button v-if="authStore.isAdmin" class="btn btn-primary" @click="openCreateDialog">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.projects.createProject') }}
            </button>
          </div>
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
        <div v-else-if="!selectedProject" class="flex h-full items-center justify-center">
          <EmptyState :title="t('admin.projects.noProjects')" :description="t('admin.projects.noProjectsDescription')" />
        </div>
        <div v-else class="grid h-full min-h-0 grid-cols-1 lg:grid-cols-[17rem_minmax(0,1fr)]">
          <aside class="border-b border-gray-100 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-900/40 lg:border-b-0 lg:border-r">
            <div class="flex h-full min-h-0 flex-col gap-3 p-3">
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ t('admin.projects.title') }}（{{ projects.length }}）
                  </h2>
                </div>
              </div>

              <div class="flex min-h-0 gap-2 overflow-x-auto pb-1 lg:flex-col lg:overflow-x-hidden lg:overflow-y-auto lg:pb-0">
                <button
                  v-for="project in projects"
                  :key="project.id"
                  type="button"
                  data-test="project-tab"
                  class="min-w-[220px] shrink-0 rounded-lg border px-3 py-2 text-left transition-colors lg:min-w-0"
                  :class="selectedProject?.id === project.id
                    ? 'border-primary-200 bg-white text-primary-900 shadow-sm dark:border-primary-800 dark:bg-dark-800 dark:text-primary-100'
                    : 'border-transparent bg-transparent text-gray-700 hover:border-gray-200 hover:bg-white dark:text-dark-200 dark:hover:border-dark-700 dark:hover:bg-dark-800'"
                  @click="selectProject(project)"
                >
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0">
                      <div class="truncate text-sm font-semibold">{{ project.name }}</div>
                      <div class="mt-0.5 truncate font-mono text-xs text-gray-500 dark:text-dark-300">{{ project.slug }}</div>
                    </div>
                    <span v-if="selectedProject?.id === project.id" class="mt-1 h-2 w-2 shrink-0 rounded-full bg-primary-500"></span>
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-1.5">
                    <span v-if="project.role" class="badge badge-primary shrink-0">
                      {{ formatProjectRole(project.role) }}
                    </span>
                    <span v-if="project.is_owner" class="badge badge-success shrink-0">{{ t('admin.projects.owner') }}</span>
                  </div>
                </button>
              </div>
            </div>
          </aside>

          <div class="flex min-h-0 min-w-0 flex-col">
            <div class="flex shrink-0 items-center gap-1 border-b border-gray-100 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800/50">
              <button
                type="button"
                class="rounded-lg px-3 py-2 text-sm font-medium transition-colors"
                :class="activeTab === 'members'
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700'"
                @click="activeTab = 'members'"
              >
                {{ t('admin.projects.members') }}
              </button>
              <button
                type="button"
                class="rounded-lg px-3 py-2 text-sm font-medium transition-colors"
                :class="activeTab === 'profiles'
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700'"
                @click="activeTab = 'profiles'"
              >
                {{ t('admin.projects.appProfiles') }}
              </button>
            </div>
            <div v-if="activeTab === 'members'" class="flex min-h-0 flex-1 flex-col">
              <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
                <div class="min-w-0">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.projects.members') }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.projects.membersDescription') }}</p>
                </div>
                <button class="btn btn-primary" :disabled="!selectedProject" @click="openMemberDialog">
                  <Icon name="userPlus" size="md" class="mr-2" />
                  {{ t('admin.projects.addMember') }}
                </button>
              </div>

              <DataTable
                :columns="memberColumns"
                :data="members"
                :loading="membersLoading"
                :actions-count="3"
                row-key="user_id"
                :expandable-actions="false"
                :virtualize-mobile="true"
              >
                <template #cell-member="{ row }">
                  <div class="flex items-center gap-2">
                    <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
                      <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                        {{ row.email.charAt(0).toUpperCase() }}
                      </span>
                    </div>
                    <div class="min-w-0">
                      <div class="truncate font-medium text-gray-900 dark:text-white">{{ row.email }}</div>
                      <div class="truncate text-xs text-gray-500 dark:text-dark-300">{{ row.username || `#${row.user_id}` }}</div>
                    </div>
                  </div>
                </template>

                <template #cell-role="{ row }">
                  <div class="space-y-1">
                    <span :class="['badge', projectRoleBadgeClass(displayMemberRole(row))]">
                      {{ formatProjectRole(displayMemberRole(row)) }}
                    </span>
                    <div v-if="row.role === 'admin'" class="text-xs text-gray-500 dark:text-dark-300">
                      {{ formatMemberPermissions(row.permissions) }}
                    </div>
                  </div>
                </template>

                <template #cell-owner="{ row }">
                  <span v-if="row.is_owner" class="badge badge-success">{{ t('admin.projects.owner') }}</span>
                  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
                </template>

                <template #cell-status="{ row }">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span :class="['badge', memberStatusBadgeClass(row.status)]">
                      {{ memberStatusLabel(row.status) }}
                    </span>
                    <span v-if="row.user_status === 'disabled'" class="badge badge-danger">
                      {{ t('admin.projects.accountDisabled') }}
                    </span>
                  </div>
                </template>

                <template #cell-actions="{ row }">
                  <div class="project-row-actions">
                    <button
                      class="project-row-action flex-col"
                      :data-test="`edit-member-${row.user_id}`"
                      @click="openMemberEditDialog(row)"
                    >
                      <Icon name="edit" size="sm" />
                      <span class="text-xs">{{ t('common.edit') }}</span>
                    </button>
                    <button
                      :class="[
                        'project-row-action flex-col disabled:cursor-not-allowed disabled:opacity-50',
                        row.status === 'active'
                          ? 'project-row-action-warning'
                          : 'project-row-action-success'
                      ]"
                      :data-test="`toggle-member-status-${row.user_id}`"
                      :disabled="row.is_owner || savingMember"
                      @click="toggleMemberStatus(row)"
                    >
                      <Icon v-if="row.status === 'active'" name="ban" size="sm" />
                      <Icon v-else name="checkCircle" size="sm" />
                      <span class="text-xs">{{ row.status === 'active' ? t('admin.users.disable') : t('admin.users.enable') }}</span>
                    </button>
                    <button
                      class="project-member-action-menu-trigger project-row-action flex-col"
                      :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': activeMemberMenuUserID === row.user_id }"
                      :data-test="`member-more-${row.user_id}`"
                      @click="openMemberActionMenu(row, $event)"
                    >
                      <Icon name="more" size="sm" />
                      <span class="text-xs">{{ t('common.more') }}</span>
                    </button>
                  </div>
                </template>

                <template #empty>
                  <div class="py-6 text-center text-sm text-gray-500 dark:text-dark-300">
                    {{ t('admin.projects.noMembers') }}
                  </div>
                </template>
              </DataTable>
            </div>

            <div v-else class="flex min-h-0 flex-1 flex-col">
              <div class="flex flex-wrap items-end justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
                <div class="min-w-0">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.projects.appProfiles') }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.projects.profilesDescription') }}</p>
                </div>
                <div class="flex flex-wrap items-end gap-2">
                  <div class="min-w-56">
                    <label class="input-label">{{ t('admin.projects.activeScope') }}</label>
                    <select
                      v-model="pendingProfileScope"
                      class="input py-2 text-sm"
                      data-test="profile-scope-select"
                      :disabled="!selectedProject || profilesLoading"
                    >
                      <option value="unrestricted">{{ t('admin.projects.unrestrictedMode') }}</option>
                      <option v-for="profile in configProfiles" :key="profile.id" :value="profileScopeValue(profile)">
                        {{ profile.name }}
                      </option>
                    </select>
                  </div>
                  <button
                    class="btn btn-secondary btn-sm"
                    data-test="apply-profile-scope"
                    :disabled="!selectedProject || !isProfileScopeDirty"
                    @click="applyPendingProfileScope"
                  >
                    <Icon name="check" size="sm" class="mr-1" />
                    {{ t('common.apply') }}
                  </button>
                  <button class="btn btn-primary btn-sm" :disabled="!selectedProject" @click="openProfileDialog()">
                    <Icon name="plus" size="sm" class="mr-1" />
                    {{ t('admin.projects.addProfile') }}
                  </button>
                </div>
              </div>

            <DataTable
              :columns="profileColumns"
              :data="configProfiles"
              :loading="profilesLoading"
              :actions-count="2"
              row-key="id"
              :expandable-actions="false"
              :virtualize-mobile="true"
            >
              <template #cell-profile="{ row }">
                <div class="min-w-0" data-test="profile-list-item">
                  <div class="flex items-center gap-2">
                    <span class="truncate font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                    <span v-if="row.is_active" class="badge badge-success">{{ t('admin.projects.activeProfile') }}</span>
                  </div>
                  <div class="mt-1 truncate text-xs text-gray-500 dark:text-dark-300">
                    {{ row.description || t('admin.projects.noDescription') }}
                  </div>
                </div>
              </template>

              <template #cell-mode>
                <span class="badge badge-primary">{{ t('admin.projects.restrictedMode') }}</span>
              </template>

              <template #cell-resources="{ row }">
                <div class="flex flex-wrap gap-1.5">
                  <span v-for="type in resourceTypes" :key="type" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                    {{ resourceTypeLabel(type) }} {{ profileBoundIDs(row, type).length }}
                  </span>
                </div>
              </template>

              <template #cell-actions="{ row }">
                <div class="project-row-actions">
                  <button
                    class="project-row-action flex-col"
                    :data-test="`edit-profile-${row.id}`"
                    :disabled="isProfileManagementDisabled(row)"
                    @click="openProfileDialog(row)"
                  >
                    <Icon name="edit" size="sm" />
                    <span class="text-xs">{{ t('common.edit') }}</span>
                  </button>
                  <button
                    class="project-row-action project-row-action-danger flex-col"
                    :data-test="`delete-profile-${row.id}`"
                    :disabled="row.is_active || isProfileManagementDisabled(row)"
                    @click="deleteProfile(row)"
                  >
                    <Icon name="trash" size="sm" />
                    <span class="text-xs">{{ t('common.delete') }}</span>
                  </button>
                </div>
              </template>

              <template #empty>
                <div class="py-6 text-center text-sm text-gray-500 dark:text-dark-300">
                  {{ t('admin.projects.noBindings') }}
                </div>
              </template>
            </DataTable>
            </div>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <Teleport to="body">
      <div
        v-if="activeMemberMenuUserID !== null && memberMenuPosition"
        class="project-member-action-menu-content fixed z-[9999] w-44 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
        :style="{ top: memberMenuPosition.top + 'px', left: memberMenuPosition.left + 'px' }"
      >
        <div class="py-1">
          <template v-for="member in members" :key="member.user_id">
            <template v-if="member.user_id === activeMemberMenuUserID">
              <button
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:text-dark-100 dark:hover:bg-dark-700"
                :data-test="`member-api-keys-${member.user_id}`"
                @click="openMemberApiKeysFromMenu(member)"
              >
                <Icon name="key" size="sm" />
                {{ t('admin.projects.apiKeys') }}
              </button>
              <button
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-900/20"
                :data-test="`remove-member-${member.user_id}`"
                :disabled="member.is_owner"
                @click="removeMemberFromMenu(member)"
              >
                <Icon name="trash" size="sm" />
                {{ t('common.delete') }}
              </button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

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

          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div v-for="type in resourceTypes" :key="type" class="rounded-lg border border-gray-200 p-2 dark:border-dark-700">
              <div class="mb-2 flex items-center justify-between gap-2 text-xs font-medium text-gray-700 dark:text-dark-200">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="truncate">{{ resourceTypeLabel(type) }}</span>
                  <span class="project-resource-count">{{ projectBoundIDs(type).length }}</span>
                </div>
                <div class="flex shrink-0 items-center gap-1.5">
                  <button
                    type="button"
                    class="project-action-button project-action-button-compact"
                    :data-test="`project-add-${type}`"
                    @click="openResourcePicker(type, 'project')"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t('admin.projects.addResource') }}
                  </button>
                </div>
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
      @close="closeMemberDialog"
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
            <button
              type="button"
              class="btn btn-secondary px-3"
              :disabled="memberSearch.loading"
              :title="t('common.refresh')"
              @click="searchMembers"
            >
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
        <div v-if="memberForm.role === 'admin'" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.projects.adminPermissions') }}</div>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label
              v-for="permission in projectAdminPermissionOptions"
              :key="permission.value"
              class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200"
            >
              <input
                v-model="memberForm.permissions"
                type="checkbox"
                :value="permission.value"
                class="checkbox"
              />
              <span>{{ permission.label }}</span>
            </label>
          </div>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="closeMemberDialog">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="member-form" type="submit" :disabled="savingMember">
          {{ savingMember ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showMemberEditDialog"
      :title="t('admin.projects.editMember')"
      width="normal"
      @close="closeMemberEditDialog"
    >
      <form id="member-edit-form" class="space-y-4" @submit.prevent="submitMemberEdit">
        <div v-if="editingMember" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
          <div class="text-sm font-medium text-gray-900 dark:text-white">{{ editingMember.email }}</div>
          <div class="mt-1 text-xs text-gray-500 dark:text-dark-300">
            {{ editingMember.username || `#${editingMember.user_id}` }}
          </div>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.projects.role') }}</label>
            <select v-model="memberEditForm.role" class="input">
              <option value="admin">{{ t('admin.users.roles.admin') }}</option>
              <option value="user">{{ t('admin.users.roles.user') }}</option>
            </select>
          </div>
          <div>
            <label class="input-label">{{ t('admin.projects.status') }}</label>
            <select v-model="memberEditForm.status" class="input" :disabled="editingMember?.is_owner">
              <option value="active">{{ t('common.active') }}</option>
              <option value="disabled">{{ t('admin.users.disabled') }}</option>
            </select>
          </div>
        </div>
        <div v-if="memberEditForm.role === 'admin'" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.projects.adminPermissions') }}</div>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label
              v-for="permission in projectAdminPermissionOptions"
              :key="permission.value"
              class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200"
            >
              <input
                v-model="memberEditForm.permissions"
                type="checkbox"
                :value="permission.value"
                class="checkbox"
              />
              <span>{{ permission.label }}</span>
            </label>
          </div>
        </div>
        <div v-if="editingMember && !editingMember.is_owner" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.projects.owner') }}</div>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('admin.projects.transferOwnerHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="transferOwner(editingMember)">
              {{ t('admin.projects.transferOwner') }}
            </button>
          </div>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="closeMemberEditDialog">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="member-edit-form" type="submit" :disabled="savingMember">
          {{ savingMember ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showProfileDialog"
      :title="profileForm.id ? t('admin.projects.editProfile') : t('admin.projects.addProfile')"
      width="wide"
      @close="closeProfileDialog"
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
        <div class="space-y-3">
          <div>
            <label class="input-label">{{ t('admin.projects.resourceBindings') }}</label>
            <p class="input-hint">{{ t('admin.projects.resourceBindingsHint') }}</p>
            <p v-if="!canEditProfileDraftBindings" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
              {{ t('admin.projects.resourceBindingsLoadFailedHint') }}
            </p>
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div
              v-for="type in resourceTypes"
              :key="type"
              class="rounded-lg border border-gray-200 dark:border-dark-700"
            >
              <div class="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-dark-700">
                <div class="flex min-w-0 items-center gap-2">
                  <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ resourceTypeLabel(type) }}</div>
                  <span class="project-resource-count">{{ profileDialogBoundIDs(type).length }}</span>
                </div>
                <div class="flex shrink-0 items-center gap-1.5">
                  <button
                    type="button"
                    class="project-action-button project-action-button-compact"
                    :data-test="`profile-add-${type}`"
                    :disabled="!canEditProfileDraftBindings"
                    @click="openProfileResourcePicker(type)"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t('admin.projects.addResource') }}
                  </button>
                </div>
              </div>
              <div class="max-h-56 overflow-auto p-2">
                <div v-if="profileDialogBoundIDs(type).length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-dark-300">
                  {{ t('admin.projects.noBindings') }}
                </div>
                <div
                  v-for="id in profileDialogBoundIDs(type)"
                  v-else
                  :key="`${type}-${id}`"
                  class="mb-2 flex items-center justify-between gap-2 rounded-md bg-gray-50 px-2 py-2 text-sm dark:bg-dark-700"
                >
                  <div class="min-w-0">
                    <div class="truncate font-medium text-gray-900 dark:text-white">{{ bindingLabel(type, id) }}</div>
                    <div class="text-xs text-gray-500 dark:text-dark-300">#{{ id }}</div>
                  </div>
                  <button
                    type="button"
                    class="text-gray-400 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:text-red-400"
                    :disabled="!canEditProfileDraftBindings"
                    @click="removeProfileDialogResourceBinding(type, id)"
                  >
                    <Icon name="x" size="sm" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="closeProfileDialog">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="profile-form" type="submit" :disabled="savingProfile">
          {{ savingProfile ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showMemberApiKeysDialog"
      :title="t('admin.projects.memberApiKeys')"
      width="wide"
      @close="closeMemberApiKeysDialog"
    >
      <div class="space-y-4">
        <div v-if="selectedApiKeyMember" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
          <div class="text-sm font-medium text-gray-900 dark:text-white">{{ selectedApiKeyMember.email }}</div>
          <div class="mt-1 text-xs text-gray-500 dark:text-dark-300">
            {{ selectedApiKeyMember.username || `#${selectedApiKeyMember.user_id}` }}
          </div>
        </div>

        <div v-if="memberApiKeysLoading" class="py-8 text-center text-sm text-gray-500 dark:text-dark-300">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="memberApiKeys.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-dark-300">
          {{ t('admin.projects.noMemberApiKeys') }}
        </div>
        <div v-else class="space-y-3">
          <div class="rounded-lg border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-700 dark:bg-dark-800/50">
            <div v-if="authStore.isAdmin" class="flex flex-wrap items-end gap-3">
              <div class="min-w-0 flex-1">
                <div class="mb-2 flex flex-wrap items-center gap-2 text-sm text-gray-600 dark:text-dark-300">
                  <span>{{ t('admin.projects.selectedCount', { count: selectedApiKeyIDs.length }) }}</span>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    :disabled="transferringApiKeys"
                    @click="selectAllMemberApiKeys"
                  >
                    {{ allMemberApiKeysSelected ? t('admin.projects.clearSelection') : t('admin.projects.selectAll') }}
                  </button>
                </div>
                <label class="input-label">{{ t('admin.projects.targetProject') }}</label>
                <select
                  class="input py-2 text-sm"
                  data-test="api-key-bulk-target"
                  :value="apiKeyTransferTargetProjectID ?? ''"
                  :disabled="transferringApiKeys || apiKeyTransferTargets.length === 0"
                  @change="setApiKeyTransferTarget(($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ t('admin.projects.selectTargetProject') }}</option>
                  <option v-for="project in apiKeyTransferTargets" :key="project.id" :value="project.id">
                    {{ formatApiKeyTransferTarget(project) }}
                  </option>
                </select>
              </div>
              <button
                class="btn btn-primary"
                data-test="transfer-selected-api-keys"
                :disabled="selectedApiKeyIDs.length === 0 || !apiKeyTransferTargetProjectID || transferringApiKeys"
                @click="transferSelectedApiKeys"
              >
                <Icon name="swap" size="sm" class="mr-1" />
                {{ transferringApiKeys ? t('common.saving') : t('common.apply') }}
              </button>
            </div>
            <p v-else class="text-sm text-gray-600 dark:text-dark-300">
              {{ t('admin.projects.apiKeyTransferSuperAdminOnly') }}
            </p>
          </div>

          <div class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-dark-700">
            <label
              v-for="key in memberApiKeys"
              :key="key.id"
              class="flex cursor-pointer items-start gap-3 border-b border-gray-100 p-3 last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60"
              :class="selectedApiKeyIDs.includes(key.id) ? 'bg-primary-50/70 dark:bg-primary-900/20' : ''"
            >
              <input
                v-if="authStore.isAdmin"
                type="checkbox"
                class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :data-test="`select-api-key-${key.id}`"
                :checked="selectedApiKeyIDs.includes(key.id)"
                :disabled="transferringApiKeys"
                @change="toggleApiKeySelection(key.id, ($event.target as HTMLInputElement).checked)"
              />
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-center gap-2">
                  <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ key.name || `#${key.id}` }}</span>
                  <span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">{{ key.status }}</span>
                  <span class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                    {{ projectNameById(key.project_id) }}
                  </span>
                </span>
                <span class="mt-1 block truncate font-mono text-xs text-gray-500 dark:text-dark-300">
                  {{ formatApiKeyPreview(key.key) }}
                </span>
              </span>
            </label>
          </div>
        </div>

        <p v-if="authStore.isAdmin" class="text-xs text-gray-500 dark:text-dark-300">
          {{ t('admin.projects.apiKeyTransferHint') }}
        </p>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="closeMemberApiKeysDialog">
          {{ t('common.close') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showResourcePicker"
      :title="t('admin.projects.addResource')"
      width="normal"
      @close="showResourcePicker = false"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ resourceTypeLabel(resourcePicker.type) }}</label>
          <input
            v-model.trim="resourcePicker.query"
            class="input"
            :placeholder="t('admin.projects.searchResourcesPlaceholder')"
          />
        </div>
        <div class="flex items-center justify-between gap-3 text-sm">
          <span class="text-gray-500 dark:text-dark-300">
            {{ t('admin.projects.selectedCount', { count: resourcePicker.selectedIds.length }) }}
          </span>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="resourcePickerCandidates.length === 0" @click="selectAllResourcePickerCandidates">
              {{ t('admin.projects.selectAll') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="resourcePicker.selectedIds.length === 0" @click="resourcePicker.selectedIds = []">
              {{ t('admin.projects.clearSelection') }}
            </button>
          </div>
        </div>
        <div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <div v-if="resourcePicker.loading" class="py-8 text-center text-sm text-gray-500 dark:text-dark-300">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="resourcePickerCandidates.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-dark-300">
            {{ t('admin.projects.noBindings') }}
          </div>
          <label
            v-for="candidate in resourcePickerCandidates"
            v-else
            :key="candidate.key"
            class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-3 py-2 last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60"
          >
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="resourcePicker.selectedIds.includes(candidate.id)"
              @change="toggleResourcePickerCandidate(candidate.id, ($event.target as HTMLInputElement).checked)"
            />
            <span class="min-w-0">
              <span class="block truncate text-sm font-medium text-gray-900 dark:text-white">{{ candidate.title }}</span>
              <span class="block truncate text-xs text-gray-500 dark:text-dark-300">{{ candidate.subtitle }}</span>
            </span>
          </label>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showResourcePicker = false">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" :disabled="resourcePicker.loading" @click="applyResourcePickerSelection">
          {{ t('admin.projects.applySelection') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteMemberConfirm"
      :title="t('admin.projects.removeMemberConfirmTitle')"
      :message="t('admin.projects.removeMemberConfirmMessage', { email: pendingDeleteMember?.email })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRemoveMember"
      @cancel="closeDeleteMemberConfirm"
    />

    <ConfirmDialog
      :show="showDeleteProfileConfirm"
      :title="t('admin.projects.deleteProfileConfirmTitle')"
      :message="t('admin.projects.deleteProfileConfirmMessage', { name: pendingDeleteProfile?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDeleteProfile"
      @cancel="closeDeleteProfileConfirm"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { ApiKey } from '@/types'
import type { Column } from '@/components/common/types'
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
import { AdminPermissions, defaultProjectAdminPermissions } from '@/constants/adminPermissions'

type CandidateItem = {
  id: number
  key: string
  type: ProjectResourceType
  title: string
  subtitle: string
}

type ProjectProfileScopeValue = 'unrestricted' | `profile:${number}`
type ResourcePickerTarget = 'project' | 'profile-draft'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const membersLoading = ref(false)
const profilesLoading = ref(false)
const savingProject = ref(false)
const savingMember = ref(false)
const savingProfile = ref(false)
const projects = ref<AdminProject[]>([])
const members = ref<ProjectMember[]>([])
const profiles = ref<ProjectProfile[]>([])
const selectedProject = ref<AdminProject | null>(null)
const selectedProfile = ref<ProjectProfile | null>(null)
const pendingProfileScope = ref<ProjectProfileScopeValue>('unrestricted')
const bindings = ref<ProjectProfileBindings | null>(null)
const activeTab = ref<'members' | 'profiles'>('members')
const showProjectDialog = ref(false)
const showMemberDialog = ref(false)
const showMemberEditDialog = ref(false)
const showProfileDialog = ref(false)
const showMemberApiKeysDialog = ref(false)
const showResourcePicker = ref(false)
const bindingNames = reactive<Record<ProjectResourceType, Record<number, string>>>({
  groups: {},
  accounts: {},
  proxies: {},
  subscriptions: {}
})
const profileBindingsByID = reactive<Record<number, ProjectProfileBindings>>({})
const editingMember = ref<ProjectMember | null>(null)
const memberEditForm = reactive({
  role: 'user' as AssignableProjectRole,
  status: 'active' as ProjectMemberStatus,
  permissions: [] as string[]
})
const selectedApiKeyMember = ref<ProjectMember | null>(null)
const memberApiKeys = ref<ApiKey[]>([])
const memberApiKeysLoading = ref(false)
type APIKeyTransferTarget = AdminProject & { memberStatus: ProjectMemberStatus }

const apiKeyTransferTargets = ref<APIKeyTransferTarget[]>([])
const selectedApiKeyIDs = ref<number[]>([])
const apiKeyTransferTargetProjectID = ref<number | undefined>()
const transferringApiKeys = ref(false)
const activeMemberMenuUserID = ref<number | null>(null)
const memberMenuPosition = ref<{ top: number; left: number } | null>(null)
const showDeleteMemberConfirm = ref(false)
const pendingDeleteMember = ref<ProjectMember | null>(null)
const showDeleteProfileConfirm = ref(false)
const pendingDeleteProfile = ref<ProjectProfile | null>(null)

const resourceTypes: ProjectResourceType[] = ['groups', 'accounts', 'proxies', 'subscriptions']

const projectAdminPermissionOptions = computed(() => [
  { value: AdminPermissions.dashboard, label: t('admin.projects.permissions.dashboard') },
  { value: AdminPermissions.ops, label: t('admin.projects.permissions.ops') },
  { value: AdminPermissions.users, label: t('admin.projects.permissions.users') },
  { value: AdminPermissions.groups, label: t('admin.projects.permissions.groups') },
  { value: AdminPermissions.proxies, label: t('admin.projects.permissions.proxies') },
  { value: AdminPermissions.subscriptions, label: t('admin.projects.permissions.subscriptions') },
  { value: AdminPermissions.accounts, label: t('admin.projects.permissions.accounts') },
  { value: AdminPermissions.usage, label: t('admin.projects.permissions.usage') }
])

const memberColumns = computed<Column[]>(() => [
  { key: 'member', label: t('admin.projects.member') },
  { key: 'role', label: t('admin.projects.role') },
  { key: 'owner', label: t('admin.projects.owner') },
  { key: 'status', label: t('admin.projects.status') },
  { key: 'actions', label: t('common.actions'), class: 'project-member-actions-column text-right' }
])

const profileColumns = computed<Column[]>(() => [
  { key: 'profile', label: t('admin.projects.appProfiles') },
  { key: 'mode', label: t('admin.projects.profileMode') },
  { key: 'resources', label: t('admin.projects.resourceBindings') },
  { key: 'actions', label: t('common.actions'), class: 'project-profile-actions-column text-right' }
])

const projectForm = reactive({
  name: '',
  slug: '',
  description: '',
  profileMode: 'restricted' as ProjectProfileMode,
  bindings: {
    profile_id: 0,
    group_ids: [] as number[],
    account_ids: [] as number[],
    proxy_ids: [] as number[],
    subscription_ids: [] as number[]
  } satisfies ProjectProfileBindings
})

const memberForm = reactive({
  userId: undefined as number | undefined,
  role: 'user' as AssignableProjectRole,
  status: 'active' as ProjectMemberStatus,
  permissions: [] as string[]
})

const memberSearch = reactive({
  query: '',
  loading: false
})

const memberCandidates = ref<ProjectResourceUserCandidate[]>([])

const profileForm = reactive({
  id: undefined as number | undefined,
  name: '',
  description: ''
})

const profileDraftBindings = ref<ProjectProfileBindings>(emptyProjectBindings())
const profileDraftBindingsLoaded = ref(true)

const resourcePicker = reactive({
  type: 'groups' as ProjectResourceType,
  query: '',
  loading: false,
  target: 'profile-draft' as ResourcePickerTarget,
  selectedIds: [] as number[]
})

const resourcePickerResult = ref<ProjectResourceSearchResult | null>(null)

const detailLoading = computed(() => membersLoading.value || profilesLoading.value)

const configProfiles = computed(() => profiles.value.filter(profile => profile.mode === 'restricted'))

const isUnrestrictedScopeActive = computed(() => {
  return profiles.value.some(profile => profile.mode === 'unrestricted' && profile.is_active)
})

const activeProfileScope = computed<ProjectProfileScopeValue>(() => {
  const activeProfile = configProfiles.value.find(profile => profile.is_active)
  return activeProfile ? profileScopeValue(activeProfile) : 'unrestricted'
})

const isProfileScopeDirty = computed(() => pendingProfileScope.value !== activeProfileScope.value)

const allMemberApiKeysSelected = computed(() => {
  return memberApiKeys.value.length > 0 && selectedApiKeyIDs.value.length === memberApiKeys.value.length
})

const canEditProfileDraftBindings = computed(() => !profileForm.id || profileDraftBindingsLoaded.value)

const resourcePickerCandidates = computed<CandidateItem[]>(() => {
  if (!resourcePickerResult.value) return []
  return candidatesForType(resourcePicker.type, resourcePickerResult.value)
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
    const availableProfiles = configProfiles.value
    const next = selectedProfile.value
      ? availableProfiles.find(profile => profile.id === selectedProfile.value?.id)
      : availableProfiles.find(profile => profile.is_active) ?? availableProfiles[0]
    selectedProfile.value = next ?? null
    pendingProfileScope.value = activeProfileScope.value
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

async function loadBindings(profile: ProjectProfile): Promise<boolean> {
  if (!selectedProject.value) return false
  try {
    const next = await adminAPI.projects.getProfileBindings(selectedProject.value.id, profile.id)
    indexBindingDetails(next)
    bindings.value = normalizeProfileBindings(next, profile.id)
    profileBindingsByID[profile.id] = bindings.value
    return true
  } catch (error) {
    console.error('Failed to load project profile bindings:', error)
    appStore.showError(t('admin.projects.failedToLoadBindings'))
    return false
  }
}

async function selectProject(project: AdminProject) {
  closeMemberActionMenu()
  selectedProject.value = project
  selectedProfile.value = null
  pendingProfileScope.value = 'unrestricted'
  resourcePickerResult.value = null
  clearProfileBindingCache()
  await reloadSelectedProject()
}

function clearProfileBindingCache() {
  for (const key of Object.keys(profileBindingsByID)) {
    delete profileBindingsByID[Number(key)]
  }
}

function openCreateDialog() {
  projectForm.name = ''
  projectForm.slug = ''
  projectForm.description = ''
  projectForm.profileMode = 'restricted'
  projectForm.bindings = emptyProjectBindings()
  resetResourcePicker()
  showProjectDialog.value = true
}

async function submitProject() {
  savingProject.value = true
  try {
    const bindings = projectForm.bindings
    const project = await adminAPI.projects.create({
      name: projectForm.name,
      slug: projectForm.slug,
      description: projectForm.description || null,
      profile_mode: projectForm.profileMode,
      group_ids: bindings.group_ids,
      account_ids: bindings.account_ids,
      proxy_ids: bindings.proxy_ids,
      subscription_ids: bindings.subscription_ids
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
  memberForm.permissions = []
  memberSearch.query = ''
  memberCandidates.value = []
  showMemberDialog.value = true
  void searchMembers()
}

function closeMemberDialog() {
  showMemberDialog.value = false
  memberSearch.loading = false
  memberSearchRequestID += 1
  if (memberSearchTimer) {
    clearTimeout(memberSearchTimer)
    memberSearchTimer = undefined
  }
}

async function searchMembers() {
  if (!selectedProject.value) return
  if (memberSearchTimer) {
    clearTimeout(memberSearchTimer)
    memberSearchTimer = undefined
  }
  const requestID = ++memberSearchRequestID
  memberSearch.loading = true
  try {
    const result = authStore.isAdmin
      ? await adminAPI.projects.searchGlobalBindableResources(memberSearch.query, 20)
      : await adminAPI.projects.searchBindableResources(selectedProject.value.id, memberSearch.query, 20)
    if (requestID === memberSearchRequestID) {
      memberCandidates.value = result.users
    }
  } catch (error) {
    console.error('Failed to search project members:', error)
    if (requestID === memberSearchRequestID) {
      appStore.showError(t('admin.projects.failedToSearchResources'))
    }
  } finally {
    if (requestID === memberSearchRequestID) {
      memberSearch.loading = false
    }
  }
}

function selectMemberCandidate(candidate: ProjectResourceUserCandidate) {
  memberForm.userId = candidate.id
}

async function submitMember() {
  if (!selectedProject.value || !memberForm.userId) return
  savingMember.value = true
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, memberForm.userId, {
      role: memberForm.role,
      status: memberForm.status,
      permissions: memberForm.role === 'admin' ? [...memberForm.permissions] : []
    })
    appStore.showSuccess(t('admin.projects.memberSaved'))
    closeMemberDialog()
    await loadMembers()
  } catch (error) {
    console.error('Failed to save project member:', error)
    appStore.showError(t('admin.projects.failedToSaveMember'))
  } finally {
    savingMember.value = false
  }
}

function openMemberEditDialog(member: ProjectMember) {
  editingMember.value = member
  memberEditForm.role = member.role
  memberEditForm.status = member.status
  memberEditForm.permissions = member.role === 'admin'
    ? [...(member.permissions ?? defaultProjectAdminPermissions)]
    : []
  showMemberEditDialog.value = true
}

function closeMemberEditDialog() {
  showMemberEditDialog.value = false
  editingMember.value = null
}

async function submitMemberEdit() {
  if (!editingMember.value) return
  await saveMember(editingMember.value, {
    role: memberEditForm.role,
    status: memberEditForm.status,
    permissions: memberEditForm.role === 'admin' ? [...memberEditForm.permissions] : []
  })
  closeMemberEditDialog()
}

async function transferOwner(member: ProjectMember) {
  if (!selectedProject.value || member.is_owner) return
  await saveMember(member, {
    role: 'admin',
    is_owner: true,
    status: 'active',
    permissions: member.permissions?.length ? [...member.permissions] : [...defaultProjectAdminPermissions]
  })
}

async function toggleMemberStatus(member: ProjectMember) {
  if (member.is_owner || savingMember.value) return
  await saveMember(member, {
    status: member.status === 'active' ? 'disabled' : 'active'
  })
}

async function saveMember(member: ProjectMember, patch: { role?: AssignableProjectRole; is_owner?: boolean; status?: ProjectMemberStatus; permissions?: string[] }) {
  if (!selectedProject.value) return
  savingMember.value = true
  const nextRole = patch.role ?? member.role
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, member.user_id, {
      role: nextRole,
      is_owner: patch.is_owner ?? member.is_owner,
      status: patch.status ?? member.status,
      permissions: nextRole === 'admin'
        ? (patch.permissions ?? member.permissions ?? defaultProjectAdminPermissions)
        : []
    })
    appStore.showSuccess(t('admin.projects.memberSaved'))
    await loadMembers()
  } catch (error) {
    console.error('Failed to save project member:', error)
    appStore.showError(t('admin.projects.failedToSaveMember'))
    await loadMembers()
  } finally {
    savingMember.value = false
  }
}

async function openMemberApiKeysDialog(member: ProjectMember) {
  selectedApiKeyMember.value = member
  memberApiKeys.value = []
  apiKeyTransferTargets.value = []
  resetApiKeyTransferState()
  showMemberApiKeysDialog.value = true
  const tasks: Promise<void>[] = [loadMemberApiKeys(member)]
  if (authStore.isAdmin) {
    tasks.push(loadApiKeyTransferTargets(member))
  }
  await Promise.all(tasks)
}

function closeMemberApiKeysDialog() {
  showMemberApiKeysDialog.value = false
  selectedApiKeyMember.value = null
  memberApiKeys.value = []
  apiKeyTransferTargets.value = []
  resetApiKeyTransferState()
}

async function loadMemberApiKeys(member: ProjectMember) {
  if (!selectedProject.value) return
  memberApiKeysLoading.value = true
  try {
    const response = await adminAPI.users.getUserApiKeys(member.user_id, selectedProject.value.id)
    memberApiKeys.value = response.items ?? []
    pruneSelectedApiKeys()
  } catch (error) {
    console.error('Failed to load member API keys:', error)
    appStore.showError(t('admin.projects.failedToLoadMemberApiKeys'))
  } finally {
    memberApiKeysLoading.value = false
  }
}

async function loadApiKeyTransferTargets(member: ProjectMember) {
  if (!selectedProject.value) return
  const currentProjectID = selectedProject.value.id
  const targetProjects = projects.value.filter(project => project.id !== currentProjectID)
  const allowed: APIKeyTransferTarget[] = []

  await Promise.all(targetProjects.map(async (project) => {
    try {
      const projectMembers = await adminAPI.projects.listMembers(project.id)
      const targetMember = projectMembers.find(item => item.user_id === member.user_id)
      if (targetMember) {
        allowed.push({ ...project, memberStatus: targetMember.status })
      }
    } catch (error) {
      console.error('Failed to load target project members:', error)
    }
  }))

  const originalOrder = new Map(projects.value.map((project, index) => [project.id, index]))
  apiKeyTransferTargets.value = allowed.sort((a, b) => (originalOrder.get(a.id) ?? 0) - (originalOrder.get(b.id) ?? 0))
}

function setApiKeyTransferTarget(value: string) {
  const projectID = Number(value)
  apiKeyTransferTargetProjectID.value = Number.isSafeInteger(projectID) && projectID > 0 ? projectID : undefined
}

function formatApiKeyTransferTarget(project: APIKeyTransferTarget): string {
  if (project.memberStatus === 'disabled') {
    return `${project.name} (${t('admin.projects.apiKeyTransferDisabledMemberOption')})`
  }
  return project.name
}

function toggleApiKeySelection(keyID: number, checked: boolean) {
  if (!authStore.isAdmin) return
  if (checked) {
    if (!selectedApiKeyIDs.value.includes(keyID)) {
      selectedApiKeyIDs.value = [...selectedApiKeyIDs.value, keyID]
    }
    return
  }
  selectedApiKeyIDs.value = selectedApiKeyIDs.value.filter(id => id !== keyID)
}

function selectAllMemberApiKeys() {
  if (!authStore.isAdmin) return
  if (allMemberApiKeysSelected.value) {
    selectedApiKeyIDs.value = []
    return
  }
  selectedApiKeyIDs.value = memberApiKeys.value.map(key => key.id)
}

function pruneSelectedApiKeys() {
  const availableIDs = new Set(memberApiKeys.value.map(key => key.id))
  selectedApiKeyIDs.value = selectedApiKeyIDs.value.filter(id => availableIDs.has(id))
}

async function transferSelectedApiKeys() {
  if (!authStore.isAdmin) return
  const targetProjectID = apiKeyTransferTargetProjectID.value
  const keyIDs = [...selectedApiKeyIDs.value]
  if (!targetProjectID || keyIDs.length === 0 || transferringApiKeys.value) return
  transferringApiKeys.value = true
  try {
    const results = await Promise.allSettled(
      keyIDs.map(keyID => adminAPI.apiKeys.transferApiKeyProject(keyID, targetProjectID))
    )
    const failedIDs = results.flatMap((result, index) => result.status === 'rejected' ? [keyIDs[index]] : [])
    selectedApiKeyIDs.value = failedIDs
    if (selectedApiKeyMember.value) {
      await loadMemberApiKeys(selectedApiKeyMember.value)
    }
    if (failedIDs.length > 0) {
      appStore.showError(t('admin.projects.failedToTransferApiKey'))
      return
    }
    appStore.showSuccess(t('admin.projects.apiKeyTransferred'))
  } catch (error: any) {
    console.error('Failed to transfer API key projects:', error)
    appStore.showError(error?.message || t('admin.projects.failedToTransferApiKey'))
  } finally {
    transferringApiKeys.value = false
  }
}

function resetApiKeyTransferState() {
  selectedApiKeyIDs.value = []
  apiKeyTransferTargetProjectID.value = undefined
  transferringApiKeys.value = false
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

function removeMemberFromMenu(member: ProjectMember) {
  closeMemberActionMenu()
  pendingDeleteMember.value = member
  showDeleteMemberConfirm.value = true
}

function closeDeleteMemberConfirm() {
  showDeleteMemberConfirm.value = false
  pendingDeleteMember.value = null
}

async function confirmRemoveMember() {
  const member = pendingDeleteMember.value
  if (!member) return
  showDeleteMemberConfirm.value = false
  pendingDeleteMember.value = null
  await removeMember(member)
}

async function openMemberApiKeysFromMenu(member: ProjectMember) {
  closeMemberActionMenu()
  await openMemberApiKeysDialog(member)
}

function openMemberActionMenu(member: ProjectMember, event: MouseEvent) {
  if (activeMemberMenuUserID.value === member.user_id) {
    closeMemberActionMenu()
    return
  }

  const target = event.currentTarget as HTMLElement | null
  if (!target || typeof window === 'undefined') {
    activeMemberMenuUserID.value = member.user_id
    memberMenuPosition.value = { top: event.clientY, left: event.clientX }
    return
  }

  const rect = target.getBoundingClientRect()
  const menuWidth = 176
  const menuHeight = 96
  const padding = 8
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const left = Math.max(padding, Math.min(rect.right - menuWidth, viewportWidth - menuWidth - padding))
  let top = rect.bottom + 4

  if (top + menuHeight > viewportHeight - padding) {
    top = rect.top - menuHeight - 4
    if (top < padding) top = padding
  }

  memberMenuPosition.value = { top, left }
  activeMemberMenuUserID.value = member.user_id
}

function closeMemberActionMenu() {
  activeMemberMenuUserID.value = null
  memberMenuPosition.value = null
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.project-member-action-menu-trigger') && !target.closest('.project-member-action-menu-content')) {
    closeMemberActionMenu()
  }
}

async function openProfileDialog(profile?: ProjectProfile) {
  profileForm.id = profile?.id
  profileForm.name = profile?.name ?? ''
  profileForm.description = profile?.description ?? ''
  profileDraftBindingsLoaded.value = true
  resourcePickerResult.value = null

  if (profile) {
    selectedProfile.value = profile
    const loaded = await loadBindings(profile)
    profileDraftBindingsLoaded.value = loaded
    const currentBindings = loaded
      ? bindings.value ?? profileBindingsByID[profile.id]
      : profileBindingsByID[profile.id]
    profileDraftBindings.value = cloneBindings(currentBindings ?? emptyProjectBindingsFor(profile.id))
  } else {
    selectedProfile.value = null
    bindings.value = null
    profileDraftBindings.value = emptyProjectBindings()
  }

  showProfileDialog.value = true
}

function closeProfileDialog() {
  showProfileDialog.value = false
  resourcePickerResult.value = null
  profileDraftBindings.value = emptyProjectBindings()
  profileDraftBindingsLoaded.value = true
}

async function submitProfile() {
  if (!selectedProject.value) return
  savingProfile.value = true
  try {
    if (profileForm.id) {
      const payload: { name: string; description: string | null } = {
        name: profileForm.name,
        description: profileForm.description || null
      }
      await adminAPI.projects.updateProfile(selectedProject.value.id, profileForm.id, payload)
      if (profileDraftBindingsLoaded.value) {
        const draftBindings = normalizeBindings({
          ...profileDraftBindings.value,
          profile_id: profileForm.id
        })
        const saved = await adminAPI.projects.setProfileBindings(selectedProject.value.id, profileForm.id, draftBindings)
        indexBindingDetails(saved)
        profileBindingsByID[profileForm.id] = normalizeProfileBindings(saved, profileForm.id)
      }
      appStore.showSuccess(t('admin.projects.profileSaved'))
    } else {
      const profile = await adminAPI.projects.createProfile(selectedProject.value.id, {
        name: profileForm.name,
        description: profileForm.description || null
      })
      selectedProfile.value = profile
      const draftBindings = normalizeBindings({
        ...profileDraftBindings.value,
        profile_id: profile.id
      })
      if (hasResourceBindings(draftBindings)) {
        const saved = await adminAPI.projects.setProfileBindings(selectedProject.value.id, profile.id, draftBindings)
        indexBindingDetails(saved)
        profileBindingsByID[profile.id] = normalizeProfileBindings(saved, profile.id)
      }
      appStore.showSuccess(t('admin.projects.profileCreated'))
    }
    closeProfileDialog()
    await loadProfiles()
  } catch (error) {
    console.error('Failed to save project profile:', error)
    appStore.showError(t('admin.projects.failedToSaveProfile'))
  } finally {
    savingProfile.value = false
  }
}

async function activateUnrestrictedScope() {
  if (!selectedProject.value || isUnrestrictedScopeActive.value) return
  try {
    await adminAPI.projects.activateUnrestrictedScope(selectedProject.value.id)
    await loadProfiles()
    appStore.showSuccess(t('admin.projects.profileActivated'))
  } catch (error) {
    console.error('Failed to activate unrestricted project scope:', error)
    appStore.showError(t('admin.projects.failedToActivateProfile'))
  }
}

async function applyPendingProfileScope() {
  if (!selectedProject.value || !isProfileScopeDirty.value) return
  if (pendingProfileScope.value === 'unrestricted') {
    await activateUnrestrictedScope()
    return
  }
  const profileID = profileIDFromScopeValue(pendingProfileScope.value)
  const profile = configProfiles.value.find(item => item.id === profileID)
  if (!profile || profile.is_active) return
  try {
    const updated = await adminAPI.projects.activateProfile(selectedProject.value.id, profile.id)
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

function deleteProfile(profile: ProjectProfile) {
  if (!selectedProject.value || profile.is_active) return
  pendingDeleteProfile.value = profile
  showDeleteProfileConfirm.value = true
}

function closeDeleteProfileConfirm() {
  showDeleteProfileConfirm.value = false
  pendingDeleteProfile.value = null
}

async function confirmDeleteProfile() {
  const profile = pendingDeleteProfile.value
  if (!selectedProject.value || !profile || profile.is_active) return
  showDeleteProfileConfirm.value = false
  pendingDeleteProfile.value = null
  try {
    await adminAPI.projects.deleteProfile(selectedProject.value.id, profile.id)
    if (selectedProfile.value?.id === profile.id) selectedProfile.value = null
    delete profileBindingsByID[profile.id]
    appStore.showSuccess(t('admin.projects.profileDeleted'))
    await loadProfiles()
  } catch (error) {
    console.error('Failed to delete project profile:', error)
    appStore.showError(t('admin.projects.failedToDeleteProfile'))
  }
}

function removeProjectResourceBinding(type: ProjectResourceType, id: number) {
  const ids = bindingIDsRef(projectForm.bindings, type)
  const index = ids.indexOf(id)
  if (index >= 0) ids.splice(index, 1)
}

function projectBoundIDs(type: ProjectResourceType): number[] {
  return bindingIDsRef(projectForm.bindings, type)
}

function profileDialogBoundIDs(type: ProjectResourceType): number[] {
  return bindingIDsRef(profileDraftBindings.value, type)
}

function openProfileResourcePicker(type: ProjectResourceType) {
  if (!canEditProfileDraftBindings.value) return
  openResourcePicker(type, 'profile-draft')
}

function removeProfileDialogResourceBinding(type: ProjectResourceType, id: number) {
  if (!canEditProfileDraftBindings.value) return
  const ids = bindingIDsRef(profileDraftBindings.value, type)
  const index = ids.indexOf(id)
  if (index >= 0) ids.splice(index, 1)
}

function openResourcePicker(type: ProjectResourceType, target: ResourcePickerTarget) {
  suppressResourcePickerAutoSearch = true
  if (resourcePickerSearchTimer) clearTimeout(resourcePickerSearchTimer)
  resourcePicker.type = type
  resourcePicker.target = target
  resourcePicker.query = ''
  resourcePicker.selectedIds = currentResourcePickerIDs(type, target)
  resourcePickerResult.value = null
  showResourcePicker.value = true
  void searchResourcePicker().finally(() => {
    suppressResourcePickerAutoSearch = false
  })
}

function resetResourcePicker() {
  resourcePicker.type = 'groups'
  resourcePicker.query = ''
  resourcePicker.loading = false
  resourcePicker.target = 'profile-draft'
  resourcePicker.selectedIds = []
  resourcePickerResult.value = null
}

async function searchResourcePicker() {
  if (!selectedProject.value && resourcePicker.target !== 'project') return
  resourcePicker.loading = true
  try {
    const result = resourcePicker.target === 'project' || authStore.isAdmin
      ? await adminAPI.projects.searchGlobalBindableResources(resourcePicker.query, 50)
      : await adminAPI.projects.searchBindableResources(selectedProject.value!.id, resourcePicker.query, 50)
    resourcePickerResult.value = result
    indexSearchResultNames(result)
  } catch (error) {
    console.error('Failed to search bindable resources:', error)
    appStore.showError(t('admin.projects.failedToSearchResources'))
  } finally {
    resourcePicker.loading = false
  }
}

function toggleResourcePickerCandidate(id: number, checked: boolean) {
  const exists = resourcePicker.selectedIds.includes(id)
  if (checked && !exists) {
    resourcePicker.selectedIds.push(id)
  }
  if (!checked && exists) {
    resourcePicker.selectedIds = resourcePicker.selectedIds.filter(item => item !== id)
  }
}

function selectAllResourcePickerCandidates() {
  const ids = resourcePickerCandidates.value.map(candidate => candidate.id)
  resourcePicker.selectedIds = uniqueSorted([...resourcePicker.selectedIds, ...ids])
}

async function applyResourcePickerSelection() {
  for (const candidate of resourcePickerCandidates.value) {
    if (resourcePicker.selectedIds.includes(candidate.id)) {
      bindingNames[candidate.type][candidate.id] = candidate.title
    }
  }
  const selectedIds = uniqueSorted(resourcePicker.selectedIds)

  if (resourcePicker.target === 'project') {
    const ids = bindingIDsRef(projectForm.bindings, resourcePicker.type)
    ids.splice(0, ids.length, ...selectedIds)
    showResourcePicker.value = false
    return
  }

  const ids = bindingIDsRef(profileDraftBindings.value, resourcePicker.type)
  ids.splice(0, ids.length, ...selectedIds)
  showResourcePicker.value = false
}

function cloneBindings(value: ProjectProfileBindings): ProjectProfileBindings {
  return {
    profile_id: value.profile_id,
    group_ids: [...(value.group_ids ?? [])],
    account_ids: [...(value.account_ids ?? [])],
    proxy_ids: [...(value.proxy_ids ?? [])],
    subscription_ids: [...(value.subscription_ids ?? [])]
  }
}

function emptyProjectBindings(): ProjectProfileBindings {
  return emptyProjectBindingsFor(0)
}

function emptyProjectBindingsFor(profileID: number): ProjectProfileBindings {
  return {
    profile_id: profileID,
    group_ids: [],
    account_ids: [],
    proxy_ids: [],
    subscription_ids: []
  }
}

function normalizeProfileBindings(value: Partial<ProjectProfileBindings> | null | undefined, fallbackProfileID = 0): ProjectProfileBindings {
  return normalizeBindings({
    profile_id: value?.profile_id ?? fallbackProfileID,
    group_ids: value?.group_ids ?? [],
    account_ids: value?.account_ids ?? [],
    proxy_ids: value?.proxy_ids ?? [],
    subscription_ids: value?.subscription_ids ?? []
  })
}

function normalizeBindings(value: Partial<ProjectProfileBindings>): ProjectProfileBindings {
  return {
    profile_id: value.profile_id ?? 0,
    group_ids: uniqueSorted(value.group_ids),
    account_ids: uniqueSorted(value.account_ids),
    proxy_ids: uniqueSorted(value.proxy_ids),
    subscription_ids: uniqueSorted(value.subscription_ids)
  }
}

function hasResourceBindings(value: ProjectProfileBindings): boolean {
  return resourceTypes.some(type => bindingIDsRef(value, type).length > 0)
}

function uniqueSorted(ids: number[] | null | undefined): number[] {
  return Array.from(new Set((ids ?? []).filter(id => Number.isSafeInteger(id) && id > 0))).sort((a, b) => a - b)
}

function bindingIDsRef(value: ProjectProfileBindings, type: ProjectResourceType): number[] {
  switch (type) {
    case 'groups': return value.group_ids ?? []
    case 'accounts': return value.account_ids ?? []
    case 'proxies': return value.proxy_ids ?? []
    case 'subscriptions': return value.subscription_ids ?? []
  }
}

function profileBoundIDs(profile: ProjectProfile, type: ProjectResourceType): number[] {
  const known = profileBindingsByID[profile.id]
  if (known) return bindingIDsRef(known, type)
  if (bindings.value?.profile_id === profile.id) return bindingIDsRef(bindings.value, type)
  return []
}

function currentResourcePickerIDs(type: ProjectResourceType, target: ResourcePickerTarget): number[] {
  const source = target === 'project'
    ? projectForm.bindings
    : profileDraftBindings.value
  return source ? uniqueSorted(bindingIDsRef(source, type)) : []
}

function bindingLabel(type: ProjectResourceType, id: number): string {
  return bindingNames[type][id] || `${resourceTypeLabel(type)} #${id}`
}

function projectNameById(projectID?: number): string {
  if (!projectID) return '-'
  return projects.value.find(project => project.id === projectID)?.name ?? `#${projectID}`
}

function formatApiKeyPreview(key: string): string {
  if (!key) return '-'
  if (key.length <= 16) return key
  return `${key.slice(0, 12)}...${key.slice(-6)}`
}

function candidatesForType(type: ProjectResourceType, result: ProjectResourceSearchResult): CandidateItem[] {
  switch (type) {
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
    case 'proxies':
      return result.proxies.map(item => ({
        id: item.id,
        key: `proxies-${item.id}`,
        type,
        title: item.name || `${item.host}:${item.port}`,
        subtitle: [item.protocol, `${item.host}:${item.port}`, item.status].filter(Boolean).join(' · ')
      }))
    case 'subscriptions':
      return result.subscriptions.map(item => ({
        id: item.id,
        key: `subscriptions-${item.id}`,
        type,
        title: `${item.user_email} / ${item.group_name}`,
        subtitle: [item.status, item.notes].filter(Boolean).join(' · ')
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

function indexBindingDetails(value: Partial<ProjectProfileBindings> | null | undefined) {
  if (!value) return
  indexSearchResultNames({
    users: [],
    groups: value.groups ?? [],
    accounts: value.accounts ?? [],
    proxies: value.proxies ?? [],
    subscriptions: value.subscriptions ?? [],
    api_keys: []
  })
}

function resourceTypeLabel(type: ProjectResourceType): string {
  return t(`admin.projects.resourceTypes.${type}`)
}

function profileScopeValue(profile: ProjectProfile): ProjectProfileScopeValue {
  return `profile:${profile.id}`
}

function profileIDFromScopeValue(value: ProjectProfileScopeValue): number {
  return Number(value.replace('profile:', ''))
}

function formatProjectRole(role: string): string {
  if (role === 'super_admin') return t('admin.users.roles.super_admin')
  if (role === 'admin') return t('admin.users.roles.admin')
  return t('admin.users.roles.user')
}

function displayMemberRole(member: ProjectMember): string {
  return member.user_role === 'super_admin' ? 'super_admin' : member.role
}

function projectRoleBadgeClass(role: string): string {
  if (role === 'super_admin') return 'badge-danger'
  return role === 'admin' ? 'badge-primary' : 'badge-gray'
}

function formatMemberPermissions(permissions?: string[]): string {
  const labels = projectAdminPermissionOptions.value
    .filter(option => (permissions ?? defaultProjectAdminPermissions).includes(option.value))
    .map(option => option.label)
  if (labels.length === 0) {
    return t('admin.projects.noAdminPermissions')
  }
  return labels.join(' / ')
}

function memberStatusBadgeClass(status: ProjectMemberStatus): string {
  return status === 'active' ? 'badge-success' : 'badge-danger'
}

function memberStatusLabel(status: ProjectMemberStatus): string {
  return status === 'active' ? t('common.active') : t('admin.users.disabled')
}

let resourcePickerSearchTimer: ReturnType<typeof setTimeout> | undefined
let suppressResourcePickerAutoSearch = false
let memberSearchTimer: ReturnType<typeof setTimeout> | undefined
let memberSearchRequestID = 0

watch(
  () => memberSearch.query,
  () => {
    if (!showMemberDialog.value) return
    memberForm.userId = undefined
    if (memberSearchTimer) clearTimeout(memberSearchTimer)
    memberSearchTimer = setTimeout(() => {
      void searchMembers()
    }, 250)
  }
)

watch(
  () => [resourcePicker.query, resourcePicker.type, resourcePicker.target] as const,
  () => {
    if (!showResourcePicker.value || suppressResourcePickerAutoSearch) return
    if (resourcePickerSearchTimer) clearTimeout(resourcePickerSearchTimer)
    resourcePickerSearchTimer = setTimeout(() => {
      void searchResourcePicker()
    }, 250)
  }
)

watch(
  () => memberForm.role,
  (role) => {
    if (role === 'admin' && memberForm.permissions.length === 0) {
      memberForm.permissions = [...defaultProjectAdminPermissions]
    }
    if (role !== 'admin') {
      memberForm.permissions = []
    }
  }
)

watch(
  () => memberEditForm.role,
  (role) => {
    if (role === 'admin' && memberEditForm.permissions.length === 0) {
      memberEditForm.permissions = [...(editingMember.value?.permissions ?? defaultProjectAdminPermissions)]
    }
    if (role !== 'admin') {
      memberEditForm.permissions = []
    }
  }
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  loadProjects()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.project-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.375rem;
  margin-left: auto;
  white-space: nowrap;
}

.project-row-action {
  display: inline-flex;
  width: 3.125rem;
  min-height: 3.125rem;
  flex: 0 0 3.125rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.1875rem;
  border-radius: 0.5rem;
  padding: 0.375rem 0.25rem;
  font-size: 0.75rem;
  line-height: 1rem;
  color: rgb(100 116 139);
  transition: background-color 150ms ease, color 150ms ease;
}

.project-row-action > span {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-row-action:hover:not(:disabled) {
  background: rgb(241 245 249);
  color: rgb(37 99 235);
}

.project-row-action:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.project-row-action-danger {
  color: rgb(239 68 68);
}

.project-row-action-danger:hover:not(:disabled) {
  background: rgb(254 242 242);
  color: rgb(220 38 38);
}

.project-row-action-warning:hover:not(:disabled) {
  background: rgb(255 247 237);
  color: rgb(234 88 12);
}

.project-row-action-success:hover:not(:disabled) {
  background: rgb(240 253 244);
  color: rgb(22 163 74);
}

.project-action-button {
  display: inline-flex;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  border-radius: 0.5rem;
  border: 1px solid rgb(203 213 225 / 0.8);
  background: rgb(255 255 255 / 0.85);
  padding: 0.4375rem 0.75rem;
  font-size: 0.8125rem;
  font-weight: 500;
  color: rgb(51 65 85);
  transition: background-color 150ms ease, border-color 150ms ease, color 150ms ease, box-shadow 150ms ease;
}

.project-action-button:hover:not(:disabled) {
  border-color: rgb(148 163 184 / 0.9);
  background: rgb(248 250 252);
  color: rgb(15 23 42);
}

.project-action-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.project-action-button-compact {
  min-height: 2rem;
  border-color: rgb(203 213 225 / 0.65);
  padding: 0.3125rem 0.625rem;
  font-size: 0.75rem;
}

.project-action-button-danger {
  border-color: rgb(252 165 165 / 0.75);
  background: rgb(254 242 242 / 0.9);
  color: rgb(220 38 38);
}

.project-action-button-danger:hover:not(:disabled) {
  border-color: rgb(248 113 113 / 0.9);
  background: rgb(254 226 226);
  color: rgb(185 28 28);
  box-shadow: 0 8px 18px rgb(220 38 38 / 0.12);
}

.project-resource-count {
  display: inline-flex;
  min-width: 1.5rem;
  height: 1.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  background: rgb(241 245 249);
  padding: 0 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: rgb(71 85 105);
}

:global(.project-member-actions-column) {
  width: 11.25rem;
  min-width: 11.25rem;
}

:global(.project-profile-actions-column) {
  width: 7.5rem;
  min-width: 7.5rem;
}

:global(.project-member-actions-column > div),
:global(.project-profile-actions-column > div) {
  justify-content: flex-end;
}

:global(.dark) .project-action-button {
  border-color: rgb(71 85 105 / 0.8);
  background: rgb(30 41 59 / 0.7);
  color: rgb(226 232 240);
}

:global(.dark) .project-row-action {
  color: rgb(148 163 184);
}

:global(.dark) .project-row-action:hover:not(:disabled) {
  background: rgb(51 65 85 / 0.75);
  color: rgb(96 165 250);
}

:global(.dark) .project-row-action-danger {
  color: rgb(248 113 113);
}

:global(.dark) .project-row-action-danger:hover:not(:disabled) {
  background: rgb(127 29 29 / 0.32);
  color: rgb(254 202 202);
}

:global(.dark) .project-row-action-warning:hover:not(:disabled) {
  background: rgb(124 45 18 / 0.35);
  color: rgb(251 146 60);
}

:global(.dark) .project-row-action-success:hover:not(:disabled) {
  background: rgb(20 83 45 / 0.32);
  color: rgb(74 222 128);
}

:global(.dark) .project-action-button:hover:not(:disabled) {
  border-color: rgb(100 116 139 / 0.95);
  background: rgb(51 65 85 / 0.85);
  color: rgb(248 250 252);
}

:global(.dark) .project-action-button-danger {
  border-color: rgb(127 29 29 / 0.75);
  background: rgb(127 29 29 / 0.25);
  color: rgb(248 113 113);
}

:global(.dark) .project-action-button-danger:hover:not(:disabled) {
  border-color: rgb(239 68 68 / 0.75);
  background: rgb(127 29 29 / 0.45);
  color: rgb(254 202 202);
}

:global(.dark) .project-resource-count {
  background: rgb(51 65 85 / 0.9);
  color: rgb(203 213 225);
}
</style>
