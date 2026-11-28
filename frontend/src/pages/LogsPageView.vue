<template>
  <div
    class="logs-page"
    :data-theme="settingStore.setting.theme"
    :style="{ height: containerHeight }"
  >
    <div class="logs-shell">
      <!-- 筛选与工具栏 -->
      <div class="logs-toolbar" role="search" aria-label="日志筛选工具栏">
        <div class="toolbar-search">
          <n-input
            ref="searchInputRef"
            v-model:value="searchQuery"
            size="small"
            clearable
            placeholder="搜索消息、模块或字段..."
            aria-label="搜索日志"
          >
            <template #prefix>
              <n-icon :size="15" class="toolbar-search-icon">
                <SearchOutline />
              </n-icon>
            </template>
            <template v-if="!searchQuery && !isMobile" #suffix>
              <kbd class="kbd-hint" aria-hidden="true">/</kbd>
            </template>
          </n-input>
        </div>

        <div class="toolbar-filters" role="group" aria-label="按日志级别筛选">
          <button
            type="button"
            class="level-filter-chip"
            :class="{ 'is-active': selectedLevels.size === 0 }"
            :aria-pressed="selectedLevels.size === 0"
            @click="clearLevelFilters"
          >
            <span class="chip-label">全部</span>
            <span class="chip-count">{{ levelCounts.total }}</span>
          </button>

          <button
            v-for="item in levelFilterItems"
            :key="item.key"
            type="button"
            class="level-filter-chip"
            :class="[
              `chip-${item.key.toLowerCase()}`,
              {
                'is-active': selectedLevels.has(item.key),
                'is-empty': levelCounts[item.key] === 0 && !selectedLevels.has(item.key),
              },
            ]"
            :aria-pressed="selectedLevels.has(item.key)"
            @click="toggleLevelFilter(item.key)"
          >
            <span class="chip-dot" aria-hidden="true"></span>
            <span class="chip-label">{{ item.label }}</span>
            <span class="chip-count">{{ levelCounts[item.key] }}</span>
          </button>
        </div>

        <div v-if="!isMobile || hasActiveFilters" class="toolbar-controls">
          <button
            v-if="!isMobile"
            type="button"
            class="control-chip"
            :class="{ 'is-active': wrapLines }"
            :aria-pressed="wrapLines"
            title="切换自动换行 / 单行紧凑显示"
            @click="wrapLines = !wrapLines"
          >
            <n-icon :size="14">
              <ReturnDownBackOutline />
            </n-icon>
            <span>{{ wrapLines ? '自动换行' : '单行模式' }}</span>
          </button>

          <button
            v-if="hasActiveFilters"
            type="button"
            class="control-chip control-chip--reset"
            @click="resetAllFilters"
          >
            <n-icon :size="14">
              <CloseOutline />
            </n-icon>
            <span>重置</span>
          </button>
        </div>
      </div>

      <!-- 异常断连提示条 -->
      <div v-if="connectionError" class="logs-banner logs-banner--error" role="alert">
        <div class="banner-left">
          <n-icon :size="16">
            <AlertCircleOutline />
          </n-icon>
          <span>{{ connectionError }}</span>
        </div>
        <button type="button" class="banner-action" @click="handleReconnect">
          <n-icon :size="14">
            <RefreshOutline />
          </n-icon>
          <span>立即重连</span>
        </button>
      </div>

      <!-- 暂停缓冲提示条 -->
      <div v-else-if="isPaused" class="logs-banner logs-banner--paused" role="status">
        <div class="banner-left">
          <n-icon :size="15">
            <PauseOutline />
          </n-icon>
          <span v-if="pendingCount > 0">
            实时接收已暂停，新到达 <strong>{{ pendingCount }}</strong> 条日志暂存在缓冲区
          </span>
          <span v-else>实时接收已暂停，当前视图保持静止便于排查</span>
        </div>
        <button type="button" class="banner-action banner-action--resume" @click="togglePause">
          <n-icon :size="14">
            <PlayOutline />
          </n-icon>
          <span>{{ pendingCount > 0 ? '加载并恢复' : '恢复实时接收' }}</span>
        </button>
      </div>

      <!-- 桌面端列表头 -->
      <div v-if="!isMobile" class="logs-table-head" aria-hidden="true">
        <span class="col-chevron"></span>
        <span class="col-time">时间</span>
        <span class="col-level">级别</span>
        <span class="col-caller">模块 / 位置</span>
        <span class="col-message">消息与结构化字段</span>
        <span class="col-actions"></span>
      </div>

      <!-- 主日志流区域 -->
      <div class="logs-body">
        <div v-if="isInitialConnecting" class="logs-state">
          <n-spin size="small" />
          <p class="state-title">正在连接系统日志流...</p>
          <p class="state-desc">建立连接后将自动同步最近 {{ MAX_LOG_BUFFER }} 条服务端记录</p>
        </div>

        <div v-else-if="logs.length === 0" class="logs-state">
          <div class="state-icon">
            <n-icon :size="24">
              <DocumentTextOutline />
            </n-icon>
          </div>
          <p class="state-title">暂无系统日志</p>
          <p class="state-desc">后端产生新的运行日志时将在此实时呈现</p>
        </div>

        <div v-else-if="filteredLogs.length === 0" class="logs-state">
          <div class="state-icon">
            <n-icon :size="24">
              <SearchOutline />
            </n-icon>
          </div>
          <p class="state-title">没有匹配的日志记录</p>
          <p class="state-desc">
            当前已缓存 {{ logs.length }} 条日志，尝试调整级别或搜索关键词
          </p>
          <n-button size="small" secondary type="primary" class="mt-2" @click="resetAllFilters">
            清除筛选条件
          </n-button>
        </div>

        <n-virtual-list
          v-else
          ref="virtualListRef"
          class="virtual-log-list"
          :items="filteredLogs"
          :item-size="estimatedItemSize"
          key-field="id"
          item-resizable
          @scroll="onLogScroll"
        >
          <template #default="{ item: log }">
            <div
              class="log-row"
              :class="[
                `log-row--${log.levelBucket.toLowerCase()}`,
                {
                  'is-expanded': expandedIds.has(log.id),
                  'is-nowrap': !wrapLines,
                  'is-mobile': isMobile,
                },
              ]"
            >
              <!-- 摘要行 -->
              <div
                class="log-row-summary"
                role="button"
                tabindex="0"
                :aria-expanded="expandedIds.has(log.id)"
                @click="onSummaryClick(log.id, $event)"
                @keydown.enter.prevent="toggleExpand(log.id)"
                @keydown.space.prevent="toggleExpand(log.id)"
              >
                <template v-if="!isMobile">
                  <span class="log-chevron" aria-hidden="true">
                    <n-icon :size="13">
                      <ChevronForwardOutline />
                    </n-icon>
                  </span>

                  <span class="log-time" :title="log.fullTime">{{ log.shortTime }}</span>

                  <span class="log-level-badge" :class="`level-${log.levelBucket.toLowerCase()}`">
                    {{ log.rawLevel }}
                  </span>

                  <span
                    class="log-caller"
                    :class="{ 'is-empty': !log.caller }"
                    :title="log.caller || undefined"
                  >
                    <template v-if="log.caller">
                      <template
                        v-for="(seg, idx) in getHighlighted(log.caller)"
                        :key="`c-${log.id}-${idx}`"
                      >
                        <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                        <template v-else>{{ seg.text }}</template>
                      </template>
                    </template>
                    <template v-else>—</template>
                  </span>

                  <div class="log-content">
                    <span class="log-msg">
                      <template
                        v-for="(seg, idx) in getHighlighted(log.msg || '(空消息)')"
                        :key="`m-${log.id}-${idx}`"
                      >
                        <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                        <template v-else>{{ seg.text }}</template>
                      </template>
                    </span>

                    <div v-if="log.inlineFields.length > 0" class="log-inline-fields">
                      <span
                        v-for="field in log.inlineFields"
                        :key="field.key"
                        class="field-chip"
                        :class="{ 'field-chip--error': field.isError }"
                        :title="`${field.key}: ${field.displayValue}`"
                      >
                        <span class="field-chip-key">{{ field.key }}</span>
                        <span class="field-chip-sep">=</span>
                        <span class="field-chip-val">
                          <template
                            v-for="(seg, idx) in getHighlighted(field.previewValue)"
                            :key="`f-${log.id}-${field.key}-${idx}`"
                          >
                            <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                            <template v-else>{{ seg.text }}</template>
                          </template>
                        </span>
                      </span>
                      <span v-if="log.overflowFieldCount > 0" class="field-chip field-chip--more">
                        +{{ log.overflowFieldCount }} 字段
                      </span>
                    </div>
                  </div>

                  <div class="log-row-actions">
                    <button
                      type="button"
                      class="row-icon-btn"
                      :class="{ 'is-copied': copiedRowId === log.id }"
                      :title="copiedRowId === log.id ? '已复制' : '复制此行日志'"
                      :aria-label="`复制第 ${log.id} 条日志`"
                      @click.stop="copyLogPlain(log)"
                    >
                      <n-icon :size="14">
                        <CheckmarkOutline v-if="copiedRowId === log.id" />
                        <CopyOutline v-else />
                      </n-icon>
                    </button>
                  </div>
                </template>

                <!-- 移动端双行紧凑卡片布局 -->
                <template v-else>
                  <div class="mobile-row-meta">
                    <span class="log-level-badge" :class="`level-${log.levelBucket.toLowerCase()}`">
                      {{ log.rawLevel }}
                    </span>
                    <span class="log-time" :title="log.fullTime">{{ log.shortTime }}</span>
                    <span v-if="log.caller" class="log-caller" :title="log.caller">
                      <template
                        v-for="(seg, idx) in getHighlighted(log.caller)"
                        :key="`mc-${log.id}-${idx}`"
                      >
                        <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                        <template v-else>{{ seg.text }}</template>
                      </template>
                    </span>
                    <span class="log-chevron ml-auto" aria-hidden="true">
                      <n-icon :size="13">
                        <ChevronForwardOutline />
                      </n-icon>
                    </span>
                  </div>

                  <div class="mobile-row-body">
                    <span class="log-msg">
                      <template
                        v-for="(seg, idx) in getHighlighted(log.msg || '(空消息)')"
                        :key="`mm-${log.id}-${idx}`"
                      >
                        <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                        <template v-else>{{ seg.text }}</template>
                      </template>
                    </span>

                    <div v-if="log.inlineFields.length > 0" class="log-inline-fields">
                      <span
                        v-for="field in log.inlineFields"
                        :key="field.key"
                        class="field-chip"
                        :class="{ 'field-chip--error': field.isError }"
                      >
                        <span class="field-chip-key">{{ field.key }}</span>
                        <span class="field-chip-sep">=</span>
                        <span class="field-chip-val">
                          <template
                            v-for="(seg, idx) in getHighlighted(field.previewValue)"
                            :key="`mf-${log.id}-${field.key}-${idx}`"
                          >
                            <mark v-if="seg.match" class="log-mark">{{ seg.text }}</mark>
                            <template v-else>{{ seg.text }}</template>
                          </template>
                        </span>
                      </span>
                      <span v-if="log.overflowFieldCount > 0" class="field-chip field-chip--more">
                        +{{ log.overflowFieldCount }}
                      </span>
                    </div>
                  </div>
                </template>
              </div>

              <!-- 展开详情面板 -->
              <div v-if="expandedIds.has(log.id)" class="log-detail">
                <div class="detail-meta-bar">
                  <div class="detail-meta-tags">
                    <span class="detail-meta-item">
                      <span class="meta-label">完整时间</span>
                      <code class="meta-value">{{ log.fullTime }}</code>
                    </span>
                    <span v-if="log.caller" class="detail-meta-item">
                      <span class="meta-label">调用位置</span>
                      <code class="meta-value">{{ log.caller }}</code>
                    </span>
                    <span v-if="log.loggerName" class="detail-meta-item">
                      <span class="meta-label">Logger</span>
                      <code class="meta-value">{{ log.loggerName }}</code>
                    </span>
                  </div>

                  <div class="detail-actions">
                    <button
                      v-if="log.caller"
                      type="button"
                      class="detail-action-btn"
                      @click.stop="filterByCaller(log.caller)"
                    >
                      <n-icon :size="13">
                        <SearchOutline />
                      </n-icon>
                      <span>筛选此模块</span>
                    </button>
                    <button
                      type="button"
                      class="detail-action-btn"
                      @click.stop="copyLogPlain(log)"
                    >
                      <n-icon :size="13">
                        <CopyOutline />
                      </n-icon>
                      <span>复制文本</span>
                    </button>
                    <button
                      type="button"
                      class="detail-action-btn"
                      @click.stop="copyLogJson(log)"
                    >
                      <n-icon :size="13">
                        <CodeSlashOutline />
                      </n-icon>
                      <span>复制 JSON</span>
                    </button>
                  </div>
                </div>

                <!-- 错误详情突出区 -->
                <div v-if="log.errorText" class="detail-error-box">
                  <div class="detail-section-header">
                    <span class="detail-section-title detail-section-title--error">错误信息</span>
                    <button
                      type="button"
                      class="detail-mini-copy"
                      @click.stop="copyText(log.errorText, '已复制错误信息')"
                    >
                      复制
                    </button>
                  </div>
                  <pre class="detail-error-text">{{ log.errorText }}</pre>
                </div>

                <!-- 结构化字段表 -->
                <div v-if="detailRegularFields(log).length > 0" class="detail-fields">
                  <div class="detail-section-header">
                    <span class="detail-section-title">结构化字段</span>
                  </div>
                  <dl class="detail-kv-list">
                    <div
                      v-for="field in detailRegularFields(log)"
                      :key="field.key"
                      class="detail-kv-row"
                    >
                      <dt class="detail-kv-key">{{ field.key }}</dt>
                      <dd class="detail-kv-val">
                        <pre v-if="field.isComplex || field.isMultiline" class="detail-kv-pre">{{
                          field.displayValue
                        }}</pre>
                        <code v-else class="detail-kv-inline">{{ field.displayValue }}</code>
                        <button
                          type="button"
                          class="detail-mini-copy"
                          :aria-label="`复制字段 ${field.key}`"
                          @click.stop="copyText(field.displayValue, `已复制 ${field.key}`)"
                        >
                          复制
                        </button>
                      </dd>
                    </div>
                  </dl>
                </div>

                <!-- 堆栈信息 -->
                <div v-if="log.stacktrace" class="detail-stacktrace-box">
                  <div class="detail-section-header">
                    <span class="detail-section-title">堆栈追踪</span>
                    <button
                      type="button"
                      class="detail-mini-copy"
                      @click.stop="copyText(log.stacktrace, '已复制堆栈追踪')"
                    >
                      复制
                    </button>
                  </div>
                  <pre class="detail-stacktrace">{{ log.stacktrace }}</pre>
                </div>
              </div>
            </div>
          </template>
        </n-virtual-list>

        <!-- 回到底部悬浮按钮 -->
        <Transition name="pill-fade">
          <button
            v-if="!followTail && filteredLogs.length > 0"
            type="button"
            class="scroll-bottom-pill"
            @click="jumpToBottom"
          >
            <n-icon :size="14">
              <ArrowDownOutline />
            </n-icon>
            <span v-if="unreadWhileScrolled > 0">
              {{ unreadWhileScrolled }} 条新日志 · 回到底部
            </span>
            <span v-else>回到底部</span>
          </button>
        </Transition>
      </div>
    </div>

    <!-- 顶栏 Teleport -->
    <Teleport to="#header" defer>
      <div class="logs-header" :data-theme="settingStore.setting.theme">
        <div class="logs-header-left">
          <n-h2 class="m-0 text-lg">日志</n-h2>

          <span class="stream-status-pill" :class="`status-${headerStatusVariant}`" role="status">
            <span class="status-dot" aria-hidden="true"></span>
            <span>{{ headerStatusLabel }}</span>
          </span>

          <span class="tabular-count max-md:hidden">
            <template v-if="hasActiveFilters">
              显示 {{ filteredLogs.length }} / {{ logs.length }} 条
            </template>
            <template v-else> 共 {{ logs.length }} 条 </template>
          </span>
        </div>

        <div class="logs-header-actions">
          <n-tooltip trigger="hover" :delay="400">
            <template #trigger>
              <n-button
                circle
                size="small"
                secondary
                :type="isPaused ? 'primary' : 'default'"
                :aria-label="isPaused ? '恢复实时日志流' : '暂停实时日志流'"
                @click="togglePause"
              >
                <template #icon>
                  <n-icon>
                    <PlayOutline v-if="isPaused" />
                    <PauseOutline v-else />
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ isPaused ? '恢复实时接收' : '暂停实时接收' }}
          </n-tooltip>

          <n-tooltip v-if="streamStatus !== 'connected'" trigger="hover" :delay="400">
            <template #trigger>
              <n-button
                circle
                size="small"
                secondary
                :loading="streamStatus === 'connecting'"
                aria-label="重新连接日志流"
                @click="handleReconnect"
              >
                <template #icon>
                  <n-icon>
                    <RefreshOutline />
                  </n-icon>
                </template>
              </n-button>
            </template>
            重新连接日志流
          </n-tooltip>

          <n-dropdown
            trigger="click"
            :options="exportMenuOptions"
            @select="handleExportSelect"
          >
            <n-button
              circle
              size="small"
              secondary
              :disabled="filteredLogs.length === 0"
              aria-label="导出日志"
            >
              <template #icon>
                <n-icon>
                  <DownloadOutline />
                </n-icon>
              </template>
            </n-button>
          </n-dropdown>

          <n-tooltip trigger="hover" :delay="400">
            <template #trigger>
              <n-button
                circle
                size="small"
                secondary
                :disabled="logs.length === 0 && pendingCount === 0"
                aria-label="清空当前日志视图"
                @click="handleClearLogs"
              >
                <template #icon>
                  <n-icon>
                    <TrashOutline />
                  </n-icon>
                </template>
              </n-button>
            </template>
            清空当前日志视图
          </n-tooltip>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import {
  ref,
  shallowRef,
  computed,
  nextTick,
  onMounted,
  onBeforeUnmount,
  watch,
} from 'vue'
import { useMessage, type DropdownOption, type InputInst } from 'naive-ui'
import {
  AlertCircleOutline,
  ArrowDownOutline,
  CheckmarkOutline,
  ChevronForwardOutline,
  CloseOutline,
  CodeSlashOutline,
  CopyOutline,
  DocumentTextOutline,
  DownloadOutline,
  PauseOutline,
  PlayOutline,
  RefreshOutline,
  ReturnDownBackOutline,
  SearchOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { useSettingStore } from '@/store/setting'
import { useAuthStore } from '@/store/auth'
import { useResponsive } from '@/hooks/useResponsive'
import { renderIcon } from '@/common/utils'
import {
  MAX_LOG_BUFFER,
  applyIncomingLogBatch,
  countLogsByLevel,
  filterLogs,
  formatLogEntryPlain,
  formatLogsForExport,
  normalizeLogEntry,
  resumePausedLogs,
  tokenizeHighlight,
  type HighlightSegment,
  type LogField,
  type LogLevelFilter,
  type NormalizedLogEntry,
} from './logsModel'

type StreamStatus = 'connecting' | 'connected' | 'reconnecting' | 'closed'

const message = useMessage()
const settingStore = useSettingStore()
const authStore = useAuthStore()
const { isMobile } = useResponsive()
const token = computed(() => settingStore.getToken())

const logs = shallowRef<NormalizedLogEntry[]>([])
const pendingLogs = shallowRef<NormalizedLogEntry[]>([])
const streamStatus = ref<StreamStatus>('connecting')
const connectionError = ref('')
const isPaused = ref(false)
const searchQuery = ref('')
const selectedLevels = ref<Set<LogLevelFilter>>(new Set())
const wrapLines = ref(true)
const expandedIds = ref<Set<number>>(new Set())
const followTail = ref(true)
const unreadWhileScrolled = ref(0)
const copiedRowId = ref<number | null>(null)
const virtualListRef = ref()
const searchInputRef = ref<InputInst | null>(null)

const levelFilterItems: Array<{ key: LogLevelFilter; label: string }> = [
  { key: 'ERROR', label: 'ERROR' },
  { key: 'WARN', label: 'WARN' },
  { key: 'INFO', label: 'INFO' },
  { key: 'DEBUG', label: 'DEBUG' },
]

const containerHeight = computed(
  () => `calc(100vh - ${settingStore.contentSafeTop + settingStore.contentSafeBottom}px)`,
)

const estimatedItemSize = computed(() => (isMobile.value ? 72 : 42))

const levelCounts = computed(() => countLogsByLevel(logs.value))

const filteredLogs = computed(() =>
  filterLogs(logs.value, {
    levels: selectedLevels.value,
    query: searchQuery.value,
  }),
)

const hasActiveFilters = computed(
  () => selectedLevels.value.size > 0 || searchQuery.value.trim() !== '',
)

const pendingCount = computed(() => pendingLogs.value.length)

const isInitialConnecting = computed(
  () => streamStatus.value === 'connecting' && logs.value.length === 0 && !connectionError.value,
)

const headerStatusVariant = computed(() => {
  if (connectionError.value || streamStatus.value === 'closed') {
    return 'error'
  }
  if (streamStatus.value === 'reconnecting' || streamStatus.value === 'connecting') {
    return 'connecting'
  }
  if (isPaused.value) {
    return 'paused'
  }
  return 'live'
})

const headerStatusLabel = computed(() => {
  if (streamStatus.value === 'closed') {
    return '已断开'
  }
  if (streamStatus.value === 'reconnecting') {
    return '重连中'
  }
  if (streamStatus.value === 'connecting') {
    return '连接中'
  }
  if (isPaused.value) {
    return pendingCount.value > 0 ? `已暂停 +${pendingCount.value}` : '已暂停'
  }
  return '实时连接'
})

const exportMenuOptions = computed<DropdownOption[]>(() => [
  {
    label: '导出纯文本 (.log)',
    key: 'log',
    icon: renderIcon(DocumentTextOutline),
  },
  {
    label: '导出结构化 JSON (.json)',
    key: 'json',
    icon: renderIcon(CodeSlashOutline),
  },
])

let eventSource: EventSource | null = null
let nextId = 1
let resetOnNextBatch = false
let pendingReplaceOnResume = false
let copyResetTimer: ReturnType<typeof setTimeout> | null = null

function getHighlighted(text: string): HighlightSegment[] {
  return tokenizeHighlight(text, searchQuery.value)
}

function detailRegularFields(log: NormalizedLogEntry): LogField[] {
  return log.fields.filter((f) => !(f.isError && f.displayValue === log.errorText))
}

function toggleLevelFilter(level: LogLevelFilter) {
  const next = new Set(selectedLevels.value)
  if (next.has(level)) {
    next.delete(level)
  } else {
    next.add(level)
  }
  selectedLevels.value = next
}

function clearLevelFilters() {
  if (selectedLevels.value.size > 0) {
    selectedLevels.value = new Set()
  }
}

function resetAllFilters() {
  searchQuery.value = ''
  selectedLevels.value = new Set()
}

function filterByCaller(caller: string) {
  searchQuery.value = caller
}

function toggleExpand(id: number) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  expandedIds.value = next
}

