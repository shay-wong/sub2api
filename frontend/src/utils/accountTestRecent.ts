export type AccountTestMode = 'default' | 'compact'

export interface RecentAccountTestRecord {
  accountUpdatedAt?: string | null
  testedAt: number
}

export const ACCOUNT_TEST_RECENT_SKIP_WINDOW_MINUTES = 10
export const ACCOUNT_TEST_RECENT_SKIP_WINDOW_MS = ACCOUNT_TEST_RECENT_SKIP_WINDOW_MINUTES * 60 * 1000
export const ACCOUNT_TEST_RECENT_STORAGE_KEY = 'sub2api:admin:account-test:recent-tests'

type RecentAccountTestRecords = Record<string, RecentAccountTestRecord>

export const accountTestRecentKeyFor = (accountId: number, mode: AccountTestMode) => `${accountId}:${mode}`

export const accountTestStateFingerprint = (account: { updated_at?: string | null }) => account.updated_at ?? null

const safeLocalStorage = () => {
  try {
    return globalThis.localStorage
  } catch {
    return null
  }
}

export const readRecentAccountTestRecords = (): RecentAccountTestRecords => {
  const storage = safeLocalStorage()
  if (!storage) return {}

  try {
    const raw = storage.getItem(ACCOUNT_TEST_RECENT_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as RecentAccountTestRecords
    if (!parsed || typeof parsed !== 'object') return {}
    return parsed
  } catch {
    return {}
  }
}

export const writeRecentAccountTestRecords = (records: RecentAccountTestRecords) => {
  const storage = safeLocalStorage()
  if (!storage) return

  try {
    storage.setItem(ACCOUNT_TEST_RECENT_STORAGE_KEY, JSON.stringify(records))
  } catch {
    // Ignore quota/privacy-mode failures; recent-test skipping is an optimization.
  }
}

export const pruneRecentAccountTestRecords = (records: RecentAccountTestRecords, now = Date.now()) => {
  const cutoff = now - ACCOUNT_TEST_RECENT_SKIP_WINDOW_MS
  return Object.fromEntries(
    Object.entries(records).filter(([, record]) => {
      const testedAt = Number(record?.testedAt)
      return Number.isFinite(testedAt) && testedAt >= cutoff
    })
  )
}

export const getRecentAccountTestRecords = (now = Date.now(), persistPrune = false) => {
  const records = pruneRecentAccountTestRecords(readRecentAccountTestRecords(), now)
  if (persistPrune) {
    writeRecentAccountTestRecords(records)
  }
  return records
}

export const isAccountRecentlyTested = (
  accountId: number,
  mode: AccountTestMode,
  accountUpdatedAt?: string | null,
  now = Date.now(),
  records = getRecentAccountTestRecords(now)
) => {
  const record = records[accountTestRecentKeyFor(accountId, mode)]
  const testedAt = Number(record?.testedAt)
  return (
    Number.isFinite(testedAt) &&
    record?.accountUpdatedAt === (accountUpdatedAt ?? null) &&
    now - testedAt < ACCOUNT_TEST_RECENT_SKIP_WINDOW_MS
  )
}

export const markAccountRecentlyTested = (
  accountId: number,
  mode: AccountTestMode,
  accountUpdatedAt?: string | null,
  now = Date.now()
) => {
  const records = getRecentAccountTestRecords(now)
  records[accountTestRecentKeyFor(accountId, mode)] = {
    accountUpdatedAt: accountUpdatedAt ?? null,
    testedAt: now
  }
  writeRecentAccountTestRecords(records)
}
