export type LogLevelFilter = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'
export type LogLevelBucket = LogLevelFilter | 'OTHER'

export interface LogField {
  key: string
  value: unknown
  displayValue: string
  previewValue: string
  isComplex: boolean
  isMultiline: boolean
  isError: boolean
}

export interface NormalizedLogEntry {
  id: number
  rawLevel: string
  levelBucket: LogLevelBucket
  timestampMs: number | null
  shortTime: string
  dateText: string
  fullTime: string
  caller: string
  loggerName: string
  msg: string
  errorText: string
  stacktrace: string
  fields: LogField[]
  inlineFields: LogField[]
  overflowFieldCount: number
  searchText: string
  raw: Record<string, unknown>
}

export interface LogLevelCounts {
  total: number
  DEBUG: number
  INFO: number
  WARN: number
  ERROR: number
}

export interface HighlightSegment {
  text: string
  match: boolean
}

export const MAX_LOG_BUFFER = 500
const MAX_INLINE_FIELDS = 3
const MAX_PREVIEW_VALUE_LEN = 56

function pad2(num: number): string {
  return num < 10 ? `0${num}` : String(num)
}

function pad3(num: number): string {
  if (num < 10) {
    return `00${num}`
  }
  if (num < 100) {
    return `0${num}`
  }
  return String(num)
}

export function normalizeLevelBucket(level: unknown): {
  rawLevel: string
  levelBucket: LogLevelBucket
} {
  const raw = typeof level === 'string' ? level.trim().toUpperCase() : ''
  switch (raw) {
    case 'DEBUG':
      return { rawLevel: 'DEBUG', levelBucket: 'DEBUG' }
    case 'INFO':
      return { rawLevel: 'INFO', levelBucket: 'INFO' }
    case 'WARN':
    case 'WARNING':
      return { rawLevel: 'WARN', levelBucket: 'WARN' }
    case 'ERROR':
      return { rawLevel: 'ERROR', levelBucket: 'ERROR' }
    case 'DPANIC':
    case 'PANIC':
    case 'FATAL':
      return { rawLevel: raw, levelBucket: 'ERROR' }
    default:
      return { rawLevel: raw || 'LOG', levelBucket: 'OTHER' }
  }
}

