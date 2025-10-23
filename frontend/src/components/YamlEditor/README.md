# YamlEditor 组件

一个基于 CodeMirror 6 的 YAML 编辑器组件，支持语法高亮、明暗主题切换和移动端优化。

## 文件结构

```
YamlEditor/
├── index.vue      # 主组件
├── theme.ts       # 主题配置（语法高亮和编辑器样式）
├── types.ts       # TypeScript 类型定义
└── README.md      # 本文档
```

## 使用方法

### 基础用法

```vue
<template>
  <YamlEditor
    v-model="yamlContent"
    placeholder="请输入 YAML 配置..."
    min-height="300px"
    max-height="500px"
    @change="handleChange"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import YamlEditor from '@/components/YamlEditor/index.vue'

const yamlContent = ref('')

const handleChange = (value: string) => {
  console.log('YAML 内容变化:', value)
}
</script>
```

### Props

| 属性          | 类型      | 默认值                  | 说明                      |
| ------------- | --------- | ----------------------- | ------------------------- |
| `modelValue`  | `string`  | -                       | YAML 内容（支持 v-model） |
| `placeholder` | `string`  | `'请输入 YAML 配置...'` | 占位符文本                |
| `readonly`    | `boolean` | `false`                 | 是否只读                  |
| `minHeight`   | `string`  | `'300px'`               | 最小高度                  |
| `maxHeight`   | `string`  | `'500px'`               | 最大高度                  |

### Events

| 事件                | 参数              | 说明           |
| ------------------- | ----------------- | -------------- |
| `update:modelValue` | `(value: string)` | 内容变化时触发 |
| `change`            | `(value: string)` | 内容变化时触发 |

### 暴露的方法

```typescript
// 通过 ref 访问组件实例
const editorRef = ref<YamlEditorExpose>()

// 聚焦编辑器
editorRef.value?.focus()

// 获取当前内容
const content = editorRef.value?.getValue()

// 设置内容
editorRef.value?.setValue('version: "3.8"')
```

| 方法              | 参数     | 返回值   | 说明         |
| ----------------- | -------- | -------- | ------------ |
| `focus()`         | -        | `void`   | 聚焦编辑器   |
| `getValue()`      | -        | `string` | 获取当前内容 |
| `setValue(value)` | `string` | `void`   | 设置内容     |

## 主题系统

### 主题切换

编辑器会自动根据 Naive UI 的主题设置切换明暗主题。

- **明亮主题**: GitHub Light 配色风格
- **暗黑主题**: One Dark 配色风格

### 自定义主题

如需自定义主题，可以修改 `theme.ts` 文件：

```typescript
// theme.ts

// 修改明亮主题配色
export const createLightHighlightStyle = () => {
  return HighlightStyle.define([
    { tag: t.keyword, color: '#your-color' },
    // ... 更多配置
  ])
}

// 修改暗黑主题配色
export const createDarkHighlightStyle = () => {
  return HighlightStyle.define([
    { tag: t.keyword, color: '#your-color' },
    // ... 更多配置
  ])
}
```

### 语法高亮标签

支持的 Lezer 标签：

- `t.keyword` - 关键字
- `t.propertyName` - 属性名
- `t.string` - 字符串
- `t.number` - 数字
- `t.comment` - 注释
- `t.operator` - 操作符
- `t.bool` - 布尔值
- `t.className` - 类名
- `t.typeName` - 类型名
- `t.invalid` - 无效语法

