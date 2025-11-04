# Docker Volume 管理功能实现方案

## 一、功能概述

实现 Docker Volume 的完整管理功能，包括：

- Volume 列表页面（卡片式展示）
- Volume 详情页面
- Volume 基本操作（创建、删除、检查）

## 二、数据结构设计

### 2.1 后端数据结构（Go）

```go
// VolumeInfo Volume信息
type VolumeInfo struct {
    Name       string            `json:"name"`        // Volume名称
    Driver     string            `json:"driver"`      // 驱动类型
    Mountpoint string            `json:"mountpoint"`  // 挂载点
    CreatedAt  string            `json:"createdAt"`   // 创建时间
    Labels     map[string]string `json:"labels"`      // 标签
    Scope      string            `json:"scope"`       // 作用域(local/global)
    Options    map[string]string `json:"options"`     // 驱动选项
    Status     map[string]any    `json:"status"`      // 状态信息
    UsageData  *VolumeUsageData  `json:"usageData"`   // 使用情况
}

// VolumeUsageData Volume使用数据
type VolumeUsageData struct {
    Size      int64 `json:"size"`      // 大小（字节）
    RefCount  int   `json:"refCount"`  // 引用计数（被多少容器使用）
}

// VolumeListResponse Volume列表响应
type VolumeListResponse struct {
    Volumes      []VolumeInfo `json:"volumes"`
    TotalCount   int          `json:"totalCount"`
    TotalSize    int64        `json:"totalSize"`
    UsedCount    int          `json:"usedCount"`
    UnusedCount  int          `json:"unusedCount"`
}

// VolumeDetailResponse Volume详情响应
type VolumeDetailResponse struct {
    Volume     VolumeInfo      `json:"volume"`
    Containers []ContainerRef  `json:"containers"` // 使用该Volume的容器列表
}

// ContainerRef 容器引用信息
type ContainerRef struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Image       string `json:"image"`
    Running     bool   `json:"running"`
    Destination string `json:"destination"` // 容器内挂载路径
    Mode        string `json:"mode"`        // 读写模式（rw/ro）
}
```

### 2.2 前端数据结构（TypeScript）

```typescript
// Volume信息类型
export interface VolumeInfo {
  name: string;
  driver: string;
  mountpoint: string;
  createdAt: string;
  labels: Record<string, string>;
  scope: string;
  options: Record<string, string>;
  status: Record<string, any>;
  usageData?: VolumeUsageData;
}

// Volume使用数据
export interface VolumeUsageData {
  size: number; // 字节
  refCount: number; // 引用计数
}

// Volume列表响应
export interface VolumeListResponse {
  volumes: VolumeInfo[];
  totalCount: number;
  totalSize: number;
  usedCount: number;
  unusedCount: number;
}

// 容器引用信息
export interface ContainerRef {
  id: string;
  name: string;
  image: string;
  running: boolean;
  destination: string; // 容器内挂载路径
  mode: string; // 读写模式
}

// Volume详情响应
export interface VolumeDetailResponse {
  volume: VolumeInfo;
  containers: ContainerRef[];
}

// Volume统计信息
export interface VolumeStats {
  total: number;
  used: number;
  unused: number;
  totalSize: number;
  formattedTotalSize: string;
}
```

## 三、后端实现方案

### 3.1 目录结构

```
backend/internal/
├── dockercli/
│   ├── volume.go          # Volume相关操作
│   └── client.go          # 添加Volume方法
└── api/
    └── volume_router.go   # Volume路由处理
```

### 3.2 API 路由设计

```
GET    /api/v1/volumes              # 获取Volume列表
GET    /api/v1/volumes/:name        # 获取Volume详情
POST   /api/v1/volumes              # 创建Volume
DELETE /api/v1/volumes/:name        # 删除Volume
POST   /api/v1/volumes/prune        # 清理未使用的Volume
```

### 3.3 核心实现文件

#### 3.3.1 `volume.go` - Docker Volume 操作

