<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkTest.title')"
    width="wide"
    :close-on-click-outside="false"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div class="space-y-3">
          <div class="space-y-1.5">
            <label class="input-label">{{ t('admin.accounts.openai.testMode') }}</label>
            <div
              class="grid max-w-xl grid-cols-2 rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-800"
              data-test="batch-test-mode"
            >
              <button
                type="button"
                data-test="batch-test-mode-default"
                :disabled="running"
                :class="modeButtonClass('default')"
                @click="testMode = 'default'"
              >
                {{ t('admin.accounts.openai.testModeDefault') }}
              </button>
              <button
                type="button"
                data-test="batch-test-mode-compact"
                :disabled="running"
                :class="modeButtonClass('compact')"
                @click="testMode = 'compact'"
              >
                {{ t('admin.accounts.openai.testModeCompact') }}
              </button>
            </div>
            <p class="input-hint">
              {{ t('admin.accounts.bulkTest.modeHint') }}
            </p>
          </div>
          <label
            class="flex max-w-xl items-start gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800"
            data-test="batch-test-skip-recent-option"
          >
            <input
              v-model="skipRecentlyTested"
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-900"
              :disabled="running"
            />
            <span class="min-w-0 space-y-1">
              <span class="block font-medium text-gray-800 dark:text-dark-100">
                {{ t('admin.accounts.bulkTest.skipRecentLabel', { minutes: BATCH_TEST_RECENT_SKIP_WINDOW_MINUTES }) }}
              </span>
              <span class="block text-xs text-gray-500 dark:text-dark-300">
                {{ t('admin.accounts.bulkTest.skipRecentHint', { count: recentTestCandidateCount }) }}
              </span>
            </span>
          </label>

          <div data-test="batch-test-summary" class="text-sm text-gray-600 dark:text-dark-300">
            {{ t('admin.accounts.bulkTest.summary', summary) }}
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-4">
          <div class="rounded-lg border border-gray-200 bg-white/70 p-3 text-center dark:border-dark-700 dark:bg-dark-800/60">
            <div class="text-lg font-semibold text-gray-900 dark:text-white">{{ summary.total }}</div>
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.bulkTest.total') }}</div>
          </div>
          <div class="rounded-lg border border-emerald-200 bg-emerald-50/70 p-3 text-center dark:border-emerald-800 dark:bg-emerald-500/10">
            <div class="text-lg font-semibold text-emerald-600 dark:text-emerald-400">{{ summary.success }}</div>
            <div class="text-xs text-emerald-600/80 dark:text-emerald-400/80">{{ t('admin.accounts.bulkTest.success') }}</div>
          </div>
          <div class="rounded-lg border border-red-200 bg-red-50/70 p-3 text-center dark:border-red-800 dark:bg-red-500/10">
            <div class="text-lg font-semibold text-red-600 dark:text-red-400">{{ summary.failed }}</div>
            <div class="text-xs text-red-600/80 dark:text-red-400/80">{{ t('admin.accounts.bulkTest.failed') }}</div>
          </div>
          <div class="rounded-lg border border-amber-200 bg-amber-50/70 p-3 text-center dark:border-amber-800 dark:bg-amber-500/10">
            <div class="text-lg font-semibold text-amber-600 dark:text-amber-400">{{ summary.skipped }}</div>
            <div class="text-xs text-amber-600/80 dark:text-amber-400/80">{{ t('admin.accounts.bulkTest.skipped') }}</div>
          </div>
        </div>
      </div>

      <div class="max-h-[480px] overflow-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40">
        <div
          v-if="hasTestResults"
          class="sticky top-0 z-10 flex flex-col gap-3 border-b border-gray-200 bg-white/95 p-3 backdrop-blur dark:border-dark-700 dark:bg-dark-900/95 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="flex flex-wrap items-center gap-2">
            <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-800">
              <button
                type="button"
                data-test="batch-test-filter-all"
                :class="filterButtonClass(false)"
                @click="showFailedOnly = false"
              >
                {{ t('admin.accounts.bulkTest.filterAll') }}
              </button>
              <button
                type="button"
                data-test="batch-test-filter-failed"
                :class="filterButtonClass(true)"
                :disabled="summary.failed === 0"
                @click="showFailedOnly = true"
              >
                {{ t('admin.accounts.bulkTest.filterFailed', { count: summary.failed }) }}
              </button>
            </div>
            <span v-if="summary.failed > 0" class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.bulkTest.failedSelection', { count: selectedFailedIds.length }) }}
            </span>
          </div>

          <div v-if="summary.failed > 0" class="flex flex-wrap gap-2">
            <button
              type="button"
              data-test="batch-test-select-visible-failed"
              class="btn btn-secondary btn-sm"
              :disabled="running || deletingFailed || visibleFailedItems.length === 0"
              @click="toggleVisibleFailedSelection"
            >
              {{ allVisibleFailedSelected ? t('admin.accounts.bulkTest.clearFailedSelection') : t('admin.accounts.bulkTest.selectVisibleFailed') }}
            </button>
            <button
              type="button"
              data-test="batch-test-delete-selected-failed"
              class="btn btn-danger btn-sm"
              :disabled="running || deletingFailed || selectedFailedIds.length === 0"
              @click="deleteSelectedFailedAccounts"
            >
              {{ deletingFailed ? t('admin.accounts.bulkTest.deletingFailed') : t('admin.accounts.bulkTest.deleteSelectedFailed', { count: selectedFailedIds.length }) }}
            </button>
            <button
              type="button"
              data-test="batch-test-delete-all-failed"
              class="btn btn-danger btn-sm"
              :disabled="running || deletingFailed || failedItems.length === 0"
              @click="deleteAllFailedAccounts"
            >
              {{ t('admin.accounts.bulkTest.deleteAllFailed', { count: failedItems.length }) }}
            </button>
          </div>
        </div>

        <div
          v-for="item in visibleTestItems"
          :key="item.account.id"
          class="border-b border-gray-100 p-3 last:border-b-0 dark:border-dark-700"
        >
          <div class="grid gap-3 lg:grid-cols-[2rem_minmax(0,1fr)_minmax(240px,320px)_auto] lg:items-start">
            <div class="flex h-8 items-center justify-center">
              <input
                v-if="item.status === 'failed'"
                type="checkbox"
                data-test="batch-test-failed-checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800"
                :checked="selectedFailedIds.includes(item.account.id)"
                :disabled="running || deletingFailed"
                @change="toggleFailedSelection(item.account.id, ($event.target as HTMLInputElement).checked)"
              />
            </div>
            <div class="min-w-0">
              <div class="truncate text-sm font-medium text-gray-900 dark:text-white">
                {{ item.account.name }}
              </div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                #{{ item.account.id }} · {{ item.account.platform }} · {{ item.account.type }}
              </div>
            </div>
            <div class="space-y-1">
              <div class="text-xs font-medium text-gray-600 dark:text-dark-300">
                {{ t('admin.accounts.bulkTest.modelId') }}
              </div>
              <Select
                v-model="item.selectedModelId"
                :options="modelOptionsFor(item)"
                :disabled="running || item.loadingModels"
                :placeholder="item.loadingModels ? t('common.loading') : t('admin.accounts.bulkTest.defaultModelOption')"
                searchable
                data-test="batch-test-model-select"
              />
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ item.modelLoadError || t('admin.accounts.bulkTest.modelHint') }}
              </p>
            </div>
            <div class="flex justify-start lg:justify-end">
              <span :class="statusClass(item.status)">
                {{ statusLabel(item.status) }}
              </span>
            </div>
          </div>
          <div v-if="item.error" class="mt-2 text-xs text-red-600 dark:text-red-400">
            {{ item.error }}
          </div>
          <details
            v-if="item.logs.length > 0"
            class="mt-2 rounded bg-gray-50 text-xs text-gray-700 dark:bg-dark-800 dark:text-dark-200"
            :open="item.status === 'running'"
          >
            <summary class="cursor-pointer select-none px-2 py-1.5 text-gray-500 dark:text-dark-300">
              {{ t('admin.accounts.bulkTest.logDetails', { count: item.logs.length }) }}
            </summary>
            <div class="max-h-32 overflow-auto border-t border-gray-100 p-2 font-mono dark:border-dark-700">
              <div v-for="(line, idx) in item.logs" :key="idx" class="whitespace-pre-wrap break-words">
                {{ line }}
              </div>
            </div>
          </details>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          type="button"
          data-test="batch-test-close"
          class="btn btn-secondary"
          :disabled="deletingFailed"
          @click="handleClose"
        >
          {{ running ? t('common.cancel') : t('common.close') }}
        </button>
        <button
          type="button"
          data-test="batch-test-start"
          class="btn btn-primary"
          :disabled="running || deletingFailed || testItems.length === 0"
          @click="startBatchTest"
        >
          {{ running ? t('admin.accounts.bulkTest.running') : t('admin.accounts.bulkTest.start') }}
        </button>
      </div>
    </template>
  </BaseDialog>
  <ConfirmDialog
    :show="showFailedDeleteConfirm"
    :title="t('admin.accounts.bulkTest.deleteFailedTitle')"
    :message="t('admin.accounts.bulkTest.deleteFailedConfirm', { count: failedDeleteConfirmCount })"
    :confirm-text="deletingFailed ? t('admin.accounts.bulkTest.deletingFailed') : t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    :confirm-disabled="deletingFailed"
    @confirm="confirmDeleteFailedAccounts"
    @cancel="closeFailedDeleteConfirm"
  >
    <div class="space-y-3">
      <div class="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900/50 dark:bg-red-900/20">
        <div class="flex gap-2">
          <Icon name="exclamationTriangle" size="sm" :stroke-width="2" class="mt-0.5 shrink-0 text-red-600 dark:text-red-300" />
          <div class="space-y-1">
            <p class="text-sm font-medium text-red-800 dark:text-red-200">
              {{ t('admin.accounts.bulkTest.deleteFailedSummary', { count: failedDeleteConfirmCount }) }}
            </p>
            <p class="text-xs leading-relaxed text-red-700 dark:text-red-300">
              {{ t('admin.accounts.bulkTest.deleteFailedWarning') }}
            </p>
          </div>
        </div>
      </div>
      <div v-if="failedDeleteGroups.length > 0" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70">
        <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
          {{ t('admin.accounts.bulkTest.deleteFailedErrorSummaryTitle') }}
        </div>
        <div class="space-y-2">
          <div
            v-for="group in failedDeleteGroups"
            :key="group.key"
            class="rounded-lg border bg-white dark:bg-dark-900/60"
            :class="failedDeleteGroupClass(group.severity)"
            :data-severity="group.severity"
            data-test="batch-test-delete-error-group"
          >
            <button
              type="button"
              class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left"
              :aria-expanded="isFailedDeleteGroupExpanded(group.key)"
              @click="toggleFailedDeleteGroup(group.key)"
            >
              <span class="flex min-w-0 items-center gap-2">
                <Icon
                  :name="group.severity === 'warning' ? 'exclamationTriangle' : 'xCircle'"
                  size="sm"
                  :stroke-width="2"
                  class="shrink-0"
                  :class="failedDeleteGroupIconClass(group.severity)"
                />
                <span class="truncate text-sm font-medium text-gray-800 dark:text-dark-100">{{ group.label }}</span>
                <span
                  class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="failedDeleteGroupBadgeClass(group.severity)"
                >
                  {{ group.severityLabel }}
                </span>
              </span>
              <span class="flex shrink-0 items-center gap-2 text-xs text-gray-500 dark:text-dark-300">
                {{ t('admin.accounts.bulkTest.deleteFailedGroupCount', { count: group.count }) }}
                <Icon :name="isFailedDeleteGroupExpanded(group.key) ? 'chevronDown' : 'chevronRight'" size="xs" :stroke-width="2" />
              </span>
            </button>
            <div
              v-if="isFailedDeleteGroupExpanded(group.key)"
              class="border-t border-gray-100 px-3 py-2 dark:border-dark-700"
              data-test="batch-test-delete-error-group-details"
            >
              <ul class="max-h-36 space-y-1.5 overflow-auto">
                <li
                  v-for="item in group.items"
                  :key="item.account.id"
                  class="flex min-w-0 items-center justify-between gap-3 text-sm text-gray-700 dark:text-dark-200"
                  data-test="batch-test-delete-error-account"
                >
                  <span class="truncate">{{ item.account.name }}</span>
                  <span class="shrink-0 text-xs text-gray-400 dark:text-dark-400">#{{ item.account.id }}</span>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>
      <div v-if="deletingFailed" class="space-y-2 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/60">
        <div class="flex items-center justify-between text-xs text-gray-600 dark:text-dark-300" data-test="batch-test-delete-progress">
          <span>{{ t('admin.accounts.bulkTest.deleteFailedProgress', failedDeleteProgress) }}</span>
          <span>{{ failedDeleteProgressPercent }}%</span>
        </div>
        <div
          class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"
          role="progressbar"
          :aria-valuemin="0"
          :aria-valuemax="100"
          :aria-valuenow="failedDeleteProgressPercent"
        >
          <div
            class="h-full rounded-full bg-red-500 transition-all duration-200 dark:bg-red-400"
            :style="{ width: `${failedDeleteProgressPercent}%` }"
          />
        </div>
      </div>
    </div>
  </ConfirmDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { runAccountConnectionTest, type AccountTestEvent } from '@/utils/accountTestRunner'
