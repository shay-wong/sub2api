<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkTest.title')"
    width="wide"
    :close-on-click-outside="false"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
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

          <div data-test="batch-test-summary" class="text-sm text-gray-600 dark:text-dark-300">
            {{ t('admin.accounts.bulkTest.summary', summary) }}
          </div>
        </div>

        <div class="grid grid-cols-3 gap-2">
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
        </div>
      </div>

      <div class="max-h-[480px] overflow-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40">
        <div
          v-for="item in testItems"
          :key="item.account.id"
          class="border-b border-gray-100 p-3 last:border-b-0 dark:border-dark-700"
        >
          <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(240px,320px)_auto] lg:items-start">
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
          @click="handleClose"
        >
          {{ running ? t('common.cancel') : t('common.close') }}
        </button>
        <button
          type="button"
          data-test="batch-test-start"
          class="btn btn-primary"
          :disabled="running || testItems.length === 0"
          @click="startBatchTest"
        >
          {{ running ? t('admin.accounts.bulkTest.running') : t('admin.accounts.bulkTest.start') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { adminAPI } from '@/api/admin'
import { runAccountConnectionTest, type AccountTestEvent } from '@/utils/accountTestRunner'
import type { Account, ClaudeModel } from '@/types'

type TestStatus = 'pending' | 'running' | 'success' | 'failed'
type TestMode = 'default' | 'compact'

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

const DEFAULT_MODEL_OPTION_VALUE = ''

const props = defineProps<{
  show: boolean
  accounts: Account[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'running-change', running: boolean): void
  (e: 'completed', payload: { success: number; failed: number; failedIds: number[]; errors: string[] }): void
}>()

const { t } = useI18n()
const testMode = ref<TestMode>('default')
const running = ref(false)
const testItems = ref<TestItem[]>([])
let abortController: AbortController | null = null
let cancellationRequested = false

const resetItems = () => {
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

const loadAllItemModels = async () => {
  await Promise.all(testItems.value.map(loadItemModels))
}

const abortCurrentTest = () => {
  abortController?.abort()
  abortController = null
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
      loadAllItemModels()
    } else {
      cancelBatchTest()
    }
  },
  { immediate: true }
)

const summary = computed(() => ({
  total: testItems.value.length,
  success: testItems.value.filter(item => item.status === 'success').length,
  failed: testItems.value.filter(item => item.status === 'failed').length,
  running: testItems.value.filter(item => item.status === 'running').length,
  pending: testItems.value.filter(item => item.status === 'pending').length
}))

const statusLabel = (status: TestStatus) => t(`admin.accounts.bulkTest.status.${status}`)

const statusClass = (status: TestStatus) => [
  'shrink-0 rounded-full px-2 py-1 text-xs font-medium',
  status === 'success' && 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300',
  status === 'failed' && 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300',
  status === 'running' && 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-300',
  status === 'pending' && 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
]

const modeButtonClass = (mode: TestMode) => [
  'rounded-md px-3 py-2 text-sm font-medium transition-colors',
  testMode.value === mode
    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
    : 'text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white',
  running.value && 'cursor-not-allowed opacity-70'
]

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

const startBatchTest = async () => {
  if (running.value || testItems.value.length === 0) return
  running.value = true
  emit('running-change', true)
  cancellationRequested = false

  try {
    for (const item of testItems.value) {
      item.status = 'running'
      item.logs = []
      item.error = ''
      abortController = new AbortController()
      try {
        const result = await runAccountConnectionTest({
          accountId: item.account.id,
          authToken: localStorage.getItem('auth_token'),
          modelId: item.selectedModelId || undefined,
          mode: testMode.value,
          signal: abortController.signal,
          onEvent: (event) => appendEventLog(item, event)
        })
        if (result.success) {
          item.status = 'success'
        } else {
          item.status = 'failed'
          item.error = result.error || t('admin.accounts.bulkTest.logFailed')
        }
      } catch (error: any) {
        if (error instanceof DOMException && error.name === 'AbortError') {
          item.status = 'pending'
          item.logs.push(t('admin.accounts.bulkTest.logCancelled'))
          break
        }
        item.status = 'failed'
        item.error = error?.message || t('admin.accounts.bulkTest.logFailed')
        item.logs.push(item.error)
      } finally {
        abortController = null
      }
    }
  } finally {
    running.value = false
    emit('running-change', false)
  }

  if (!cancellationRequested) {
    const failed = testItems.value.filter(item => item.status === 'failed')
    emit('completed', {
      success: summary.value.success,
      failed: failed.length,
      failedIds: failed.map(item => item.account.id),
      errors: failed.map(item => `${item.account.name}: ${item.error || t('admin.accounts.bulkTest.logFailed')}`)
    })
  }
}

const handleClose = () => {
  if (running.value) {
    cancelBatchTest()
    return
  }
  emit('close')
}
</script>