function onSummaryClick(id: number, event: MouseEvent) {
  const selection = window.getSelection()
  if (selection && selection.toString().trim().length > 0) {
    return
  }
  const target = event.target as HTMLElement | null
  if (target?.closest('button, a')) {
    return
  }
  toggleExpand(id)
}

function onLogScroll(event: Event) {
  const el = event.target
  if (el instanceof HTMLElement) {
    const atBottom = el.scrollHeight - el.clientHeight - el.scrollTop < 72
    followTail.value = atBottom
    if (atBottom) {
      unreadWhileScrolled.value = 0
    }
  }
}

const scrollToBottom = async (force = false) => {
  await nextTick()
  if (!force && !followTail.value) {
    return
  }
  if (virtualListRef.value && filteredLogs.value.length > 0) {
    virtualListRef.value.scrollTo({ position: 'bottom', behavior: 'auto' })
  }
}

function jumpToBottom() {
  followTail.value = true
  unreadWhileScrolled.value = 0
  void scrollToBottom(true)
}

function togglePause() {
  if (!isPaused.value) {
    isPaused.value = true
    return
  }
  isPaused.value = false
  const next = resumePausedLogs(
    {
      logs: logs.value,
      pendingLogs: pendingLogs.value,
      pendingReplaceOnResume,
      expandedIds: expandedIds.value,
    },
    MAX_LOG_BUFFER,
  )
  logs.value = next.logs
  pendingLogs.value = next.pendingLogs
  pendingReplaceOnResume = next.pendingReplaceOnResume
  expandedIds.value = next.expandedIds
  if (followTail.value) {
    void scrollToBottom()
  }
}

