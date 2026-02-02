<template>
  <n-modal
    :show="show"
    :mask-closable="!hasChanges"
    preset="card"
    class="file-editor-modal"
    style="width: 90vw; max-width: 1200px"
    @update:show="(val) => !val && handleClose()"
  >
    <template #header>
      <div class="flex items-center gap-2">
        <n-icon><ArrowBack /></n-icon>
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
          >wan 快捷键: <kbd>{{ isMac ? 'Cmd' : 'Ctrl' }}+S</kbd> 保存, <kbd>ESC</kbd> 关闭
        </span>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { getContainerFileContent, updateContainerFileContent } from '@/common/api'
import { downloadContainerFile } from './utils'
import { cpp } from '@codemirror/lang-cpp'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { java } from '@codemirror/lang-java'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { markdown } from '@codemirror/lang-markdown'
import { php } from '@codemirror/lang-php'
import { python } from '@codemirror/lang-python'
import { rust } from '@codemirror/lang-rust'
import { sql } from '@codemirror/lang-sql'
import { xml } from '@codemirror/lang-xml'
import { yaml } from '@codemirror/lang-yaml'
import { Compartment, EditorState } from '@codemirror/state'
import { oneDark } from '@codemirror/theme-one-dark'
import { ArrowBack, Download, Save } from '@vicons/ionicons5'
import { EditorView, basicSetup } from 'codemirror'
import { NAlert, NButton, NIcon, NModal, NSpace, NSpin, useMessage } from 'naive-ui'
import { computed, onMounted, ref, shallowRef, watch } from 'vue'

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

// 根据文件扩展名获取语言支持
const getLanguageSupport = (filename: string) => {
  const ext = filename.split('.').pop()?.toLowerCase() || ''

  const languageMap: Record<string, any> = {
    // JavaScript/TypeScript
    js: javascript(),
    jsx: javascript({ jsx: true }),
    ts: javascript({ typescript: true }),
    tsx: javascript({ typescript: true, jsx: true }),
    mjs: javascript(),
    cjs: javascript(),

    // HTML/XML
    html: html(),
    htm: html(),
    xml: xml(),
    svg: xml(),

    // CSS
    css: css(),
    scss: css(),
    sass: css(),
    less: css(),

    // JSON/YAML
    json: json(),
    yaml: yaml(),
    yml: yaml(),

    // Markdown
    md: markdown(),
    markdown: markdown(),

    // Python
    py: python(),
    pyw: python(),

    // PHP
    php: php(),

    // SQL
    sql: sql(),

    // C/C++
    c: cpp(),
    cpp: cpp(),
    cc: cpp(),
    cxx: cpp(),
    h: cpp(),
    hpp: cpp(),

    // Java
    java: java(),

    // Rust
    rs: rust(),

    // Shell
    sh: javascript(), // 用 js 作为基本高亮
    bash: javascript(),

    // Config files
    conf: javascript(),
    config: javascript(),
    ini: javascript(),
    env: javascript(),

    // Docker
    dockerfile: javascript(),

    // Go
    go: javascript(),
  }

  return languageMap[ext] || []
}

// 文件类型检测
const isBinaryFile = (filename: string): boolean => {
  const binaryExts = [
    'jpg',
    'jpeg',
    'png',
    'gif',
    'bmp',
    'ico',
    'svg',
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
  const ext = filename.split('.').pop()?.toLowerCase() || ''
  return binaryExts.includes(ext)
}

const isEditable = computed(() => {
  return !props.readOnly && !isBinaryFile(props.fileName)
})

const isMac = computed(() => {
  return typeof window !== 'undefined' && window.navigator.platform.includes('Mac')
})

// 加载文件内容
const loadFile = async () => {
  if (!props.filePath || !props.show) {
    return
  }

  // 检查是否为二进制文件
  if (isBinaryFile(props.fileName)) {
    error.value = '此文件为二进制文件，不支持在线预览。请下载后查看。'
    return
  }

  loading.value = true
  error.value = null

  try {
    console.log('Loading file:', props.filePath)
    const res = await getContainerFileContent(props.containerId, props.filePath)
    console.log('File content response:', res)

    if (res.code === 0 && res.data) {
      content.value = res.data.content || ''
      console.log('File content loaded, length:', content.value.length, 'chars')

      // 如果内容为空，显示提示
      if (content.value === '') {
        error.value = '文件内容为空或无法读取'
      }

      hasChanges.value = false
      initEditor()
    } else {
      const errorMsg = res.msg || '加载文件失败'
      console.error('Load file failed:', errorMsg, 'code:', res.code)
      throw new Error(errorMsg)
    }
  } catch (err: any) {
    console.error('Load file error:', err)
    let errorMessage = err.message || '加载文件失败'

    // 检查是否是文件过大的错误
    if (errorMessage.includes('too large') || errorMessage.includes('too big')) {
      errorMessage = `文件太大无法在线编辑（限制 1MB）\n建议使用下载功能查看完整内容`
    }

    error.value = errorMessage
    content.value = ''
  } finally {
    loading.value = false
  }
}

// 初始化编辑器
const initEditor = () => {
  if (!editorContainer.value) {
    return
  }

  // 清理旧编辑器
  if (editorView.value) {
    editorView.value.destroy()
    editorView.value = null
  }

  const language = getLanguageSupport(props.fileName)

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

// 监听显示状态
watch(
  () => props.show,
  (show) => {
    if (show) {
      loadFile()
    }
  },
)

// 键盘快捷键
const handleKeydown = (e: KeyboardEvent) => {
  // Cmd+S / Ctrl+S 保存
  if ((e.metaKey || e.ctrlKey) && e.key === 's') {
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
