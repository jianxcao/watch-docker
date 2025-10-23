# YamlEditor 主题实现说明

## 概述

YamlEditor 组件基于 **CodeMirror 6** 实现，参考了 [@codemirror/theme-one-dark](https://github.com/codemirror/theme-one-dark) 的实现方式，使用 `@lezer/highlight` 库进行语法高亮。

## 技术栈

### 核心依赖

```json
{
  "codemirror": "^6.0.2",
  "@codemirror/lang-yaml": "^6.1.2",
  "@codemirror/view": "^6.38.6",
  "@codemirror/state": "^6.5.2",
  "@codemirror/language": "^6.11.3",
  "@codemirror/commands": "^6.9.0",
  "@codemirror/theme-one-dark": "^6.1.3",
  "@lezer/highlight": "^1.2.2"
}
```

## 主题实现

### 1. 语法高亮样式

使用 `@lezer/highlight` 的 `HighlightStyle.define()` 创建语法高亮：

#### 明亮主题（Light Theme）

```typescript
const createLightHighlightStyle = () => {
  return HighlightStyle.define([
    { tag: t.keyword, color: "#d73a49" }, // 关键字：红色
    { tag: t.propertyName, color: "#6f42c1" }, // 属性名：紫色
    { tag: t.string, color: "#22863a" }, // 字符串：绿色
    { tag: t.number, color: "#005cc5" }, // 数字：蓝色
    { tag: t.comment, color: "#6a737d", fontStyle: "italic" }, // 注释：灰色斜体
    // ... 更多标签样式
  ]);
};
```

#### 暗黑主题（Dark Theme）

参考 one-dark 主题的配色方案：

```typescript
const createDarkHighlightStyle = () => {
  return HighlightStyle.define([
    { tag: t.keyword, color: "#c678dd" }, // 关键字：紫色
    { tag: t.propertyName, color: "#e06c75" }, // 属性名：红色
    { tag: t.string, color: "#98c379" }, // 字符串：绿色
    { tag: t.number, color: "#e5c07b" }, // 数字：金色
    { tag: t.comment, color: "#5c6370", fontStyle: "italic" }, // 注释：暗灰斜体
    // ... 更多标签样式
  ]);
};
```

### 2. 编辑器主题

使用 `EditorView.theme()` 创建编辑器样式：

```typescript
const createEditorTheme = () => {
  const isDarkMode = isDark.value;

  return EditorView.theme(
    {
      "&": {
        backgroundColor: isDarkMode ? "#282c34" : themeVars.value.cardColor,
        color: isDarkMode ? "#abb2bf" : themeVars.value.textColorBase,
        // ... 更多样式
      },
      ".cm-gutters": {
        backgroundColor: isDarkMode
          ? "#21252b"
          : themeVars.value.tableHeaderColor,
        // ... 行号区域样式
      },
      ".cm-activeLine": {
        backgroundColor: isDarkMode ? "#2c313c" : themeVars.value.hoverColor,
        // ... 当前行高亮
      },
      // ... 更多样式
    },
    { dark: isDarkMode }
  );
};
```

### 3. Lezer 高亮标签

使用的 Lezer 标签（来自 `@lezer/highlight`）：

| 标签             | 用途     | 示例                    |
| ---------------- | -------- | ----------------------- | --- |
| `t.keyword`      | 关键字   | `true`, `false`, `null` |
| `t.propertyName` | 属性名   | YAML 中的 key           |
| `t.string`       | 字符串   | `"hello"`, `'world'`    |
| `t.number`       | 数字     | `123`, `3.14`           |
| `t.comment`      | 注释     | `# 这是注释`            |
| `t.operator`     | 操作符   | `:`, `-`, `             | `   |
| `t.bool`         | 布尔值   | `true`, `false`         |
| `t.className`    | 类名     | 服务名称等              |
| `t.typeName`     | 类型名   | 类型定义                |
| `t.invalid`      | 无效语法 | 错误标记                |

## 主题切换

### 动态主题切换实现

使用 `Compartment` 实现主题动态切换：

```typescript
// 创建主题隔间
const themeCompartment = new Compartment();

// 初始化时配置
EditorState.create({
  extensions: [
    themeCompartment.of(createThemeExtensions()),
    // ... 其他扩展
  ],
});

// 监听主题变化
watch(isDark, () => {
  if (!editorView) return;
  editorView.dispatch({
    effects: themeCompartment.reconfigure(createThemeExtensions()),
  });
});
```

### 主题扩展组合

```typescript
const createThemeExtensions = () => {
  const highlightStyle = isDark.value
    ? createDarkHighlightStyle()
    : createLightHighlightStyle();

  return [createEditorTheme(), syntaxHighlighting(highlightStyle)];
};
```

## 配色方案

### 明亮主题配色

基于 GitHub 风格：

- **背景色**: Naive UI 的 `cardColor`
- **文字色**: Naive UI 的 `textColorBase`
- **关键字**: `#d73a49` (红色)
- **字符串**: `#22863a` (绿色)
- **数字**: `#005cc5` (蓝色)
- **注释**: `#6a737d` (灰色)

### 暗黑主题配色

基于 One Dark 风格：

- **背景色**: `#282c34`
- **文字色**: `#abb2bf`
- **关键字**: `#c678dd` (紫色)
- **字符串**: `#98c379` (绿色)
- **数字**: `#e5c07b` (金色)
- **注释**: `#5c6370` (暗灰)

## YAML 语法高亮示例

```yaml
# 这是注释 (comment)
version: "3.8" # version 是属性名，"3.8" 是字符串

services: # services 是属性名
  web: # web 是属性名
    image: nginx:latest # image 是属性名，nginx:latest 是字符串
    ports:
      - "8080:80" # 字符串
    environment:
      - DEBUG=true # DEBUG 是属性名，true 是布尔值
      - PORT=3000 # PORT 是属性名，3000 是数字
    restart: unless-stopped # 属性名和值
```

**高亮效果**：

- `#` 开头的注释 → 灰色斜体
- `version`, `services`, `image` 等键 → 紫色/红色（取决于主题）
- `"3.8"`, `"8080:80"` 等字符串 → 绿色
- `true`, `false` 等布尔值 → 橙色/蓝色
- `3000` 等数字 → 金色/蓝色

## 特性

### ✅ 已实现

- [x] 明暗主题自动切换
- [x] 完整的 YAML 语法高亮
- [x] 行号显示
- [x] 当前行高亮
- [x] 选择区域高亮
- [x] 括号匹配
- [x] 搜索匹配高亮
- [x] 只读模式样式
- [x] 响应式设计
- [x] 移动端优化

### 🎨 样式细节

1. **行号区域**

   - 明亮主题：浅灰背景
   - 暗黑主题：深灰背景（#21252b）

2. **当前行高亮**

   - 明亮主题：使用 Naive UI 的 hoverColor
   - 暗黑主题：使用 #2c313c

3. **选择区域**

   - 明亮主题：主题色半透明
   - 暗黑主题：#3e4451

4. **光标**
   - 使用 Naive UI 的 primaryColor
   - 宽度：2px

## 性能优化

1. **按需加载**: 只加载 YAML 语言支持
2. **虚拟渲染**: CodeMirror 6 使用虚拟 DOM
3. **增量更新**: 只更新变化的部分
4. **主题缓存**: 使用 Compartment 避免重复创建

## 移动端适配

```typescript
// 触摸友好
EditorView.domEventHandlers({
  touchstart: () => false,
})

// 自动换行
EditorView.lineWrapping

// 响应式字体大小
@media (max-width: 768px) {
  .cm-content {
    font-size: 13px;
  }
}
```

## 参考资源

- [CodeMirror 6 官方文档](https://codemirror.net/docs/)
- [One Dark 主题源码](https://github.com/codemirror/theme-one-dark)
- [Lezer 高亮标签](https://lezer.codemirror.net/docs/ref/#highlight.tags)
- [GitHub One Dark 配色](https://github.com/atom/one-dark-syntax)

## 扩展建议

### 未来可以添加的功能

1. **更多主题**

   - Solarized Light/Dark
   - Dracula
   - Material Theme

2. **增强功能**

   - YAML Schema 验证
   - 自动补全
   - 代码折叠
   - 搜索替换

3. **自定义配色**
   - 允许用户自定义配色方案
   - 导入/导出配色主题

## 总结

YamlEditor 组件使用 CodeMirror 6 和 @lezer/highlight 实现了：

- ✅ 完整的语法高亮
- ✅ 明暗主题无缝切换
- ✅ 出色的移动端体验
- ✅ 高性能渲染
- ✅ 美观的配色方案

参考了 one-dark 主题的最佳实践，为用户提供了专业级的 YAML 编辑体验。
