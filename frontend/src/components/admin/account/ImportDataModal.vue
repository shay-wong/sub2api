<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="import-data-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.dataImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.accounts.dataImportWarning') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.dataFormat') }}</label>
        <Select
          v-model="format"
          :options="formatOptions"
          class="w-full"
          data-testid="account-import-format-select"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.dataImportFile') }}</label>
        <div
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200">
              {{ fileName || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">JSON (.json)</div>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          multiple
          class="hidden"
          accept="application/json,.json"
          @change="handleFileChange"
        />
      </div>

      <div
        v-if="importProgress"
        class="space-y-3 rounded-lg border border-primary-100 bg-primary-50/70 p-3 dark:border-primary-900/60 dark:bg-primary-900/15"
        data-testid="account-import-progress"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ importProgressStatus }}
              </span>
              <span
                v-if="importProgressConcurrencyText"
                class="rounded-full bg-primary-100 px-2 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-900/50 dark:text-primary-200"
              >
                {{ importProgressConcurrencyText }}
              </span>
            </div>
            <div
              class="truncate text-xs text-gray-500 dark:text-dark-300"
              :title="importProgressFileText"
            >
              {{ importProgressFileText }}
            </div>
          </div>
          <div class="shrink-0 text-sm font-semibold text-primary-600 dark:text-primary-300">
            {{ importProgressPercent }}%
          </div>
        </div>
        <div
          class="h-2 overflow-hidden rounded-full bg-white/80 dark:bg-dark-700"
          role="progressbar"
          :aria-valuenow="importProgressPercent"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-label="importProgressStatus"
        >
          <div
            class="h-full rounded-full bg-primary-500 transition-all duration-300 ease-out dark:bg-primary-400"
            :style="{ width: `${importProgressPercent}%` }"
          ></div>
        </div>
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} — {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="import-data-form"
          :disabled="importing"
        >
          {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type {
  AdminAccountDataFormat,
  AdminCPADataAccount,
  AdminDataAccount,
  AdminDataImportResult,
  AdminDataPayload,
  AdminDataProxy
} from '@/types'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported', options?: { close?: boolean }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

// 控制浏览器侧导入并发，避免一次性打满后端写入路径。
const DATA_IMPORT_CONCURRENCY = 10
const DATA_IMPORT_ACCOUNT_CHUNK_SIZE = 25
const CPA_DATA_TYPE = 'cpa-auth-files'

const importing = ref(false)
const files = ref<File[]>([])
const result = ref<AdminDataImportResult | null>(null)
const format = ref<AdminAccountDataFormat | 'auto'>('auto')

type ImportProgressStage = 'reading' | 'importing'

interface ImportProgressState {
  stage: ImportProgressStage
  completed: number
  total: number
  activeFileNames: string[]
  concurrency: number
}

interface ParsedImportFile {
  fileName: string
  data: unknown
}

interface ImportPayloadTask {
  fileName: string
  data: unknown
  format?: AdminAccountDataFormat
  failureKind?: 'account' | 'proxy'
  failureCount?: number
}

interface PreparedImportPlan {
  proxyTask: ImportPayloadTask | null
  accountTasks: ImportPayloadTask[]
}

interface ChunkableCPADataPayload {
  type?: string | null
  accounts: AdminCPADataAccount[]
}

const importProgress = ref<ImportProgressState | null>(null)
const formatOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.dataFormatAuto') },
  { value: 'sub2api', label: t('admin.accounts.dataFormatSub2API') },
  { value: 'cpa', label: t('admin.accounts.dataFormatCPA') }
])

const fileInput = ref<HTMLInputElement | null>(null)
const fileName = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0].name
  return t('admin.accounts.dataImportSelectedFiles', { count: files.value.length })
})