function handleClearLogs() {
  logs.value = []
  pendingLogs.value = []
  pendingReplaceOnResume = false
  expandedIds.value = new Set()
  unreadWhileScrolled.value = 0
  followTail.value = true
  message.success('已清空当前日志视图')
}

async function copyText(text: string, successMsg: string) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    message.success(successMsg)
    return true
  } catch {
    message.error('复制失败，请检查浏览器剪贴板权限')
    return false
  }
}

async function copyLogPlain(log: NormalizedLogEntry) {
  const ok = await copyText(formatLogEntryPlain(log), '已复制日志内容')
  if (ok) {
    copiedRowId.value = log.id
    if (copyResetTimer) {
      clearTimeout(copyResetTimer)
    }
    copyResetTimer = setTimeout(() => {
      copiedRowId.value = null
      copyResetTimer = null
    }, 1500)
  }
}

async function copyLogJson(log: NormalizedLogEntry) {
  const payload = JSON.stringify(log.raw, null, 2)
  await copyText(payload, '已复制结构化 JSON')
}

function handleExportSelect(key: string | number) {
  const format = key === 'json' ? 'json' : 'log'
  const entries = filteredLogs.value
  if (entries.length === 0) {
    message.warning('当前没有可导出的日志')
    return
  }
  const content = formatLogsForExport(entries, format)
  const mime = format === 'json' ? 'application/json;charset=utf-8' : 'text/plain;charset=utf-8'
  const now = new Date()
  const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  const filename = `watch-docker-logs-${stamp}.${format}`

  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  setTimeout(() => URL.revokeObjectURL(url), 1000)
  message.success(`已导出 ${entries.length} 条日志`)
}

