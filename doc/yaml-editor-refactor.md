# YamlEditor 组件重构说明

## 重构目标

将 YamlEditor 组件从单文件重构为模块化文件夹结构，提高代码可维护性和可读性。

## 重构前后对比

### 重构前

```
components/
  YamlEditor.vue     # 350行，包含所有逻辑
```

### 重构后

```
components/
  YamlEditor/
    ├── index.vue    # 主组件，150行
    ├── theme.ts     # 主题配置，200行
    ├── types.ts     # 类型定义，20行
    └── README.md    # 组件文档
```

## 文件职责

### 1. `index.vue` - 主组件

**职责**: 组件逻辑和生命周期管理

**内容**:

- 组件 Props 和 Emits
- 编辑器初始化和销毁
- 状态管理（主题、内容、只读）
- 事件监听和处理
- 暴露的方法

**代码行数**: ~150 行

**改进**:

- ✅ 清晰的函数命名
- ✅ 完整的注释
- ✅ 逻辑分离
- ✅ 易于理解

### 2. `theme.ts` - 主题配置

**职责**: 编辑器主题和语法高亮

**内容**:

- 明亮主题语法高亮（`createLightHighlightStyle`）
- 暗黑主题语法高亮（`createDarkHighlightStyle`）
- 编辑器样式主题（`createEditorTheme`）
- 主题扩展组合（`createThemeExtensions`）

**代码行数**: ~200 行

**特点**:

- ✅ 独立的主题配置
- ✅ 易于自定义配色
- ✅ 支持主题切换
- ✅ 完整的语法高亮规则

### 3. `types.ts` - 类型定义

**职责**: TypeScript 类型定义

**内容**:

- `YamlEditorProps` - 组件 Props
- `YamlEditorEmits` - 组件 Emits
- `YamlEditorExpose` - 暴露的方法

**代码行数**: ~20 行

**优点**:

- ✅ 类型集中管理
- ✅ 易于导入和复用
- ✅ 提供完整的类型提示

### 4. `README.md` - 组件文档

**职责**: 组件使用说明

**内容**:

- 使用方法和示例
- Props、Events、Methods 说明
- 主题自定义指南
- 开发指南
- 故障排查

## 重构收益

### 1. 代码可读性提升

**重构前**:

```vue
<!-- 350行的单文件，难以快速定位功能 -->
<script setup lang="ts">
// 所有代码混在一起
const createLightHighlightStyle = () => {
  /* 50行 */
};
const createDarkHighlightStyle = () => {
  /* 50行 */
};
const createEditorTheme = () => {
  /* 80行 */
};
// ... 其他逻辑
</script>
```

**重构后**:

```typescript
// theme.ts - 专注主题配置
export const createLightHighlightStyle = () => {
  /* ... */
};
export const createDarkHighlightStyle = () => {
  /* ... */
};
export const createEditorTheme = () => {
  /* ... */
};

// index.vue - 专注组件逻辑
const initEditor = () => {
  /* ... */
};
const updateTheme = () => {
  /* ... */
};
```

### 2. 可维护性提升

| 场景         | 重构前          | 重构后              |
| ------------ | --------------- | ------------------- |
| 修改主题配色 | 在 350 行中查找 | 直接编辑 `theme.ts` |
| 添加新功能   | 代码越来越长    | 专注 `index.vue`    |
| 修改类型定义 | 在组件中查找    | 直接编辑 `types.ts` |
| 查看文档     | 需要阅读代码    | 查看 `README.md`    |

### 3. 复用性提升

```typescript
// 主题配置可以在其他组件中复用
import {
  createDarkHighlightStyle,
  createEditorTheme,
} from "@/components/YamlEditor/theme";

// 类型定义可以在其他地方引用
import type { YamlEditorProps } from "@/components/YamlEditor/types";
```

### 4. 团队协作提升

- ✅ 清晰的文件结构，新成员快速上手
- ✅ 明确的职责分工，减少冲突
- ✅ 完整的文档，降低沟通成本
- ✅ 独立的配置文件，支持并行开发

## 重构细节

### 主题函数重构

**重构前**:

```typescript
const createTheme = () => {
  return EditorView.theme(
    {
      // ... 大量样式
    },
    { dark: !!isDark }
  );
};
```

**重构后**:

```typescript
// theme.ts
export const createEditorTheme = (
  isDarkMode: boolean,
  themeVars: ThemeCommonVars & CustomThemeCommonVars,
  minHeight: string,
  maxHeight: string
) => {
  return EditorView.theme({
    // ... 样式
  }, { dark: isDarkMode })
}

// 组合函数
export const createThemeExtensions = (...) => {
  const highlightStyle = isDarkMode
    ? createDarkHighlightStyle()
    : createLightHighlightStyle()
  const editorTheme = createEditorTheme(...)
  return [editorTheme, syntaxHighlighting(highlightStyle)]
}
```

