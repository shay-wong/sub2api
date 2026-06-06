<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div><p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p><p class="text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p></div>
      </div>
      <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-3 flex items-center justify-between gap-3">
          <h4 class="text-xs font-semibold text-gray-500 dark:text-dark-400">
            {{ t('admin.users.groupRateLimits') }}
          </h4>
          <svg v-if="groupRateLimitsLoading" class="h-4 w-4 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
        </div>
        <div v-if="!groupRateLimitsLoading && groupRateLimits.length === 0" class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.users.groupRateLimitNoActiveWindow') }}
        </div>
        <div v-else class="grid gap-2 sm:grid-cols-2">
          <div
            v-for="record in groupRateLimits"
            :key="record.group_id"
            class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60"
            :data-testid="`user-group-rate-limit-${record.group_id}`"
          >
            <div class="mb-2 flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ record.group_name || `#${record.group_id}` }}
                </p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ formatGroupRateLimitUsage(record) }}
                </p>
              </div>
              <button
                type="button"
                class="rounded-md px-2 py-1 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:text-dark-300 dark:hover:bg-dark-600"
                :data-testid="`reset-user-group-rate-limit-${record.group_id}`"
                :disabled="resettingGroupRateLimitIds.has(record.group_id)"
                @click="resetGroupRateLimit(record)"
              >
                {{ resettingGroupRateLimitIds.has(record.group_id) ? t('common.saving') : t('admin.users.groupRateLimitReset') }}
              </button>
            </div>
            <div v-if="getGroupRateLimitValue(record) > 0" class="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
              <div
                :class="groupRateLimitBarClass(record)"
                :style="{ width: groupRateLimitPercent(record) + '%' }"
              />
            </div>
            <p class="mt-1 truncate text-[11px] text-gray-400 dark:text-dark-500">
              {{ formatGroupRateLimitReset(record) }}
            </p>
          </div>
        </div>
      </div>
      <div v-if="loading" class="flex justify-center py-8"><svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg></div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center"><p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p></div>
      <div v-else ref="scrollContainerRef" class="max-h-96 space-y-3 overflow-y-auto" @scroll="closeGroupSelector">
        <div v-for="key in apiKeys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex items-center gap-2"><span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span><span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">{{ key.status }}</span></div>
              <p class="truncate font-mono text-sm text-gray-500">{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}</p>
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
            <div class="flex items-center gap-1">
              <span>{{ t('admin.users.group') }}:</span>
              <button
                :ref="(el) => setGroupButtonRef(key.id, el)"
                @click="openGroupSelector(key)"
                class="-mx-1 -my-0.5 flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :disabled="updatingKeyIds.has(key.id)"
              >
                <GroupBadge
                  v-if="key.group_id && key.group"
                  :name="key.group.name"
                  :platform="key.group.platform"
                  :subscription-type="key.group.subscription_type"
                  :rate-multiplier="key.group.rate_multiplier"
                />
                <span v-else class="text-gray-400 italic">{{ t('admin.users.none') }}</span>
                <svg v-if="updatingKeyIds.has(key.id)" class="h-3 w-3 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                <svg v-else class="h-3 w-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" /></svg>
              </button>
            </div>
            <div class="flex items-center gap-1"><span>{{ t('admin.users.columns.created') }}: {{ formatDateTime(key.created_at) }}</span></div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>

  <!-- Group Selector Dropdown -->
  <Teleport to="body">
    <div
      v-if="groupSelectorKeyId !== null && dropdownPosition"
      ref="dropdownRef"
      class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-64 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
      :style="{ top: dropdownPosition.top + 'px', left: dropdownPosition.left + 'px' }"
    >
      <div class="max-h-64 overflow-y-auto p-1.5">
        <!-- Unbind option -->
        <button
          @click="changeGroup(selectedKeyForGroup!, null)"
          :class="[
            'flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors',
            !selectedKeyForGroup?.group_id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <span class="text-gray-500 italic">{{ t('admin.users.none') }}</span>
          <svg
            v-if="!selectedKeyForGroup?.group_id"
            class="ml-auto h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"
          ><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
        </button>
        <!-- Group options -->
        <button
          v-for="group in allGroups"
          :key="group.id"
          @click="changeGroup(selectedKeyForGroup!, group.id)"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
            selectedKeyForGroup?.group_id === group.id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <GroupOptionItem
            :name="group.name"
            :platform="group.platform"
            :subscription-type="group.subscription_type"
            :rate-multiplier="group.rate_multiplier"
            :description="group.description"
            :selected="selectedKeyForGroup?.group_id === group.id"
          />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatCurrency, formatDateTime } from '@/utils/format'
import type { AdminUser, AdminGroup, ApiKey, UserGroupRateLimitWindow } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close'])
const { t } = useI18n()
const appStore = useAppStore()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const groupRateLimits = ref<UserGroupRateLimitWindow[]>([])
const loading = ref(false)
const groupRateLimitsLoading = ref(false)
const updatingKeyIds = ref(new Set<number>())
const resettingGroupRateLimitIds = ref(new Set<number>())
const groupSelectorKeyId = ref<number | null>(null)
const dropdownPosition = ref<{ top: number; left: number } | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const scrollContainerRef = ref<HTMLElement | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())

const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