function handleGlobalKeydown(event: KeyboardEvent) {
  const target = event.target as HTMLElement | null
  const isTyping =
    target?.tagName === 'INPUT' ||
    target?.tagName === 'TEXTAREA' ||
    Boolean(target?.isContentEditable)

  if (event.key === '/' && !isTyping && !event.metaKey && !event.ctrlKey && !event.altKey) {
    event.preventDefault()
    searchInputRef.value?.focus()
  } else if (event.key === 'Escape' && target && target.closest('.toolbar-search')) {
    if (searchQuery.value) {
      searchQuery.value = ''
    } else {
      ;(target as HTMLElement).blur()
    }
  }
}

const connectEventSource = () => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  if (authStore.authEnabled && !token.value) {
    streamStatus.value = 'closed'
    return
  }
  const url = token.value
    ? `/api/v1/logs?token=${encodeURIComponent(token.value)}`
    : '/api/v1/logs'
  streamStatus.value = logs.value.length > 0 ? 'reconnecting' : 'connecting'
  connectionError.value = ''
  followTail.value = true
  unreadWhileScrolled.value = 0

  const es = new EventSource(url)
  eventSource = es

  es.onopen = () => {
    if (es !== eventSource) {
      return
    }
    streamStatus.value = 'connected'
    connectionError.value = ''
    resetOnNextBatch = logs.value.length > 0
  }

  es.onmessage = (e) => {
    if (es !== eventSource) {
      return
    }
    try {
      const logArray = JSON.parse(e.data)
      if (!Array.isArray(logArray)) {
        return
      }
      const normalizedBatch = logArray.map((rawItem: Record<string, unknown>) =>
        normalizeLogEntry(rawItem, nextId++),
      )

      const shouldReset = resetOnNextBatch
      resetOnNextBatch = false
      const shouldScroll = followTail.value
      const next = applyIncomingLogBatch(
        {
          logs: logs.value,
          pendingLogs: pendingLogs.value,
          pendingReplaceOnResume,
          expandedIds: expandedIds.value,
        },
        normalizedBatch,
        {
          isPaused: isPaused.value,
          resetBatch: shouldReset,
          maxSize: MAX_LOG_BUFFER,
        },
      )

      logs.value = next.logs
      pendingLogs.value = next.pendingLogs
      pendingReplaceOnResume = next.pendingReplaceOnResume
      expandedIds.value = next.expandedIds

      if (isPaused.value) {
        return
      }

      if (shouldScroll) {
        void scrollToBottom()
      } else if (!next.replacedVisible) {
        const matchedNew = filterLogs(normalizedBatch, {
          levels: selectedLevels.value,
          query: searchQuery.value,
        }).length
        if (matchedNew > 0) {
          unreadWhileScrolled.value += matchedNew
        }
      }
    } catch (err) {
      console.error('Failed to parse log SSE message:', err)
    }
  }

  es.onerror = () => {
    if (es !== eventSource) {
      return
    }
    if (es.readyState === EventSource.CLOSED) {
      streamStatus.value = 'closed'
      connectionError.value = '日志流连接已关闭'
      if (authStore.authEnabled) {
        const headers: Record<string, string> = token.value
          ? { Authorization: `Bearer ${token.value}` }
          : {}
        void fetch('/api/v1/config', { headers })
          .then((res) => {
            if (res.status === 401) {
              authStore.forceLogout()
            }
          })
          .catch(() => {})
      }
      return
    }
    streamStatus.value = 'reconnecting'
    if (!connectionError.value) {
      connectionError.value = '日志流连接中断，正在自动重试...'
      message.warning('日志流连接中断')
    }
  }
}