import {
  ACCOUNT_TEST_RECENT_SKIP_WINDOW_MINUTES,
  accountTestStateFingerprint,
  getRecentAccountTestRecords,
  isAccountRecentlyTested,
  markAccountRecentlyTested,
  type AccountTestMode
} from '@/utils/accountTestRecent'
import type { Account, ClaudeModel } from '@/types'

type TestStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped'
type TestMode = AccountTestMode
type FailedDeleteSeverity = 'error' | 'warning'

interface TestItem {
  account: Account
  status: TestStatus
  logs: string[]
  error: string
  selectedModelId: string
  modelOptions: SelectOption[]
  loadingModels: boolean
  modelLoadError: string
}

interface FailedDeleteGroup {
  key: string
  label: string
  severity: FailedDeleteSeverity
  severityLabel: string
  count: number
  items: TestItem[]
}

const DEFAULT_MODEL_OPTION_VALUE = ''
const BATCH_TEST_CONCURRENCY = 20
const MODEL_LOAD_CONCURRENCY = 10
const BATCH_TEST_START_STAGGER_MIN_MS = 5
const BATCH_TEST_START_STAGGER_MAX_MS = 25
const BATCH_TEST_TIMEOUT_SECONDS = 120
const BATCH_TEST_TIMEOUT_MS = BATCH_TEST_TIMEOUT_SECONDS * 1000
const BATCH_TEST_RECENT_SKIP_WINDOW_MINUTES = ACCOUNT_TEST_RECENT_SKIP_WINDOW_MINUTES
const BATCH_TEST_RECENT_HINT_REFRESH_MS = 30 * 1000