watch(
  () => props.show,
  (v) => {
    if (v && props.user) {
      load()
      loadGroups()
      loadGroupRateLimits()
    } else {
      closeGroupSelector()
      groupRateLimits.value = []
    }
  }
)

const load = async () => {
  if (!props.user) return
  loading.value = true
  groupButtonRefs.value.clear()
  try {
    const res = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = res.items || []
  } catch (error) {
    console.error('Failed to load API keys:', error)
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    const groups = await adminAPI.groups.getAll()
    allGroups.value = groups
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const loadGroupRateLimits = async () => {
  if (!props.user) return
  groupRateLimitsLoading.value = true
  try {
    const res = await adminAPI.users.getUserGroupRateLimits(props.user.id)
    groupRateLimits.value = res.group_rate_limits || []
  } catch (error) {
    console.error('Failed to load group rate limits:', error)
    appStore.showError(t('admin.users.groupRateLimitLoadFailed'))
  } finally {
    groupRateLimitsLoading.value = false
  }
}

const DROPDOWN_HEIGHT = 272 // max-h-64 = 16rem = 256px + padding
const DROPDOWN_GAP = 4

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    closeGroupSelector()
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const spaceBelow = window.innerHeight - rect.bottom
      const openUpward = spaceBelow < DROPDOWN_HEIGHT && rect.top > spaceBelow
      dropdownPosition.value = {
        top: openUpward ? rect.top - DROPDOWN_HEIGHT - DROPDOWN_GAP : rect.bottom + DROPDOWN_GAP,
        left: rect.left
      }
    }
    groupSelectorKeyId.value = key.id
  }
}

const closeGroupSelector = () => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  closeGroupSelector()
  if (key.group_id === newGroupId || (!key.group_id && newGroupId === null)) return

  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, newGroupId)
    // Update local data
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
    }
    if (result.auto_granted_group_access && result.granted_group_name) {
      appStore.showSuccess(t('admin.users.groupChangedWithGrant', { group: result.granted_group_name }))
    } else {
      appStore.showSuccess(t('admin.users.groupChangedSuccess'))
    }
    loadGroupRateLimits()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupChangeFailed'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const replaceGroupRateLimit = (updated: UserGroupRateLimitWindow) => {
  const idx = groupRateLimits.value.findIndex((r) => r.group_id === updated.group_id)
  if (idx !== -1) {
    groupRateLimits.value[idx] = updated
  } else {
    groupRateLimits.value.push(updated)
  }
}

const resetGroupRateLimit = async (record: UserGroupRateLimitWindow) => {
  if (!props.user) return
  closeGroupSelector()
  resettingGroupRateLimitIds.value.add(record.group_id)
  try {
    const result = await adminAPI.users.resetUserGroupRateLimit(props.user.id, record.group_id)
    replaceGroupRateLimit(result.group_rate_limit)
    appStore.showSuccess(t('admin.users.groupRateLimitResetSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupRateLimitResetFailed'))
  } finally {
    resettingGroupRateLimitIds.value.delete(record.group_id)
  }
}

const getGroupRateLimitValue = (record: UserGroupRateLimitWindow) => Number(record.rate_limit_5h || 0)
const getGroupUsageValue = (record: UserGroupRateLimitWindow) => Number(record.usage_5h_usd || 0)

const groupRateLimitPercent = (record: UserGroupRateLimitWindow) => {
  const limit = getGroupRateLimitValue(record)
  if (limit <= 0) return 0
  return Math.min((getGroupUsageValue(record) / limit) * 100, 100)
}

const formatGroupRateLimitUsage = (record: UserGroupRateLimitWindow) => {
  const usage = formatCurrency(getGroupUsageValue(record))
  const limit = getGroupRateLimitValue(record)
  if (limit <= 0) {
    return t('admin.users.groupRateLimitUsage', {
      usage,
      limit: t('admin.users.groupRateLimitUnlimited')
    })
  }
  return t('admin.users.groupRateLimitUsage', {
    usage,
    limit: formatCurrency(limit)
  })
}

const formatGroupRateLimitReset = (record: UserGroupRateLimitWindow) => {
  if (!record.window_5h_reset_at) return t('admin.users.groupRateLimitNoActiveWindow')
  return t('admin.users.resetsAt', { time: formatDateTime(record.window_5h_reset_at) })
}

const groupRateLimitBarClass = (record: UserGroupRateLimitWindow) => {
  const limit = getGroupRateLimitValue(record)
  const usage = getGroupUsageValue(record)
  if (usage >= limit) return 'h-full rounded-full bg-red-500 transition-all'
  if (usage >= limit * 0.8) return 'h-full rounded-full bg-yellow-500 transition-all'
  return 'h-full rounded-full bg-emerald-500 transition-all'
}

const handleKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && groupSelectorKeyId.value !== null) {
    event.stopPropagation()
    closeGroupSelector()
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (dropdownRef.value && !dropdownRef.value.contains(target)) {
    // Check if the click is on one of the group trigger buttons
    for (const el of groupButtonRefs.value.values()) {
      if (el.contains(target)) return
    }
    closeGroupSelector()
  }
}

const handleClose = () => {
  closeGroupSelector()
  groupRateLimits.value = []
  emit('close')
}

onMounted(() => {
  if (props.show && props.user) {
    load()
    loadGroups()
    loadGroupRateLimits()
  }
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeyDown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeyDown, true)
})
</script>