function handleReconnect() {
  connectEventSource()
}

watch(token, (newToken, oldToken) => {
  if (newToken !== oldToken) {
    connectEventSource()
  }
})

watch([searchQuery, selectedLevels], () => {
  unreadWhileScrolled.value = 0
  if (followTail.value) {
    void scrollToBottom()
  }
})

onMounted(() => {
  connectEventSource()
  window.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if (copyResetTimer) {
    clearTimeout(copyResetTimer)
    copyResetTimer = null
  }
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})
</script>

<style lang="less">
.layout-logs {
  .n-layout-scroll-container {
    overflow: hidden;
  }
}
</style>

<style scoped lang="less">
@import '@/styles/mix.less';

.logs-page {
  --log-font-mono:
    'JetBrains Mono', 'SFMono-Regular', ui-monospace, Menlo, Monaco, Consolas, 'Liberation Mono',
    monospace;
  --log-shell-bg: #ffffff;
  --log-toolbar-bg: #f8fafc;
  --log-head-bg: #f1f5f9;
  --log-detail-bg: #f8fafc;
  --log-border: rgba(15, 23, 42, 0.09);
  --log-border-strong: rgba(15, 23, 42, 0.16);
  --log-row-hover: rgba(15, 23, 42, 0.035);
  --log-row-expanded: rgba(24, 160, 88, 0.045);
  --log-text-primary: #0f172a;
  --log-text-secondary: #475569;
  --log-text-muted: #526074;
  --log-focus-ring: #18a058;

  --log-debug-fg: #475569;
  --log-debug-bg: rgba(71, 85, 105, 0.1);
  --log-debug-border: rgba(71, 85, 105, 0.22);

  --log-info-fg: #0369a1;
  --log-info-bg: rgba(2, 132, 199, 0.1);
  --log-info-border: rgba(2, 132, 199, 0.24);

  --log-warn-fg: #b45309;
  --log-warn-bg: rgba(217, 119, 6, 0.11);
  --log-warn-border: rgba(217, 119, 6, 0.28);
  --log-warn-row-bg: rgba(245, 158, 11, 0.055);
  --log-warn-row-hover: rgba(245, 158, 11, 0.1);
  --log-warn-row-expanded: rgba(245, 158, 11, 0.09);
  --log-warn-strip: #d97706;

  --log-error-fg: #b91c1c;
  --log-error-bg: rgba(220, 38, 38, 0.1);
  --log-error-border: rgba(220, 38, 38, 0.28);
  --log-error-row-bg: rgba(239, 68, 68, 0.055);
  --log-error-row-hover: rgba(239, 68, 68, 0.1);
  --log-error-row-expanded: rgba(239, 68, 68, 0.09);
  --log-error-strip: #dc2626;

  --log-chip-bg: rgba(15, 23, 42, 0.045);
  --log-chip-border: rgba(15, 23, 42, 0.1);
  --log-mark-bg: rgba(250, 204, 21, 0.45);
  --log-mark-fg: #713f12;

  display: flex;
  flex-direction: column;
  width: 100%;
  box-sizing: border-box;
  overflow: hidden;
}

