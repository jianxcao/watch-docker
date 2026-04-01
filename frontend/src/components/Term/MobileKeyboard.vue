<template>
  <div class="keyboard-layer">
    <Transition name="panel">
      <div
        v-if="expanded"
        ref="keyboardPanelRef"
        class="mobile-keyboard"
        role="toolbar"
        aria-label="终端快捷键"
      >
        <div class="keyboard-topbar">
          <div class="keyboard-topbar-meta">
            <span class="keyboard-title">终端快捷键</span>
            <span v-if="modifierStatusText" class="modifier-indicator">
              {{ modifierStatusText }}
            </span>
          </div>
          <div class="keyboard-topbar-actions">
            <button
              class="topbar-button topbar-button-clear"
              aria-label="清空终端"
              @click="emit('clear')"
            >
              Clear
            </button>
            <button
              class="topbar-button"
              :class="{ 'topbar-button-active': showDrawer }"
              aria-label="更多按键"
              @click="toggleDrawer"
            >
              ···
            </button>
            <button class="topbar-button" aria-label="收起快捷键" @click="setExpanded(false)">
              ✕
            </button>
          </div>
        </div>

        <div class="keyboard-row">
          <button
            v-for="key in row1Keys"
            :key="key.id"
            class="key-button"
            :class="{
              'key-modifier': key.isModifier,
              'key-active': activeModifiers.includes(key.id),
            }"
            :aria-label="key.description"
            @click="handleKeyClick(key)"
            @touchstart.passive="handleTouchStart()"
          >
            <span v-if="key.icon" class="key-icon">{{ key.icon }}</span>
            <span v-else class="key-label">{{ key.label }}</span>
          </button>
        </div>

        <div class="keyboard-row">
          <button
            v-for="key in row2Keys"
            :key="key.id"
            class="key-button"
            :aria-label="key.description"
            @click="handleKeyClick(key)"
            @touchstart.passive="handleTouchStart()"
          >
            <span class="key-label">{{ key.label }}</span>
          </button>
          <button
            class="key-button key-backspace"
            aria-label="退格"
            @click="sendSequence('\x7f')"
            @touchstart.passive="handleTouchStart()"
          >
            <span class="key-icon">⌫</span>
          </button>
        </div>

        <Transition name="drawer">
          <div v-if="showDrawer" class="keyboard-drawer">
            <div class="drawer-header">
              <span class="drawer-title">更多按键</span>
              <button class="drawer-close" @click="showDrawer = false" aria-label="关闭">
                <span>✕</span>
              </button>
            </div>
            <div class="drawer-content">
              <div class="drawer-section">
                <span class="section-label">功能键</span>
                <div class="drawer-keys">
                  <button
                    v-for="fkey in functionKeys"
                    :key="fkey.id"
                    class="key-button key-small"
                    :aria-label="fkey.description"
                    @click="handleKeyClick(fkey)"
                  >
                    {{ fkey.label }}
                  </button>
                </div>
              </div>
              <div class="drawer-section">
                <span class="section-label">导航</span>
                <div class="drawer-keys">
                  <button
                    v-for="navKey in navigationKeys"
                    :key="navKey.id"
                    class="key-button key-small"
                    :aria-label="navKey.description"
                    @click="handleKeyClick(navKey)"
                  >
                    <span v-if="navKey.icon">{{ navKey.icon }}</span>
                    <span v-else>{{ navKey.label }}</span>
                  </button>
                </div>
              </div>
              <div class="drawer-section">
                <span class="section-label">快速命令</span>
                <div class="drawer-keys">
                  <button
                    v-for="cmd in quickCommands"
                    :key="cmd.id"
                    class="key-button key-small key-command"
                    :aria-label="cmd.description"
                    @click="handleKeyClick(cmd)"
                  >
                    {{ cmd.label }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>

    <button
      class="keyboard-fab"
      :class="{ 'keyboard-fab-active': expanded }"
      aria-label="打开终端快捷键"
      @click="setExpanded(!expanded)"
    >
      <span class="keyboard-fab-icon">⌘</span>
      <!-- <span class="keyboard-fab-label">{{ expanded ? '收起快捷键' : '快捷键' }}</span> -->
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'

interface KeyConfig {
  id: string
  label: string
  description: string
  sequence: string
  icon?: string
  isModifier?: boolean
}

interface Emits {
  (e: 'key', sequence: string): void
  (e: 'clear'): void
  (e: 'update:expanded', value: boolean): void
  (e: 'height-change', value: number): void
}

interface Props {
  expanded?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  expanded: false,
})

const emit = defineEmits<Emits>()

const showDrawer = ref(false)
const activeModifiers = ref<string[]>([])
const keyboardPanelRef = useTemplateRef<HTMLElement>('keyboardPanelRef')
const resizeObserver = ref<ResizeObserver>()

