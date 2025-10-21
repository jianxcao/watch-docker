<template>
  <div ref="terminalRef" class="term-wrapper" :style="{ height: props.height }"></div>
</template>

<script setup lang="ts">
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal, type ITerminalOptions, type ITheme } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { onMounted, ref } from 'vue'
import { WebglAddon } from '@xterm/addon-webgl'
import { FitAddon } from '@xterm/addon-fit'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import { useTheme as useTermTheme } from './config'
import { useSettingStore } from '@/store/setting'
import { isMobile, isTablet } from '@/common/utils'
const settingStore = useSettingStore()
const termTheme = useTermTheme(settingStore.setting.theme)

export interface TermConfig {
  theme?: ITheme
  fontSize?: number
  fontFamily?: string
  rows?: number
  cols?: number
  scrollback?: number
  cursorBlink?: boolean
  convertEol?: boolean
  disableStdin?: boolean // 是否禁用输入（日志模式）
}

interface Props {
  config?: TermConfig
  autoFit?: boolean
  height?: string
}

interface Emits {
  (e: 'ready', terminal: Terminal): void
  (e: 'data', data: string): void
  (e: 'resize', size: { cols: number; rows: number }): void
}

const props = withDefaults(defineProps<Props>(), {
  autoFit: true,
})

const emit = defineEmits<Emits>()

const terminalRef = useTemplateRef<HTMLElement>('terminalRef')
const terminal = shallowRef<Terminal>()
const resizeObserver = ref<ResizeObserver>()
const isCleanedUp = ref(false)
// 添加插件
let fitAddon: FitAddon | undefined = new FitAddon()
let webLinksAddon: WebLinksAddon | undefined = new WebLinksAddon()
let webglAddon: WebglAddon | undefined = new WebglAddon()
let clipboardAddon: ClipboardAddon | undefined = new ClipboardAddon()
let unicode11Addon: Unicode11Addon | undefined = new Unicode11Addon()
// 初始化终端
const initTerminal = () => {
  if (!terminalRef.value || isCleanedUp.value) {
    return
  }

  const config: ITerminalOptions = Object.assign(
    {
      theme: termTheme,
      convertEol: true,
      fontSize: 13,
      fontFamily: 'Monaco, Consolas, "Courier New", monospace',
      scrollback: 1000,
      cursorBlink: true,
      disableStdin: false,
      altClickMovesCursor: true,
      // 优化触摸滚动体验
      smoothScrollDuration: 0, // 启用平滑滚动，提供惯性效果
      allowTransparency: true,
      macOptionClickForcesSelection: true,
    },
    props.config,
    {
      allowProposedApi: true,
    },
  )

  // 创建终端实例
  terminal.value = new Terminal(config)
  watchEffect(() => {
    if (terminal.value) {
      terminal.value.options.theme = termTheme
    }
  })

  // 挂载到 DOM
  terminal.value.open(terminalRef.value)

  // terminal.value.loadAddon(webglAddon!)
  terminal.value.loadAddon(clipboardAddon!)
  terminal.value.loadAddon(webLinksAddon!)
  terminal.value.loadAddon(fitAddon!)
  terminal.value.loadAddon(unicode11Addon!)
  terminal.value.unicode.activeVersion = '11'
  terminal.value?.textarea?.setAttribute('enterkeyhint', 'send')
  // 自适应大小
  if (props.autoFit) {
    fitAddon?.fit()

    // 监听窗口大小变化
    resizeObserver.value = new ResizeObserver(() => {
      fitAddon?.fit()
      if (terminal.value) {
        emit('resize', {
          cols: terminal.value.cols,
          rows: terminal.value.rows,
        })
      }
    })
    resizeObserver.value.observe(terminalRef.value)
  }
  // 监听用户输入（用于交互式终端）
  if (!config.disableStdin) {
    terminal.value.onData((data) => {
      emit('data', data)
    })
    // isSupportTouch
    if (isMobile() || isTablet()) {
      terminal.value?.element?.addEventListener('keyup', function () {
        terminal.value?.element?.focus()
      })
      terminal.value.element?.focus()
    } else {
      terminal.value?.focus()
    }
  } else {
    terminal.value.textarea?.setAttribute('disabled', 'disabled')
  }

  watchEffect(() => {
    if (config.disableStdin) {
      terminal.value?.textarea?.setAttribute('disabled', 'disabled')
    } else {
      terminal.value?.textarea?.removeAttribute('disabled')
    }
  })

  // 触发 ready 事件
  emit('ready', terminal.value)
}

// 写入文本
const write = (data: string) => {
  terminal.value?.write(data)
}

// 写入一行
const writeln = (data: string) => {
  terminal.value?.writeln(data)
}

// 清空终端
const clear = () => {
  terminal.value?.clear()
}

// 重置终端
const reset = () => {
  terminal.value?.reset()
}

// 调整大小
const fit = () => {
  fitAddon?.fit()
}

// 滚动到底部
const scrollToBottom = () => {
  terminal.value?.scrollToBottom()
}

// 获取终端实例
const getTerminal = () => {
  return terminal.value
}

// 清理资源
const cleanup = () => {
  // 防止重复清理
  if (isCleanedUp.value) {
    return
  }

  isCleanedUp.value = true
  try {
    // 1. 先断开 ResizeObserver
    if (resizeObserver.value) {
      resizeObserver.value.disconnect()
      resizeObserver.value = undefined
    }

    // 2. 清理 terminal（会自动清理已加载的 addons）
    if (terminal.value) {
      try {
        // Send Ctrl+C to the terminal
        terminal.value?.write('\x03')
        if (fitAddon) {
          fitAddon.dispose()
        }
        if (webLinksAddon) {
          webLinksAddon.dispose()
        }
        if (clipboardAddon) {
          clipboardAddon.dispose()
        }
        if (webglAddon) {
          webglAddon.dispose()
        }
        terminal.value.dispose()
      } catch (e) {
        // 忽略 dispose 错误
        console.warn('Terminal dispose warning:', e)
      }
      terminal.value = undefined
    }

    // 3. 清理 addon 引用
    fitAddon = undefined
    webLinksAddon = undefined
    webglAddon = undefined
    clipboardAddon = undefined
    unicode11Addon = undefined
  } catch (error) {
    console.error('Failed to cleanup terminal:', error)
  }
}

// 组件挂载后初始化
onMounted(() => {
  isCleanedUp.value = false
  initTerminal()
})

// 组件卸载前清理
onUnmounted(() => {
  cleanup()
})

// 暴露方法给父组件
defineExpose({
  write,
  writeln,
  clear,
  reset,
  fit,
  scrollToBottom,
  getTerminal,
})
</script>

<style scoped lang="less">
@import '@/styles/mix.less';

.term-wrapper {
  width: 100%;
  height: 100%;
  // 优化滚动性能
  -webkit-overflow-scrolling: touch;
  overflow: hidden;
  // 允许触摸滚动

  :deep(.xterm-viewport) {
    .scrollbar();
    // 启用硬件加速
    transform: translateZ(0);
    will-change: scroll-position;
    // 优化触摸滚动流畅度
    -webkit-overflow-scrolling: touch;
    overscroll-behavior-y: contain; // 防止过度滚动影响父元素

    overflow: auto;
  }
}
</style>