const errorItems = computed(() => result.value?.errors || [])
const importProgressPercent = computed(() => {
  const progress = importProgress.value
  if (!progress || progress.total <= 0) return 0
  return Math.min(100, Math.round((progress.completed / progress.total) * 100))
})
const importProgressStatus = computed(() => {
  const progress = importProgress.value
  if (!progress) return ''
  if (progress.stage === 'reading') {
    return t('admin.accounts.dataImportProgressReading', {
      current: Math.min(progress.completed + 1, progress.total),
      total: progress.total
    })
  }

  return t('admin.accounts.dataImportProgressImporting', {
    completed: progress.completed,
    total: progress.total
  })
})
const importProgressConcurrencyText = computed(() => {
  const progress = importProgress.value
  if (!progress || progress.stage !== 'importing' || progress.concurrency <= 1) return ''
  return t('admin.accounts.dataImportProgressConcurrency', {
    count: progress.concurrency
  })
})
const formatActiveFileNames = (fileNames: string[]): string => {
  const visibleNames = fileNames.slice(0, 3).join(', ')
  if (fileNames.length <= 3) return visibleNames

  return t('admin.accounts.dataImportProgressMoreFiles', {
    files: visibleNames,
    count: fileNames.length - 3
  })
}
const importProgressFileText = computed(() => {
  const activeFileNames = importProgress.value?.activeFileNames || []
  if (activeFileNames.length > 0) {
    return t('admin.accounts.dataImportProgressCurrentFiles', {
      files: formatActiveFileNames(activeFileNames)
    })
  }

  return t('admin.accounts.dataImportProgressStarting')
})

const createEmptyImportResult = (): AdminDataImportResult => ({
  account_created: 0,
  account_failed: 0,
  proxy_created: 0,
  proxy_reused: 0,
  proxy_failed: 0
})

const addImportResult = (target: AdminDataImportResult, itemResult: AdminDataImportResult) => {
  target.account_created += itemResult.account_created
  target.account_failed += itemResult.account_failed
  target.proxy_created += itemResult.proxy_created
  target.proxy_reused += itemResult.proxy_reused
  target.proxy_failed += itemResult.proxy_failed
  if (itemResult.errors?.length) {
    target.errors = [...(target.errors || []), ...itemResult.errors]
  }
}

const prefixImportErrors = (
  itemResult: AdminDataImportResult,
  sourceFileName: string
): AdminDataImportResult => ({
  ...itemResult,
  errors: itemResult.errors?.map((item) => ({
    ...item,
    message: `${sourceFileName}: ${item.message}`
  }))
})

const markRequestFailed = (
  sourceFileName: string,
  error: unknown,
  kind: 'account' | 'proxy' = 'account',
  failureCount = 1
): AdminDataImportResult => {
  const res = createEmptyImportResult()
  const count = Math.max(1, failureCount)
  if (kind === 'proxy') {
    res.proxy_failed = count
  } else {
    res.account_failed = count
  }
  res.errors = [
    {
      kind,
      name: sourceFileName,
      message: getErrorMessage(error, t('admin.accounts.dataImportFailed'))
    }
  ]
  return res
}

const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === 'object' && value !== null
)

const isSub2APIDataPayload = (value: unknown): value is AdminDataPayload => (
  isRecord(value) && Array.isArray(value.proxies) && Array.isArray(value.accounts)
)

const isCPAAccountData = (value: unknown): value is AdminCPADataAccount => (
  isRecord(value) && typeof value.access_token === 'string'
)

const isCPAAccountArray = (value: unknown): value is AdminCPADataAccount[] => (
  Array.isArray(value) && value.length > 0 && value.every(isCPAAccountData)
)

const isSupportedCPADataType = (value: unknown): boolean => (
  value == null || (typeof value === 'string' && [CPA_DATA_TYPE, ''].includes(value.trim()))
)

const isCPADataCandidate = (value: unknown): boolean => (
  (
    Array.isArray(value) &&
    value.length > 0 &&
    isCPAAccountData(value[0])
  ) ||
  (
    isRecord(value) &&
    Array.isArray(value.accounts) &&
    !Array.isArray(value.proxies) &&
    (
      value.type === CPA_DATA_TYPE ||
      value.accounts.some(isCPAAccountData)
    )
  )
)

const isChunkableCPADataPayload = (value: unknown): value is ChunkableCPADataPayload => (
  isRecord(value) &&
  Array.isArray(value.accounts) &&
  !Array.isArray(value.proxies) &&
  isSupportedCPADataType(value.type) &&
  value.accounts.length > 0 &&
  value.accounts.every(isCPAAccountData)
)

const resolveParsedFormat = (data: unknown): AdminAccountDataFormat => {
  if (format.value !== 'auto') return format.value
  if (isCPAAccountData(data) || isCPAAccountArray(data) || isCPADataCandidate(data)) return 'cpa'
  return 'sub2api'
}

const resolveChunkableCPAAccounts = (data: unknown): AdminCPADataAccount[] | null => {
  if (isChunkableCPADataPayload(data)) return data.accounts
  if (isCPAAccountArray(data)) return data
  if (isCPAAccountData(data)) return [data]
  return null
}