.logs-page[data-theme='dark'] {
  --log-shell-bg: #11141b;
  --log-toolbar-bg: #151a23;
  --log-head-bg: #131822;
  --log-detail-bg: #0c0f15;
  --log-border: rgba(148, 163, 184, 0.12);
  --log-border-strong: rgba(148, 163, 184, 0.22);
  --log-row-hover: rgba(148, 163, 184, 0.065);
  --log-row-expanded: rgba(16, 185, 129, 0.07);
  --log-text-primary: #edf2f7;
  --log-text-secondary: #9fb0c7;
  --log-text-muted: #8b9bb4;
  --log-focus-ring: #34d399;

  --log-debug-fg: #cbd5e1;
  --log-debug-bg: rgba(148, 163, 184, 0.14);
  --log-debug-border: rgba(148, 163, 184, 0.28);

  --log-info-fg: #38bdf8;
  --log-info-bg: rgba(56, 189, 248, 0.13);
  --log-info-border: rgba(56, 189, 248, 0.28);

  --log-warn-fg: #fbbf24;
  --log-warn-bg: rgba(245, 158, 11, 0.16);
  --log-warn-border: rgba(245, 158, 11, 0.34);
  --log-warn-row-bg: rgba(245, 158, 11, 0.055);
  --log-warn-row-hover: rgba(245, 158, 11, 0.11);
  --log-warn-row-expanded: rgba(245, 158, 11, 0.095);
  --log-warn-strip: #f59e0b;

  --log-error-fg: #f87171;
  --log-error-bg: rgba(239, 68, 68, 0.17);
  --log-error-border: rgba(239, 68, 68, 0.36);
  --log-error-row-bg: rgba(239, 68, 68, 0.075);
  --log-error-row-hover: rgba(239, 68, 68, 0.13);
  --log-error-row-expanded: rgba(239, 68, 68, 0.11);
  --log-error-strip: #ef4444;

  --log-chip-bg: rgba(148, 163, 184, 0.1);
  --log-chip-border: rgba(148, 163, 184, 0.16);
  --log-mark-bg: rgba(250, 204, 21, 0.26);
  --log-mark-fg: #fef08a;
}

.logs-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  height: 100%;
  gap: 12px;
  padding-right: 4px;
}

.logs-header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.logs-header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.tabular-count {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  color: #475569;
}

.logs-header[data-theme='dark'] .tabular-count {
  color: #9fb0c7;
}

.stream-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 9px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  &.status-live {
    color: #047857;
    background: rgba(16, 185, 129, 0.12);
    border: 1px solid rgba(16, 185, 129, 0.26);

    .status-dot {
      background: #10b981;
      animation: live-pulse 2.2s ease-in-out infinite;
    }
  }

  &.status-paused {
    color: #b45309;
    background: rgba(245, 158, 11, 0.12);
    border: 1px solid rgba(245, 158, 11, 0.28);

    .status-dot {
      background: #f59e0b;
    }
  }

  &.status-connecting {
    color: #0369a1;
    background: rgba(14, 165, 233, 0.12);
    border: 1px solid rgba(14, 165, 233, 0.26);

    .status-dot {
      background: #0ea5e9;
      animation: live-pulse 1.2s ease-in-out infinite;
    }
  }

  &.status-error {
    color: #b91c1c;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.28);

    .status-dot {
      background: #ef4444;
    }
  }
}

.logs-header[data-theme='dark'] .stream-status-pill {
  &.status-live {
    color: #34d399;
    background: rgba(16, 185, 129, 0.16);
    border-color: rgba(16, 185, 129, 0.34);
  }

  &.status-paused {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.16);
    border-color: rgba(245, 158, 11, 0.34);
  }

  &.status-connecting {
    color: #38bdf8;
    background: rgba(56, 189, 248, 0.16);
    border-color: rgba(56, 189, 248, 0.34);
  }

  &.status-error {
    color: #f87171;
    background: rgba(239, 68, 68, 0.18);
    border-color: rgba(239, 68, 68, 0.36);
  }
}

@keyframes live-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.45;
    transform: scale(0.82);
  }
}

.logs-shell {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  background: var(--log-shell-bg);
  border: 1px solid var(--log-border);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}

.logs-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: var(--log-toolbar-bg);
  border-bottom: 1px solid var(--log-border);
  flex-shrink: 0;
}

.toolbar-search {
  flex: 1 1 240px;
  max-width: 380px;
  min-width: 180px;

  .toolbar-search-icon {
    color: var(--log-text-muted);
  }

  .kbd-hint {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 18px;
    padding: 0 4px;
    font-family: var(--log-font-mono);
    font-size: 11px;
    line-height: 1;
    color: var(--log-text-muted);
    background: var(--log-chip-bg);
    border: 1px solid var(--log-chip-border);
    border-radius: 4px;
  }
}

.toolbar-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.level-filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border-radius: 7px;
  border: 1px solid var(--log-border);
  background: transparent;
  color: var(--log-text-secondary);
  font-size: 12px;
  font-weight: 500;
  font-family: var(--log-font-mono);
  cursor: pointer;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease,
    transform 0.12s ease;

  &:hover {
    background: var(--log-row-hover);
    border-color: var(--log-border-strong);
    color: var(--log-text-primary);
  }

  &:active {
    transform: translateY(1px);
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 2px;
  }

  &.is-empty {
    opacity: 0.62;
  }

  .chip-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .chip-count {
    font-variant-numeric: tabular-nums;
    font-size: 11px;
    opacity: 0.85;
  }

  &.is-active {
    background: rgba(24, 160, 88, 0.12);
    border-color: rgba(24, 160, 88, 0.36);
    color: var(--log-text-primary);
  }

  &.chip-error {
    .chip-dot {
      background: var(--log-error-fg);
    }
    &.is-active {
      background: var(--log-error-bg);
      border-color: var(--log-error-border);
      color: var(--log-error-fg);
    }
  }

  &.chip-warn {
    .chip-dot {
      background: var(--log-warn-fg);
    }
    &.is-active {
      background: var(--log-warn-bg);
      border-color: var(--log-warn-border);
      color: var(--log-warn-fg);
    }
  }

  &.chip-info {
    .chip-dot {
      background: var(--log-info-fg);
    }
    &.is-active {
      background: var(--log-info-bg);
      border-color: var(--log-info-border);
      color: var(--log-info-fg);
    }
  }

  &.chip-debug {
    .chip-dot {
      background: var(--log-debug-fg);
    }
    &.is-active {
      background: var(--log-debug-bg);
      border-color: var(--log-debug-border);
      color: var(--log-debug-fg);
    }
  }
}

