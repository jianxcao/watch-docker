# Watch Docker 前端设计文档

## 1. 项目概述

Watch Docker 前端是一个 Docker 容器和镜像管理界面，支持容器状态监控、一键更新、镜像管理等功能。前端采用现代化的技术栈，支持响应式设计，兼容移动端和PC端。

## 2. 技术栈

### 核心框架
- **Vue 3** - 前端框架，使用 Composition API
- **TypeScript** - 类型安全的 JavaScript
- **Vite** - 构建工具和开发服务器

### UI 框架和样式
- **Naive UI** - 主要组件库
- **UnoCSS** - 原子化CSS框架
- **TailwindCSS@4** - CSS工具类
- **Less** - CSS预处理器

### 状态管理和路由
- **Pinia** - 状态管理
- **Vue Router** - 路由管理

### 工具库
- **Axios** - HTTP请求库
- **dayjs** - 日期处理库
- **PostCSS** - CSS后处理器

## 3. 项目结构设计

```
frontend/src/
├── components/           # 公共组件
│   ├── Layout.vue       # 布局组件（已存在）
│   ├── LoadingView.vue  # 加载组件（已存在）
│   ├── ContainerCard.vue    # 容器卡片组件
│   ├── ImageCard.vue        # 镜像卡片组件
│   ├── StatusBadge.vue      # 状态徽章组件
│   ├── UpdateButton.vue     # 更新按钮组件
│   ├── ConfirmDialog.vue    # 确认对话框组件
│   └── MobileDrawer.vue     # 移动端抽屉菜单
├── pages/               # 页面组件
│   ├── Home.vue         # 首页/概览页面（已存在）
│   ├── Containers.vue   # 容器列表页面
│   ├── Images.vue       # 镜像列表页面
│   └── Settings.vue     # 设置页面
├── hooks/               # Vue 3 Composition API Hooks（新增）
│   ├── useApi.ts        # API请求相关hooks
│   ├── useContainer.ts  # 容器操作相关hooks
│   ├── useImage.ts      # 镜像操作相关hooks
│   ├── useConfig.ts     # 配置管理hooks
│   └── useResponsive.ts # 响应式设计hooks
├── store/               # Pinia 状态管理
│   ├── setting.ts       # 设置状态（已存在）
│   ├── container.ts     # 容器状态管理
│   ├── image.ts         # 镜像状态管理
│   └── app.ts           # 应用全局状态
├── common/              # 公共工具
│   ├── axiosConfig.ts   # API配置（已存在）
│   ├── types.ts         # 类型定义（已存在）
│   └── utils.ts         # 工具函数（已存在）
├── constants/           # 常量定义
│   ├── code.ts          # 状态码（已存在）
│   ├── msg.ts           # 消息定义（已存在）
│   └── api.ts           # API接口常量（新增）
├── router/              # 路由配置
│   └── index.ts         # 路由定义（已存在）
└── styles/              # 样式文件
    └── mix.less         # 混合样式（已存在）
```

## 4. API接口对接方案

### 4.1 接口类型定义

```typescript
// common/types.ts 新增接口类型

// 基础响应类型
interface BaseResponse<T = any> {
  code: number
  msg: string
  data: T
}

// 容器状态类型
interface ContainerStatus {
  id: string
  name: string
  image: string
  running: boolean
  currentDigest: string
  remoteDigest: string
  status: 'UpToDate' | 'UpdateAvailable' | 'Skipped' | 'Error'
  skipped: boolean
  skipReason: string
  labels: Record<string, string>
  lastCheckedAt: string
}

// 镜像信息类型
interface ImageInfo {
  id: string
  repoTags: string[]
  repoDigests: string[]
  size: number
  created: number
}

// 配置类型
interface Config {
  server: {
    addr: string
  }
  docker: {
    host: string
    includeStopped: boolean
  }
  scan: {
    interval: string
    cron: string
    initialScanOnStart: boolean
    concurrency: number
    cacheTTL: string
  }
  update: {
    enabled: boolean
    autoUpdateCron: string
    allowComposeUpdate: boolean
    recreateStrategy: string
    removeOldContainer: boolean
  }
  policy: {
    skipLabels: string[]
    onlyLabels: string[]
    excludeLabels: string[]
    skipLocalBuild: boolean
    skipPinnedDigest: boolean
    skipSemverPinned: boolean
    floatingTags: string[]
  }
  registry: {
    auth: Array<{
      host: string
      username: string
      password: string
    }>
  }
  logging: {
    level: string
  }
}
```

