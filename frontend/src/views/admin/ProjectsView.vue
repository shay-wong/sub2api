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
          <button class="btn btn-primary" @click="openCreateDialog">
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
            :action-text="t('admin.projects.createProject')"
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
                        <div class="truncate font-medium text-gray-900 dark:text-white">
                          {{ project.name }}
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
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 p-4 dark:border-dark-700">
              <div class="min-w-0">
                <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
                  {{ selectedProject?.name || t('admin.projects.members') }}
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
                  {{ t('admin.projects.membersDescription') }}
                </p>
              </div>
              <div class="flex items-center gap-2">
                <button class="btn btn-secondary" :disabled="!selectedProject || membersLoading" @click="loadMembers">
                  <Icon name="refresh" size="sm" :class="membersLoading ? 'animate-spin' : ''" />
                </button>
                <button class="btn btn-secondary" :disabled="!selectedProject" @click="openMoveDialog">
                  <Icon name="arrowRight" size="md" class="mr-2" />
                  {{ t('admin.projects.moveResources') }}
                </button>
                <button class="btn btn-primary" :disabled="!selectedProject" @click="openMemberDialog">
                  <Icon name="plus" size="md" class="mr-2" />
                  {{ t('admin.projects.addMember') }}
                </button>
              </div>
            </div>

            <div class="table-wrapper min-h-0 flex-1">
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
                      <div class="text-xs text-gray-500 dark:text-dark-300">{{ member.username || '-' }}</div>
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
                      <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-dark-200">
                        <input
                          type="checkbox"
                          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                          :checked="member.is_owner"
                          @change="toggleMemberOwner(member, ($event.target as HTMLInputElement).checked)"
                        />
                        {{ member.is_owner ? t('common.yes') : t('common.no') }}
                      </label>
                    </td>
                    <td>
                      <span
                        class="inline-flex rounded-full px-2 py-1 text-xs font-medium"
                        :class="member.status === 'active' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'"
                      >
                        {{ member.status === 'active' ? t('common.active') : t('admin.users.disabled') }}
                      </span>
                    </td>
                    <td>
                      <button class="btn btn-danger btn-sm" @click="removeMember(member)">
                        {{ t('common.delete') }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
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
          <label class="input-label">{{ t('admin.projects.userId') }}</label>
          <input v-model.number="memberForm.userId" type="number" min="1" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.role') }}</label>
          <select v-model="memberForm.role" class="input">
            <option value="admin">{{ t('admin.users.roles.admin') }}</option>
            <option value="user">{{ t('admin.users.roles.user') }}</option>
          </select>
        </div>
        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
          <input v-model="memberForm.isOwner" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.projects.owner') }}
        </label>
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
      :show="showMoveDialog"
      :title="t('admin.projects.moveResources')"
      width="normal"
      @close="showMoveDialog = false"
    >
      <form id="move-resources-form" class="space-y-4" @submit.prevent="submitMoveResources">
        <div>
          <label class="input-label">{{ t('admin.projects.accountIds') }}</label>
          <textarea v-model.trim="moveForm.accountIds" class="input min-h-20 resize-y font-mono" :placeholder="t('admin.projects.idListPlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.apiKeyIds') }}</label>
          <textarea v-model.trim="moveForm.apiKeyIds" class="input min-h-20 resize-y font-mono" :placeholder="t('admin.projects.idListPlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.projects.groupIds') }}</label>
          <textarea v-model.trim="moveForm.groupIds" class="input min-h-20 resize-y font-mono" :placeholder="t('admin.projects.idListPlaceholder')" />
        </div>
        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
          <input v-model="moveForm.moveUsageHistory" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.projects.moveUsageHistory') }}
        </label>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showMoveDialog = false">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" form="move-resources-form" type="submit" :disabled="movingResources">
          {{ movingResources ? t('common.saving') : t('admin.projects.moveResources') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { AdminProject, AssignableProjectRole, ProjectMember } from '@/api/admin/projects'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const membersLoading = ref(false)
const savingProject = ref(false)
const savingMember = ref(false)
const movingResources = ref(false)
const projects = ref<AdminProject[]>([])
const members = ref<ProjectMember[]>([])
const selectedProject = ref<AdminProject | null>(null)
const showProjectDialog = ref(false)
const showMemberDialog = ref(false)
const showMoveDialog = ref(false)

const projectForm = reactive({
  name: '',
  slug: '',
  description: ''
})

const memberForm = reactive({
  userId: undefined as number | undefined,
  role: 'user' as AssignableProjectRole,
  isOwner: false
})

const moveForm = reactive({
  accountIds: '',
  apiKeyIds: '',
  groupIds: '',
  moveUsageHistory: true
})

async function loadProjects() {
  loading.value = true
  try {
    projects.value = await adminAPI.projects.list()
    if (!selectedProject.value || !projects.value.some(project => project.id === selectedProject.value?.id)) {
      selectedProject.value = projects.value[0] ?? null
    }
    if (selectedProject.value) {
      await loadMembers()
    } else {
      members.value = []
    }
  } catch (error) {
    console.error('Failed to load projects:', error)
    appStore.showError(t('admin.projects.failedToLoad'))
  } finally {
    loading.value = false
  }
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

async function selectProject(project: AdminProject) {
  selectedProject.value = project
  await loadMembers()
}

function openCreateDialog() {
  projectForm.name = ''
  projectForm.slug = ''
  projectForm.description = ''
  showProjectDialog.value = true
}

async function submitProject() {
  savingProject.value = true
  try {
    const project = await adminAPI.projects.create({
      name: projectForm.name,
      slug: projectForm.slug,
      description: projectForm.description || null
    })
    appStore.showSuccess(t('admin.projects.projectCreated'))
    showProjectDialog.value = false
    await loadProjects()
    selectedProject.value = projects.value.find(item => item.id === project.id) ?? selectedProject.value
    await loadMembers()
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
  memberForm.isOwner = false
  showMemberDialog.value = true
}

function openMoveDialog() {
  moveForm.accountIds = ''
  moveForm.apiKeyIds = ''
  moveForm.groupIds = ''
  moveForm.moveUsageHistory = true
  showMoveDialog.value = true
}

function parseIDList(value: string): number[] {
  const seen = new Set<number>()
  return value
    .split(/[\s,，;；]+/)
    .map(item => Number.parseInt(item, 10))
    .filter(id => Number.isSafeInteger(id) && id > 0)
    .filter(id => {
      if (seen.has(id)) return false
      seen.add(id)
      return true
    })
}

async function submitMember() {
  if (!selectedProject.value || !memberForm.userId) return
  savingMember.value = true
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, memberForm.userId, {
      role: memberForm.role,
      is_owner: memberForm.isOwner
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

async function submitMoveResources() {
  if (!selectedProject.value) return
  const accountIds = parseIDList(moveForm.accountIds)
  const apiKeyIds = parseIDList(moveForm.apiKeyIds)
  const groupIds = parseIDList(moveForm.groupIds)
  if (accountIds.length === 0 && apiKeyIds.length === 0 && groupIds.length === 0) {
    appStore.showError(t('admin.projects.moveSelectResources'))
    return
  }
  movingResources.value = true
  try {
    const result = await adminAPI.projects.moveResources(selectedProject.value.id, {
      account_ids: accountIds,
      api_key_ids: apiKeyIds,
      group_ids: groupIds,
      move_usage_history: moveForm.moveUsageHistory
    })
    appStore.showSuccess(t('admin.projects.resourcesMoved', {
      accounts: result.accounts_moved,
      apiKeys: result.api_keys_moved,
      groups: result.groups_moved
    }))
    showMoveDialog.value = false
  } catch (error) {
    console.error('Failed to move project resources:', error)
    appStore.showError(t('admin.projects.failedToMoveResources'))
  } finally {
    movingResources.value = false
  }
}

async function updateMemberRole(member: ProjectMember, role: AssignableProjectRole) {
  if (!selectedProject.value || role === member.role) return
  await saveMember(member, { role })
}

async function toggleMemberOwner(member: ProjectMember, isOwner: boolean) {
  if (!selectedProject.value || isOwner === member.is_owner) return
  await saveMember(member, { is_owner: isOwner })
}

async function saveMember(member: ProjectMember, patch: { role?: AssignableProjectRole; is_owner?: boolean }) {
  if (!selectedProject.value) return
  try {
    await adminAPI.projects.setMember(selectedProject.value.id, member.user_id, {
      role: patch.role ?? member.role,
      is_owner: patch.is_owner ?? member.is_owner
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

onMounted(() => {
  loadProjects()
})
</script>
