import { EditorView } from 'codemirror'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import type { ThemeCommonVars, CustomThemeCommonVars } from 'naive-ui'

/**
 * 创建明亮主题的语法高亮样式
 * 配色参考 GitHub Light 主题
 */
export const createLightHighlightStyle = () => {
  return HighlightStyle.define([
    // 关键字
    { tag: t.keyword, color: '#d73a49' },
    // 属性名、变量名
    { tag: [t.name, t.deleted, t.character, t.propertyName, t.macroName], color: '#6f42c1' },
    // 函数名
    { tag: [t.function(t.variableName), t.labelName], color: '#005cc5' },
    // 常量
    { tag: [t.color, t.constant(t.name), t.standard(t.name)], color: '#005cc5' },
    // 定义、分隔符
    { tag: [t.definition(t.name), t.separator], color: '#24292e' },
    // 类型名、类名、数字等
    {
      tag: [
        t.typeName,
        t.className,
        t.number,
        t.changed,
        t.annotation,
        t.modifier,
        t.self,
        t.namespace,
      ],
      color: '#005cc5',
    },
    // 操作符
    {
      tag: [t.operator, t.operatorKeyword, t.url, t.escape, t.regexp, t.link, t.special(t.string)],
      color: '#d73a49',
    },
    // 注释、元数据
    { tag: [t.meta, t.comment], color: '#6a737d', fontStyle: 'italic' },
    // 文本样式
    { tag: t.strong, fontWeight: 'bold' },
    { tag: t.emphasis, fontStyle: 'italic' },
    { tag: t.strikethrough, textDecoration: 'line-through' },
    { tag: t.link, color: '#032f62', textDecoration: 'underline' },
    { tag: t.heading, fontWeight: 'bold', color: '#005cc5' },
    // 布尔值、特殊变量
    { tag: [t.atom, t.bool, t.special(t.variableName)], color: '#005cc5' },
    // 字符串
    { tag: [t.processingInstruction, t.string, t.inserted], color: '#22863a' },
    // 无效语法
    { tag: t.invalid, color: '#cb2431' },
  ])
}

/**
 * 创建暗黑主题的语法高亮样式
 * 配色参考 One Dark 主题
 * @see https://github.com/codemirror/theme-one-dark
 */
export const createDarkHighlightStyle = () => {
  return HighlightStyle.define([
    // 关键字
    { tag: t.keyword, color: '#c678dd' },
    // 属性名、变量名
    { tag: [t.name, t.deleted, t.character, t.propertyName, t.macroName], color: '#e06c75' },
    // 函数名
    { tag: [t.function(t.variableName), t.labelName], color: '#61afef' },
    // 常量
    { tag: [t.color, t.constant(t.name), t.standard(t.name)], color: '#d19a66' },
    // 定义、分隔符
    { tag: [t.definition(t.name), t.separator], color: '#abb2bf' },
    // 类型名、类名、数字等
    {
      tag: [
        t.typeName,
        t.className,
        t.number,
        t.changed,
        t.annotation,
        t.modifier,
        t.self,
        t.namespace,
      ],
      color: '#e5c07b',
    },
    // 操作符
    {
      tag: [t.operator, t.operatorKeyword, t.url, t.escape, t.regexp, t.link, t.special(t.string)],
      color: '#56b6c2',
    },
    // 注释、元数据
    { tag: [t.meta, t.comment], color: '#5c6370', fontStyle: 'italic' },
    // 文本样式
    { tag: t.strong, fontWeight: 'bold' },
    { tag: t.emphasis, fontStyle: 'italic' },
    { tag: t.strikethrough, textDecoration: 'line-through' },
    { tag: t.link, color: '#61afef', textDecoration: 'underline' },
    { tag: t.heading, fontWeight: 'bold', color: '#e06c75' },
    // 布尔值、特殊变量
    { tag: [t.atom, t.bool, t.special(t.variableName)], color: '#d19a66' },
    // 字符串
    { tag: [t.processingInstruction, t.string, t.inserted], color: '#98c379' },
    // 无效语法
    { tag: t.invalid, color: '#e06c75' },
  ])
}

/**
 * 创建编辑器主题样式
 */