```go
package dockercli

import (
    "context"
    "github.com/docker/docker/api/types/volume"
    "github.com/docker/docker/api/types/filters"
)

// ListVolumes 获取Volume列表
func (c *Client) ListVolumes(ctx context.Context) ([]VolumeInfo, error)

// GetVolume 获取Volume详情
func (c *Client) GetVolume(ctx context.Context, name string) (*VolumeInfo, error)

// CreateVolume 创建Volume
func (c *Client) CreateVolume(ctx context.Context, req *VolumeCreateRequest) (*VolumeInfo, error)

// RemoveVolume 删除Volume
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error

// PruneVolumes 清理未使用的Volume
func (c *Client) PruneVolumes(ctx context.Context) (*VolumePruneResponse, error)

// GetVolumeContainers 获取使用该Volume的容器列表
func (c *Client) GetVolumeContainers(ctx context.Context, volumeName string) ([]ContainerRef, error)
```

#### 3.3.2 `volume_router.go` - API 路由处理

```go
package api

import (
    "github.com/gin-gonic/gin"
    "net/http"
)

// setupVolumeRoutes 设置Volume相关路由
func (s *Server) setupVolumeRoutes(rg *gin.RouterGroup) {
    volumes := rg.Group("/volumes")
    {
        volumes.GET("", s.handleListVolumes())
        volumes.GET("/:name", s.handleGetVolume())
        volumes.POST("", s.handleCreateVolume())
        volumes.DELETE("/:name", s.handleDeleteVolume())
        volumes.POST("/prune", s.handlePruneVolumes())
    }
}

// handleListVolumes 获取Volume列表
func (s *Server) handleListVolumes() gin.HandlerFunc

// handleGetVolume 获取Volume详情
func (s *Server) handleGetVolume() gin.HandlerFunc

// handleCreateVolume 创建Volume
func (s *Server) handleCreateVolume() gin.HandlerFunc

// handleDeleteVolume 删除Volume
func (s *Server) handleDeleteVolume() gin.HandlerFunc

// handlePruneVolumes 清理未使用的Volume
func (s *Server) handlePruneVolumes() gin.HandlerFunc
```

## 四、前端实现方案

### 4.1 目录结构

```
frontend/src/
├── pages/
│   ├── VolumesView.vue           # Volume列表页面
│   └── VolumeDetailView.vue      # Volume详情页面
├── components/
│   └── VolumeCard.vue            # Volume卡片组件
├── store/
│   └── volume.ts                 # Volume状态管理
├── hooks/
│   └── useVolume.ts              # Volume操作hooks
└── common/
    ├── api.ts                    # 添加Volume API
    └── types.ts                  # 添加Volume类型定义
```

### 4.2 页面设计

#### 4.2.1 Volume 列表页面 (`VolumesView.vue`)

**功能特性：**

- 卡片式网格布局展示 Volume
- **搜索功能**（参考 `ContainersView.vue` 的搜索实现）
  - 搜索框支持按 Volume 名称、驱动类型、挂载点搜索
  - 实时过滤，使用 `searchKeyword` 响应式变量
  - 不区分大小写搜索
- **过滤功能**（下拉菜单）
  - 全部 Volume
  - 使用中（被容器使用）
  - 未使用（无容器使用）
  - 本地作用域（Local）
  - 全局作用域（Global）
- **排序功能**（下拉菜单）
  - 按名称排序（升序/降序）
  - 按创建时间排序（升序/降序）
  - 按大小排序（升序/降序）
  - 点击相同字段切换升序/降序
- 统计信息展示（总数、总大小、使用中、未使用）
- 刷新按钮
- 创建 Volume 按钮
- 清理未使用 Volume 按钮

**展示字段：**

- Volume 名称
- 驱动类型（Driver）
- 作用域（Scope: Local/Global）
- 创建时间
- 使用情况（被 X 个容器使用）
- Volume 大小
- 挂载点路径
- 操作菜单（查看详情、删除）

**布局：**

- 响应式网格布局
  - 移动端：1 列
  - 平板：2 列
  - 笔记本：3 列
  - 桌面：4 列