### 4.2 API接口方法

```typescript
// common/api.ts (新增文件)

export const containerApi = {
  // 获取容器列表
  getContainers: () => axios.get<BaseResponse<{ containers: ContainerStatus[] }>>('/api/containers'),
  
  // 更新单个容器
  updateContainer: (id: string, image?: string) => 
    axios.post<BaseResponse<{ ok: boolean }>>(`/api/containers/${id}/update`, { image }),
  
  // 批量更新容器
  batchUpdate: () => 
    axios.post<BaseResponse<{ updated: string[], failed: Record<string, string> }>>('/api/updates/run'),
  
  // 启动容器
  startContainer: (id: string) => 
    axios.post<BaseResponse<{ ok: boolean }>>(`/api/containers/${id}/start`),
  
  // 停止容器
  stopContainer: (id: string) => 
    axios.post<BaseResponse<{ ok: boolean }>>(`/api/containers/${id}/stop`),
  
  // 删除容器
  deleteContainer: (id: string) => 
    axios.delete<BaseResponse<{ ok: boolean }>>(`/api/containers/${id}`)
}

export const imageApi = {
  // 获取镜像列表
  getImages: () => axios.get<BaseResponse<{ images: ImageInfo[] }>>('/api/images'),
  
  // 删除镜像
  deleteImage: (ref: string, force: boolean = false) => 
    axios.delete<BaseResponse<{ ok: boolean }>>('/api/images', { 
      data: { ref, force } 
    })
}
```

## 5. 页面功能设计

### 5.1 布局设计

- **PC端**: 左侧固定导航菜单 + 右侧内容区域
- **移动端**: 顶部标题栏 + 左侧抽屉菜单 + 内容区域

### 5.2 容器列表页面 (Containers.vue)

**功能特性:**
- 容器状态监控（运行/停止状态）
- 更新状态显示（最新/有更新/跳过/错误）
- 单个容器操作：启动/停止/更新/删除
- 批量更新所有可更新容器（悬浮按钮）
- 容器信息展示：名称、镜像、标签、最后检查时间

**UI组件:**
- 容器卡片列表
- 状态徽章（不同颜色表示不同状态）
- 操作按钮组
- 悬浮的"一键更新"按钮

### 5.3 镜像列表页面 (Images.vue)

**功能特性:**
- 镜像列表展示
- 镜像大小、创建时间显示
- 删除未使用的镜像
- 使用中镜像的删除提示

**UI组件:**
- 镜像卡片列表
- 删除确认对话框
- 使用状态提示

### 5.4 设置页面 (Settings.vue)

**功能特性:**
- 配置项分组展示和编辑
- 实时保存配置更改
- 配置项验证

**配置分组:**
- 服务器设置（端口等）
- Docker设置（主机地址、是否包含停止容器）
- 扫描设置（间隔、并发数等）
- 更新设置（自动更新、策略等）
- 策略设置（跳过规则、标签过滤）
- 仓库认证设置
- 日志设置

## 6. 组件设计

### 6.1 ContainerCard.vue - 容器卡片组件

```vue
<template>
  <n-card>
    <div class="container-card">
      <div class="header">
        <h3>{{ container.name }}</h3>
        <StatusBadge :status="container.status" :running="container.running" />
      </div>
      <div class="content">
        <p class="image">镜像: {{ container.image }}</p>
        <p class="digest" v-if="container.currentDigest">
          摘要: {{ container.currentDigest.slice(0, 12) }}...
        </p>
        <p class="last-checked">
          最后检查: {{ formatTime(container.lastCheckedAt) }}
        </p>
      </div>
      <div class="actions">
        <n-button-group>
          <n-button v-if="!container.running" @click="start" type="primary" size="small">
            启动
          </n-button>
          <n-button v-else @click="stop" type="warning" size="small">
            停止
          </n-button>
          <n-button 
            v-if="container.status === 'UpdateAvailable'" 
            @click="update" 
            type="info" 
            size="small"
          >
            更新
          </n-button>
          <n-button @click="remove" type="error" size="small">
            删除
          </n-button>
        </n-button-group>
      </div>
    </div>
  </n-card>
</template>
```

