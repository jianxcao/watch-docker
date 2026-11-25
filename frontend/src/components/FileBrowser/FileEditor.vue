<template>
  <n-modal
    :show="show"
    :mask-closable="!hasChanges"
    :close-on-esc="false"
    preset="card"
    class="file-editor-modal"
    style="width: 90vw; max-width: 1200px"
    @update:show="(val) => !val && handleClose()"
  >
    <template #header>
      <div class="flex items-center gap-2">
        <n-button text @click="handleClose" title="关闭编辑器">
          <template #icon>
            <n-icon><ArrowBack /></n-icon>
          </template>
        </n-button>
        <span>{{ fileName }}</span>
        <span v-if="hasChanges" class="text-warning">●</span>
        <span v-if="readOnly" class="text-xs text-gray-500">(只读)</span>
      </div>
    </template>

    <template #header-extra>
      <n-space>
        <n-button
          v-if="isEditable && hasChanges"
          type="primary"
          :loading="saving"
          @click="saveFile"
        >
          <template #icon>
            <n-icon><Save /></n-icon>
          </template>
          保存
        </n-button>

        <n-button @click="downloadFile">
          <template #icon>
            <n-icon><Download /></n-icon>
          </template>
          下载
        </n-button>
      </n-space>
    </template>

    <n-spin :show="loading">
      <div class="file-editor-content">
        <n-alert
          v-if="error"
          type="error"
          :title="error"
          closable
          @close="error = null"
          class="mb-4"
        />

        <div
          ref="editorContainer"
          class="editor-container"
          :class="{ 'editor-readonly': !isEditable }"
        />
      </div>
    </n-spin>

    <template #footer>
      <div class="flex justify-between items-center text-sm text-gray-500">
        <span>{{ filePath }}</span>
        <span v-if="isEditable"
          >快捷键: <kbd>{{ isMac ? 'Cmd' : 'Ctrl' }}+S</kbd> 保存, <kbd>ESC</kbd> 关闭
        </span>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { getContainerFileContent, updateContainerFileContent } from '@/common/api'
import { downloadContainerFile } from './utils'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import { oneDark } from '@codemirror/theme-one-dark'
import { ArrowBack, Download, Save } from '@vicons/ionicons5'
import { EditorView, basicSetup } from 'codemirror'
import { NAlert, NButton, NIcon, NModal, NSpace, NSpin, useMessage } from 'naive-ui'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

interface FileEditorProps {
  show: boolean
  containerId: string
  filePath: string
  fileName: string
  readOnly?: boolean
}

const props = withDefaults(defineProps<FileEditorProps>(), {
  readOnly: false,
})

const emit = defineEmits<{
  'update:show': [value: boolean]
  close: []
  saved: []
}>()

const message = useMessage()

// 状态
const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const content = ref('')
const editorView = shallowRef<EditorView | null>(null)
const editorContainer = ref<HTMLElement | null>(null)
const hasChanges = ref(false)

// 语言配置
const languageConf = new Compartment()
const readOnlyConf = new Compartment()