const inferCPAFailureCount = (data: unknown): number => {
  if (Array.isArray(data)) return Math.max(1, data.length)
  if (isRecord(data) && Array.isArray(data.accounts)) return Math.max(1, data.accounts.length)
  return 1
}

const buildProxyKey = (proxy: AdminDataProxy): string => (
  [
    String(proxy.protocol || '').trim(),
    String(proxy.host || '').trim(),
    proxy.port,
    String(proxy.username || '').trim(),
    String(proxy.password || '').trim()
  ].join('|')
)

const normalizeProxyKey = (proxy: AdminDataProxy): string => (
  proxy.proxy_key || buildProxyKey(proxy)
)

const normalizeAccountProxyKeys = (
  accounts: AdminDataAccount[],
  proxyKeyAlias: Map<string, string>
): AdminDataAccount[] => (
  accounts.map((account) => {
    if (!account.proxy_key) return account
    const normalizedProxyKey = proxyKeyAlias.get(account.proxy_key)
    if (!normalizedProxyKey || normalizedProxyKey === account.proxy_key) return account
    return {
      ...account,
      proxy_key: normalizedProxyKey
    }
  })
)

const createSub2APIPayload = (
  proxies: AdminDataProxy[],
  accounts: AdminDataAccount[]
): AdminDataPayload => ({
  type: 'sub2api-data',
  version: 1,
  exported_at: new Date().toISOString(),
  proxies,
  accounts
})

const chunkArray = <T,>(items: T[], size: number): T[][] => {
  const chunks: T[][] = []
  for (let index = 0; index < items.length; index += size) {
    chunks.push(items.slice(index, index + size))
  }
  return chunks
}

const createAccountTasks = (
  accounts: AdminDataAccount[],
  sourceFileName: string
): ImportPayloadTask[] => {
  return chunkArray(accounts, DATA_IMPORT_ACCOUNT_CHUNK_SIZE).map((items, index, chunks) => ({
    fileName: chunks.length > 1 ? `${sourceFileName} #${index + 1}` : sourceFileName,
    data: createSub2APIPayload([], items),
    format: 'sub2api',
    failureCount: items.length
  }))
}

const createCPATasks = (
  accounts: AdminCPADataAccount[],
  sourceFileName: string
): ImportPayloadTask[] => {
  return chunkArray(accounts, DATA_IMPORT_ACCOUNT_CHUNK_SIZE).map((items, index, chunks) => ({
    fileName: chunks.length > 1 ? `${sourceFileName} #${index + 1}` : sourceFileName,
    data: {
      type: 'cpa-auth-files',
      exported_at: new Date().toISOString(),
      accounts: items
    },
    format: 'cpa',
    failureCount: items.length
  }))
}

const prepareImportPlan = (dataPayloads: ParsedImportFile[]): PreparedImportPlan => {
  const proxyByKey = new Map<string, AdminDataProxy>()
  const proxyKeyAlias = new Map<string, string>()
  const accountTasks: ImportPayloadTask[] = []
  const resolvedPayloads = dataPayloads.map((payload) => ({
    ...payload,
    payloadFormat: resolveParsedFormat(payload.data)
  }))

  for (const payload of resolvedPayloads) {
    if (payload.payloadFormat !== 'sub2api' || !isSub2APIDataPayload(payload.data)) {
      continue
    }

    for (const proxy of payload.data.proxies) {
      const sourceKey = normalizeProxyKey(proxy)
      const canonicalKey = buildProxyKey(proxy)
      proxyKeyAlias.set(sourceKey, canonicalKey)
      proxyKeyAlias.set(canonicalKey, canonicalKey)
      if (!proxyByKey.has(canonicalKey)) {
        proxyByKey.set(canonicalKey, {
          ...proxy,
          proxy_key: canonicalKey
        })
      }
    }
  }

  for (const payload of resolvedPayloads) {
    if (payload.payloadFormat === 'cpa') {
      const cpaAccounts = resolveChunkableCPAAccounts(payload.data)
      if (cpaAccounts) {
        accountTasks.push(...createCPATasks(cpaAccounts, payload.fileName))
      } else {
        accountTasks.push({
          fileName: payload.fileName,
          data: payload.data,
          format: 'cpa',
          failureCount: inferCPAFailureCount(payload.data)
        })
      }
      continue
    }

    if (!isSub2APIDataPayload(payload.data)) {
      accountTasks.push({
        fileName: payload.fileName,
        data: payload.data,
        format: 'sub2api'
      })
      continue
    }

    accountTasks.push(...createAccountTasks(
      normalizeAccountProxyKeys(payload.data.accounts, proxyKeyAlias),
      payload.fileName
    ))
  }

  const proxies = Array.from(proxyByKey.values())
  return {
    proxyTask: proxies.length > 0
      ? {
          fileName: t('admin.accounts.dataImportProxyPreparation'),
          data: createSub2APIPayload(proxies, []),
          format: 'sub2api',
          failureKind: 'proxy',
          failureCount: proxies.length
        }
      : null,
    accountTasks
  }
}