### 6.2 StatusBadge.vue - 状态徽章组件

```vue
<template>
  <n-tag :type="badgeType" size="small">
    <n-icon :component="statusIcon" class="mr-1" />
    {{ statusText }}
  </n-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle, AlertCircle, XCircle, Minus } from '@vicons/feather'

const props = defineProps<{
  status: string
  running: boolean
}>()

const badgeType = computed(() => {
  if (!props.running) return 'warning'
  switch (props.status) {
    case 'UpToDate': return 'success'
    case 'UpdateAvailable': return 'info'
    case 'Error': return 'error'
    default: return 'default'
  }
})

const statusText = computed(() => {
  if (!props.running) return '已停止'
  switch (props.status) {
    case 'UpToDate': return '最新'
    case 'UpdateAvailable': return '可更新'
    case 'Skipped': return '跳过'
    case 'Error': return '错误'
    default: return '未知'
  }
})

const statusIcon = computed(() => {
  if (!props.running) return Minus
  switch (props.status) {
    case 'UpToDate': return CheckCircle
    case 'UpdateAvailable': return AlertCircle
    case 'Error': return XCircle
    default: return Minus
  }
})
</script>
```

## 7. 状态管理设计

### 7.1 容器状态管理 (store/container.ts)

```typescript
import { defineStore } from 'pinia'
import { containerApi } from '@/common/api'

export const useContainerStore = defineStore('container', () => {
  const containers = ref<ContainerStatus[]>([])
  const loading = ref(false)
  const updating = ref(new Set<string>())

  // 获取容器列表
  const fetchContainers = async () => {
    loading.value = true
    try {
      const { data } = await containerApi.getContainers()
      containers.value = data.data.containers
    } catch (error) {
      console.error('获取容器列表失败:', error)
    } finally {
      loading.value = false
    }
  }

  // 更新单个容器
  const updateContainer = async (id: string, image?: string) => {
    updating.value.add(id)
    try {
      await containerApi.updateContainer(id, image)
      await fetchContainers() // 重新获取列表
    } finally {
      updating.value.delete(id)
    }
  }

  // 批量更新
  const batchUpdate = async () => {
    loading.value = true
    try {
      const { data } = await containerApi.batchUpdate()
      await fetchContainers()
      return data.data
    } finally {
      loading.value = false
    }
  }

  // 计算属性
  const updateableContainers = computed(() => 
    containers.value.filter(c => c.status === 'UpdateAvailable' && !c.skipped)
  )

  const runningContainers = computed(() => 
    containers.value.filter(c => c.running)
  )

  return {
    containers,
    loading,
    updating,
    fetchContainers,
    updateContainer,
    batchUpdate,
    updateableContainers,
    runningContainers
  }
})
```

### 7.2 镜像状态管理 (store/image.ts)

```typescript
import { defineStore } from 'pinia'
import { imageApi } from '@/common/api'

export const useImageStore = defineStore('image', () => {
  const images = ref<ImageInfo[]>([])
  const loading = ref(false)

  const fetchImages = async () => {
    loading.value = true
    try {
      const { data } = await imageApi.getImages()
      images.value = data.data.images
    } catch (error) {
      console.error('获取镜像列表失败:', error)
    } finally {
      loading.value = false
    }
  }

  const deleteImage = async (ref: string, force: boolean = false) => {
    try {
      await imageApi.deleteImage(ref, force)
      await fetchImages()
    } catch (error) {
      throw error
    }
  }

  return {
    images,
    loading,
    fetchImages,
    deleteImage
  }
})
```

## 8. Hooks 设计