// 修饰键状态管理：Ctrl 和 Alt 是切换式的
const isCtrlActive = computed(() => activeModifiers.value.includes('ctrl'))
const isAltActive = computed(() => activeModifiers.value.includes('alt'))
const expanded = computed(() => props.expanded)
const modifierStatusText = computed(() => {
  if (isCtrlActive.value && isAltActive.value) {
    return 'Ctrl + Alt 已激活，等待下一个按键'
  }
  if (isCtrlActive.value) {
    return 'Ctrl 已激活，等待下一个按键'
  }
  if (isAltActive.value) {
    return 'Alt 已激活，等待下一个按键'
  }
  return ''
})

// 第一行按键
const row1Keys = computed<KeyConfig[]>(() => [
  { id: 'up', label: '↑', description: '上箭头', sequence: '\x1b[A', icon: '↑' },
  { id: 'down', label: '↓', description: '下箭头', sequence: '\x1b[B', icon: '↓' },
  { id: 'left', label: '←', description: '左箭头', sequence: '\x1b[D', icon: '←' },
  { id: 'right', label: '→', description: '右箭头', sequence: '\x1b[C', icon: '→' },
  { id: 'esc', label: 'Esc', description: '退出', sequence: '\x1b' },
  { id: 'tab', label: 'Tab', description: '制表符', sequence: '\t' },
  { id: 'ctrl', label: 'Ctrl', description: '控制键', sequence: '', isModifier: true },
  { id: 'alt', label: 'Alt', description: 'Alt键', sequence: '', isModifier: true },
])

// 第二行按键
const row2Keys = computed<KeyConfig[]>(() => [
  { id: 'slash', label: '/', description: '斜杠', sequence: '/' },
  { id: 'pipe', label: '|', description: '管道符', sequence: '|' },
  { id: 'tilde', label: '~', description: '波浪号', sequence: '~' },
  { id: 'dash', label: '-', description: '减号', sequence: '-' },
  { id: 'ctrl-c', label: '^C', description: '中断 (Ctrl+C)', sequence: '\x03' },
  { id: 'ctrl-d', label: '^D', description: '退出 (Ctrl+D)', sequence: '\x04' },
  { id: 'ctrl-z', label: '^Z', description: '挂起 (Ctrl+Z)', sequence: '\x1a' },
  { id: 'ctrl-l', label: '^L', description: '清屏 (Ctrl+L)', sequence: '\x0c' },
])

// 功能键
const functionKeys = computed<KeyConfig[]>(() =>
  Array.from({ length: 12 }, (_, i) => ({
    id: `f${i + 1}`,
    label: `F${i + 1}`,
    description: `功能键 ${i + 1}`,
    sequence: `\x1bO${String.fromCharCode(80 + i)}`, // F1-F4: \x1bOP-OS, F5-F12: \x1b[15~-24~
  })),
)

// 导航键
const navigationKeys = computed<KeyConfig[]>(() => [
  { id: 'home', label: 'Home', description: '行首', sequence: '\x1b[H', icon: '⤒' },
  { id: 'end', label: 'End', description: '行尾', sequence: '\x1b[F', icon: '⤓' },
  { id: 'pgup', label: 'PgUp', description: '上翻页', sequence: '\x1b[5~', icon: '⇞' },
  { id: 'pgdn', label: 'PgDn', description: '下翻页', sequence: '\x1b[6~', icon: '⇟' },
  { id: 'insert', label: 'Ins', description: '插入', sequence: '\x1b[2~' },
  { id: 'delete', label: 'Del', description: '删除', sequence: '\x1b[3~' },
])

// 快速命令
const quickCommands = computed<KeyConfig[]>(() => [
  { id: 'cmd-clear', label: 'clear', description: '清屏', sequence: 'clear\n' },
  { id: 'cmd-exit', label: 'exit', description: '退出', sequence: 'exit\n' },
  { id: 'cmd-ls', label: 'ls', description: '列出文件', sequence: 'ls -la\n' },
  { id: 'cmd-cd-up', label: 'cd ..', description: '上级目录', sequence: 'cd ..\n' },
])

// 处理按键点击
const handleKeyClick = (key: KeyConfig) => {
  // 处理修饰键（切换模式）
  if (key.isModifier) {
    toggleModifier(key.id)
    return
  }

  // 构建序列（考虑激活的修饰键）
  let sequence = key.sequence
  if (isCtrlActive.value && key.id !== 'ctrl') {
    // 如果 Ctrl 激活，将字母/符号转换为 Ctrl 组合
    const charCode = key.sequence.charCodeAt(0)
    if (charCode >= 97 && charCode <= 122) {
      // a-z -> Ctrl+a-z (0x01-0x1a)
      sequence = String.fromCharCode(charCode - 96)
    } else if (charCode >= 65 && charCode <= 90) {
      // A-Z -> Ctrl+A-Z
      sequence = String.fromCharCode(charCode - 64)
    }
    activeModifiers.value = activeModifiers.value.filter((m) => m !== 'ctrl')
  }
  if (isAltActive.value && key.id !== 'alt') {
    // Alt 前缀：\x1b + key
    sequence = '\x1b' + key.sequence
    activeModifiers.value = activeModifiers.value.filter((m) => m !== 'alt')
  }

  sendSequence(sequence)
}