const hasImportErrors = (res: AdminDataImportResult): boolean => (
  res.account_failed > 0 || res.proxy_failed > 0 || Boolean(res.errors?.length)
)

const hasImportSuccess = (res: AdminDataImportResult): boolean => (
  res.account_created > 0 || res.proxy_created > 0 || res.proxy_reused > 0
)

const importOnePayload = async (
  payload: ImportPayloadTask
): Promise<AdminDataImportResult> => {
  try {
    const itemResult = await adminAPI.accounts.importData({
      data: payload.data,
      format: payload.format,
      skip_default_group_bind: true
    })
    return prefixImportErrors(itemResult, payload.fileName)
  } catch (error: unknown) {
    return markRequestFailed(payload.fileName, error, payload.failureKind, payload.failureCount)
  }
}

const importPayloadsConcurrently = async (
  dataPayloads: ImportPayloadTask[]
): Promise<AdminDataImportResult> => {
  const res = createEmptyImportResult()
  if (dataPayloads.length === 0) {
    return res
  }

  const total = dataPayloads.length
  const concurrency = Math.min(DATA_IMPORT_CONCURRENCY, total)
  const activeFileNames = new Set<string>()
  let completed = 0
  let nextIndex = 0

  const updateProgress = () => {
    importProgress.value = {
      stage: 'importing',
      completed,
      total,
      activeFileNames: Array.from(activeFileNames),
      concurrency
    }
  }

  updateProgress()

  const runWorker = async () => {
    while (nextIndex < dataPayloads.length) {
      const payload = dataPayloads[nextIndex]
      nextIndex += 1
      activeFileNames.add(payload.fileName)
      updateProgress()

      const itemResult = await importOnePayload(payload)
      addImportResult(res, itemResult)

      activeFileNames.delete(payload.fileName)
      completed += 1
      updateProgress()
    }
  }

  await Promise.all(Array.from({ length: concurrency }, () => runWorker()))
  return res
}

watch(
  () => props.show,
  (open) => {
    if (open) {
      files.value = []
      result.value = null
      importProgress.value = null
      format.value = 'auto'
      if (fileInput.value) {
        fileInput.value.value = ''
      }
    }
  }
)

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  files.value = Array.from(target.files || [])
  result.value = null
  importProgress.value = null
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }

  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }

  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const getErrorMessage = (error: unknown, fallback: string): string => {
  return error instanceof Error && error.message ? error.message : fallback
}

const handleImport = async () => {
  if (files.value.length === 0) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  result.value = null
  importProgress.value = null
  try {
    const dataPayloads: ParsedImportFile[] = []
    for (const [index, sourceFile] of files.value.entries()) {
      importProgress.value = {
        stage: 'reading',
        completed: index,
        total: files.value.length,
        activeFileNames: [sourceFile.name],
        concurrency: 1
      }
      const text = await readFileAsText(sourceFile)
      try {
        dataPayloads.push({
          fileName: sourceFile.name,
          data: JSON.parse(text)
        })
      } catch (error) {
        if (error instanceof SyntaxError) {
          appStore.showError(`${sourceFile.name}: ${t('admin.accounts.dataImportParseFailed')}`)
          return
        }
        throw error
      }
    }

    const importPlan = prepareImportPlan(dataPayloads)
    const res = createEmptyImportResult()
    if (importPlan.proxyTask) {
      importProgress.value = {
        stage: 'importing',
        completed: 0,
        total: 1,
        activeFileNames: [importPlan.proxyTask.fileName],
        concurrency: 1
      }
      addImportResult(res, await importOnePayload(importPlan.proxyTask))
    }
    addImportResult(res, await importPayloadsConcurrently(importPlan.accountTasks))

    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
    }
    if (hasImportErrors(res)) {
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
      if (hasImportSuccess(res)) {
        emit('imported', { close: false })
      }
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: unknown) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(getErrorMessage(error, t('admin.accounts.dataImportFailed')))
    }
  } finally {
    importing.value = false
    importProgress.value = null
  }
}
</script>
