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
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
          :class="isDraggingFiles
            ? 'border-primary-400 bg-primary-50 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          data-testid="account-import-drop-zone"
          @dragenter.prevent="handleFileDragEnter"
          @dragover.prevent="handleFileDragOver"
          @dragleave.prevent="handleFileDragLeave"
          @drop.prevent="handleFileDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200">
              {{ fileName || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.dataImportDropHint') }}
            </div>
          </div>
          <div class="flex shrink-0 flex-wrap justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="importing"
              @click="openFilePicker"
            >
              {{ t('common.chooseFile') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="importing"
              @click="openDirectoryPicker"
            >
              {{ t('admin.accounts.dataImportChooseFolder') }}
            </button>
          </div>
        </div>
        <div
          v-if="files.length > 0"
          class="mt-3 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40"
          data-testid="account-import-selected-files"
        >
          <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-3 py-2 dark:border-dark-700">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">
              {{ t('admin.accounts.dataImportSelectedFiles', { count: files.length }) }}
            </span>
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-dark-100"
              :disabled="importing"
              data-testid="account-import-clear-files"
              @click="clearSelectedFiles"
            >
              <Icon name="x" size="xs" :stroke-width="2" />
              {{ t('admin.accounts.dataImportClearFiles') }}
            </button>
          </div>
          <ul class="max-h-40 divide-y divide-gray-100 overflow-auto dark:divide-dark-700">
            <li
              v-for="(file, index) in files"
              :key="getImportFileKey(file, index)"
              class="flex min-w-0 items-center justify-between gap-3 px-3 py-2"
            >
              <span
                class="truncate text-sm text-gray-700 dark:text-dark-200"
                :title="getImportFileDisplayName(file)"
              >
                {{ getImportFileDisplayName(file) }}
              </span>
              <button
                type="button"
                class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-900/20 dark:hover:text-red-300"
                :disabled="importing"
                :aria-label="t('admin.accounts.dataImportRemoveFile', { name: getImportFileDisplayName(file) })"
                data-testid="account-import-remove-file"
                @click="removeSelectedFile(index)"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>
            </li>
          </ul>
        </div>
        <input
          ref="fileInput"
          type="file"
          multiple
          class="hidden"
          accept="application/json,text/plain,.json,.txt"
          data-testid="account-import-file-input"
          @change="handleFileChange"
        />
        <input
          ref="directoryInput"
          type="file"
          multiple
          webkitdirectory
          class="hidden"
          accept="application/json,text/plain,.json,.txt"
          data-testid="account-import-directory-input"
          @change="handleDirectoryChange"
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
import { computed, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type {
  AdminAccountDataFormat,
  AdminCPADataAccount,
  AdminDataAccount,
  AdminDataImportResult,
  AdminDataPayload,
  AdminDataProxy,
  CodexSessionImportResult
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
const TXT_RT_SEPARATOR = '----'
const TXT_RT_EXPIRED_AT = '1970-01-01T00:00:00Z'
const TXT_RT_ACCOUNT_NAME_PREFIX = 'openai-rt'
const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

type AccountImportFormat =
  | AdminAccountDataFormat
  | 'auto'
  | 'txt-refresh-token'
  | 'txt-mobile-refresh-token'
  | 'codex-session'

const importing = ref(false)
const files = shallowRef<File[]>([])
const result = ref<AccountImportResult | null>(null)
const format = ref<AccountImportFormat>('auto')
const fileDragDepth = ref(0)

interface AccountImportResult extends AdminDataImportResult {
  account_updated: number
  account_skipped: number
}

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
  format?: AdminAccountDataFormat
}

interface ImportProgressTask {
  fileName: string
  failureKind?: 'account' | 'proxy'
  failureCount?: number
  progressCount?: number
}

interface ImportPayloadTask extends ImportProgressTask {
  data: unknown
  format?: AdminAccountDataFormat
}

interface CodexImportPayloadTask extends ImportProgressTask {
  content: string
}

interface PreparedImportPlan {
  proxyTask: ImportPayloadTask | null
  accountTasks: ImportPayloadTask[]
}

interface ChunkableCPADataPayload {
  type?: string | null
  accounts: AdminCPADataAccount[]
}

interface DroppedFileSystemEntry {
  isFile: boolean
  isDirectory: boolean
  name: string
  fullPath?: string
}

interface DroppedFileSystemFileEntry extends DroppedFileSystemEntry {
  isFile: true
  file: (successCallback: (file: File) => void, errorCallback?: (error: unknown) => void) => void
}

interface DroppedFileSystemDirectoryEntry extends DroppedFileSystemEntry {
  isDirectory: true
  createReader: () => {
    readEntries: (
      successCallback: (entries: DroppedFileSystemEntry[]) => void,
      errorCallback?: (error: unknown) => void
    ) => void
  }
}

const importProgress = ref<ImportProgressState | null>(null)
const formatOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.dataFormatAuto') },
  { value: 'sub2api', label: t('admin.accounts.dataFormatSub2API') },
  { value: 'cpa', label: t('admin.accounts.dataFormatCPA') },
  { value: 'txt-refresh-token', label: t('admin.accounts.dataFormatOpenAIRTTxt') },
  { value: 'txt-mobile-refresh-token', label: t('admin.accounts.dataFormatOpenAIMobileRTTxt') },
  { value: 'codex-session', label: t('admin.accounts.dataFormatCodexSession') }
])

const fileInput = ref<HTMLInputElement | null>(null)
const directoryInput = ref<HTMLInputElement | null>(null)
const filePathOverrides = new WeakMap<File, string>()
const IMPORT_FILE_EXTENSIONS = ['.json', '.txt']
const isDraggingFiles = computed(() => fileDragDepth.value > 0)
const fileName = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return getImportFileDisplayName(files.value[0])
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

const getNativeImportFilePath = (file: File): string => {
  const path = (file as File & { webkitRelativePath?: string }).webkitRelativePath
  return typeof path === 'string' ? path : ''
}

const getImportFileDisplayName = (file: File): string => (
  filePathOverrides.get(file) || getNativeImportFilePath(file) || file.name
)

const getImportFileKey = (file: File, index: number): string => (
  `${getImportFileDisplayName(file)}-${file.size}-${file.lastModified}-${index}`
)

const isSupportedImportFile = (file: File): boolean => {
  const name = getImportFileDisplayName(file).toLowerCase()
  return IMPORT_FILE_EXTENSIONS.some((extension) => name.endsWith(extension))
}

const resetFileInputs = () => {
  if (fileInput.value) {
    fileInput.value.value = ''
  }
  if (directoryInput.value) {
    directoryInput.value.value = ''
  }
}

const createEmptyImportResult = (): AccountImportResult => ({
  account_created: 0,
  account_updated: 0,
  account_skipped: 0,
  account_failed: 0,
  proxy_created: 0,
  proxy_reused: 0,
  proxy_failed: 0
})

const addImportResult = (target: AccountImportResult, itemResult: AdminDataImportResult) => {
  target.account_created += itemResult.account_created
  target.account_updated += (itemResult as Partial<AccountImportResult>).account_updated || 0
  target.account_skipped += (itemResult as Partial<AccountImportResult>).account_skipped || 0
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

const getImportTaskProgressCount = (payload: ImportProgressTask): number => (
  Math.max(1, payload.progressCount ?? payload.failureCount ?? 1)
)

const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === 'object' && value !== null
)

const isSub2APIDataPayload = (value: unknown): value is AdminDataPayload => (
  isRecord(value) && Array.isArray(value.proxies) && Array.isArray(value.accounts)
)

const hasOwnRecordKey = (value: Record<string, unknown>, key: string): boolean => (
  Object.prototype.hasOwnProperty.call(value, key)
)

const isCPAAccountData = (value: unknown): value is AdminCPADataAccount => (
  isRecord(value) &&
  typeof value.access_token === 'string' &&
  !hasOwnRecordKey(value, 'credentials') &&
  !hasOwnRecordKey(value, 'platform')
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

const isAutoCPADataCandidate = (value: unknown): boolean => (
  isRecord(value) &&
  value.type === CPA_DATA_TYPE &&
  Array.isArray(value.accounts) &&
  !Array.isArray(value.proxies)
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
  if (format.value === 'sub2api' || format.value === 'cpa') return format.value
  if (isAutoCPADataCandidate(data)) return 'cpa'
  return 'sub2api'
}

const getRecordStringValue = (value: Record<string, unknown>, key: string): string => (
  hasOwnRecordKey(value, key) && typeof value[key] === 'string' ? String(value[key]).trim() : ''
)

const getNestedRecordStringValue = (
  value: Record<string, unknown>,
  parentKey: string,
  childKey: string
): string => {
  const nested = value[parentKey]
  if (!isRecord(nested)) return ''
  return getRecordStringValue(nested, childKey)
}

const isCodexSessionJSONCandidate = (value: unknown): boolean => {
  if (isCPAAccountData(value) || isCPADataCandidate(value)) {
    return false
  }
  if (Array.isArray(value)) {
    return value.length > 0 && value.some(isCodexSessionJSONCandidate)
  }
  if (!isRecord(value) || isSub2APIDataPayload(value)) {
    return false
  }

  return Boolean(
    getNestedRecordStringValue(value, 'tokens', 'access_token') ||
    getNestedRecordStringValue(value, 'tokens', 'accessToken') ||
    getRecordStringValue(value, 'accessToken')
  )
}

const estimateCodexImportCount = (value: unknown): number => {
  if (Array.isArray(value)) {
    return value.reduce((sum, item) => sum + estimateCodexImportCount(item), 0)
  }
  return 1
}

const estimateCodexContentProgressCount = (content: string): number => {
  const trimmed = content.replace(/^\uFEFF/, '').trim()
  if (!trimmed) return 1

  try {
    return Math.max(1, estimateCodexImportCount(JSON.parse(trimmed)))
  } catch {
    return Math.max(1, getTxtRefreshTokenImportLines(trimmed).length)
  }
}

const createCodexImportTasks = (
  content: string,
  sourceFileName: string
): CodexImportPayloadTask[] => {
  const trimmed = content.replace(/^\uFEFF/, '').trim()
  if (!trimmed) {
    throw new SyntaxError('Empty Codex import content')
  }

  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      return chunkArray(parsed, DATA_IMPORT_ACCOUNT_CHUNK_SIZE).map((items, index, chunks) => ({
        fileName: chunks.length > 1 ? `${sourceFileName} #${index + 1}` : sourceFileName,
        content: JSON.stringify(items),
        failureCount: Math.max(1, estimateCodexImportCount(items)),
        progressCount: Math.max(1, estimateCodexImportCount(items))
      }))
    }
  } catch {
    const lines = getTxtRefreshTokenImportLines(trimmed)
    if (lines.length > 1) {
      return chunkArray(lines, DATA_IMPORT_ACCOUNT_CHUNK_SIZE).map((items, index, chunks) => ({
        fileName: chunks.length > 1 ? `${sourceFileName} #${index + 1}` : sourceFileName,
        content: items.join('\n'),
        failureCount: items.length,
        progressCount: items.length
      }))
    }
  }

  return [{
    fileName: sourceFileName,
    content: trimmed,
    failureCount: estimateCodexContentProgressCount(trimmed),
    progressCount: estimateCodexContentProgressCount(trimmed)
  }]
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

const getTxtRefreshTokenImportLines = (content: string): string[] => (
  content
    .replace(/^\uFEFF/, '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
)

const looksLikeTxtRefreshTokenImport = (content: string): boolean => (
  getTxtRefreshTokenImportLines(content)
    .some((line) => line.includes(TXT_RT_SEPARATOR))
)

const normalizeTxtTokenMarker = (
  marker: string
): 'access_token' | 'refresh_token' | 'mobile_refresh_token' | null => {
  const normalizedMarker = marker.trim().toLowerCase()
  if (['at', 'access_token'].includes(normalizedMarker)) return 'access_token'
  if (['rt', 'refresh_token'].includes(normalizedMarker)) return 'refresh_token'
  if (['mobile_rt', 'mobile_refresh_token'].includes(normalizedMarker)) return 'mobile_refresh_token'
  return null
}

const looksLikeEmail = (value: string): boolean => (
  /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim())
)

const buildTxtImportAccountName = (email: string | undefined, index: number): string => (
  email || `${TXT_RT_ACCOUNT_NAME_PREFIX}-${index + 1}`
)

const parseTxtRefreshTokenImport = (content: string, options?: { mobile?: boolean }): AdminDataPayload => {
  const lines = getTxtRefreshTokenImportLines(content)
  const accounts = lines.map((line, index): AdminDataAccount => {
    const parts = line.includes(TXT_RT_SEPARATOR)
      ? line.split(TXT_RT_SEPARATOR).map((part) => part.trim())
      : [options?.mobile ? 'mobile_rt' : 'rt', line.trim()]
    if (parts.length < 2) {
      throw new SyntaxError(`Invalid TXT refresh token import line ${index + 1}`)
    }

    const tokenCredentials: Record<string, string> = {}
    const identityParts: string[] = []
    for (let partIndex = 0; partIndex < parts.length; partIndex += 1) {
      const credentialKey = normalizeTxtTokenMarker(parts[partIndex])
      if (!credentialKey) {
        identityParts.push(parts[partIndex])
        continue
      }

      const credentialValue = parts[partIndex + 1]
      if (!credentialValue) {
        throw new SyntaxError(`Invalid TXT refresh token import line ${index + 1}`)
      }
      if (credentialKey === 'mobile_refresh_token') {
        tokenCredentials.refresh_token = credentialValue
        tokenCredentials.client_id = OPENAI_MOBILE_RT_CLIENT_ID
      } else {
        tokenCredentials[credentialKey] = credentialValue
      }
      partIndex += 1
    }

    const emailParts = identityParts.filter(looksLikeEmail)
    if (emailParts.length > 1 || !tokenCredentials.refresh_token) {
      throw new SyntaxError(`Invalid TXT refresh token import line ${index + 1}`)
    }
    const email = emailParts[0]
    const accountName = buildTxtImportAccountName(email, index)

    return {
      name: accountName,
      platform: 'openai',
      type: 'oauth',
      credentials: {
        ...tokenCredentials,
        ...(email ? { email } : {}),
        expires_at: TXT_RT_EXPIRED_AT
      },
      extra: {
        ...(email ? { email } : {}),
        import_source: 'txt_refresh_token'
      },
      concurrency: 10,
      priority: 1,
      rate_multiplier: 1,
      auto_pause_on_expired: true
    }
  })

  if (accounts.length === 0) {
    throw new SyntaxError('Empty TXT refresh token import')
  }

  return createSub2APIPayload([], accounts)
}

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
    failureCount: items.length,
    progressCount: items.length
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
    failureCount: items.length,
    progressCount: items.length
  }))
}