完整标签列表参见：[@lezer/highlight 文档](https://lezer.codemirror.net/docs/ref/#highlight.tags)

## 快捷键

### 🔍 搜索和导航

- `Cmd+F` / `Ctrl+F` - 打开搜索面板（优先于浏览器搜索）
- `Cmd+G` / `Ctrl+G` - 查找下一个
- `Shift+Cmd+G` / `Shift+Ctrl+G` - 查找上一个
- `Cmd+H` / `Ctrl+H` - 打开替换功能
- `Escape` - 关闭搜索面板

### ✏️ 编辑操作

- `Cmd+Z` / `Ctrl+Z` - 撤销
- `Cmd+Shift+Z` / `Ctrl+Y` - 重做
- `Tab` - 缩进
- `Shift+Tab` - 减少缩进

> **💡 提示**：编辑器会拦截 `Cmd+F`/`Ctrl+F` 快捷键，确保触发编辑器内的搜索功能而非浏览器搜索。搜索面板已优化到顶部，样式与应用整体风格统一，提供更好的编辑体验。

## 特性

### ✅ 已实现

- [x] 完整的 YAML 语法高亮
- [x] 明暗主题自动切换
- [x] 行号显示
- [x] 当前行高亮
- [x] 选择区域高亮
- [x] 括号匹配和自动闭合
- [x] 代码折叠
- [x] 自动补全
- [x] 搜索和替换功能（顶部面板，快捷键优先拦截）
- [x] 只读模式
- [x] 自动换行
- [x] 响应式设计
- [x] 移动端触摸优化

### 🎨 样式特性

- 自定义光标颜色（使用主题色）
- 聚焦时边框高亮
- 平滑的主题切换
- 统一的圆角和间距

### 📱 移动端优化

- 触摸友好的滚动
- 自适应字体大小
- 合理的行号宽度
- 虚拟键盘适配

## 依赖

```json
{
  "codemirror": "^6.0.2",
  "@codemirror/lang-yaml": "^6.1.2",
  "@codemirror/view": "^6.38.6",
  "@codemirror/state": "^6.5.2",
  "@codemirror/language": "^6.11.3",
  "@codemirror/commands": "^6.9.0",
  "@codemirror/search": "^6.5.11",
  "@codemirror/autocomplete": "^6.19.0",
  "@codemirror/commands": "^6.9.0",
  "@lezer/highlight": "^1.2.2"
}
```

## 性能优化

1. **按需加载**: 只加载必要的语言支持
2. **虚拟渲染**: CodeMirror 6 的虚拟 DOM
3. **增量更新**: 只更新变化部分
4. **主题缓存**: 使用 Compartment 避免重复创建

## 开发指南

### 添加新的语法高亮规则

编辑 `theme.ts`：

```typescript
export const createLightHighlightStyle = () => {
  return HighlightStyle.define([
    // 添加新规则
    { tag: t.yourTag, color: '#color', fontStyle: 'italic' },
    // ... 现有规则
  ])
}
```

### 修改编辑器样式

编辑 `theme.ts` 的 `createEditorTheme` 函数：

```typescript
export const createEditorTheme = (...) => {
  return EditorView.theme({
    // 修改或添加样式
    '.cm-yourClass': {
      color: 'red',
    },
  }, { dark: isDarkMode })
}
```

### 添加新功能

在 `index.vue` 中添加新的扩展：

```typescript
EditorState.create({
  extensions: [
    // 现有扩展...
    yourNewExtension(),
  ],
})
```

## 故障排查

### 主题不切换

检查 `useSettingStore` 是否正确返回主题状态：

```typescript
const isDark = computed(() => settingStore.setting.theme === 'dark')
```

### 语法高亮不工作

确保安装了所有依赖：

```bash
pnpm install @codemirror/language @lezer/highlight
```

### 移动端输入问题

检查是否正确处理触摸事件：

```typescript
EditorView.domEventHandlers({
  touchstart: () => false,
})
```

## 相关文档

- [CodeMirror 6 官方文档](https://codemirror.net/docs/)
- [Lezer 高亮系统](https://lezer.codemirror.net/docs/ref/#highlight)
- [One Dark 主题参考](https://github.com/codemirror/theme-one-dark)
- [项目主题实现文档](../../../doc/yaml-editor-theme.md)

## 贡献

欢迎贡献改进！请确保：

1. 代码符合项目的 ESLint 规范
2. 添加必要的注释和类型定义
3. 测试明暗主题下的显示效果
4. 在移动端测试功能正常

## 许可

遵循项目整体许可协议。