#### 4.2.2 Volume 详情页面 (`VolumeDetailView.vue`)

**展示内容：**

**基本信息区域：**

- Volume 名称
- 驱动类型
- 作用域
- 创建时间
- 挂载点
- 大小

**配置信息区域：**

- 标签（Labels）
- 驱动选项（Options）
- 状态信息（Status）

**已连接的容器区域：**

- 容器列表
  - 容器名称
  - 容器镜像
  - 运行状态
  - 挂载路径
  - 读写模式（rw/ro）

**操作按钮：**

- 返回列表
- 删除 Volume
- 刷新

#### 4.2.3 Volume 卡片组件 (`VolumeCard.vue`)

**卡片布局：**

```
┌─────────────────────────────┐
│ 🗄️ [Volume图标]     [菜单] │
│   Volume名称                │
│   驱动类型标签              │
├─────────────────────────────┤
│ 📅 创建时间                 │
│ 📍 作用域: Local            │
├─────────────────────────────┤
│ 使用情况                    │
│ 📦 容器数: 2                │
│ 💾 大小: 1.2 GB             │
└─────────────────────────────┘
```

**卡片状态：**

- 使用中：绿色边框/高亮
- 未使用：灰色/默认样式
- 悬停效果：上移+阴影

### 4.3 搜索和过滤实现（参考 ContainersView.vue）

#### 4.3.1 页面状态和变量

```typescript
// VolumesView.vue <script setup>

// 搜索和过滤状态
const searchKeyword = ref("");
const statusFilter = ref<string | null>(null); // 过滤状态: null | 'used' | 'unused' | 'local' | 'global'
const sortBy = ref<string>("name"); // 默认按名称排序
const sortOrder = ref<"asc" | "desc">("asc"); // 排序方向，默认升序

// 过滤菜单选项
const statusFilterMenuOptions = computed(() => [
  {
    label: "全部",
    key: null,
    icon: renderIcon(AppsOutline),
  },
  {
    label: "使用中",
    key: "used",
    icon: renderIcon(CheckmarkCircleOutline),
  },
  {
    label: "未使用",
    key: "unused",
    icon: renderIcon(CloseCircleOutline),
  },
  {
    label: "本地作用域",
    key: "local",
    icon: renderIcon(HomeOutline),
  },
  {
    label: "全局作用域",
    key: "global",
    icon: renderIcon(GlobeOutline),
  },
]);

// 排序菜单选项
const sortMenuOptions = computed(() => [
  {
    label: `名称 ${
      sortBy.value === "name" ? (sortOrder.value === "asc" ? "↑" : "↓") : ""
    }`,
    key: "name",
    icon: renderIcon(TextOutline),
  },
  {
    label: `创建时间 ${
      sortBy.value === "created" ? (sortOrder.value === "asc" ? "↑" : "↓") : ""
    }`,
    key: "created",
    icon: renderIcon(CalendarOutline),
  },
  {
    label: `大小 ${
      sortBy.value === "size" ? (sortOrder.value === "asc" ? "↑" : "↓") : ""
    }`,
    key: "size",
    icon: renderIcon(ArchiveOutline),
  },
]);

// 处理过滤器菜单选择
const handleFilterSelect = (key: string | null) => {
  statusFilter.value = key;
};

// 判断排序按钮是否应该显示为主色（激活状态）
const isSortActive = computed(() => {
  return sortBy.value !== "name" || sortOrder.value !== "asc";
});

// 处理排序菜单选择
const handleSortSelect = (key: string) => {
  if (sortBy.value === key) {
    // 如果选择的是相同字段，切换升序/降序
    sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
  } else {
    // 如果选择的是不同字段，设置新字段并默认为升序
    sortBy.value = key;
    sortOrder.value = "asc";
  }
};

// 过滤和排序后的 Volume 列表
const filteredVolumes = computed(() => {
  let volumes = volumeStore.volumes;

  // 1. 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase();
    volumes = volumes.filter((volume) => {
      // 搜索 Volume 名称
      const matchesName = volume.name.toLowerCase().includes(keyword);

      // 搜索驱动类型
      const matchesDriver = volume.driver.toLowerCase().includes(keyword);

      // 搜索挂载点
      const matchesMountpoint = volume.mountpoint
        .toLowerCase()
        .includes(keyword);

      return matchesName || matchesDriver || matchesMountpoint;
    });
  }

  // 2. 状态过滤
  if (statusFilter.value) {
    volumes = volumes.filter((volume) => {
      switch (statusFilter.value) {
        case "used":
          return volume.usageData && volume.usageData.refCount > 0;
        case "unused":
          return !volume.usageData || volume.usageData.refCount === 0;
        case "local":
          return volume.scope === "local";
        case "global":
          return volume.scope === "global";
        default:
          return true;
      }
    });
  }

  // 3. 排序
  return volumes.sort((a, b) => {
    let result = 0;

    switch (sortBy.value) {
      case "name":
        result = a.name.localeCompare(b.name);
        break;
      case "created":
        result =
          new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
        break;
      case "size":
        const sizeA = a.usageData?.size || 0;
        const sizeB = b.usageData?.size || 0;
        result = sizeA - sizeB;
        break;
      default:
        result = 0;
    }

    // 根据排序方向调整结果
    return sortOrder.value === "asc" ? result : -result;
  });
});
```