const prepareImportPlan = (dataPayloads: ParsedImportFile[]): PreparedImportPlan => {
  const proxyByKey = new Map<string, AdminDataProxy>()
  const proxyKeyAlias = new Map<string, string>()
  const accountTasks: ImportPayloadTask[] = []
  const resolvedPayloads = dataPayloads.map((payload) => ({
    ...payload,
    payloadFormat: payload.format || resolveParsedFormat(payload.data)
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
        const accountCount = inferCPAFailureCount(payload.data)
        accountTasks.push({
          fileName: payload.fileName,
          data: payload.data,
          format: 'cpa',
          failureCount: accountCount,
          progressCount: accountCount
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
          failureCount: proxies.length,
          progressCount: proxies.length
        }
      : null,
    accountTasks
  }
}

const hasImportErrors = (res: AdminDataImportResult): boolean => (
  res.account_failed > 0 || res.proxy_failed > 0 || Boolean(res.errors?.length)
)

const hasImportSuccess = (res: AdminDataImportResult): boolean => (
  res.account_created > 0 ||
  ((res as Partial<AccountImportResult>).account_updated || 0) > 0 ||
  res.proxy_created > 0 ||
  res.proxy_reused > 0
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

const mapCodexImportResult = (
  itemResult: CodexSessionImportResult,
  sourceFileName: string
): AccountImportResult => {
  const res = createEmptyImportResult()
  res.account_created = itemResult.created
  res.account_updated = itemResult.updated
  res.account_skipped = itemResult.skipped
  res.account_failed = itemResult.failed

  const mappedErrors = (itemResult.errors || []).map((item) => ({
    kind: 'account' as const,
    name: item.name || sourceFileName,
    message: `${sourceFileName}: #${item.index} ${item.message}`
  }))
  const fallbackErrors = mappedErrors.length > 0
    ? []
    : (itemResult.items || [])
        .filter((item) => item.action === 'failed' && item.message)
        .map((item) => ({
          kind: 'account' as const,
          name: item.name || sourceFileName,
          message: `${sourceFileName}: #${item.index} ${item.message}`
        }))
  const errors = [...mappedErrors, ...fallbackErrors]
  if (errors.length > 0) {
    res.errors = errors
  }
  return res
}

const importOneCodexPayload = async (
  payload: CodexImportPayloadTask
): Promise<AdminDataImportResult> => {
  try {
    const itemResult = await adminAPI.accounts.importCodexSession({
      content: payload.content,
      update_existing: true,
      skip_default_group_bind: true
    })
    return mapCodexImportResult(itemResult, payload.fileName)
  } catch (error: unknown) {
    return markRequestFailed(payload.fileName, error, payload.failureKind, payload.failureCount)
  }
}

interface QueuedImportTask extends ImportProgressTask {
  run: () => Promise<AdminDataImportResult>
}

const importTasksConcurrently = async (
  tasks: QueuedImportTask[]
): Promise<AccountImportResult> => {
  const res = createEmptyImportResult()
  if (tasks.length === 0) {
    return res
  }

  const total = tasks.reduce((sum, payload) => sum + getImportTaskProgressCount(payload), 0)
  const concurrency = Math.min(DATA_IMPORT_CONCURRENCY, tasks.length)
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
    while (nextIndex < tasks.length) {
      const payload = tasks[nextIndex]
      nextIndex += 1
      activeFileNames.add(payload.fileName)
      updateProgress()

      const itemResult = await payload.run()
      addImportResult(res, itemResult)

      activeFileNames.delete(payload.fileName)
      completed += getImportTaskProgressCount(payload)
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
      resetFileInputs()
    }
  }
)

const openFilePicker = () => {
  if (importing.value) return
  fileInput.value?.click()
}

const openDirectoryPicker = () => {
  if (importing.value) return
  directoryInput.value?.click()
}

const setSelectedFiles = (selectedFiles: File[]) => {
  if (importing.value) return
  const picked = selectedFiles.filter(isSupportedImportFile)
  if (picked.length === 0) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }
  if (picked.length < selectedFiles.length) {
    appStore.showWarning(t('admin.accounts.dataImportIgnoredFiles', { count: selectedFiles.length - picked.length }))
  }
  files.value = picked
  result.value = null
  importProgress.value = null
}

const clearSelectedFiles = () => {
  if (importing.value) return
  files.value = []
  result.value = null
  importProgress.value = null
  resetFileInputs()
}

const removeSelectedFile = (index: number) => {
  if (importing.value) return
  files.value = files.value.filter((_, fileIndex) => fileIndex !== index)
  result.value = null
  importProgress.value = null
  resetFileInputs()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(Array.from(target.files || []))
}

const handleDirectoryChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(Array.from(target.files || []))
}

const handleFileDragEnter = () => {
  if (importing.value) return
  fileDragDepth.value += 1
}

const handleFileDragOver = (event: DragEvent) => {
  if (importing.value) return
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy'
  }
}