**改进**:

- 参数化配置，更灵活
- 函数职责单一
- 易于测试和复用

### 组件逻辑重构

**重构前**:

```typescript
// 所有逻辑混在一起
onMounted(() => {
  // 50行初始化代码
});

watch(isDark, () => {
  // 直接在这里处理
});
```

**重构后**:

```typescript
// 提取为独立函数
const initEditor = () => {
  // 初始化逻辑
};

const updateTheme = () => {
  // 主题更新逻辑
};

// 清晰的调用
onMounted(() => {
  initEditor();
});

watch(isDark, () => {
  updateTheme();
});
```

**改进**:

- 函数命名清晰
- 逻辑分离
- 易于维护

## 引用路径更新

### ComposeCreateModal.vue

```diff
- import YamlEditor from './YamlEditor.vue'
+ import YamlEditor from './YamlEditor/index.vue'
```

### 推荐的导入方式

```typescript
// 方式 1: 导入组件
import YamlEditor from "@/components/YamlEditor/index.vue";

// 方式 2: 导入类型
import type {
  YamlEditorProps,
  YamlEditorExpose,
} from "@/components/YamlEditor/types";

// 方式 3: 导入主题函数
import { createDarkHighlightStyle } from "@/components/YamlEditor/theme";
```

## 最佳实践

### 1. 文件组织

```
ComponentName/
├── index.vue        # 主组件
├── types.ts         # 类型定义
├── utils.ts         # 工具函数
├── constants.ts     # 常量
├── hooks.ts         # 自定义 Hooks（可选）
├── styles.less      # 样式（可选）
└── README.md        # 文档
```

### 2. 命名规范

- **文件夹**: PascalCase（如 `YamlEditor`）
- **主组件**: `index.vue`
- **辅助文件**: camelCase（如 `theme.ts`, `utils.ts`）
- **类型文件**: `types.ts`

### 3. 导出规范

```typescript
// types.ts - 导出类型
export interface YamlEditorProps {
  /* ... */
}

// theme.ts - 导出函数
export const createLightHighlightStyle = () => {
  /* ... */
};
export const createDarkHighlightStyle = () => {
  /* ... */
};

// index.vue - 默认导出组件
export default {
  /* ... */
};
```

### 4. 文档规范

每个组件文件夹应包含 `README.md`，内容包括:

- 组件描述
- 使用示例
- Props/Events/Methods 说明
- 开发指南
- 故障排查

## 测试验证

### ✅ 功能测试

- [x] 编辑器正常初始化
- [x] 内容双向绑定正常
- [x] 语法高亮正常
- [x] 主题切换正常
- [x] 只读模式正常
- [x] 暴露的方法正常

### ✅ 样式测试

- [x] 明亮主题显示正常
- [x] 暗黑主题显示正常
- [x] 移动端适配正常
- [x] 聚焦效果正常

### ✅ 类型测试

- [x] TypeScript 类型正确
- [x] 无 Lint 错误
- [x] 编辑器智能提示正常

## 后续优化建议

### 1. 添加单元测试

```typescript
// YamlEditor.test.ts
import { mount } from "@vue/test-utils";
import YamlEditor from "./index.vue";

describe("YamlEditor", () => {
  it("should render correctly", () => {
    const wrapper = mount(YamlEditor);
    expect(wrapper.find(".yaml-editor-container").exists()).toBe(true);
  });
});
```

### 2. 提取 Hooks

```typescript
// hooks.ts
export const useYamlEditor = () => {
  const editorRef = ref<YamlEditorExpose>();

  const focus = () => editorRef.value?.focus();
  const getValue = () => editorRef.value?.getValue();

  return { editorRef, focus, getValue };
};
```

### 3. 主题预设

```typescript
// themes.ts
export const themes = {
  github: {
    /* ... */
  },
  oneDark: {
    /* ... */
  },
  solarized: {
    /* ... */
  },
};
```

## 总结

通过本次重构：

1. **提升了代码质量**

   - 从 350 行单文件拆分为多个职责明确的文件
   - 代码结构清晰，易于理解和维护

2. **提升了开发效率**

   - 主题配置独立，修改更方便
   - 类型定义集中，智能提示更好
   - 文档完善，学习成本更低

3. **提升了可扩展性**

   - 模块化设计，易于添加新功能
   - 配置与逻辑分离，易于复用
   - 标准化结构，易于推广

4. **符合最佳实践**
   - 单一职责原则
   - 模块化设计
   - 文档驱动开发
   - 类型安全

这是一次成功的重构，为项目的长期维护打下了良好的基础。