#### 4.3.2 页面模板（完整代码，参考 ContainersView.vue）

**重要说明**：

- ✅ 顶部 header 使用 `<Teleport to="#header" defer>` 传送到布局中
- ✅ 搜索、过滤、排序功能在页面内
- ✅ 统计信息和操作按钮在 Teleport 的 header 中
- ✅ 样式类名保持与 ContainersView.vue 一致

```vue
<template>
  <div class="volumes-page">
    <!-- 页面内容：搜索、过滤、排序 -->
    <n-space>
      <!-- 过滤器菜单 -->
      <n-dropdown
        :options="statusFilterMenuOptions"
        @select="handleFilterSelect"
      >
        <n-button
          circle
          size="small"
          :type="statusFilter ? 'primary' : 'default'"
        >
          <template #icon>
            <n-icon>
              <FunnelOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- 排序菜单 -->
      <n-dropdown :options="sortMenuOptions" @select="handleSortSelect">
        <n-button
          circle
          size="small"
          :type="isSortActive ? 'primary' : 'default'"
        >
          <template #icon>
            <n-icon>
              <SwapVerticalOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- 搜索框 -->
      <n-input
        v-model:value="searchKeyword"
        placeholder="名称、驱动或挂载点"
        clearable
        class="lg:w-[400px]!"
      >
        <template #prefix>
          <n-icon>
            <SearchOutline />
          </n-icon>
        </template>
      </n-input>
    </n-space>

    <!-- Volume 列表 -->
    <div class="volumes-content">
      <n-spin :show="volumeStore.loading && filteredVolumes.length === 0">
        <!-- 空状态 -->
        <div
          v-if="filteredVolumes.length === 0 && !volumeStore.loading"
          class="empty-state"
        >
          <n-empty description="没有找到 Volume">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <!-- Volume 卡片网格 -->
        <div
          v-else
          class="volumes-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet,
            'grid-cols-3': isLaptop || isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <VolumeCard
            v-for="volume in filteredVolumes"
            :key="volume.name"
            :volume="volume"
            @delete="() => handleDelete(volume)"
            @detail="() => handleDetail(volume)"
          />
        </div>
      </n-spin>
    </div>

    <!-- ⭐ 关键：使用 Teleport 传送到页面头部（参考 ContainersView.vue 第 109 行）-->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">Volume 管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ volumeStore.stats.total }} 个 Volume， 总大小
            {{ volumeStore.stats.formattedTotalSize }}， 使用中
            {{ volumeStore.stats.used }} 个
          </n-text>
        </div>
        <div class="flex gap-2">
          <!-- 刷新按钮 -->
          <n-button
            @click="handleRefresh"
            :loading="volumeStore.loading"
            circle
            size="tiny"
          >
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 创建按钮 -->
          <n-button @click="showCreateModal = true" circle size="tiny">
            <template #icon>
              <n-icon>
                <AddOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 清理按钮（可选） -->
          <n-button @click="handlePrune" circle size="tiny">
            <template #icon>
              <n-icon>
                <TrashOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>
  </div>
</template>
```