const handleFileDragLeave = () => {
  if (importing.value) return
  fileDragDepth.value = Math.max(0, fileDragDepth.value - 1)
}

const readFileEntry = (entry: DroppedFileSystemFileEntry): Promise<File> => (
  new Promise((resolve, reject) => {
    entry.file((file) => {
      const fullPath = entry.fullPath?.replace(/^\/+/, '')
      if (fullPath) {
        filePathOverrides.set(file, fullPath)
      }
      resolve(file)
    }, reject)
  })
)

const readDirectoryEntries = (entry: DroppedFileSystemDirectoryEntry): Promise<DroppedFileSystemEntry[]> => (
  new Promise((resolve, reject) => {
    const reader = entry.createReader()
    const entries: DroppedFileSystemEntry[] = []
    const readBatch = () => {
      reader.readEntries((batch) => {
        if (batch.length === 0) {
          resolve(entries)
          return
        }
        entries.push(...batch)
        readBatch()
      }, reject)
    }
    readBatch()
  })
)

const readEntryFiles = async (entry: DroppedFileSystemEntry): Promise<File[]> => {
  if (entry.isFile) {
    return [await readFileEntry(entry as DroppedFileSystemFileEntry)]
  }
  if (!entry.isDirectory) return []

  const children = await readDirectoryEntries(entry as DroppedFileSystemDirectoryEntry)
  const nestedFiles = await Promise.all(children.map(readEntryFiles))
  return nestedFiles.flat()
}