.toolbar-controls {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}

.control-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 28px;
  padding: 0 10px;
  border-radius: 7px;
  border: 1px solid var(--log-border);
  background: transparent;
  color: var(--log-text-secondary);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease,
    transform 0.12s ease;

  &:hover {
    background: var(--log-row-hover);
    border-color: var(--log-border-strong);
    color: var(--log-text-primary);
  }

  &:active {
    transform: translateY(1px);
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 2px;
  }

  &.is-active {
    background: var(--log-chip-bg);
    border-color: var(--log-border-strong);
    color: var(--log-text-primary);
  }

  &--reset {
    color: var(--log-error-fg);
    border-color: var(--log-error-border);
    background: var(--log-error-bg);

    &:hover {
      color: var(--log-error-fg);
      border-color: var(--log-error-fg);
      background: var(--log-error-bg);
    }
  }
}

.logs-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 14px;
  font-size: 12.5px;
  border-bottom: 1px solid var(--log-border);
  flex-shrink: 0;

  .banner-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  &--error {
    color: var(--log-error-fg);
    background: var(--log-error-bg);
  }

  &--paused {
    color: var(--log-warn-fg);
    background: var(--log-warn-bg);
  }
}

.banner-action {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid currentColor;
  background: transparent;
  color: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition: opacity 0.15s ease, transform 0.12s ease;

  &:hover {
    opacity: 0.85;
  }

  &:active {
    transform: translateY(1px);
  }

  &:focus-visible {
    outline: 2px solid currentColor;
    outline-offset: 2px;
  }
}

.logs-table-head {
  display: grid;
  grid-template-columns: 20px 96px 62px minmax(0, 200px) minmax(0, 1fr) 32px;
  align-items: center;
  gap: 10px;
  padding: 7px 14px;
  background: var(--log-head-bg);
  border-bottom: 1px solid var(--log-border);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--log-text-muted);
  flex-shrink: 0;
}

.logs-body {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.virtual-log-list {
  height: 100%;

  :deep(.v-vl) {
    .scrollbar();
  }
}

.logs-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 32px 20px;
  text-align: center;
  gap: 6px;

  .state-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    border-radius: 10px;
    background: var(--log-chip-bg);
    border: 1px solid var(--log-chip-border);
    color: var(--log-text-secondary);
    margin-bottom: 6px;
  }

  .state-title {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--log-text-primary);
  }

  .state-desc {
    margin: 0;
    font-size: 12.5px;
    color: var(--log-text-secondary);
  }
}

.log-row {
  position: relative;
  border-bottom: 1px solid var(--log-border);
  border-left: 2px solid transparent;
  transition: background-color 0.14s ease;

  &:hover {
    background: var(--log-row-hover);

    .row-icon-btn {
      opacity: 1;
    }
  }

  &.is-expanded {
    background: var(--log-row-expanded);

    .log-chevron {
      transform: rotate(90deg);
      color: var(--log-text-primary);
    }
  }

  &--warn {
    border-left-color: var(--log-warn-strip);
    background: var(--log-warn-row-bg);

    &:hover {
      background: var(--log-warn-row-hover);
    }

    &.is-expanded {
      background: var(--log-warn-row-expanded);
    }
  }

  &--error {
    border-left-color: var(--log-error-strip);
    background: var(--log-error-row-bg);

    &:hover {
      background: var(--log-error-row-hover);
    }

    &.is-expanded {
      background: var(--log-error-row-expanded);
    }
  }
}

.log-row-summary {
  display: grid;
  grid-template-columns: 20px 96px 62px minmax(0, 200px) minmax(0, 1fr) 32px;
  align-items: baseline;
  gap: 10px;
  padding: 8px 14px 8px 12px;
  font-family: var(--log-font-mono);
  font-size: 12.5px;
  line-height: 1.55;
  cursor: pointer;

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: -2px;
  }
}

.log-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  color: var(--log-text-muted);
  transition: transform 0.16s ease, color 0.16s ease;
  align-self: center;
}

.log-time {
  color: var(--log-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  font-size: 12px;
}

.log-level-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 20px;
  padding: 0 6px;
  border-radius: 5px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.03em;
  line-height: 1;
  border: 1px solid transparent;
  white-space: nowrap;

  &.level-debug {
    color: var(--log-debug-fg);
    background: var(--log-debug-bg);
    border-color: var(--log-debug-border);
  }

  &.level-info {
    color: var(--log-info-fg);
    background: var(--log-info-bg);
    border-color: var(--log-info-border);
  }

  &.level-warn {
    color: var(--log-warn-fg);
    background: var(--log-warn-bg);
    border-color: var(--log-warn-border);
  }

  &.level-error {
    color: var(--log-error-fg);
    background: var(--log-error-bg);
    border-color: var(--log-error-border);
  }

  &.level-other {
    color: var(--log-text-secondary);
    background: var(--log-chip-bg);
    border-color: var(--log-chip-border);
  }
}

.log-caller {
  color: var(--log-text-secondary);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &.is-empty {
    color: var(--log-text-muted);
    opacity: 0.6;
  }
}

.log-content {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px 8px;
  min-width: 0;
}

.log-msg {
  color: var(--log-text-primary);
  word-break: break-word;
  white-space: pre-wrap;
}

.log-inline-fields {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.field-chip {
  display: inline-flex;
  align-items: baseline;
  max-width: 100%;
  padding: 1px 6px;
  border-radius: 5px;
  font-size: 11.5px;
  line-height: 1.45;
  background: var(--log-chip-bg);
  border: 1px solid var(--log-chip-border);
  color: var(--log-text-secondary);
  overflow: hidden;

  .field-chip-key {
    color: var(--log-text-muted);
    font-weight: 500;
    flex-shrink: 0;
  }

  .field-chip-sep {
    opacity: 0.6;
    margin: 0 2px;
    flex-shrink: 0;
  }

  .field-chip-val {
    color: var(--log-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &--error {
    background: var(--log-error-bg);
    border-color: var(--log-error-border);

    .field-chip-key,
    .field-chip-val {
      color: var(--log-error-fg);
    }
  }

  &--more {
    font-weight: 500;
    color: var(--log-text-secondary);
  }
}

.log-mark {
  background: var(--log-mark-bg);
  color: var(--log-mark-fg);
  padding: 0 2px;
  border-radius: 3px;
}

.log-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  align-self: center;
}

.row-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--log-text-secondary);
  cursor: pointer;
  opacity: 0;
  transition:
    opacity 0.14s ease,
    background-color 0.14s ease,
    color 0.14s ease,
    border-color 0.14s ease;

  &:hover,
  &:focus-visible,
  &.is-copied {
    opacity: 1;
    background: var(--log-chip-bg);
    border-color: var(--log-chip-border);
    color: var(--log-text-primary);
  }

  &.is-copied {
    color: #10b981;
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 1px;
  }
}