### 4.4 核心组件实现

#### 4.4.1 Volume Store (`store/volume.ts`)

```typescript
import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { volumeApi } from "@/common/api";
import type { VolumeInfo, VolumeStats } from "@/common/types";

export const useVolumeStore = defineStore("volume", () => {
  // 状态
  const volumes = ref<VolumeInfo[]>([]);
  const loading = ref(false);

  // 计算属性
  const usedVolumes = computed(() =>
    volumes.value.filter((v) => v.usageData && v.usageData.refCount > 0)
  );

  const unusedVolumes = computed(() =>
    volumes.value.filter((v) => !v.usageData || v.usageData.refCount === 0)
  );

  const stats = computed<VolumeStats>(() => ({
    total: volumes.value.length,
    used: usedVolumes.value.length,
    unused: unusedVolumes.value.length,
    totalSize: volumes.value.reduce(
      (sum, v) => sum + (v.usageData?.size || 0),
      0
    ),
    formattedTotalSize: formatBytes(totalSize.value),
  }));

  // 方法
  const fetchVolumes = async () => {
    /* ... */
  };
  const createVolume = async (data) => {
    /* ... */
  };
  const deleteVolume = async (name: string, force: boolean) => {
    /* ... */
  };
  const pruneVolumes = async () => {
    /* ... */
  };
  const findVolumeByName = (name: string) => {
    /* ... */
  };

  return {
    volumes,
    loading,
    usedVolumes,
    unusedVolumes,
    stats,
    fetchVolumes,
    createVolume,
    deleteVolume,
    pruneVolumes,
    findVolumeByName,
  };
});
```

#### 4.4.2 Volume API (`common/api.ts`)

```typescript
export const volumeApi = {
  // 获取Volume列表
  getVolumes: () => request.get<VolumeListResponse>("/volumes"),

  // 获取Volume详情
  getVolume: (name: string) =>
    request.get<VolumeDetailResponse>(`/volumes/${name}`),

  // 创建Volume
  createVolume: (data: VolumeCreateRequest) => request.post("/volumes", data),

  // 删除Volume
  deleteVolume: (name: string, force: boolean = false) =>
    request.delete(`/volumes/${name}`, { params: { force } }),

  // 清理未使用的Volume
  pruneVolumes: () => request.post("/volumes/prune"),
};
```

### 4.5 路由配置

```typescript
// router/index.ts
{
  path: '/volumes',
  name: 'Volumes',
  component: () => import('@/pages/VolumesView.vue'),
  meta: {
    title: 'Volume管理',
    icon: 'SaveOutline'
  }
},
{
  path: '/volumes/:name',
  name: 'VolumeDetail',
  component: () => import('@/pages/VolumeDetailView.vue'),
  meta: {
    title: 'Volume详情',
    hidden: true
  }
}
```

### 4.6 侧边栏菜单

```typescript
// components/SiderContent.vue 中添加
{
  key: 'volumes',
  label: 'Volume',
  icon: SaveOutline,
  path: '/volumes'
}
```

### 4.7 完整的 <style> 部分（必须包含）

```vue
<style scoped lang="less">
// ⭐ 这个类名用于 Teleport 的内容（参考 ContainersView.vue 第 441 行）
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.volumes-page {
  width: 100%;

  .volumes-content {
    position: relative;
    min-height: 400px;
    padding-top: 16px;

    .n-spin-container {
      min-height: 400px;
    }
  }

  .empty-state {
    padding: 60px 0;
    text-align: center;
  }

  .volumes-grid {
    display: grid;
    gap: 16px;

    &.grid-cols-1 {
      grid-template-columns: 1fr;
    }

    &.grid-cols-2 {
      grid-template-columns: repeat(2, minmax(1fr, 50%));
    }

    &.grid-cols-3 {
      grid-template-columns: repeat(3, minmax(1fr, 33.33%));
    }

    &.grid-cols-4 {
      grid-template-columns: repeat(4, minmax(1fr, 25%));
    }
  }
}

// 响应式调整
@media (max-width: 768px) {
  .volumes-page {
    .volumes-grid {
      gap: 8px;
    }
  }
}
</style>
```