const isDroppedFileSystemEntry = (value: unknown): value is DroppedFileSystemEntry => (
  isRecord(value) &&
  typeof value.name === 'string' &&
  typeof value.isFile === 'boolean' &&
  typeof value.isDirectory === 'boolean'
)

const getDroppedFiles = async (event: DragEvent): Promise<File[]> => {
  const items = Array.from(event.dataTransfer?.items || [])
  const entries = items
    .map((item) => {
      const getAsEntry = (item as unknown as { webkitGetAsEntry?: () => unknown }).webkitGetAsEntry
      return typeof getAsEntry === 'function' ? getAsEntry.call(item) : undefined
    })
    .filter(isDroppedFileSystemEntry)

  if (entries.length > 0) {
    const entryFiles = await Promise.all(entries.map(readEntryFiles))
    return entryFiles.flat()
  }

  return Array.from(event.dataTransfer?.files || [])
}

const handleFileDrop = async (event: DragEvent) => {
  fileDragDepth.value = 0
  if (importing.value) return
  try {
    const droppedFiles = await getDroppedFiles(event)
    if (droppedFiles.length > 0) {
      setSelectedFiles(droppedFiles)
      resetFileInputs()
    }
  } catch (error: unknown) {
    appStore.showError(getErrorMessage(error, t('admin.accounts.dataImportFailed')))
  }
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

const pushParsedImportFile = (
  sourceFileName: string,
  text: string,
  dataPayloads: ParsedImportFile[],
  codexTasks: CodexImportPayloadTask[]
) => {
  if (format.value === 'txt-refresh-token' || format.value === 'txt-mobile-refresh-token') {
    dataPayloads.push({
      fileName: sourceFileName,
      data: parseTxtRefreshTokenImport(text, {
        mobile: format.value === 'txt-mobile-refresh-token'
      }),
      format: 'sub2api'
    })
    return
  }

  if (format.value === 'codex-session') {
    codexTasks.push(...createCodexImportTasks(text, sourceFileName))
    return
  }

  try {
    const parsed = JSON.parse(text)
    if (format.value === 'auto' && isCodexSessionJSONCandidate(parsed)) {
      codexTasks.push(...createCodexImportTasks(text, sourceFileName))
      return
    }

    dataPayloads.push({
      fileName: sourceFileName,
      data: parsed,
      format: format.value === 'sub2api' || format.value === 'cpa' ? format.value : undefined
    })
  } catch (error) {
    if (!(error instanceof SyntaxError)) {
      throw error
    }

    if (
      format.value === 'auto' &&
      looksLikeTxtRefreshTokenImport(text)
    ) {
      dataPayloads.push({
        fileName: sourceFileName,
        data: parseTxtRefreshTokenImport(text),
        format: 'sub2api'
      })
      return
    }

    throw new SyntaxError()
  }
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
    const codexTasks: CodexImportPayloadTask[] = []
    for (const [index, sourceFile] of files.value.entries()) {
      const sourceFileName = getImportFileDisplayName(sourceFile)
      importProgress.value = {
        stage: 'reading',
        completed: index,
        total: files.value.length,
        activeFileNames: [sourceFileName],
        concurrency: 1
      }
      const text = await readFileAsText(sourceFile)
      try {
        pushParsedImportFile(sourceFileName, text, dataPayloads, codexTasks)
      } catch (error) {
        if (error instanceof SyntaxError) {
          const message = error.message
          appStore.showError(`${sourceFileName}: ${
            message ? message : t('admin.accounts.dataImportParseFailed')
          }`)
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
        total: getImportTaskProgressCount(importPlan.proxyTask),
        activeFileNames: [importPlan.proxyTask.fileName],
        concurrency: 1
      }
      addImportResult(res, await importOnePayload(importPlan.proxyTask))
    }
    const accountImportTasks: QueuedImportTask[] = [
      ...importPlan.accountTasks.map((payload) => ({
        ...payload,
        run: () => importOnePayload(payload)
      })),
      ...codexTasks.map((payload) => ({
        ...payload,
        run: () => importOneCodexPayload(payload)
      }))
    ]
    addImportResult(res, await importTasksConcurrently(accountImportTasks))

    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_updated: res.account_updated,
      account_skipped: res.account_skipped,
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