/* 单行紧凑模式 */
.log-row.is-nowrap:not(.is-mobile) {
  .log-content {
    flex-wrap: nowrap;
    overflow: hidden;
  }

  .log-msg {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 0 1 auto;
    min-width: 0;
  }

  .log-inline-fields {
    flex-wrap: nowrap;
    overflow: hidden;
    flex: 0 1 auto;
  }
}

/* 展开结构化详情面板 */
.log-detail {
  margin: 0 14px 10px 42px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--log-detail-bg);
  border: 1px solid var(--log-border-strong);
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-family: var(--log-font-mono);
  font-size: 12px;
}

.detail-meta-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 14px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--log-border);
}

.detail-meta-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
}

.detail-meta-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;

  .meta-label {
    font-family: inherit;
    font-size: 11px;
    color: var(--log-text-muted);
  }

  .meta-value {
    font-family: var(--log-font-mono);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
    color: var(--log-text-primary);
    background: var(--log-chip-bg);
    padding: 1px 6px;
    border-radius: 4px;
  }
}

.detail-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.detail-action-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  padding: 0 9px;
  border-radius: 6px;
  border: 1px solid var(--log-border-strong);
  background: var(--log-shell-bg);
  color: var(--log-text-secondary);
  font-size: 11.5px;
  font-weight: 500;
  cursor: pointer;
  transition:
    background-color 0.14s ease,
    color 0.14s ease,
    border-color 0.14s ease;

  &:hover {
    color: var(--log-text-primary);
    background: var(--log-row-hover);
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 2px;
  }
}

.detail-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.detail-section-title {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--log-text-muted);

  &--error {
    color: var(--log-error-fg);
  }
}

.detail-mini-copy {
  padding: 2px 7px;
  border-radius: 4px;
  border: 1px solid var(--log-border);
  background: transparent;
  color: var(--log-text-secondary);
  font-size: 11px;
  cursor: pointer;
  flex-shrink: 0;
  transition: background-color 0.14s ease, color 0.14s ease;

  &:hover {
    background: var(--log-chip-bg);
    color: var(--log-text-primary);
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 1px;
  }
}

.detail-error-box {
  padding: 10px 12px;
  border-radius: 7px;
  background: var(--log-error-bg);
  border: 1px solid var(--log-error-border);

  .detail-error-text {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--log-error-fg);
    font-family: var(--log-font-mono);
    font-size: 12px;
    line-height: 1.55;
  }
}

.detail-kv-list {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-kv-row {
  display: grid;
  grid-template-columns: minmax(90px, 160px) minmax(0, 1fr);
  gap: 8px 12px;
  align-items: start;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--log-shell-bg);
  border: 1px solid var(--log-border);
}

.detail-kv-key {
  color: var(--log-text-secondary);
  font-weight: 600;
  word-break: break-word;
}

.detail-kv-val {
  margin: 0;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.detail-kv-inline {
  color: var(--log-text-primary);
  font-family: var(--log-font-mono);
  word-break: break-all;
  white-space: pre-wrap;
  min-width: 0;
  flex: 1;
}

.detail-kv-pre,
.detail-stacktrace {
  margin: 0;
  color: var(--log-text-primary);
  font-family: var(--log-font-mono);
  font-size: 11.5px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
  min-width: 0;
}

.detail-stacktrace-box {
  padding: 10px 12px;
  border-radius: 7px;
  background: var(--log-chip-bg);
  border: 1px solid var(--log-border);
}

/* 回到底部悬浮按钮 */
.scroll-bottom-pill {
  position: absolute;
  right: 18px;
  bottom: 16px;
  z-index: 12;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 14px;
  border-radius: 999px;
  border: 1px solid rgba(16, 185, 129, 0.36);
  background: #10b981;
  color: #ffffff;
  font-size: 12.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  box-shadow: 0 6px 18px rgba(16, 185, 129, 0.28);
  transition: transform 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;

  &:hover {
    background: #059669;
    transform: translateY(-1px);
  }

  &:active {
    transform: translateY(0);
  }

  &:focus-visible {
    outline: 2px solid var(--log-focus-ring);
    outline-offset: 2px;
  }
}

.pill-fade-enter-active,
.pill-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.pill-fade-enter-from,
.pill-fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

/* 中等宽度适配 */
@media (max-width: 1024px) {
  .logs-table-head,
  .log-row-summary {
    grid-template-columns: 18px 92px 58px minmax(0, 128px) minmax(0, 1fr) 30px;
    gap: 8px;
  }
}

/* 移动端适配 */
@media (max-width: 767px) {
  .logs-shell {
    border-radius: 10px;
  }

  .logs-toolbar {
    padding: 8px 10px;
    gap: 8px;
  }

  .toolbar-search {
    flex: 1 1 100%;
    max-width: none;
    min-width: 0;
  }

  .toolbar-controls {
    margin-left: 0;
  }

  .log-row.is-mobile {
    .log-row-summary {
      display: flex;
      flex-direction: column;
      align-items: stretch;
      gap: 5px;
      padding: 9px 10px;
    }

    .mobile-row-meta {
      display: flex;
      align-items: center;
      gap: 7px;
      min-width: 0;
    }

    .mobile-row-body {
      display: flex;
      flex-direction: column;
      gap: 5px;
      min-width: 0;
    }

    .log-detail {
      margin: 0 10px 10px;
      padding: 10px;
    }

    .detail-kv-row {
      grid-template-columns: minmax(0, 1fr);
      gap: 3px;
    }
  }

  .scroll-bottom-pill {
    right: 12px;
    bottom: 12px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .stream-status-pill .status-dot {
    animation: none !important;
  }

  .level-filter-chip,
  .control-chip,
  .log-row,
  .log-chevron,
  .row-icon-btn,
  .scroll-bottom-pill,
  .pill-fade-enter-active,
  .pill-fade-leave-active {
    transition: none !important;
  }
}
</style>