export function parseLogTimestamp(rawTime: unknown): {
  timestampMs: number | null
  shortTime: string
  dateText: string
  fullTime: string
} {
  let ms: number | null = null

  if (typeof rawTime === 'number' && Number.isFinite(rawTime)) {
    ms = Math.abs(rawTime) < 1e11 ? rawTime * 1000 : rawTime
  } else if (typeof rawTime === 'string') {
    const trimmed = rawTime.trim()
    if (trimmed !== '') {
      if (/^-?\d+(\.\d+)?$/.test(trimmed)) {
        const num = Number(trimmed)
        if (Number.isFinite(num)) {
          ms = Math.abs(num) < 1e11 ? num * 1000 : num
        }
      } else {
        const parsed = Date.parse(trimmed)
        if (!Number.isNaN(parsed)) {
          ms = parsed
        }
      }
    }
  } else if (rawTime instanceof Date && !Number.isNaN(rawTime.getTime())) {
    ms = rawTime.getTime()
  }

  if (ms === null || !Number.isFinite(ms)) {
    const fallback = typeof rawTime === 'string' && rawTime.trim() ? rawTime.trim() : '--:--:--.---'
    return {
      timestampMs: null,
      shortTime: '--:--:--.---',
      dateText: '',
      fullTime: fallback,
    }
  }

  const d = new Date(ms)
  if (Number.isNaN(d.getTime())) {
    return {
      timestampMs: null,
      shortTime: '--:--:--.---',
      dateText: '',
      fullTime: '--:--:--.---',
    }
  }

  const dateText = `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
  const shortTime = `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}.${pad3(d.getMilliseconds())}`
  return {
    timestampMs: d.getTime(),
    shortTime,
    dateText,
    fullTime: `${dateText} ${shortTime}`,
  }
}

function formatFieldValue(value: unknown): {
  displayValue: string
  previewValue: string
  isComplex: boolean
  isMultiline: boolean
} {
  if (value === null) {
    return { displayValue: 'null', previewValue: 'null', isComplex: false, isMultiline: false }
  }
  if (value === undefined) {
    return {
      displayValue: 'undefined',
      previewValue: 'undefined',
      isComplex: false,
      isMultiline: false,
    }
  }
  if (typeof value === 'string') {
    const isMultiline = value.includes('\n')
    const collapsed = isMultiline ? value.replace(/\s+/g, ' ').trim() : value
    const previewValue =
      collapsed.length > MAX_PREVIEW_VALUE_LEN
        ? `${collapsed.slice(0, MAX_PREVIEW_VALUE_LEN - 1)}…`
        : collapsed
    return { displayValue: value, previewValue, isComplex: false, isMultiline }
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    const text = String(value)
    return { displayValue: text, previewValue: text, isComplex: false, isMultiline: false }
  }

  let pretty = ''
  let compact = ''
  try {
    pretty = JSON.stringify(value, null, 2)
    compact = JSON.stringify(value)
  } catch {
    pretty = String(value)
    compact = pretty
  }
  const previewValue =
    compact.length > MAX_PREVIEW_VALUE_LEN
      ? `${compact.slice(0, MAX_PREVIEW_VALUE_LEN - 1)}…`
      : compact
  return {
    displayValue: pretty,
    previewValue,
    isComplex: true,
    isMultiline: pretty.includes('\n'),
  }
}

export function normalizeLogEntry(
  rawEntry: Record<string, unknown>,
  id: number,
): NormalizedLogEntry {
  const safeRaw: Record<string, unknown> =
    rawEntry && typeof rawEntry === 'object' && !Array.isArray(rawEntry) ? { ...rawEntry } : {}
  delete safeRaw._k

  const { rawLevel, levelBucket } = normalizeLevelBucket(safeRaw.level)
  const { timestampMs, shortTime, dateText, fullTime } = parseLogTimestamp(safeRaw.time)
  const caller = typeof safeRaw.caller === 'string' ? safeRaw.caller.trim() : ''
  const loggerName = typeof safeRaw.logger === 'string' ? safeRaw.logger.trim() : ''
  const msg =
    typeof safeRaw.msg === 'string'
      ? safeRaw.msg
      : safeRaw.msg !== undefined && safeRaw.msg !== null
        ? String(safeRaw.msg)
        : ''
  const stacktrace = typeof safeRaw.stacktrace === 'string' ? safeRaw.stacktrace : ''

  let errorText = ''
  const regularFields: LogField[] = []
  let errorField: LogField | null = null

  for (const [key, value] of Object.entries(safeRaw)) {
    if (
      key === 'level' ||
      key === 'time' ||
      key === 'msg' ||
      key === 'caller' ||
      key === 'logger' ||
      key === 'stacktrace' ||
      key === '_k'
    ) {
      continue
    }
    const formatted = formatFieldValue(value)
    const isError = key === 'error' || key === 'err'
    const field: LogField = {
      key,
      value,
      displayValue: formatted.displayValue,
      previewValue: formatted.previewValue,
      isComplex: formatted.isComplex,
      isMultiline: formatted.isMultiline,
      isError,
    }
    if (isError && !errorField) {
      errorField = field
      errorText = formatted.displayValue
    } else {
      regularFields.push(field)
    }
  }

  const fields = errorField ? [errorField, ...regularFields] : regularFields
  const inlineFields = fields.slice(0, MAX_INLINE_FIELDS)
  const overflowFieldCount = Math.max(0, fields.length - inlineFields.length)

  const searchParts: string[] = [rawLevel, fullTime, caller, loggerName, msg]
  if (stacktrace) {
    searchParts.push(stacktrace)
  }
  for (const field of fields) {
    searchParts.push(`${field.key}=${field.displayValue}`)
  }

  return {
    id,
    rawLevel,
    levelBucket,
    timestampMs,
    shortTime,
    dateText,
    fullTime,
    caller,
    loggerName,
    msg,
    errorText,
    stacktrace,
    fields,
    inlineFields,
    overflowFieldCount,
    searchText: searchParts.join(' ').toLowerCase(),
    raw: safeRaw,
  }
}

export function appendLogEntries(
  existing: readonly NormalizedLogEntry[],
  incoming: readonly NormalizedLogEntry[],
  maxSize = MAX_LOG_BUFFER,
): NormalizedLogEntry[] {
  if (incoming.length === 0) {
    return existing.length > maxSize ? existing.slice(existing.length - maxSize) : [...existing]
  }
  const combined = existing.length === 0 ? [...incoming] : [...existing, ...incoming]
  if (combined.length > maxSize) {
    return combined.slice(combined.length - maxSize)
  }
  return combined
}

export function countLogsByLevel(entries: readonly NormalizedLogEntry[]): LogLevelCounts {
  const counts: LogLevelCounts = {
    total: entries.length,
    DEBUG: 0,
    INFO: 0,
    WARN: 0,
    ERROR: 0,
  }
  for (const entry of entries) {
    if (entry.levelBucket !== 'OTHER') {
      counts[entry.levelBucket]++
    }
  }
  return counts
}

export function parseSearchTerms(query: string): string[] {
  const trimmed = query.trim().toLowerCase()
  if (!trimmed) {
    return []
  }
  return trimmed.split(/\s+/).filter(Boolean)
}

export function filterLogs(
  entries: readonly NormalizedLogEntry[],
  options: {
    levels: ReadonlySet<LogLevelFilter>
    query: string
  },
): NormalizedLogEntry[] {
  const hasLevelFilter = options.levels.size > 0
  const terms = parseSearchTerms(options.query)
  const hasQuery = terms.length > 0

  if (!hasLevelFilter && !hasQuery) {
    return entries as NormalizedLogEntry[]
  }

  return entries.filter((entry) => {
    if (hasLevelFilter) {
      if (entry.levelBucket === 'OTHER' || !options.levels.has(entry.levelBucket)) {
        return false
      }
    }
    if (hasQuery) {
      for (const term of terms) {
        if (!entry.searchText.includes(term)) {
          return false
        }
      }
    }
    return true
  })
}

export function tokenizeHighlight(text: string, query: string): HighlightSegment[] {
  if (!text) {
    return []
  }
  const terms = parseSearchTerms(query)
  if (terms.length === 0) {
    return [{ text, match: false }]
  }

  const lower = text.toLowerCase()
  const ranges: Array<[number, number]> = []

  for (const term of terms) {
    let start = 0
    while (start < lower.length) {
      const idx = lower.indexOf(term, start)
      if (idx === -1) {
        break
      }
      ranges.push([idx, idx + term.length])
      start = idx + term.length
    }
  }

  if (ranges.length === 0) {
    return [{ text, match: false }]
  }

  ranges.sort((a, b) => a[0] - b[0] || b[1] - a[1])
  const merged: Array<[number, number]> = []
  for (const [start, end] of ranges) {
    const prev = merged[merged.length - 1]
    if (!prev || start > prev[1]) {
      merged.push([start, end])
    } else if (end > prev[1]) {
      prev[1] = end
    }
  }

  const segments: HighlightSegment[] = []
  let cursor = 0
  for (const [start, end] of merged) {
    if (start > cursor) {
      segments.push({ text: text.slice(cursor, start), match: false })
    }
    segments.push({ text: text.slice(start, end), match: true })
    cursor = end
  }
  if (cursor < text.length) {
    segments.push({ text: text.slice(cursor), match: false })
  }
  return segments
}

export function formatLogEntryPlain(entry: NormalizedLogEntry): string {
  const parts: string[] = [entry.fullTime, `[${entry.rawLevel}]`]
  if (entry.caller) {
    parts.push(`(${entry.caller})`)
  }
  if (entry.msg) {
    parts.push(entry.msg)
  }
  if (entry.fields.length > 0) {
    const serializedFields = entry.fields
      .map((field) => `${field.key}=${field.isComplex ? JSON.stringify(field.value) : field.displayValue}`)
      .join(' ')
    parts.push(serializedFields)
  }
  if (entry.stacktrace) {
    parts.push(`\n${entry.stacktrace}`)
  }
  return parts.join(' ')
}

export interface LogStreamBufferState {
  logs: readonly NormalizedLogEntry[]
  pendingLogs: readonly NormalizedLogEntry[]
  pendingReplaceOnResume: boolean
  expandedIds: ReadonlySet<number>
}

export interface LogStreamBatchResult {
  logs: NormalizedLogEntry[]
  pendingLogs: NormalizedLogEntry[]
  pendingReplaceOnResume: boolean
  expandedIds: Set<number>
  replacedVisible: boolean
}

export function pruneExpandedIds(
  expandedIds: ReadonlySet<number>,
  entries: readonly NormalizedLogEntry[],
): Set<number> {
  if (expandedIds.size === 0) {
    return new Set()
  }
  const retained = new Set<number>()
  for (const entry of entries) {
    if (expandedIds.has(entry.id)) {
      retained.add(entry.id)
    }
  }
  return retained
}

export function applyIncomingLogBatch(
  state: LogStreamBufferState,
  incoming: readonly NormalizedLogEntry[],
  options: {
    isPaused: boolean
    resetBatch: boolean
    maxSize?: number
  },
): LogStreamBatchResult {
  const maxSize = options.maxSize ?? MAX_LOG_BUFFER

  if (options.isPaused) {
    if (options.resetBatch) {
      return {
        logs: [...state.logs],
        pendingLogs: appendLogEntries([], incoming, maxSize),
        pendingReplaceOnResume: true,
        expandedIds: new Set(state.expandedIds),
        replacedVisible: false,
      }
    }
    return {
      logs: [...state.logs],
      pendingLogs: appendLogEntries(state.pendingLogs, incoming, maxSize),
      pendingReplaceOnResume: state.pendingReplaceOnResume,
      expandedIds: new Set(state.expandedIds),
      replacedVisible: false,
    }
  }

  if (options.resetBatch) {
    const nextLogs = appendLogEntries([], incoming, maxSize)
    return {
      logs: nextLogs,
      pendingLogs: [],
      pendingReplaceOnResume: false,
      expandedIds: new Set(),
      replacedVisible: true,
    }
  }

  const nextLogs = appendLogEntries(state.logs, incoming, maxSize)
  return {
    logs: nextLogs,
    pendingLogs: [...state.pendingLogs],
    pendingReplaceOnResume: false,
    expandedIds: pruneExpandedIds(state.expandedIds, nextLogs),
    replacedVisible: false,
  }
}

export function resumePausedLogs(
  state: LogStreamBufferState,
  maxSize = MAX_LOG_BUFFER,
): LogStreamBatchResult {
  if (state.pendingReplaceOnResume) {
    const nextLogs = appendLogEntries([], state.pendingLogs, maxSize)
    return {
      logs: nextLogs,
      pendingLogs: [],
      pendingReplaceOnResume: false,
      expandedIds: new Set(),
      replacedVisible: true,
    }
  }
  if (state.pendingLogs.length === 0) {
    return {
      logs: [...state.logs],
      pendingLogs: [],
      pendingReplaceOnResume: false,
      expandedIds: new Set(state.expandedIds),
      replacedVisible: false,
    }
  }
  const nextLogs = appendLogEntries(state.logs, state.pendingLogs, maxSize)
  return {
    logs: nextLogs,
    pendingLogs: [],
    pendingReplaceOnResume: false,
    expandedIds: pruneExpandedIds(state.expandedIds, nextLogs),
    replacedVisible: false,
  }
}

export function formatLogsForExport(
  entries: readonly NormalizedLogEntry[],
  format: 'log' | 'json',
): string {
  if (format === 'json') {
    return JSON.stringify(
      entries.map((entry) => ({
        time: entry.fullTime,
        level: entry.rawLevel,
        ...(entry.caller ? { caller: entry.caller } : {}),
        ...(entry.loggerName ? { logger: entry.loggerName } : {}),
        msg: entry.msg,
        ...Object.fromEntries(entry.fields.map((f) => [f.key, f.value])),
        ...(entry.stacktrace ? { stacktrace: entry.stacktrace } : {}),
      })),
      null,
      2,
    )
  }
  return entries.map(formatLogEntryPlain).join('\n')
}
