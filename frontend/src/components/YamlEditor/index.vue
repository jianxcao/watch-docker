<template>
  <div ref="editorContainer" class="yaml-editor-container"></div>
</template>

<script setup lang="ts">
defineOptions({
  name: 'YamlEditor',
})

import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import {
  EditorView,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
} from '@codemirror/view'
import { EditorState, Compartment } from '@codemirror/state'
import {
  defaultHighlightStyle,
  syntaxHighlighting,
  indentOnInput,
  bracketMatching,
  foldGutter,
} from '@codemirror/language'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import {
  closeBrackets,
  closeBracketsKeymap,
  autocompletion,
  completionKeymap,
} from '@codemirror/autocomplete'
import { keymap } from '@codemirror/view'
import { yaml } from '@codemirror/lang-yaml'
import { useThemeVars } from 'naive-ui'
import { useSettingStore } from '@/store/setting'
import { createThemeExtensions } from './theme'
import type { YamlEditorProps, YamlEditorEmits, YamlEditorExpose } from './types'

const props = withDefaults(defineProps<YamlEditorProps>(), {
  placeholder: '请输入 YAML 配置...',
  readonly: false,
  minHeight: '300px',
  maxHeight: '500px',
})

const emit = defineEmits<YamlEditorEmits>()

// 响应式状态
const settingStore = useSettingStore()
const editorContainer = ref<HTMLElement>()
const themeVars = useThemeVars()
const isDark = computed(() => settingStore.setting.theme === 'dark')

// 编辑器实例和配置
let editorView: EditorView | null = null
let themeCompartment: Compartment
let readOnlyCompartment: Compartment

/**
 * 初始化编辑器
 */
const initEditor = () => {
  if (!editorContainer.value) {
    return
  }

  themeCompartment = new Compartment()
  readOnlyCompartment = new Compartment()

  const startState = EditorState.create({
    doc: props.modelValue || '',
    extensions: [
      // 行号
      lineNumbers(),
      // 代码折叠
      foldGutter(),
      // 历史记录（撤销/重做）
      history(),
      // 括号匹配
      bracketMatching(),
      // 自动闭合括号
      closeBrackets(),
      // 自动补全
      autocompletion(),
      // 高亮当前行
      highlightActiveLine(),
      highlightActiveLineGutter(),
      // 高亮选中匹配
      highlightSelectionMatches(),
      // 搜索功能（支持 Cmd+F / Ctrl+F）
      search({
        top: true, // 将搜索面板放在顶部而不是底部
      }),
      // 缩进输入
      indentOnInput(),
      // 默认语法高亮
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      // 键盘映射（searchKeymap 需要在 defaultKeymap 之前，以优先捕获搜索快捷键）
      keymap.of([
        ...searchKeymap,
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...historyKeymap,
        ...completionKeymap,
        indentWithTab,
      ]),
      // YAML 语言支持
      yaml(),
      // 主题扩展
      themeCompartment.of(
        createThemeExtensions(isDark.value, themeVars.value, props.minHeight, props.maxHeight),
      ),
      // 只读模式
      readOnlyCompartment.of(EditorState.readOnly.of(props.readonly)),
      // 内容变化监听
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !props.readonly) {
          const value = update.state.doc.toString()
          emit('update:modelValue', value)
          emit('change', value)
        }
      }),
      // 占位符
      EditorView.contentAttributes.of({
        'aria-placeholder': props.placeholder,
      }),
      // 自动换行（移动端优化）
      EditorView.lineWrapping,
      // 键盘事件处理（阻止浏览器默认搜索）
      EditorView.domEventHandlers({
        touchstart: () => {
          // 移动端触摸优化
          return false
        },
      }),
    ],
  })

  editorView = new EditorView({
    state: startState,
    parent: editorContainer.value,
  })
}

/**
 * 更新主题
 */
const updateTheme = () => {
  if (!editorView) {
    return
  }
  editorView.dispatch({
    effects: themeCompartment.reconfigure(
      createThemeExtensions(isDark.value, themeVars.value, props.minHeight, props.maxHeight),
    ),
  })
}

/**
 * 更新编辑器内容
 */
const updateContent = (newValue: string) => {
  if (!editorView) {
    return
  }
  const currentValue = editorView.state.doc.toString()
  if (newValue !== currentValue) {
    editorView.dispatch({
      changes: {
        from: 0,
        to: editorView.state.doc.length,
        insert: newValue || '',
      },
    })
  }
}

/**
 * 更新只读状态
 */
const updateReadonly = (readonly: boolean) => {
  if (!editorView) {
    return
  }
  editorView.dispatch({
    effects: readOnlyCompartment.reconfigure(EditorState.readOnly.of(readonly)),
  })
}

/**
 * 聚焦编辑器
 */
const focus = () => {
  editorView?.focus()
}

/**
 * 获取编辑器内容
 */
const getValue = () => {
  return editorView?.state.doc.toString() || ''
}

/**
 * 设置编辑器内容
 */
const setValue = (value: string) => {
  if (!editorView) {
    return
  }
  editorView.dispatch({
    changes: {
      from: 0,
      to: editorView.state.doc.length,
      insert: value,
    },
  })
}

/**
 * 销毁编辑器
 */
const destroy = () => {
  if (editorView) {
    editorView.destroy()
    editorView = null
  }
}

// 生命周期
onMounted(() => {
  initEditor()
})

onBeforeUnmount(() => {
  destroy()
})

// 监听器
watch([isDark, () => props.minHeight, () => props.maxHeight], () => {
  updateTheme()
})

watch(
  () => props.modelValue,
  (newValue) => {
    updateContent(newValue)
  },
)

watch(
  () => props.readonly,
  (newReadonly) => {
    updateReadonly(newReadonly)
  },
)

// 暴露方法
defineExpose<YamlEditorExpose>({
  focus,
  getValue,
  setValue,
})
</script>

<style scoped lang="less">
@import '@/styles/mix.less';
.yaml-editor-container {
  width: 100%;
  height: 100%;
  overflow: auto;

  :deep(.cm-editor) {
    outline: none;

    &.cm-focused {
      outline: none;
      border-color: v-bind('themeVars.primaryColor');
    }
  }

  :deep(.cm-scroller) {
    overflow: auto;
    .scrollbar();
  }

  // 移动端优化
  @media (max-width: 768px) {
    :deep(.cm-content) {
      font-size: 13px;
    }

    :deep(.cm-gutters) {
      min-width: 28px;
    }

    // 搜索面板移动端优化
    :deep(.cm-panel.cm-search) {
      padding: 6px 8px;
      font-size: 12px;

      input {
        font-size: 12px;
        padding: 3px 6px;
      }

      button {
        font-size: 12px;
        padding: 3px 8px;
      }
    }
  }
}
</style>