// 按需懒加载 CodeMirror 语言扩展，避免将所有语法包打入初始包
const loadLanguageSupport = async (filename: string): Promise<Extension> => {
  const ext = filename.trimEnd().split('.').pop()?.toLowerCase() || ''
  switch (ext) {
    case 'js':
    case 'mjs':
    case 'cjs':
    case 'sh':
    case 'bash':
    case 'conf':
    case 'config':
    case 'ini':
    case 'env':
    case 'dockerfile':
    case 'go': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return javascript()
    }
    case 'jsx': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return javascript({ jsx: true })
    }
    case 'ts': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return javascript({ typescript: true })
    }
    case 'tsx': {
      const { javascript } = await import('@codemirror/lang-javascript')
      return javascript({ typescript: true, jsx: true })
    }
    case 'html':
    case 'htm': {
      const { html } = await import('@codemirror/lang-html')
      return html()
    }
    case 'xml':
    case 'svg': {
      const { xml } = await import('@codemirror/lang-xml')
      return xml()
    }
    case 'css':
    case 'scss':
    case 'sass':
    case 'less': {
      const { css } = await import('@codemirror/lang-css')
      return css()
    }
    case 'json': {
      const { json } = await import('@codemirror/lang-json')
      return json()
    }
    case 'yaml':
    case 'yml': {
      const { yaml } = await import('@codemirror/lang-yaml')
      return yaml()
    }
    case 'md':
    case 'markdown': {
      const { markdown } = await import('@codemirror/lang-markdown')
      return markdown()
    }
    case 'py':
    case 'pyw': {
      const { python } = await import('@codemirror/lang-python')
      return python()
    }
    case 'php': {
      const { php } = await import('@codemirror/lang-php')
      return php()
    }
    case 'sql': {
      const { sql } = await import('@codemirror/lang-sql')
      return sql()
    }
    case 'c':
    case 'cpp':
    case 'cc':
    case 'cxx':
    case 'h':
    case 'hpp': {
      const { cpp } = await import('@codemirror/lang-cpp')
      return cpp()
    }
    case 'java': {
      const { java } = await import('@codemirror/lang-java')
      return java()
    }
    case 'rs': {
      const { rust } = await import('@codemirror/lang-rust')
      return rust()
    }
    default:
      return []
  }
}

// 文件类型检测（移除 svg，作为 XML 文本可编辑）
const isBinaryFile = (filename: string): boolean => {
  const binaryExts = [
    'jpg',
    'jpeg',
    'png',
    'gif',
    'bmp',
    'ico',
    'zip',
    'tar',
    'gz',
    'bz2',
    '7z',
    'rar',
    'pdf',
    'doc',
    'docx',
    'xls',
    'xlsx',
    'exe',
    'dll',
    'so',
    'dylib',
    'mp3',
    'mp4',
    'avi',
    'mov',
    'wav',
  ]
  const ext = filename.trimEnd().split('.').pop()?.toLowerCase() || ''
  return binaryExts.includes(ext)
}

const isEditable = computed(() => {
  return !props.readOnly && !isBinaryFile(props.fileName)
})

const isMac = computed(() => {
  return typeof window !== 'undefined' && window.navigator.platform.includes('Mac')
})

// 加载请求序号（防止异步加载期间切换文件/关闭后产生竞态覆盖）
let loadRequestId = 0

// 加载文件内容
const loadFile = async () => {
  if (!props.filePath || !props.show) {
    return
  }

  const requestId = ++loadRequestId
  error.value = null
  hasChanges.value = false

  if (editorView.value) {
    editorView.value.destroy()
    editorView.value = null
  }

  // 检查是否为二进制文件
  if (isBinaryFile(props.fileName)) {
    loading.value = false
    content.value = ''
    error.value = '此文件为二进制文件，不支持在线预览。请下载后查看。'
    return
  }

  loading.value = true

  try {
    const res = await getContainerFileContent(props.containerId, props.filePath)
    if (requestId !== loadRequestId || !props.show) {
      return
    }

    if (res.code === 0 && res.data) {
      content.value = res.data.content ?? ''
      hasChanges.value = false
      await initEditor(requestId)
    } else {
      const errorMsg = res.msg || '加载文件失败'
      console.error('Load file failed:', errorMsg, 'code:', res.code)
      throw new Error(errorMsg)
    }
  } catch (err: any) {
    if (requestId !== loadRequestId) {
      return
    }
    console.error('Load file error:', err)
    let errorMessage = err.message || '加载文件失败'

    // 检查是否是文件过大的错误
    if (errorMessage.includes('too large') || errorMessage.includes('too big')) {
      errorMessage = `文件太大无法在线编辑（限制 1MB）\n建议使用下载功能查看完整内容`
    }

    error.value = errorMessage
    content.value = ''
  } finally {
    if (requestId === loadRequestId) {
      loading.value = false
    }
  }
}