### 8.1 useContainer.ts - 容器操作Hook

```typescript
import { useContainerStore } from '@/store/container'
import { useMessage } from 'naive-ui'

export function useContainer() {
  const store = useContainerStore()
  const message = useMessage()

  const handleStart = async (id: string, name: string) => {
    try {
      await containerApi.startContainer(id)
      message.success(`容器 ${name} 启动成功`)
      await store.fetchContainers()
    } catch (error) {
      message.error(`启动容器失败: ${error.message}`)
    }
  }

  const handleStop = async (id: string, name: string) => {
    try {
      await containerApi.stopContainer(id)
      message.success(`容器 ${name} 停止成功`)
      await store.fetchContainers()
    } catch (error) {
      message.error(`停止容器失败: ${error.message}`)
    }
  }

  const handleUpdate = async (id: string, name: string, image?: string) => {
    try {
      await store.updateContainer(id, image)
      message.success(`容器 ${name} 更新成功`)
    } catch (error) {
      message.error(`更新容器失败: ${error.message}`)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    try {
      await containerApi.deleteContainer(id)
      message.success(`容器 ${name} 删除成功`)
      await store.fetchContainers()
    } catch (error) {
      message.error(`删除容器失败: ${error.message}`)
    }
  }

  return {
    handleStart,
    handleStop,
    handleUpdate,
    handleDelete
  }
}
```

### 8.2 useResponsive.ts - 响应式设计Hook

```typescript
import { useBreakpoints } from '@vueuse/core'

export function useResponsive() {
  const breakpoints = useBreakpoints({
    mobile: 640,
    tablet: 768,
    laptop: 1024,
    desktop: 1280,
  })

  const isMobile = breakpoints.smaller('tablet')
  const isTablet = breakpoints.between('tablet', 'laptop')
  const isDesktop = breakpoints.greaterOrEqual('laptop')

  return {
    isMobile,
    isTablet,
    isDesktop
  }
}
```

## 9. 响应式UI设计

### 9.1 布局适配
- **移动端 (< 768px)**: 
  - 单列布局
  - 抽屉式侧边菜单
  - 卡片堆叠排列
  - 悬浮操作按钮

- **平板端 (768px - 1024px)**:
  - 两列布局
  - 固定侧边菜单
  - 卡片网格排列

- **桌面端 (> 1024px)**:
  - 三列或更多列布局
  - 完整侧边菜单
  - 密集型卡片排列

### 9.2 组件响应式
```vue
<template>
  <div class="container-list" :class="{ 'mobile': isMobile }">
    <div 
      class="grid"
      :class="{
        'grid-cols-1': isMobile,
        'grid-cols-2': isTablet,
        'grid-cols-3': isDesktop && containers.length > 6,
        'grid-cols-2': isDesktop && containers.length <= 6
      }"
    >
      <ContainerCard 
        v-for="container in containers" 
        :key="container.id"
        :container="container"
      />
    </div>
  </div>
</template>
```

## 10. 开发计划

### 阶段一：基础架构
1. 完善项目结构，创建缺失的目录和文件
2. 配置类型定义和API接口
3. 设置Pinia状态管理
4. 实现基础布局组件

### 阶段二：核心功能
1. 实现容器列表页面
2. 实现镜像列表页面
3. 实现基础的CRUD操作
4. 添加状态管理和数据流

### 阶段三：高级功能
1. 实现设置页面
2. 添加批量操作功能
3. 实现响应式设计
4. 添加错误处理和用户反馈

### 阶段四：优化和测试
1. 性能优化
2. 用户体验优化
3. 错误边界处理
4. 测试和调试

## 11. 注意事项

1. **错误处理**: 所有API调用都需要适当的错误处理和用户提示
2. **加载状态**: 提供清晰的加载状态指示
3. **用户确认**: 危险操作（删除、强制更新）需要用户确认
4. **实时更新**: 考虑使用轮询或WebSocket实现状态实时更新
5. **权限控制**: 根据后端配置限制某些操作的可用性
6. **无障碍性**: 遵循Web无障碍性标准，确保良好的键盘导航和屏幕阅读器支持

