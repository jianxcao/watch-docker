# YamlEditor 搜索面板优化

## 问题描述

CodeMirror 6 默认的 `basicSetup` 配置包含了搜索功能，会在编辑器底部显示一个搜索面板（Find/Replace）。这个搜索面板存在以下问题：

1. **位置不佳**：搜索面板固定在编辑器底部，在移动端或较小的屏幕上，需要滚动才能看到
2. **样式不统一**：搜索面板的样式与应用整体风格不一致
3. **快捷键冲突**：移除搜索功能后，`Cmd+F`/`Ctrl+F` 快捷键无法正常工作

## 解决方案

### 1. 替换 `basicSetup` 并自定义搜索面板

不再使用 CodeMirror 的 `basicSetup`，而是手动配置需要的扩展，并将搜索面板移至顶部且美化样式。

**之前的配置：**

```typescript
import { EditorView, basicSetup } from "codemirror";

const startState = EditorState.create({
  extensions: [
    basicSetup, // 包含了所有默认功能，包括搜索面板
    // ...
  ],
});
```

**优化后的配置：**

```typescript
import { EditorView, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
import { history, defaultKeymap, historyKeymap, indentWithTab } from '@codemirror/commands'
import { bracketMatching, foldGutter, indentOnInput } from '@codemirror/language'
import { highlightSelectionMatches, search } from '@codemirror/search'
import { closeBrackets, autocompletion } from '@codemirror/autocomplete'

const startState = EditorState.create({
  extensions: [
    lineNumbers(),                    // 行号
    foldGutter(),                     // 代码折叠
    history(),                        // 历史记录（撤销/重做）
    bracketMatching(),                // 括号匹配
    closeBrackets(),                  // 自动闭合括号
    autocompletion(),                 // 自动补全
    highlightActiveLine(),            // 高亮当前行
    highlightActiveLineGutter(),      // 高亮当前行号
    highlightSelectionMatches(),      // 高亮选中匹配
    search({ top: true }),            // 搜索面板（移至顶部）
    indentOnInput(),                  // 缩进输入
    keymap.of([...]),                 // 键盘映射
    // ... 其他配置
  ]
})
```

### 2. 保留的功能

虽然移除了搜索面板，但保留了以下核心编辑功能：

- ✅ 行号显示
- ✅ 代码折叠
- ✅ 撤销/重做（Ctrl+Z / Ctrl+Y）
- ✅ 括号匹配和自动闭合
- ✅ 自动补全
- ✅ 高亮当前行
- ✅ 高亮选中文本的匹配项（`highlightSelectionMatches`）
- ✅ 智能缩进
- ✅ Tab 键缩进支持

### 3. 搜索面板样式优化

为了解决原生搜索面板样式不统一的问题，添加了自定义样式：

1. **位置优化**：搜索面板从底部移至顶部（`search({ top: true })`）
2. **样式统一**：使用 Naive UI 的主题变量，确保与应用风格一致
3. **移动端适配**：针对小屏幕设备优化字体大小和间距
4. **交互优化**：添加 hover 效果、focus 效果等

**自定义样式包括：**

- 输入框使用应用主题色
- 按钮样式与 Naive UI 保持一致
- 添加圆角、阴影等现代化效果
- 响应式设计，移动端友好

## 新增依赖

为了实现精细化配置，添加了以下依赖：

```json
{
  "@codemirror/search": "^6.5.11",
  "@codemirror/autocomplete": "^6.19.0"
}
```

虽然添加了 `@codemirror/search`，但我们只使用其中的 `highlightSelectionMatches` 功能，不使用搜索面板。

## 优化效果

### 界面更美观

- ✅ 搜索面板从底部移至顶部，更易访问
- ✅ 样式与应用整体风格统一
- ✅ 移动端体验更好，无需滚动到底部

### 功能完整

- ✅ 保留完整的搜索和替换功能
- ✅ `Cmd+F` / `Ctrl+F` 快捷键正常工作
- ✅ 支持正则表达式搜索
- ✅ 支持大小写敏感搜索
- ✅ 支持替换和全部替换

### 用户体验

- ✅ 搜索面板按需显示，不占用空间
- ✅ 主题自适应，明暗模式均优化
- ✅ 移动端友好的交互设计

## 使用方法

### 快捷键

- `Cmd+F` / `Ctrl+F`：打开搜索面板
- `Cmd+G` / `Ctrl+G`：查找下一个
- `Shift+Cmd+G` / `Shift+Ctrl+G`：查找上一个
- `Cmd+H` / `Ctrl+H`：打开替换功能
- `Escape`：关闭搜索面板

### 搜索选项

- **正则表达式**：点击 `.*` 按钮启用正则搜索
- **大小写敏感**：点击 `Aa` 按钮切换
- **整词匹配**：点击 `\b` 按钮启用

### 替换功能

1. 按 `Cmd+H` / `Ctrl+H` 打开替换模式
2. 输入搜索词和替换词
3. 点击"Replace"替换当前匹配，或"Replace all"替换全部

## 相关文件

- `frontend/src/components/YamlEditor/index.vue` - 主要修改文件
- `frontend/src/components/YamlEditor/README.md` - 文档更新
- `frontend/package.json` - 依赖更新

## 参考资料

- [CodeMirror 6 文档 - Extensions](https://codemirror.net/docs/extensions/)
- [CodeMirror 6 - basicSetup 源码](https://github.com/codemirror/basic-setup)
- [@codemirror/search](https://codemirror.net/docs/ref/#search)