// 发送序列
const sendSequence = (sequence: string) => {
  emit('key', sequence)
}

const clearActiveModifiers = () => {
  activeModifiers.value = []
}

const setExpanded = (value: boolean) => {
  if (!value) {
    showDrawer.value = false
    clearActiveModifiers()
    emit('height-change', 0)
  }
  emit('update:expanded', value)
}

// 切换修饰键状态
const toggleModifier = (modifierId: string) => {
  const index = activeModifiers.value.indexOf(modifierId)
  if (index === -1) {
    activeModifiers.value.push(modifierId)
  } else {
    activeModifiers.value.splice(index, 1)
  }
}

const getBaseSequenceFromKeyboardEvent = (event: KeyboardEvent) => {
  const { key } = event

  if (key.length === 1) {
    return key
  }

  switch (key) {
    case 'Enter':
      return '\r'
    case 'Tab':
      return '\t'
    case 'Escape':
      return '\x1b'
    case 'Backspace':
      return '\x7f'
    case 'Delete':
      return '\x1b[3~'
    case 'Insert':
      return '\x1b[2~'
    case 'Home':
      return '\x1b[H'
    case 'End':
      return '\x1b[F'
    case 'PageUp':
      return '\x1b[5~'
    case 'PageDown':
      return '\x1b[6~'
    case 'ArrowUp':
      return '\x1b[A'
    case 'ArrowDown':
      return '\x1b[B'
    case 'ArrowLeft':
      return '\x1b[D'
    case 'ArrowRight':
      return '\x1b[C'
    case 'Space':
    case ' ':
      return ' '
    default:
      return ''
  }
}

const applyVirtualModifiers = (baseSequence: string) => {
  let sequence = baseSequence

  if (isCtrlActive.value && baseSequence.length === 1) {
    const charCode = baseSequence.toUpperCase().charCodeAt(0)
    if (charCode >= 64 && charCode <= 95) {
      sequence = String.fromCharCode(charCode - 64)
    }
  }

  if (isAltActive.value) {
    sequence = `\x1b${sequence}`
  }

  clearActiveModifiers()
  return sequence
}

const handlePhysicalKeydown = (event: KeyboardEvent) => {
  if (!activeModifiers.value.length) {
    return
  }

  if (event.metaKey || event.ctrlKey || event.altKey) {
    return
  }

  if (event.key === 'Control' || event.key === 'Alt' || event.key === 'Shift') {
    return
  }

  const baseSequence = getBaseSequenceFromKeyboardEvent(event)
  if (!baseSequence) {
    return
  }

  event.preventDefault()
  event.stopPropagation()
  sendSequence(applyVirtualModifiers(baseSequence))
}

// 触摸反馈
const handleTouchStart = () => {
  // 轻微震动反馈（如果支持）
  if (navigator.vibrate) {
    navigator.vibrate(10)
  }
}

// 切换抽屉
const toggleDrawer = () => {
  showDrawer.value = !showDrawer.value
}

const emitPanelHeight = () => {
  emit('height-change', keyboardPanelRef.value?.offsetHeight ?? 0)
}

watch(
  () => props.expanded,
  async (value) => {
    if (value) {
      await nextTick()
      emitPanelHeight()
      resizeObserver.value?.disconnect()
      if (keyboardPanelRef.value) {
        resizeObserver.value = new ResizeObserver(() => {
          emitPanelHeight()
        })
        resizeObserver.value.observe(keyboardPanelRef.value)
      }
      return
    }

    resizeObserver.value?.disconnect()
    resizeObserver.value = undefined
    emit('height-change', 0)
  },
  { immediate: true },
)

onUnmounted(() => {
  window.removeEventListener('keydown', handlePhysicalKeydown, true)
  resizeObserver.value?.disconnect()
})

onMounted(() => {
  window.addEventListener('keydown', handlePhysicalKeydown, true)
})
</script>

<style scoped lang="less">
.keyboard-layer {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 100;
  pointer-events: none;
}

.mobile-keyboard {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 100;
  pointer-events: auto;
  background: var(--n-color);
  border-top: 1px solid var(--n-border-color);
  padding: 10px 8px;
  padding-bottom: calc(8px + var(--bottom-inset, 0px));
  box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.18);
}

