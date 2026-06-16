<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-5">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.permissions.title') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.permissions.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <div class="relative">
            <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model.trim="search"
              type="search"
              class="input w-64 pl-9"
              :placeholder="t('admin.permissions.searchPlaceholder')"
            />
          </div>
          <button class="btn btn-secondary" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('common.refresh') }}</span>
          </button>
        </div>
      </div>

      <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
        <div v-if="loading" class="flex items-center justify-center py-16 text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="filteredSubjects.length === 0" class="flex items-center justify-center py-16 text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.permissions.empty') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
                  {{ t('admin.permissions.user') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
                  {{ t('admin.permissions.role') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
                  {{ t('admin.permissions.groups') }}
                </th>
                <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
                  {{ t('common.actions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-for="subject in filteredSubjects" :key="subject.id" class="align-top">
                <td class="px-4 py-4">
                  <div class="font-medium text-gray-900 dark:text-white">
                    {{ subject.username || subject.email }}
                  </div>
                  <div class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                    {{ subject.email }}
                  </div>
                  <div class="mt-2 inline-flex rounded-md px-2 py-0.5 text-xs font-medium" :class="subject.status === 'active' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'">
                    {{ statusLabel(subject.status) }}
                  </div>
                </td>
                <td class="px-4 py-4">
                  <select v-model="drafts[subject.id].role" class="input w-36">
                    <option value="user">{{ t('admin.permissions.roleUser') }}</option>
                    <option value="operator">{{ t('admin.permissions.roleOperator') }}</option>
                  </select>
                </td>
                <td class="px-4 py-4">
                  <div class="flex flex-col gap-2">
                    <select
                      v-model="drafts[subject.id].groupIDs"
                      multiple
                      :disabled="drafts[subject.id].role !== 'operator'"
                      class="input min-h-32 w-full min-w-80 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <option v-for="group in groups" :key="group.id" :value="group.id">
                        {{ group.name }}
                      </option>
                    </select>
                    <div class="flex flex-wrap gap-1.5">
                      <span
                        v-for="groupID in drafts[subject.id].groupIDs"
                        :key="groupID"
                        class="rounded-md bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                      >
                        {{ groupName(groupID) }}
                      </span>
                      <span v-if="drafts[subject.id].role === 'operator' && drafts[subject.id].groupIDs.length === 0" class="text-xs text-amber-600 dark:text-amber-300">
                        {{ t('admin.permissions.noGroupsSelected') }}
                      </span>
                    </div>
                  </div>
                </td>
                <td class="px-4 py-4 text-right">
                  <button
                    class="btn btn-primary btn-sm"
                    :disabled="savingID === subject.id || !isDirty(subject)"
                    @click="save(subject)"
                  >
                    <Icon v-if="savingID === subject.id" name="refresh" size="sm" class="animate-spin" />
                    <Icon v-else name="check" size="sm" />
                    <span>{{ t('common.save') }}</span>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { OperatorPermissionSubject } from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { AdminGroup } from '@/types'

interface OperatorDraft {
  role: 'operator' | 'user'
  groupIDs: number[]
}

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const savingID = ref<number | null>(null)
const search = ref('')
const subjects = ref<OperatorPermissionSubject[]>([])
const groups = ref<AdminGroup[]>([])
const drafts = reactive<Record<number, OperatorDraft>>({})

const groupNameByID = computed(() => {
  const out = new Map<number, string>()
  for (const group of groups.value) {
    out.set(group.id, group.name)
  }
  return out
})

const filteredSubjects = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return subjects.value
  return subjects.value.filter((subject) => {
    return (
      subject.email.toLowerCase().includes(q) ||
      subject.username.toLowerCase().includes(q) ||
      String(subject.id).includes(q)
    )
  })
})

function syncDraft(subject: OperatorPermissionSubject) {
  drafts[subject.id] = {
    role: subject.role,
    groupIDs: [...(subject.group_ids ?? [])]
  }
}

function normalizeGroupIDs(groupIDs: number[]): number[] {
  return [...new Set(groupIDs.map(Number).filter((id) => Number.isFinite(id) && id > 0))].sort((a, b) => a - b)
}

function sameIDs(left: number[], right: number[]): boolean {
  const a = normalizeGroupIDs(left)
  const b = normalizeGroupIDs(right)
  return a.length === b.length && a.every((id, index) => id === b[index])
}

function isDirty(subject: OperatorPermissionSubject): boolean {
  const draft = drafts[subject.id]
  if (!draft) return false
  return draft.role !== subject.role || !sameIDs(draft.groupIDs, subject.group_ids ?? [])
}

function groupName(groupID: number): string {
  return groupNameByID.value.get(Number(groupID)) ?? `#${groupID}`
}

function statusLabel(status: string): string {
  return status === 'active' ? t('admin.permissions.statusActive') : t('admin.permissions.statusDisabled')
}

async function load() {
  loading.value = true
  try {
    const [nextSubjects, nextGroups] = await Promise.all([
      adminAPI.permissions.listOperators(),
      adminAPI.groups.getAllIncludingInactive()
    ])
    subjects.value = nextSubjects
    groups.value = nextGroups
    for (const subject of nextSubjects) {
      syncDraft(subject)
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.permissions.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function save(subject: OperatorPermissionSubject) {
  const draft = drafts[subject.id]
  if (!draft) return

  savingID.value = subject.id
  try {
    const updated = await adminAPI.permissions.updateOperator(subject.id, {
      role: draft.role,
      group_ids: draft.role === 'operator' ? normalizeGroupIDs(draft.groupIDs) : []
    })
    const index = subjects.value.findIndex((item) => item.id === subject.id)
    if (index >= 0) {
      subjects.value[index] = updated
    }
    syncDraft(updated)
    appStore.showSuccess(t('admin.permissions.saved'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.permissions.saveFailed'))
  } finally {
    savingID.value = null
  }
}

onMounted(() => {
  load()
})
</script>