### 4.8 工具函数和导入（参考 ContainersView.vue）

```typescript
// <script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { useVolumeStore } from "@/store/volume";
import { useResponsive } from "@/hooks/useResponsive";
import { renderIcon } from "@/common/utils"; // ⭐ 从 utils 导入
import VolumeCard from "@/components/VolumeCard.vue";
import {
  SearchOutline,
  RefreshOutline,
  FunnelOutline,
  SwapVerticalOutline,
  AppsOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  HomeOutline,
  GlobeOutline,
  TextOutline,
  CalendarOutline,
  ArchiveOutline,
  AddOutline,
  TrashOutline,
} from "@vicons/ionicons5";

const volumeStore = useVolumeStore();
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } =
  useResponsive();

// ... 其余代码
```

**注意**：`renderIcon` 已经在 `@/common/utils.ts` 中定义，直接导入使用即可！

## 五、UI/UX 设计细节

### 5.1 颜色方案

- **使用中状态**：绿色系 (#00bc7d)
- **未使用状态**：灰色系 (#62748e)
- **驱动标签**：蓝色系
- **危险操作**：红色系

### 5.2 图标选择

**主要图标：**

- Volume 主图标：`SaveOutline` / `DatabaseOutline`
- 驱动类型：`HardwareChipOutline`
- 作用域：`GlobeOutline` / `HomeOutline`
- 容器数：`CubeOutline`
- 大小：`ArchiveOutline`
- 创建时间：`TimeOutline`
- 挂载点：`FolderOpenOutline`

**搜索和过滤图标（参考 ContainersView.vue）：**

- 搜索：`SearchOutline`
- 过滤：`FunnelOutline`
- 排序：`SwapVerticalOutline`
- 全部：`AppsOutline`
- 刷新：`RefreshOutline`
- 创建：`AddOutline`
- 删除：`TrashOutline`
- 名称排序：`TextOutline`
- 时间排序：`CalendarOutline`

### 5.3 重要实现细节

#### 5.3.1 Teleport 使用（关键！）

**必须使用 Teleport 将统计信息传送到页面头部：**

```vue
<!-- ✅ 正确：使用 Teleport 传送到 #header -->
<Teleport to="#header" defer>
  <div class="welcome-card">
    <!-- 统计信息和操作按钮 -->
  </div>
</Teleport>
```

**为什么使用 Teleport？**

1. 统一的页面布局：所有页面的 header 统计信息都在同一位置
2. LayoutView.vue 提供了 `#header` 插槽位置
3. 保持与其他页面（容器、镜像、Compose）的一致性

**参考实现：**

- 容器页面：`ContainersView.vue` 第 109 行
- 镜像页面：`ImagesView.vue` 第 71 行
- Compose 页面：`ComposeView.vue` 相应位置

#### 5.3.2 CSS 样式类名

**必须使用与 ContainersView.vue 相同的类名：**

```less
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.volumes-page {
  width: 100%;

  .volumes-content {
    position: relative;
    min-height: 400px;
    padding-top: 16px;

    .n-spin-container {
      min-height: 400px;
    }
  }

  .empty-state {
    padding: 60px 0;
    text-align: center;
  }

  .volumes-grid {
    display: grid;
    gap: 16px;

    &.grid-cols-1 {
      grid-template-columns: 1fr;
    }

    &.grid-cols-2 {
      grid-template-columns: repeat(2, minmax(1fr, 50%));
    }

    &.grid-cols-3 {
      grid-template-columns: repeat(3, minmax(1fr, 33.33%));
    }

    &.grid-cols-4 {
      grid-template-columns: repeat(4, minmax(1fr, 25%));
    }
  }
}
```

#### 5.3.3 交互细节

1. **卡片悬停效果**：上移 2px + 阴影增强
2. **加载状态**：使用 `n-spin` 组件包裹内容
3. **空状态**：使用 `n-empty` 组件，提供刷新按钮
4. **确认对话框**：删除操作使用 `dialog.warning` 二次确认
5. **Toast 提示**：使用 `message.success/error` 提示操作结果
6. **下拉刷新**：移动端支持
7. **搜索实时响应**：`v-model:value` 绑定，自动触发 `computed` 重新计算
8. **过滤器高亮**：选中过滤条件时按钮显示 `primary` 类型
9. **排序指示器**：在菜单项标签中显示 ↑/↓ 箭头指示排序方向
10. **⭐ Teleport 传送**：统计信息必须使用 `<Teleport to="#header" defer>` 传送到页面头部

## 六、关键实现要点总结 ⭐

### 6.1 Teleport 实现（最重要！）

**必须按照以下方式实现，参考 `ContainersView.vue` 第 109-140 行：**

```vue
<template>
  <div class="volumes-page">
    <!-- 1️⃣ 页面内容：搜索、过滤、列表（在页面内） -->
    <n-space><!-- 搜索、过滤、排序 --></n-space>
    <div class="volumes-content"><!-- Volume 列表 --></div>

    <!-- 2️⃣ Teleport：统计信息和操作按钮（传送到顶部） -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2>Volume 管理</n-h2>
          <n-text>统计信息</n-text>
        </div>
        <div class="flex gap-2">
          <!-- 操作按钮 -->
        </div>
      </div>
    </Teleport>
  </div>
</template>
```

**关键点：**

1. ✅ `to="#header"`：传送目标是 `#header` 元素
2. ✅ `defer`：延迟挂载，确保目标元素已存在
3. ✅ `.welcome-card`：必须使用这个类名（LayoutView.vue 中有对应样式）
4. ✅ `<div class="flex gap-2">`：按钮容器使用 flex 布局（参考 ContainersView.vue 第 121 行）
5. ❌ 不要使用 `<n-space>`，使用 `<div class="flex gap-2">` 代替
6. ❌ 不要使用 `<n-tooltip>` 包裹按钮，直接使用按钮即可

### 6.2 搜索和过滤实现要点

```typescript
// 1️⃣ 状态变量（参考 ContainersView.vue 第 184-187 行）
const searchKeyword = ref("");
const statusFilter = ref<string | null>(null);
const sortBy = ref<string>("name");
const sortOrder = ref<"asc" | "desc">("asc");

// 2️⃣ 过滤菜单选项（参考 ContainersView.vue 第 196-232 行）
const statusFilterMenuOptions = computed(() => [
  { label: "全部", key: null, icon: renderIcon(AppsOutline) },
  // ...
]);

// 3️⃣ 排序菜单选项（带方向指示器）
const sortMenuOptions = computed(() => [
  {
    label: `名称 ${
      sortBy.value === "name" ? (sortOrder.value === "asc" ? "↑" : "↓") : ""
    }`,
    key: "name",
    icon: renderIcon(TextOutline),
  },
  // ...
]);

// 4️⃣ 过滤和排序逻辑（三段式：搜索 → 过滤 → 排序）
const filteredVolumes = computed(() => {
  let volumes = volumeStore.volumes;

  // 1. 搜索
  if (searchKeyword.value) {
    /* ... */
  }

  // 2. 过滤
  if (statusFilter.value) {
    /* ... */
  }

  // 3. 排序
  return volumes.sort((a, b) => {
    let result = 0;
    // 排序逻辑
    return sortOrder.value === "asc" ? result : -result;
  });
});
```

### 6.3 必需的导入和工具

```typescript
// ⭐ 从现有工具导入（不需要重新定义）
import { renderIcon } from "@/common/utils";
import { useResponsive } from "@/hooks/useResponsive";

// ⭐ 所有需要的图标
import {
  SearchOutline, // 搜索图标
  RefreshOutline, // 刷新图标
  FunnelOutline, // 过滤图标
  SwapVerticalOutline, // 排序图标
  AppsOutline, // 全部图标
  CheckmarkCircleOutline, // 使用中图标
  CloseCircleOutline, // 未使用图标
  HomeOutline, // 本地图标
  GlobeOutline, // 全局图标
  TextOutline, // 名称图标
  CalendarOutline, // 时间图标
  ArchiveOutline, // 大小图标
  AddOutline, // 创建图标
  TrashOutline, // 删除图标
} from "@vicons/ionicons5";
```

### 6.4 CSS 样式要点

```less
// ⭐ 必须包含这个类名（用于 Teleport 的内容）
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

// ⭐ 页面主容器
.volumes-page {
  width: 100%;

  .volumes-content {
    position: relative;
    min-height: 400px;
    padding-top: 16px;
  }

  .volumes-grid {
    display: grid;
    gap: 16px;
    // 响应式网格列
  }
}
```

## 七、实现步骤

### 第一阶段：后端实现（1-2 天）

1. ✅ 实现 `dockercli/volume.go`
2. ✅ 实现 `api/volume_router.go`
3. ✅ 注册路由到主路由
4. ✅ 测试 API 接口

### 第二阶段：前端基础（1-2 天）

1. ✅ 添加类型定义到 `types.ts`
2. ✅ 实现 Volume API 到 `api.ts`
3. ✅ 实现 Volume Store
4. ✅ 实现 Volume Hooks

### 第三阶段：前端 UI（2-3 天）

1. ✅ 实现 `VolumeCard.vue` 组件
2. ✅ 实现 `VolumesView.vue` 列表页面
3. ✅ 实现 `VolumeDetailView.vue` 详情页面
4. ✅ 配置路由和菜单

### 第四阶段：测试和优化（1 天）

1. ✅ 功能测试
2. ✅ UI/UX 优化
3. ✅ 性能优化
4. ✅ 响应式适配

## 七、技术要点

### 7.1 后端技术要点

1. **使用 Docker SDK**：

   - `client.VolumeList()`
   - `client.VolumeInspect()`
   - `client.VolumeCreate()`
   - `client.VolumeRemove()`
   - `client.VolumesPrune()`

2. **容器关联查询**：

   - 遍历所有容器的 Mounts
   - 匹配 Volume 名称
   - 构建容器引用列表

3. **错误处理**：
   - Volume 不存在
   - Volume 正在使用中（无法删除）
   - 权限问题

### 7.2 前端技术要点

1. **状态管理**：

   - Pinia Store 管理全局 Volume 状态
   - 实时数据同步

2. **性能优化**：

   - 虚拟滚动（大量 Volume 时）
   - 防抖搜索
   - 懒加载详情

3. **用户体验**：
   - 乐观更新
   - 错误边界
   - 加载状态
   - 空状态处理

## 八、测试用例

### 8.1 后端测试

- [ ] 列表查询
- [ ] 详情查询
- [ ] 创建 Volume
- [ ] 删除 Volume
- [ ] 清理未使用 Volume
- [ ] 错误场景处理

### 8.2 前端测试

- [ ] 列表渲染
- [ ] 搜索过滤
- [ ] 排序功能
- [ ] 创建 Volume
- [ ] 删除 Volume
- [ ] 详情页展示
- [ ] 响应式布局
- [ ] 错误处理

## 九、参考资料

- Docker SDK for Go: https://docs.docker.com/engine/api/sdk/
- Docker Volume API: https://docs.docker.com/engine/api/v1.43/#tag/Volume
- Naive UI 组件库: https://www.naiveui.com/
- Vue 3 文档: https://vuejs.org/

## 十、注意事项

1. **权限问题**：确保 Docker socket 权限正确
2. **数据一致性**：Volume 被容器使用时不能删除
3. **性能考虑**：大量 Volume 时需要分页或虚拟滚动
4. **兼容性**：支持不同的 Volume 驱动（local, nfs 等）
5. **安全性**：删除操作需要二次确认