.keyboard-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.keyboard-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color);
}

.keyboard-topbar-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.modifier-indicator {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-primary-color) 14%, transparent);
  color: var(--n-primary-color);
  font-size: 12px;
  font-weight: 700;
  border: 1px solid color-mix(in srgb, var(--n-primary-color) 35%, var(--n-border-color));
}

.keyboard-topbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.topbar-button {
  min-width: 32px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--n-border-color);
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-color) 90%, transparent);
  color: var(--n-text-color-2);
  cursor: pointer;
}

.topbar-button-active {
  color: #fff;
  background: var(--n-primary-color);
  border-color: var(--n-primary-color);
}

.topbar-button-clear {
  font-weight: 600;
}

.keyboard-row {
  display: flex;
  justify-content: space-between;
  gap: 4px;
  margin-bottom: 4px;

  &:last-child {
    margin-bottom: 0;
  }
}

.key-button {
  flex: 1;
  min-width: 0;
  height: 44px;
  min-height: 44px;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
  background: var(--n-color);
  color: var(--n-text-color);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
  user-select: none;
  -webkit-tap-highlight-color: transparent;

  &:active {
    transform: scale(0.95);
    background: var(--n-action-color);
  }

  &.key-modifier {
    background: var(--n-action-color);
  }

  &.key-active {
    background: linear-gradient(
      135deg,
      color-mix(in srgb, var(--n-primary-color) 88%, white 12%),
      var(--n-primary-color)
    );
    color: #fff;
    border-color: var(--n-primary-color);
    box-shadow:
      0 0 0 1px color-mix(in srgb, var(--n-primary-color) 25%, transparent),
      0 8px 18px rgba(0, 0, 0, 0.16);
    transform: translateY(-1px);
  }

  &.key-more {
    flex: 0.6;
    font-size: 18px;
    letter-spacing: 2px;
  }

  &.key-backspace {
    flex: 0.8;
    font-size: 18px;
  }

  &.key-small {
    height: 38px;
    font-size: 12px;
  }

  &.key-command {
    font-family: monospace;
    font-size: 11px;
  }
}

.key-icon {
  font-size: 16px;
}

.key-label {
  font-size: inherit;
}

// 抽屉样式
.keyboard-drawer {
  position: absolute;
  bottom: calc(100% - 2px);
  left: 8px;
  right: 8px;
  background: var(--n-color);
  border: 1px solid var(--n-border-color);
  border-bottom: none;
  border-radius: 16px 16px 0 0;
  max-height: 50vh;
  overflow-y: auto;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.15);
}

.drawer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--n-border-color);
  position: sticky;
  top: 0;
  background: var(--n-color);
}

.drawer-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--n-text-color);
}

.drawer-close {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--n-text-color-3);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-size: 16px;

  &:active {
    background: var(--n-action-color);
  }
}

.drawer-content {
  padding: 12px;
}

.drawer-section {
  margin-bottom: 16px;

  &:last-child {
    margin-bottom: 0;
  }
}

.section-label {
  display: block;
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-bottom: 8px;
  font-weight: 500;
}

.drawer-keys {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.keyboard-fab {
  position: fixed;
  right: 16px;
  bottom: calc(12px + var(--bottom-inset, 0px));
  height: 44px;
  padding: 0 14px;
  border: 1px solid color-mix(in srgb, var(--n-primary-color) 28%, var(--n-border-color));
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-color) 80%, var(--n-primary-color) 20%);
  color: var(--n-text-color);
  box-shadow: 0 10px 24px rgba(0, 0, 0, 0.18);
  cursor: pointer;
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  justify-content: center;
  transition:
    transform 0.2s ease,
    bottom 0.2s ease,
    opacity 0.2s ease,
    background 0.2s ease,
    color 0.2s ease,
    border-color 0.2s ease;
}

.keyboard-fab-active {
  bottom: calc(126px + var(--bottom-inset, 0px));
  background: var(--n-primary-color);
  color: #fff;
  border-color: var(--n-primary-color);
}

.keyboard-fab:active {
  transform: scale(0.96);
}

.keyboard-fab-icon {
  font-size: 16px;
  line-height: 1;
}

.keyboard-fab-label {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.panel-enter-active,
.panel-leave-active {
  transition:
    transform 0.2s ease,
    opacity 0.2s ease;
}

.panel-enter-from,
.panel-leave-to {
  transform: translateY(100%);
  opacity: 0;
}

// 抽屉动画
.drawer-enter-active,
.drawer-leave-active {
  transition:
    transform 0.2s ease,
    opacity 0.2s ease;
}

.drawer-enter-from,
.drawer-leave-to {
  transform: translateY(20px);
  opacity: 0;
}
</style>