export const createEditorTheme = (
  isDarkMode: boolean,
  themeVars: ThemeCommonVars & CustomThemeCommonVars,
  minHeight: string,
  maxHeight: string,
) => {
  return EditorView.theme(
    {
      // 编辑器容器
      '&': {
        backgroundColor: isDarkMode ? 'rgba(0, 0, 0, 0.1)' : themeVars.cardColor,
        color: themeVars.textColorBase,
        fontSize: '14px',
        border: `1px solid ${themeVars.borderColor}`,
        borderRadius: '4px',
        height: '100%',
      },
      // 内容区域
      '.cm-content': {
        caretColor: themeVars.primaryColor,
        fontFamily: 'Menlo, Monaco, Consolas, "Courier New", monospace',
        padding: '8px 0',
        minHeight,
        maxHeight,
      },
      // 代码行
      '.cm-line': {
        padding: '0 8px',
        lineHeight: '1.6',
      },
      // 行号区域
      '.cm-gutters': {
        backgroundColor: isDarkMode ? 'rgba(0, 0, 0, 0.1)' : themeVars.tableHeaderColor,
        color: themeVars.textColor3,
        border: 'none',
        borderRight: `1px solid ${themeVars.borderColor}`,
        borderTopLeftRadius: '4px',
        borderBottomLeftRadius: '4px',
        paddingRight: '8px',
      },
      // 当前行号高亮
      '.cm-activeLineGutter': {
        backgroundColor: isDarkMode ? '#2c313c' : themeVars.hoverColor,
      },
      // 当前行背景高亮
      '.cm-activeLine': {
        backgroundColor: isDarkMode ? '#2c313c' : themeVars.hoverColor,
      },
      // 选择区域
      '.cm-selectionBackground, ::selection': {
        backgroundColor: isDarkMode ? '#3e4451' : `${themeVars.primaryColor}33`,
      },
      '&.cm-focused .cm-selectionBackground, &.cm-focused ::selection': {
        backgroundColor: isDarkMode ? '#3e4451' : `${themeVars.primaryColor}44`,
      },
      // 光标
      '.cm-cursor, .cm-dropCursor': {
        borderLeftColor: themeVars.primaryColor,
        borderLeftWidth: '2px',
      },
      // 括号匹配
      '.cm-matchingBracket, .cm-nonmatchingBracket': {
        backgroundColor: isDarkMode ? '#515a6b' : '#e6e6e6',
        outline: '1px solid transparent',
      },
      // 占位符
      '.cm-placeholder': {
        color: isDarkMode ? '#5c6370' : themeVars.placeholderColor,
      },
      // 只读模式
      '&.cm-editor.cm-readonly': {
        backgroundColor: isDarkMode ? '#1b1d23' : themeVars.actionColor,
        cursor: 'not-allowed',
      },
      // 搜索匹配高亮
      '.cm-searchMatch': {
        backgroundColor: isDarkMode ? '#496685' : '#ffd33d',
      },
      '.cm-searchMatch.cm-searchMatch-selected': {
        backgroundColor: isDarkMode ? '#314365' : '#ff9632',
      },
      // 搜索面板样式优化（顶部面板）
      '.cm-panel.cm-search': {
        backgroundColor: isDarkMode ? '#21252b' : themeVars.cardColor,
        borderBottom: `1px solid ${themeVars.borderColor}`,
        padding: '8px 12px',
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        flexWrap: 'wrap',
      },
      '.cm-panel.cm-search input': {
        backgroundColor: isDarkMode ? '#282c34' : themeVars.inputColor,
        color: themeVars.textColorBase,
        border: `1px solid ${themeVars.borderColor}`,
        borderRadius: '4px',
        padding: '4px 8px',
        fontSize: '13px',
        outline: 'none',
      },
      '.cm-panel.cm-search input:focus': {
        borderColor: themeVars.primaryColor,
        boxShadow: `0 0 0 2px ${themeVars.primaryColorSuppl}`,
      },
      '.cm-panel.cm-search button': {
        backgroundColor: isDarkMode ? '#2c313c' : themeVars.buttonColor2,
        color: themeVars.textColorBase,
        border: `1px solid ${themeVars.borderColor}`,
        borderRadius: '4px',
        padding: '4px 12px',
        fontSize: '13px',
        cursor: 'pointer',
        transition: 'all 0.2s',
      },
      '.cm-panel.cm-search button:hover': {
        backgroundColor: isDarkMode ? '#3e4451' : themeVars.buttonColor2Hover,
        borderColor: themeVars.primaryColor,
      },
      '.cm-panel.cm-search button[name="close"]': {
        marginLeft: 'auto',
        padding: '4px 8px',
      },
      '.cm-panel.cm-search label': {
        color: themeVars.textColor2,
        fontSize: '13px',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
      },
    },
    { dark: isDarkMode },
  )
}

/**
 * 创建完整的主题扩展
 */
export const createThemeExtensions = (
  isDarkMode: boolean,
  themeVars: ThemeCommonVars & CustomThemeCommonVars,
  minHeight: string,
  maxHeight: string,
) => {
  const highlightStyle = isDarkMode ? createDarkHighlightStyle() : createLightHighlightStyle()
  const editorTheme = createEditorTheme(isDarkMode, themeVars, minHeight, maxHeight)

  return [editorTheme, syntaxHighlighting(highlightStyle)]
}