// 初始化编辑器
const initEditor = async (requestId: number) => {
  if (!editorContainer.value) {
    return
  }

  const language = await loadLanguageSupport(props.fileName)
  if (requestId !== loadRequestId || !props.show || !editorContainer.value) {
    return
  }

  // 清理旧编辑器
  if (editorView.value) {
    editorView.value.destroy()
    editorView.value = null
  }

  const state = EditorState.create({
    doc: content.value,
    extensions: [
      basicSetup,
      oneDark,
      languageConf.of(language),
      readOnlyConf.of(EditorState.readOnly.of(!isEditable.value)),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !props.readOnly) {
          hasChanges.value = true
        }
      }),
    ],
  })

  editorView.value = new EditorView({
    state,
    parent: editorContainer.value,
  })
}

// 保存文件
const saveFile = async () => {
  if (!editorView.value || props.readOnly) {
    return
  }

  saving.value = true
  error.value = null

  try {
    const newContent = editorView.value.state.doc.toString()
    const res = await updateContainerFileContent(props.containerId, props.filePath, newContent)

    if (res.code === 0) {
      message.success('文件保存成功')
      hasChanges.value = false
      content.value = newContent
      emit('saved')
    } else {
      throw new Error(res.msg || '保存失败')
    }
  } catch (err: any) {
    const errorMsg = err.message || '保存失败'
    error.value = errorMsg
    message.error(errorMsg)
  } finally {
    saving.value = false
  }
}

// 下载文件
const downloadFile = async () => {
  try {
    await downloadContainerFile(props.containerId, props.filePath, props.fileName)
    message.success('开始下载')
  } catch (err: any) {
    message.error(err.message || '下载失败')
  }
}

// 关闭对话框
const handleClose = () => {
  if (hasChanges.value) {
    const confirmed = window.confirm('文件有未保存的更改，确定要关闭吗？')
    if (!confirmed) {
      return
    }
  }

  loadRequestId++
  loading.value = false

  emit('update:show', false)
  emit('close')

  // 清理编辑器
  if (editorView.value) {
    editorView.value.destroy()
    editorView.value = null
  }

  hasChanges.value = false
  error.value = null
}

// 监听显示状态与目标文件变化
watch(
  [() => props.show, () => props.containerId, () => props.filePath],
  ([show]) => {
    if (show) {
      void loadFile()
    } else {
      loadRequestId++
      loading.value = false
      if (editorView.value) {
        editorView.value.destroy()
        editorView.value = null
      }
      hasChanges.value = false
      error.value = null
    }
  },
)

// 键盘快捷键（仅在编辑器弹窗打开时生效）
const handleKeydown = (e: KeyboardEvent) => {
  if (!props.show) {
    return
  }
  // Cmd+S / Ctrl+S 保存
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    if (isEditable.value && hasChanges.value) {
      saveFile()
    }
  }
  // ESC 关闭
  if (e.key === 'Escape') {
    handleClose()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (editorView.value) {
    editorView.value.destroy()
    editorView.value = null
  }
})
</script>

<style scoped lang="less">
.file-editor-content {
  height: calc(80vh - 120px);
  min-height: 400px;
  overflow: hidden;
}

.editor-container {
  height: 100%;
  overflow: auto;

  :deep(.cm-editor) {
    height: 100%;
    font-size: 14px;
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
  }

  :deep(.cm-scroller) {
    overflow: auto;
  }
}

.editor-readonly {
  :deep(.cm-cursor) {
    display: none;
  }
}

kbd {
  padding: 2px 6px;
  border: 1px solid #ccc;
  border-radius: 3px;
  background-color: #f7f7f7;
  font-family: monospace;
  font-size: 12px;
}
</style>

<style lang="less">
@import '@/styles/mix.less';
.file-editor-modal {
  &.n-modal {
    @supports (backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)) {
      background-color: color-mix(in srgb, var(--n-color) 30%, transparent);
      backdrop-filter: blur(20px) brightness(95%);
    }
  }
  .n-card-header {
    padding: 8px 20px;
  }
  .n-card__content {
    padding: 0 8px 8px 8px;
  }
  .cm-scroller {
    .scrollbar();
  }
}
</style>