const props = defineProps<{
  show: boolean
  accounts: Account[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'running-change', running: boolean): void
  (e: 'completed', payload: { success: number; failed: number; skipped: number; failedIds: number[]; errors: string[] }): void
  (e: 'deleted', payload: { success: number; failed: number; deletedIds: number[] }): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const testMode = ref<TestMode>('default')
const skipRecentlyTested = ref(true)
const running = ref(false)
const deletingFailed = ref(false)
const showFailedDeleteConfirm = ref(false)
const pendingFailedDeleteIds = ref<number[]>([])
const showFailedOnly = ref(false)
const selectedFailedIds = ref<number[]>([])
const testItems = ref<TestItem[]>([])
const recentTestRecords = ref(getRecentAccountTestRecords())
const recentTestNow = ref(Date.now())
const abortControllers = new Set<AbortController>()
const expandedFailedDeleteGroupKeys = ref<string[]>([])
const failedDeleteProgress = ref({ completed: 0, total: 0 })
let cancellationRequested = false
let recentTestRefreshTimer: number | null = null

const refreshRecentTestCandidates = (persistPrune = false, now = Date.now()) => {
  recentTestNow.value = now
  recentTestRecords.value = getRecentAccountTestRecords(now, persistPrune)
}

const stopRecentTestCandidateRefresh = () => {
  if (recentTestRefreshTimer === null) return
  window.clearInterval(recentTestRefreshTimer)
  recentTestRefreshTimer = null
}

const startRecentTestCandidateRefresh = () => {
  refreshRecentTestCandidates()
  if (recentTestRefreshTimer !== null) return
  recentTestRefreshTimer = window.setInterval(() => {
    refreshRecentTestCandidates(true)
  }, BATCH_TEST_RECENT_HINT_REFRESH_MS)
}

const resetItems = () => {
  refreshRecentTestCandidates()
  testItems.value = props.accounts.map(account => ({
    account,
    status: 'pending',
    logs: [],
    error: '',
    selectedModelId: DEFAULT_MODEL_OPTION_VALUE,
    modelOptions: [],
    loadingModels: false,
    modelLoadError: ''
  }))
  selectedFailedIds.value = []
  showFailedOnly.value = false
}

const defaultModelOption = computed<SelectOption>(() => ({
  value: DEFAULT_MODEL_OPTION_VALUE,
  label: t('admin.accounts.bulkTest.defaultModelOption')
}))

const toModelOption = (model: ClaudeModel): SelectOption => ({
  value: model.id,
  label: model.display_name || model.id
})

const sortModelOptions = (options: SelectOption[]) => [...options].sort((a, b) => String(a.label).localeCompare(String(b.label)))

const modelOptionsFor = (item: TestItem) => [
  defaultModelOption.value,
  ...item.modelOptions
]

const loadItemModels = async (item: TestItem) => {
  item.loadingModels = true
  item.modelLoadError = ''
  try {
    const models = await adminAPI.accounts.getAvailableModels(item.account.id)
    item.modelOptions = sortModelOptions(models.map(toModelOption))
  } catch (error) {
    console.error('Failed to load batch test models:', error)
    item.modelOptions = []
    item.modelLoadError = t('admin.accounts.bulkTest.modelLoadFailed')
  } finally {
    item.loadingModels = false
  }
}

const loadItemModelsLimited = async (items: TestItem[]) => {
  let nextIndex = 0
  const workerCount = Math.min(MODEL_LOAD_CONCURRENCY, items.length)
  const workers = Array.from({ length: workerCount }, async () => {
    while (nextIndex < items.length) {
      const item = items[nextIndex]
      nextIndex += 1
      await loadItemModels(item)
    }
  })
  await Promise.all(workers)
}

const loadAllItemModels = async () => {
  await loadItemModelsLimited(testItems.value)
}

const abortCurrentTest = () => {
  abortControllers.forEach(controller => controller.abort())
  abortControllers.clear()
}

const cancelBatchTest = () => {
  cancellationRequested = true
  abortCurrentTest()
}

watch(
  () => [props.show, props.accounts] as const,
  ([show]) => {
    if (show) {
      resetItems()
      startRecentTestCandidateRefresh()
      loadAllItemModels()
    } else {
      stopRecentTestCandidateRefresh()
      cancelBatchTest()
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  stopRecentTestCandidateRefresh()
})

const summary = computed(() => ({
  total: testItems.value.length,
  success: testItems.value.filter(item => item.status === 'success').length,
  failed: testItems.value.filter(item => item.status === 'failed').length,
  running: testItems.value.filter(item => item.status === 'running').length,
  pending: testItems.value.filter(item => item.status === 'pending').length,
  skipped: testItems.value.filter(item => item.status === 'skipped').length
}))
const recentTestCandidateCount = computed(() => {
  const now = recentTestNow.value
  const records = recentTestRecords.value
  return testItems.value.filter(item => (
    isAccountRecentlyTested(item.account.id, testMode.value, accountTestStateFingerprint(item.account), now, records)
  )).length
})

const failedItems = computed(() => testItems.value.filter(item => item.status === 'failed'))
const visibleTestItems = computed(() => showFailedOnly.value ? failedItems.value : testItems.value)
const visibleFailedItems = computed(() => visibleTestItems.value.filter(item => item.status === 'failed'))
const hasTestResults = computed(() => testItems.value.some(item => item.status === 'success' || item.status === 'failed'))
const allVisibleFailedSelected = computed(() => (
  visibleFailedItems.value.length > 0 &&
  visibleFailedItems.value.every(item => selectedFailedIds.value.includes(item.account.id))
))
const failedDeleteConfirmCount = computed(() => pendingFailedDeleteIds.value.length)
const failedDeleteItems = computed(() => {
  const pendingIds = new Set(pendingFailedDeleteIds.value)
  return failedItems.value.filter(item => pendingIds.has(item.account.id))
})
const failedDeleteGroups = computed<FailedDeleteGroup[]>(() => {
  const groups = new Map<string, TestItem[]>()
  failedDeleteItems.value.forEach((item) => {
    const code = extractErrorCode(item.error)
    const key = code || 'unknown'
    groups.set(key, [...(groups.get(key) || []), item])
  })

  return Array.from(groups.entries())
    .map(([key, items]) => {
      const code = key === 'unknown' ? '' : key
      const severity = failedDeleteSeverityFor(code)
      return {
        key,
        label: failedDeleteGroupLabelFor(key),
        severity,
        severityLabel: t(`admin.accounts.bulkTest.deleteFailedSeverity.${severity}`),
        count: items.length,
        items
      }
    })
    .sort((a, b) => {
      if (a.severity !== b.severity) return a.severity === 'error' ? -1 : 1
      return b.count - a.count || a.key.localeCompare(b.key)
    })
})
const failedDeleteProgressPercent = computed(() => {
  if (failedDeleteProgress.value.total <= 0) return 0
  return Math.round((failedDeleteProgress.value.completed / failedDeleteProgress.value.total) * 100)
})

const statusLabel = (status: TestStatus) => t(`admin.accounts.bulkTest.status.${status}`)

const statusClass = (status: TestStatus) => [
  'shrink-0 rounded-full px-2 py-1 text-xs font-medium',
  status === 'success' && 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300',
  status === 'failed' && 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300',
  status === 'running' && 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-300',
  status === 'skipped' && 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300',
  status === 'pending' && 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
]

const modeButtonClass = (mode: TestMode) => [
  'rounded-md px-3 py-2 text-sm font-medium transition-colors',
  testMode.value === mode
    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
    : 'text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white',
  running.value && 'cursor-not-allowed opacity-70'
]

const filterButtonClass = (failedOnly: boolean) => [
  'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
  showFailedOnly.value === failedOnly
    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
    : 'text-gray-500 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-50 dark:text-dark-300 dark:hover:text-white'
]

const extractErrorCode = (message: string | null | undefined): string => {
  if (!message) return ''
  if (
    message === t('admin.accounts.bulkTest.logTimeout', { seconds: BATCH_TEST_TIMEOUT_SECONDS }) ||
    /(?:timeout|timed out|超时)/i.test(message)
  ) {
    return 'timeout'
  }
  const patterns = [
    /"(?:(?:error_)?code|status_code|status)"\s*:\s*"?(?<code>[A-Za-z0-9_.-]+)"?/i,
    /\b(?:error_code|status_code|status|code|HTTP)\s*[:=]\s*["']?(?<code>[A-Za-z0-9_.-]+)["']?/i,
    /\b(?:returned|returns|returning|响应|返回)\s+(?<code>[4-5]\d{2})\b/i,
    /\b(?:invalid_[a-z0-9_]+|rate_limit_exceeded|insufficient_quota|context_length_exceeded|upstream_error)\b/i
  ]
  for (const pattern of patterns) {
    const match = message.match(pattern)
    if (!match) continue
    const rawCode = (match.groups?.code || match[1] || match[0]).replace(/^["']|["']$/g, '')
    if (!rawCode) continue
    const numericCode = Number(rawCode)
    if (/^\d{3}$/.test(rawCode) && (numericCode < 100 || numericCode > 599)) continue
    return rawCode
  }
  return ''
}

const failedDeleteSeverityFor = (code: string): FailedDeleteSeverity => (
  code === '429' || code.toLowerCase() === 'rate_limit_exceeded' ? 'warning' : 'error'
)

const failedDeleteGroupLabelFor = (key: string) => {
  if (key === 'unknown') return t('admin.accounts.bulkTest.deleteFailedErrorGroupUnknown')
  if (key === 'timeout') return t('admin.accounts.bulkTest.deleteFailedErrorGroupTimeout')
  return t('admin.accounts.bulkTest.deleteFailedErrorGroupCode', { code: key })
}

const failedDeleteGroupClass = (severity: FailedDeleteSeverity) => [
  severity === 'warning'
    ? 'border-amber-200 dark:border-amber-900/60'
    : 'border-red-200 dark:border-red-900/60'
]

const failedDeleteGroupIconClass = (severity: FailedDeleteSeverity) => (
  severity === 'warning'
    ? 'text-amber-600 dark:text-amber-300'
    : 'text-red-600 dark:text-red-300'
)

const failedDeleteGroupBadgeClass = (severity: FailedDeleteSeverity) => (
  severity === 'warning'
    ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300'
    : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
)

const isFailedDeleteGroupExpanded = (key: string) => expandedFailedDeleteGroupKeys.value.includes(key)

const toggleFailedDeleteGroup = (key: string) => {
  expandedFailedDeleteGroupKeys.value = isFailedDeleteGroupExpanded(key)
    ? expandedFailedDeleteGroupKeys.value.filter(item => item !== key)
    : [...expandedFailedDeleteGroupKeys.value, key]
}

const appendEventLog = (item: TestItem, event: AccountTestEvent) => {
  if (event.type === 'test_start') {
    item.logs.push(event.model
      ? t('admin.accounts.bulkTest.logStartedWithModel', { model: event.model })
      : t('admin.accounts.bulkTest.logStarted'))
    return
  }
  if (event.type === 'content' && event.text) {
    item.logs.push(event.text)
    return
  }
  if (event.type === 'image') {
    item.logs.push(t('admin.accounts.bulkTest.logImageReceived'))
    return
  }
  if (event.type === 'test_complete') {
    item.logs.push(event.success
      ? t('admin.accounts.bulkTest.logCompleted')
      : event.error || t('admin.accounts.bulkTest.logFailed'))
    return
  }
  if (event.type === 'error') {
    item.logs.push(event.error || t('admin.accounts.bulkTest.logFailed'))
  }
}

const sleep = (ms: number) => new Promise<void>(resolve => window.setTimeout(resolve, ms))

type StartStaggerDelay = number | (() => number)

const randomBatchTestStartStaggerMs = () => (
  BATCH_TEST_START_STAGGER_MIN_MS +
  Math.floor(Math.random() * (BATCH_TEST_START_STAGGER_MAX_MS - BATCH_TEST_START_STAGGER_MIN_MS + 1))
)

const hasStartStagger = (delay: StartStaggerDelay) => (
  typeof delay === 'function' || delay > 0
)

const resolveStartStaggerMs = (delay: StartStaggerDelay) => (
  Math.max(0, typeof delay === 'function' ? delay() : delay)
)

const runLimited = async <T,>(
  items: T[],
  concurrency: number,
  worker: (item: T) => Promise<void>,
  shouldStop: () => boolean = () => cancellationRequested,
  startStaggerMs: StartStaggerDelay = 0
) => {
  let nextIndex = 0
  const workerCount = Math.min(concurrency, items.length)
  let nextStartAt = Date.now()
  let startGate = Promise.resolve()

  const waitForStartSlot = async () => {
    if (!hasStartStagger(startStaggerMs)) return

    const previousGate = startGate
    let releaseStartGate: (() => void) | undefined
    startGate = new Promise<void>((resolve) => {
      releaseStartGate = resolve
    })

    await previousGate

    try {
      const now = Date.now()
      const waitMs = Math.max(0, nextStartAt - now)
      nextStartAt = Math.max(now, nextStartAt) + resolveStartStaggerMs(startStaggerMs)
      if (waitMs > 0) {
        await sleep(waitMs)
      }
    } finally {
      releaseStartGate?.()
    }
  }

  const workers = Array.from({ length: workerCount }, async () => {
    while (!shouldStop()) {
      const itemIndex = nextIndex
      nextIndex += 1
      if (itemIndex >= items.length) return
      const item = items[itemIndex]
      await waitForStartSlot()
      if (shouldStop()) return
      await worker(item)
    }
  })
  await Promise.all(workers)
}

const runSingleTest = async (item: TestItem, mode: TestMode) => {
  item.status = 'running'
  item.logs = []
  item.error = ''
  const controller = new AbortController()
  let timeoutElapsed = false
  const timeoutId = window.setTimeout(() => {
    timeoutElapsed = true
    controller.abort()
  }, BATCH_TEST_TIMEOUT_MS)
  abortControllers.add(controller)
  try {
    const result = await runAccountConnectionTest({
      accountId: item.account.id,
      authToken: localStorage.getItem('auth_token'),
      modelId: item.selectedModelId || undefined,
      mode,
      signal: controller.signal,
      onEvent: (event) => appendEventLog(item, event)
    })
    if (result.success) {
      item.status = 'success'
    } else {
      item.status = 'failed'
      item.error = result.error || t('admin.accounts.bulkTest.logFailed')
    }
    markAccountRecentlyTested(item.account.id, mode, accountTestStateFingerprint(item.account))
    refreshRecentTestCandidates()
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      if (timeoutElapsed) {
        item.status = 'failed'
        item.error = t('admin.accounts.bulkTest.logTimeout', { seconds: BATCH_TEST_TIMEOUT_SECONDS })
        item.logs.push(item.error)
        markAccountRecentlyTested(item.account.id, mode, accountTestStateFingerprint(item.account))
        refreshRecentTestCandidates()
        return
      }
      item.status = 'pending'
      item.logs.push(t('admin.accounts.bulkTest.logCancelled'))
      return
    }
    item.status = 'failed'
    item.error = error instanceof Error && error.message ? error.message : t('admin.accounts.bulkTest.logFailed')
    item.logs.push(item.error)
    markAccountRecentlyTested(item.account.id, mode, accountTestStateFingerprint(item.account))
    refreshRecentTestCandidates()
  } finally {
    window.clearTimeout(timeoutId)
    abortControllers.delete(controller)
  }
}

const pruneFailedSelection = () => {
  const failedIds = new Set(failedItems.value.map(item => item.account.id))
  selectedFailedIds.value = selectedFailedIds.value.filter(id => failedIds.has(id))
}

const startBatchTest = async () => {
  if (running.value || deletingFailed.value || testItems.value.length === 0) return
  running.value = true
  emit('running-change', true)
  cancellationRequested = false
  selectedFailedIds.value = []
  showFailedOnly.value = false
  const mode = testMode.value
  const now = Date.now()
  const recentRecords = getRecentAccountTestRecords(now, true)
  refreshRecentTestCandidates(true, now)
  testItems.value.forEach((item) => {
    item.status = 'pending'
    item.logs = []
    item.error = ''
  })

  const itemsToTest = testItems.value.filter((item) => {
    if (!skipRecentlyTested.value || !isAccountRecentlyTested(item.account.id, mode, accountTestStateFingerprint(item.account), now, recentRecords)) return true
    item.status = 'skipped'
    item.logs = [t('admin.accounts.bulkTest.logSkippedRecent', { minutes: BATCH_TEST_RECENT_SKIP_WINDOW_MINUTES })]
    return false
  })

  try {
    if (itemsToTest.length > 0) {
      await runLimited(
        itemsToTest,
        BATCH_TEST_CONCURRENCY,
        item => runSingleTest(item, mode),
        () => cancellationRequested,
        randomBatchTestStartStaggerMs
      )
    }
  } finally {
    running.value = false
    abortControllers.clear()
    emit('running-change', false)
  }

  pruneFailedSelection()

  if (!cancellationRequested) {
    const failed = testItems.value.filter(item => item.status === 'failed')
    emit('completed', {
      success: summary.value.success,
      failed: failed.length,
      skipped: summary.value.skipped,
      failedIds: failed.map(item => item.account.id),
      errors: failed.map(item => `${item.account.name}: ${item.error || t('admin.accounts.bulkTest.logFailed')}`)
    })
  }
}

const toggleFailedSelection = (id: number, checked: boolean) => {
  const selected = new Set(selectedFailedIds.value)
  if (checked) {
    selected.add(id)
  } else {
    selected.delete(id)
  }
  selectedFailedIds.value = Array.from(selected)
}

const toggleVisibleFailedSelection = () => {
  if (allVisibleFailedSelected.value) {
    const visibleIds = new Set(visibleFailedItems.value.map(item => item.account.id))
    selectedFailedIds.value = selectedFailedIds.value.filter(id => !visibleIds.has(id))
    return
  }

  const selected = new Set(selectedFailedIds.value)
  visibleFailedItems.value.forEach(item => selected.add(item.account.id))
  selectedFailedIds.value = Array.from(selected)
}

const requestDeleteFailedAccounts = (ids: number[]) => {
  const uniqueIds = Array.from(new Set(ids)).filter(id => failedItems.value.some(item => item.account.id === id))
  if (uniqueIds.length === 0 || deletingFailed.value) return
  pendingFailedDeleteIds.value = uniqueIds
  expandedFailedDeleteGroupKeys.value = []
  failedDeleteProgress.value = { completed: 0, total: 0 }
  showFailedDeleteConfirm.value = true
}

const closeFailedDeleteConfirm = () => {
  if (deletingFailed.value) return
  showFailedDeleteConfirm.value = false
  pendingFailedDeleteIds.value = []
  expandedFailedDeleteGroupKeys.value = []
  failedDeleteProgress.value = { completed: 0, total: 0 }
}

const confirmDeleteFailedAccounts = async () => {
  const uniqueIds = [...pendingFailedDeleteIds.value]
  if (uniqueIds.length === 0 || deletingFailed.value) return

  deletingFailed.value = true
  failedDeleteProgress.value = { completed: 0, total: uniqueIds.length }
  const deletedIds: number[] = []
  let failed = 0

  try {
    const results = await Promise.allSettled(uniqueIds.map(async (id) => {
      try {
        await adminAPI.accounts.delete(id)
        return id
      } finally {
        failedDeleteProgress.value = {
          completed: Math.min(failedDeleteProgress.value.completed + 1, failedDeleteProgress.value.total),
          total: failedDeleteProgress.value.total
        }
      }
    }))
    results.forEach((result) => {
      if (result.status === 'fulfilled') {
        deletedIds.push(result.value)
      } else {
        failed += 1
      }
    })
  } finally {
    deletingFailed.value = false
  }

  if (deletedIds.length > 0) {
    const deletedSet = new Set(deletedIds)
    testItems.value = testItems.value.filter(item => !deletedSet.has(item.account.id))
    selectedFailedIds.value = selectedFailedIds.value.filter(id => !deletedSet.has(id))
    emit('deleted', { success: deletedIds.length, failed, deletedIds })
  }

  if (failed > 0) {
    appStore.showError(t('admin.accounts.bulkTest.deleteFailedPartial', {
      success: deletedIds.length,
      failed
    }))
  } else {
    appStore.showSuccess(t('admin.accounts.bulkTest.deleteFailedSuccess', { count: deletedIds.length }))
  }
  closeFailedDeleteConfirm()
}

const deleteSelectedFailedAccounts = () => {
  requestDeleteFailedAccounts(selectedFailedIds.value)
}

const deleteAllFailedAccounts = () => {
  requestDeleteFailedAccounts(failedItems.value.map(item => item.account.id))
}

const handleClose = () => {
  if (running.value) {
    cancelBatchTest()
    return
  }
  emit('close')
}
</script>
