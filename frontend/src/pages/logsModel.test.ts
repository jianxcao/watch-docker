import { describe, expect, it } from 'vitest'
import {
  appendLogEntries,
  applyIncomingLogBatch,
  countLogsByLevel,
  filterLogs,
  formatLogEntryPlain,
  formatLogsForExport,
  normalizeLogEntry,
  parseLogTimestamp,
  pruneExpandedIds,
  resumePausedLogs,
  tokenizeHighlight,
  type LogLevelFilter,
} from './logsModel'

describe('logsModel', () => {
  it('normalizes zap JSON log entries with caller, error hoisting, and tabular timestamps', () => {
    const ts = Date.UTC(2026, 10, 28, 2, 15, 32, 418)
    const entry = normalizeLogEntry(
      {
        level: 'ERROR',
        time: ts,
        caller: 'api/image_router.go:49',
        msg: 'remove image',
        ref: 'sha256:49340b',
        error: 'conflict: image is referenced in multiple repositories',
        retries: 2,
        meta: { force: false },
        extraA: 'a',
        _k: 999,
      },
      1,
    )

    expect(entry.id).toBe(1)
    expect(entry.rawLevel).toBe('ERROR')
    expect(entry.levelBucket).toBe('ERROR')
    expect(entry.caller).toBe('api/image_router.go:49')
    expect(entry.errorText).toBe('conflict: image is referenced in multiple repositories')
    expect(entry.shortTime).toMatch(/^\d{2}:\d{2}:\d{2}\.418$/)
    expect(entry.fullTime).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.418$/)
    expect(entry.fields[0].key).toBe('error')
    expect(entry.fields[0].isError).toBe(true)
    expect(entry.fields).toHaveLength(5)
    expect(entry.inlineFields).toHaveLength(3)
    expect(entry.overflowFieldCount).toBe(2)
    expect(entry.raw._k).toBeUndefined()
  })

  it('normalizes WARNING and FATAL/PANIC levels into WARN and ERROR buckets', () => {
    const warn = normalizeLogEntry({ level: 'warning', time: 1764296227123, msg: 'slow' }, 1)
    const fatal = normalizeLogEntry({ level: 'fatal', time: 1764296227123, msg: 'boom' }, 2)
    const unknown = normalizeLogEntry({ level: 'trace', time: 1764296227123, msg: 'raw' }, 3)

    expect(warn.rawLevel).toBe('WARN')
    expect(warn.levelBucket).toBe('WARN')
    expect(fatal.rawLevel).toBe('FATAL')
    expect(fatal.levelBucket).toBe('ERROR')
    expect(unknown.rawLevel).toBe('TRACE')
    expect(unknown.levelBucket).toBe('OTHER')
  })

  it('parses epoch seconds, epoch millis, ISO strings, and invalid timestamps safely', () => {
    const fromSec = parseLogTimestamp(1764296227)
    const fromMs = parseLogTimestamp(1764296227123.45)
    const fromIso = parseLogTimestamp('2026-11-28T02:15:32.418Z')
    const invalid = parseLogTimestamp('not-a-date')

    expect(fromSec.timestampMs).toBe(1764296227000)
    expect(fromMs.shortTime).toMatch(/\.123$/)
    expect(fromIso.shortTime).toMatch(/\.418$/)
    expect(invalid.timestampMs).toBeNull()
    expect(invalid.shortTime).toBe('--:--:--.---')
    expect(invalid.fullTime).toBe('not-a-date')
  })

  it('filters by multiple severity buckets and multi-term search across message, caller, and fields', () => {
    const entries = [
      normalizeLogEntry(
        { level: 'INFO', time: 1764296227000, caller: 'scanner/scanner.go:143', msg: '批量扫描镜像', total: 5 },
        1,
      ),
      normalizeLogEntry(
        { level: 'WARN', time: 1764296228000, caller: 'registry/client.go:88', msg: '镜像源响应超时', mirror: 'daocloud' },
        2,
      ),
      normalizeLogEntry(
        { level: 'ERROR', time: 1764296229000, caller: 'api/router.go:49', msg: '删除镜像失败', error: 'conflict' },
        3,
      ),
      normalizeLogEntry(
        { level: 'DEBUG', time: 1764296230000, caller: 'wsstream/hub.go:12', msg: 'heartbeat' },
        4,
      ),
    ]

    expect(countLogsByLevel(entries)).toEqual({
      total: 4,
      DEBUG: 1,
      INFO: 1,
      WARN: 1,
      ERROR: 1,
    })

    const warnAndError = filterLogs(entries, {
      levels: new Set<LogLevelFilter>(['WARN', 'ERROR']),
      query: '',
    })
    expect(warnAndError.map((e) => e.id)).toEqual([2, 3])

    const byFieldAndCaller = filterLogs(entries, {
      levels: new Set(),
      query: 'registry daocloud',
    })
    expect(byFieldAndCaller.map((e) => e.id)).toEqual([2])

    const byErrorAndLevel = filterLogs(entries, {
      levels: new Set<LogLevelFilter>(['ERROR']),
      query: 'conflict',
    })
    expect(byErrorAndLevel.map((e) => e.id)).toEqual([3])
  })

  it('tokenizes overlapping case-insensitive highlight matches safely', () => {
    const segments = tokenizeHighlight('Docker Compose docker-cli', 'docker compose')
    expect(segments).toEqual([
      { text: 'Docker', match: true },
      { text: ' ', match: false },
      { text: 'Compose', match: true },
      { text: ' ', match: false },
      { text: 'docker', match: true },
      { text: '-cli', match: false },
    ])

    const overlapped = tokenizeHighlight('abcdef', 'abcd cdef')
    expect(overlapped).toEqual([{ text: 'abcdef', match: true }])
  })

  it('caps appended buffer size and exports clean plain text and JSON', () => {
    const first = [
      normalizeLogEntry({ level: 'INFO', time: 1764296227000, msg: 'a' }, 1),
      normalizeLogEntry({ level: 'INFO', time: 1764296228000, msg: 'b' }, 2),
    ]
    const next = [
      normalizeLogEntry({ level: 'WARN', time: 1764296229000, caller: 'app.go:9', msg: 'c', code: 503 }, 3),
    ]
    const capped = appendLogEntries(first, next, 2)
    expect(capped.map((e) => e.id)).toEqual([2, 3])

    const plain = formatLogEntryPlain(capped[1])
    expect(plain).toContain('[WARN] (app.go:9) c code=503')

    const exportedJson = JSON.parse(formatLogsForExport(capped, 'json'))
    expect(exportedJson).toHaveLength(2)
    expect(exportedJson[1]).toMatchObject({ level: 'WARN', caller: 'app.go:9', msg: 'c', code: 503 })
  })

  it('keeps frozen logs intact when SSE reconnects while paused and replaces cleanly on resume', () => {
    const initial = [
      normalizeLogEntry({ level: 'INFO', time: 1764296227000, msg: 'old-1' }, 1),
      normalizeLogEntry({ level: 'ERROR', time: 1764296228000, msg: 'old-err' }, 2),
    ]
    const expanded = new Set([2])

    // Reconnect snapshot arrives while paused: visible logs & expanded detail must stay frozen
    const reconnectBatch = [
      normalizeLogEntry({ level: 'INFO', time: 1764296227000, msg: 'old-1' }, 3),
      normalizeLogEntry({ level: 'ERROR', time: 1764296228000, msg: 'old-err' }, 4),
      normalizeLogEntry({ level: 'INFO', time: 1764296229000, msg: 'new-live' }, 5),
    ]
    const pausedAfterReconnect = applyIncomingLogBatch(
      {
        logs: initial,
        pendingLogs: [],
        pendingReplaceOnResume: false,
        expandedIds: expanded,
      },
      reconnectBatch,
      { isPaused: true, resetBatch: true, maxSize: 10 },
    )

    expect(pausedAfterReconnect.logs.map((e) => e.id)).toEqual([1, 2])
    expect(pausedAfterReconnect.expandedIds.has(2)).toBe(true)
    expect(pausedAfterReconnect.pendingReplaceOnResume).toBe(true)
    expect(pausedAfterReconnect.pendingLogs.map((e) => e.id)).toEqual([3, 4, 5])

    // Additional live log while still paused appends to pending snapshot
    const extraWhilePaused = applyIncomingLogBatch(
      pausedAfterReconnect,
      [normalizeLogEntry({ level: 'WARN', time: 1764296230000, msg: 'extra' }, 6)],
      { isPaused: true, resetBatch: false, maxSize: 10 },
    )
    expect(extraWhilePaused.pendingReplaceOnResume).toBe(true)
    expect(extraWhilePaused.pendingLogs.map((e) => e.id)).toEqual([3, 4, 5, 6])

    // Resuming replaces visible logs with the deduplicated snapshot (no duplicate old-1/old-err)
    const resumed = resumePausedLogs(extraWhilePaused, 10)
    expect(resumed.replacedVisible).toBe(true)
    expect(resumed.pendingReplaceOnResume).toBe(false)
    expect(resumed.pendingLogs).toHaveLength(0)
    expect(resumed.logs.map((e) => e.id)).toEqual([3, 4, 5, 6])
    expect(resumed.expandedIds.size).toBe(0)
  })

  it('prunes evicted IDs from expandedIds when log buffer rolls over', () => {
    const entries = [
      normalizeLogEntry({ level: 'INFO', time: 1764296227000, msg: 'a' }, 1),
      normalizeLogEntry({ level: 'INFO', time: 1764296228000, msg: 'b' }, 2),
    ]
    const afterRoll = applyIncomingLogBatch(
      {
        logs: entries,
        pendingLogs: [],
        pendingReplaceOnResume: false,
        expandedIds: new Set([1, 2]),
      },
      [normalizeLogEntry({ level: 'INFO', time: 1764296229000, msg: 'c' }, 3)],
      { isPaused: false, resetBatch: false, maxSize: 2 },
    )
    expect(afterRoll.logs.map((e) => e.id)).toEqual([2, 3])
    expect([...afterRoll.expandedIds]).toEqual([2])
    expect([...pruneExpandedIds(new Set([99, 2]), afterRoll.logs)]).toEqual([2])
  })
})
