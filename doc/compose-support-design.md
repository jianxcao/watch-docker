# Docker Compose 支持设计方案

## 1. 项目概述

本设计方案旨在为 watch-docker 项目增加 Docker Compose 项目管理功能。用户可以通过 Web 界面查看、启动、停止、重新创建和删除 Docker Compose 项目，实现对多容器应用的统一管理。

## 2. 技术选型

### 2.1 后端技术栈

- **核心库**: `github.com/compose-spec/compose-go/v2` - 用于解析 Docker Compose 文件
- **框架**: Gin (已有) - REST API 服务器
- **Docker 客户端**: Docker SDK for Go (已有) - 与 Docker 引擎交互
- **编程语言**: Go 1.19+

### 2.2 前端技术栈

- **框架**: Vue 3 (已有) - 前端框架  
- **UI 组件库**: Naive UI (已有) - 组件库
- **状态管理**: Pinia (已有) - 状态管理
- **HTTP 客户端**: Axios (已有) - API 调用

## 3. 功能设计

### 3.1 核心功能

- **列出 Compose 项目**: 扫描指定目录，发现 docker-compose.yml 文件
- **查看项目状态**: 显示项目中各服务的运行状态
- **启动项目**: 执行 `docker compose up -d`
- **停止项目**: 执行 `docker compose stop`
- **重新创建项目**: 执行 `docker compose down && docker compose up -d`
- **删除项目**: 执行 `docker compose down --volumes --remove-orphans`

### 3.2 扩展功能

- **查看项目日志**: 获取项目中各服务的日志
- **服务扩缩容**: 调整服务副本数量
- **项目导入**: 支持上传 docker-compose.yml 文件
- **环境变量管理**: 管理项目的 .env 文件

## 4. 后端设计

### 4.1 目录结构

```
backend/internal/
├── composecli/          # Compose 客户端封装
│   ├── client.go        # Compose 客户端主文件
│   ├── project.go       # Compose 项目操作
│   └── types.go         # 类型定义
├── api/
│   └── compose_router.go # Compose API 路由
└── config/
    └── config.go        # 配置文件 (新增 Compose 配置)
```

### 4.2 核心数据结构

```go
// ComposeProject Docker Compose 项目信息
type ComposeProject struct {
    ID           string            `json:"id"`           // 项目唯一标识
    Name         string            `json:"name"`         // 项目名称
    Path         string            `json:"path"`         // 项目路径
    ComposeFile  string            `json:"composeFile"`  // compose 文件路径
    Status       string            `json:"status"`       // 项目状态：running/stopped/partial/error
    Services     []ComposeService  `json:"services"`     // 服务列表
    Networks     []ComposeNetwork  `json:"networks"`     // 网络列表
    Volumes      []ComposeVolume   `json:"volumes"`      // 卷列表
    CreatedAt    time.Time         `json:"createdAt"`    // 创建时间
    UpdatedAt    time.Time         `json:"updatedAt"`    // 更新时间
}

// ComposeService Docker Compose 服务信息
type ComposeService struct {
    Name        string            `json:"name"`         // 服务名称
    Image       string            `json:"image"`        // 镜像名称
    Status      string            `json:"status"`       // 服务状态
    ContainerID string            `json:"containerId"`  // 关联容器ID
    Replicas    int               `json:"replicas"`     // 副本数
    Ports       []PortMapping     `json:"ports"`        // 端口映射
    Environment map[string]string `json:"environment"`  // 环境变量
}

// ComposeNetwork Docker Compose 网络信息  
type ComposeNetwork struct {
    Name     string `json:"name"`     // 网络名称
    Driver   string `json:"driver"`   // 驱动类型
    External bool   `json:"external"` // 是否外部网络
}

// ComposeVolume Docker Compose 卷信息
type ComposeVolume struct {
    Name     string `json:"name"`     // 卷名称
    Driver   string `json:"driver"`   // 驱动类型
    External bool   `json:"external"` // 是否外部卷
}

// PortMapping 端口映射信息
type PortMapping struct {
    HostPort      int    `json:"hostPort"`      // 主机端口
    ContainerPort int    `json:"containerPort"` // 容器端口
    Protocol      string `json:"protocol"`      // 协议类型
}
```

### 4.3 API 设计

```go
// REST API 路由设计
GET    /api/v1/compose              // 获取 Compose 项目列表
GET    /api/v1/compose/:name        // 获取指定项目详情
POST   /api/v1/compose/:name/start  // 启动项目
POST   /api/v1/compose/:name/stop   // 停止项目
POST   /api/v1/compose/:name/restart // 重新创建项目
DELETE /api/v1/compose/:name        // 删除项目
GET    /api/v1/compose/:name/logs   // 获取项目日志
POST   /api/v1/compose/import       // 导入项目文件
GET    /api/v1/compose/:name/services/:service/scale // 服务扩缩容
```

### 4.4 核心实现

#### 4.4.1 Compose 客户端封装 (`internal/composecli/client.go`)

```go
package composecli

import (
    "context"
    "path/filepath"
    
    "github.com/compose-spec/compose-go/v2/loader"
    "github.com/compose-spec/compose-go/v2/types"
    "github.com/docker/docker/client"
)

type Client struct {
    docker       *client.Client
    projectPaths []string  // 项目扫描路径
}

func NewClient(docker *client.Client, projectPaths []string) *Client {
    return &Client{
        docker:       docker,
        projectPaths: projectPaths,
    }
}

// ScanProjects 扫描发现 Compose 项目
func (c *Client) ScanProjects(ctx context.Context) ([]ComposeProject, error) {
    var projects []ComposeProject
    
    for _, basePath := range c.projectPaths {
        err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                return nil // 忽略错误，继续扫描
            }
            
            // 查找 docker-compose.yml 或 compose.yml 文件
            if info.Name() == "docker-compose.yml" || info.Name() == "compose.yml" {
                project, err := c.loadProject(ctx, path)
                if err == nil {
                    projects = append(projects, project)
                }
            }
            return nil
        })
        if err != nil {
            return nil, err
        }
    }
    
    return projects, nil
}

// loadProject 加载 Compose 项目
func (c *Client) loadProject(ctx context.Context, composePath string) (ComposeProject, error) {
    // 使用 compose-go 解析项目
    configFiles := []types.ConfigFile{{Filename: composePath}}
    configDetails := types.ConfigDetails{
        ConfigFiles: configFiles,
        WorkingDir:  filepath.Dir(composePath),
    }
    
    project, err := loader.LoadWithContext(ctx, configDetails)
    if err != nil {
        return ComposeProject{}, err
    }
    
    // 获取项目运行状态
    status, services, err := c.getProjectStatus(ctx, project)
    if err != nil {
        return ComposeProject{}, err
    }
    
    return ComposeProject{
        ID:          project.Name,
        Name:        project.Name,
        Path:        filepath.Dir(composePath),
        ComposeFile: composePath,
        Status:      status,
        Services:    services,
        CreatedAt:   time.Now(), // 实际应从文件获取
        UpdatedAt:   time.Now(),
    }, nil
}

// StartProject 启动项目
func (c *Client) StartProject(ctx context.Context, projectName string) error {
    // 实现 docker compose up -d 逻辑
    return nil
}

// StopProject 停止项目  
func (c *Client) StopProject(ctx context.Context, projectName string) error {
    // 实现 docker compose stop 逻辑
    return nil
}

// RestartProject 重新创建项目
func (c *Client) RestartProject(ctx context.Context, projectName string) error {
    // 实现 docker compose down && docker compose up -d 逻辑
    return nil
}

// DeleteProject 删除项目
func (c *Client) DeleteProject(ctx context.Context, projectName string) error {
    // 实现 docker compose down --volumes --remove-orphans 逻辑
    return nil
}
```

#### 4.4.2 API 路由实现 (`internal/api/compose_router.go`)

```go
package api

import (
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

// setupComposeRoutes 设置 Compose 相关路由
func (s *Server) setupComposeRoutes(protected *gin.RouterGroup) {
    protected.GET("/compose", s.handleListComposeProjects())
    protected.GET("/compose/:name", s.handleGetComposeProject())
    protected.POST("/compose/:name/start", s.handleStartComposeProject())
    protected.POST("/compose/:name/stop", s.handleStopComposeProject())
    protected.POST("/compose/:name/restart", s.handleRestartComposeProject())
    protected.DELETE("/compose/:name", s.handleDeleteComposeProject())
    protected.GET("/compose/:name/logs", s.handleComposeProjectLogs())
    protected.POST("/compose/import", s.handleImportComposeProject())
}

// handleListComposeProjects 获取 Compose 项目列表
func (s *Server) handleListComposeProjects() gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
        defer cancel()
        
        projects, err := s.composeClient.ScanProjects(ctx)
        if err != nil {
            s.logger.Error("scan compose projects failed", zap.Error(err))
            c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "扫描 Compose 项目失败"))
            return
        }
        
        c.JSON(http.StatusOK, NewSuccessRes(gin.H{"projects": projects}))
    }
}

// handleStartComposeProject 启动 Compose 项目
func (s *Server) handleStartComposeProject() gin.HandlerFunc {
    return func(c *gin.Context) {
        projectName := c.Param("name")
        
        ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
        defer cancel()
        
        if err := s.composeClient.StartProject(ctx, projectName); err != nil {
            s.logger.Error("start compose project failed", 
                zap.String("project", projectName), zap.Error(err))
            c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
            return
        }
        
        c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
    }
}

// ... 其他处理函数类似实现
```

### 4.5 配置文件扩展

在 `internal/config/config.go` 中添加 Compose 相关配置：

```go
type Config struct {
    // ... 现有配置
    Compose ComposeConfig `json:"compose" yaml:"compose"`
}

type ComposeConfig struct {
    Enabled     bool     `json:"enabled" yaml:"enabled"`           // 是否启用 Compose 功能
    ProjectPaths []string `json:"projectPaths" yaml:"projectPaths"` // 项目扫描路径
    ScanInterval int      `json:"scanInterval" yaml:"scanInterval"` // 扫描间隔(秒)
}
```

## 5. 前端设计

### 5.1 目录结构

```
frontend/src/
├── pages/
│   └── ComposeView.vue      # Compose 项目列表页面
├── components/
│   ├── ComposeCard.vue      # Compose 项目卡片组件
│   └── ComposeImportModal.vue # Compose 项目导入弹窗
├── store/
│   └── compose.ts           # Compose 状态管理
├── hooks/
│   └── useCompose.ts        # Compose 相关 hooks
└── common/
    └── api.ts              # API 接口 (扩展)
```

### 5.2 状态管理 (`store/compose.ts`)

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as composeApi from '@/common/api'

export interface ComposeProject {
  id: string
  name: string
  path: string
  composeFile: string
  status: 'running' | 'stopped' | 'partial' | 'error'
  services: ComposeService[]
  networks: ComposeNetwork[]
  volumes: ComposeVolume[]
  createdAt: string
  updatedAt: string
}

export interface ComposeService {
  name: string
  image: string
  status: string
  containerId: string
  replicas: number
  ports: PortMapping[]
  environment: Record<string, string>
}

export const useComposeStore = defineStore('compose', () => {
  const projects = ref<ComposeProject[]>([])
  const loading = ref(false)
  const selectedProject = ref<ComposeProject | null>(null)
  
  // 计算属性
  const stats = computed(() => ({
    total: projects.value.length,
    running: projects.value.filter(p => p.status === 'running').length,
    stopped: projects.value.filter(p => p.status === 'stopped').length,
    error: projects.value.filter(p => p.status === 'error').length,
  }))
  
  // 获取项目列表
  const fetchProjects = async (force = false) => {
    if (loading.value && !force) return
    
    loading.value = true
    try {
      const response = await composeApi.getComposeProjects()
      projects.value = response.data.projects || []
    } catch (error) {
      console.error('获取 Compose 项目列表失败:', error)
      throw error
    } finally {
      loading.value = false
    }
  }
  
  // 启动项目
  const startProject = async (projectName: string) => {
    try {
      await composeApi.startComposeProject(projectName)
      await fetchProjects(true) // 刷新列表
    } catch (error) {
      console.error('启动项目失败:', error)
      throw error
    }
  }
  
  // 停止项目
  const stopProject = async (projectName: string) => {
    try {
      await composeApi.stopComposeProject(projectName)
      await fetchProjects(true) // 刷新列表
    } catch (error) {
      console.error('停止项目失败:', error)
      throw error
    }
  }
  
  // 重新创建项目
  const restartProject = async (projectName: string) => {
    try {
      await composeApi.restartComposeProject(projectName)
      await fetchProjects(true) // 刷新列表
    } catch (error) {
      console.error('重新创建项目失败:', error)
      throw error
    }
  }
  
  // 删除项目
  const deleteProject = async (projectName: string) => {
    try {
      await composeApi.deleteComposeProject(projectName)
      await fetchProjects(true) // 刷新列表
    } catch (error) {
      console.error('删除项目失败:', error)
      throw error
    }
  }
  
  return {
    projects,
    loading,
    selectedProject,
    stats,
    fetchProjects,
    startProject,
    stopProject,
    restartProject,
    deleteProject,
  }
})
```

### 5.3 Compose 项目列表页面 (`pages/ComposeView.vue`)

```vue
<template>
  <div class="compose-page">
    <!-- 页面头部 -->
    <n-space>
      <!-- 状态过滤器 -->
      <n-dropdown :options="statusFilterMenuOptions" @select="handleFilterSelect">
        <n-button circle size="small" :type="statusFilter ? 'primary' : 'default'">
          <template #icon>
            <n-icon><FunnelOutline /></n-icon>
          </template>
        </n-button>
      </n-dropdown>
      
      <!-- 搜索 -->
      <n-input 
        v-model:value="searchKeyword" 
        placeholder="搜索项目名称" 
        style="width: 200px;" 
        clearable
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
    </n-space>
    
    <!-- 项目列表 -->
    <div class="compose-content">
      <n-spin :show="composeStore.loading && filteredProjects.length === 0">
        <div v-if="filteredProjects.length === 0 && !composeStore.loading" class="empty-state">
          <n-empty description="没有找到 Compose 项目">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>
        
        <div v-else class="compose-grid" :class="{
          'grid-cols-1': isMobile,
          'grid-cols-2': isTablet || isLaptop,
          'grid-cols-3': isDesktop,
          'grid-cols-4': isDesktopLarge,
        }">
          <ComposeCard 
            v-for="project in filteredProjects" 
            :key="project.id" 
            :project="project"
            :loading="operationLoading"
            @start="() => handleStart(project)"
            @stop="() => handleStop(project)"
            @restart="() => handleRestart(project)"
            @delete="() => handleDelete(project)"
          />
        </div>
      </n-spin>
    </div>
    
    <!-- 头部信息 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">Compose 管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ composeStore.stats.total }} 个项目，
            {{ composeStore.stats.running }} 个运行中，
            {{ composeStore.stats.stopped }} 个已停止
          </n-text>
        </div>
        <div class="flex gap-2">
          <!-- 导入按钮 -->
          <n-button @click="showImportModal = true" circle size="tiny">
            <template #icon>
              <n-icon><CloudUploadOutline /></n-icon>
            </template>
          </n-button>
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="composeStore.loading" circle size="tiny">
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>
    
    <!-- 导入弹窗 -->
    <ComposeImportModal v-model:show="showImportModal" @success="handleImportSuccess" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useComposeStore } from '@/store/compose'
import { useCompose } from '@/hooks/useCompose'
import { useResponsive } from '@/hooks/useResponsive'
import { renderIcon } from '@/common/utils'
import ComposeCard from '@/components/ComposeCard.vue'
import ComposeImportModal from '@/components/ComposeImportModal.vue'
import type { ComposeProject } from '@/store/compose'
import {
  SearchOutline,
  RefreshOutline,
  CloudUploadOutline,
  FunnelOutline,
  AppsOutline,
  PlayOutline,
  StopOutline,
  CloseCircleOutline,
} from '@vicons/ionicons5'

const composeStore = useComposeStore()
const composeHooks = useCompose()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()

// 搜索和过滤
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const operationLoading = ref(false)
const showImportModal = ref(false)

// 状态过滤菜单选项
const statusFilterMenuOptions = computed(() => [
  {
    label: '全部',
    key: null,
    icon: renderIcon(AppsOutline)
  },
  {
    label: '运行中',
    key: 'running',
    icon: renderIcon(PlayOutline),
  },
  {
    label: '已停止',
    key: 'stopped',
    icon: renderIcon(StopOutline),
  },
  {
    label: '错误',
    key: 'error',
    icon: renderIcon(CloseCircleOutline),
  },
])

// 过滤后的项目列表
const filteredProjects = computed(() => {
  let projects = composeStore.projects
  
  // 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    projects = projects.filter(project =>
      project.name.toLowerCase().includes(keyword)
    )
  }
  
  // 状态过滤
  if (statusFilter.value) {
    projects = projects.filter(project => project.status === statusFilter.value)
  }
  
  return projects
})

// 处理过滤器选择
const handleFilterSelect = (key: string | null) => {
  statusFilter.value = key
}

// 操作处理函数
const handleStart = async (project: ComposeProject) => {
  operationLoading.value = true
  try {
    await composeHooks.handleStart(project)
  } finally {
    operationLoading.value = false
  }
}

const handleStop = async (project: ComposeProject) => {
  operationLoading.value = true
  try {
    await composeHooks.handleStop(project)
  } finally {
    operationLoading.value = false
  }
}

const handleRestart = async (project: ComposeProject) => {
  operationLoading.value = true
  try {
    await composeHooks.handleRestart(project)
  } finally {
    operationLoading.value = false
  }
}

const handleDelete = async (project: ComposeProject) => {
  operationLoading.value = true
  try {
    await composeHooks.handleDelete(project)
  } finally {
    operationLoading.value = false
  }
}

const handleRefresh = async () => {
  await composeStore.fetchProjects(true)
}

const handleImportSuccess = async () => {
  showImportModal.value = false
  await composeStore.fetchProjects(true)
}

// 页面初始化
onMounted(async () => {
  await composeStore.fetchProjects()
})
</script>

<style scoped lang="less">
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.compose-page {
  width: 100%;

  .compose-content {
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

  .compose-grid {
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
  .compose-page {
    .compose-grid {
      gap: 8px;
    }
  }
}
</style>
```

### 5.4 Compose 项目卡片组件 (`components/ComposeCard.vue`)

```vue
<template>
  <div class="compose-card" :data-theme="settingStore.setting.theme" :class="{ 'card-running': isRunning }">
    <!-- 状态指示条 -->
    <div class="status-bar" :class="project.status"></div>
    <div class="card-content">
      <!-- 项目头部信息 -->
      <div class="compose-header">
        <div class="compose-logo">
          <n-icon size="24">
            <ComposeLogo />
          </n-icon>
          <div class="absolute -top-1 -right-1">
            <div class="w-4 h-4 rounded-full flex items-center justify-center" :class="statusConfig.color">
              <div class="w-2 h-2 rounded-full" v-if="isRunning" :class="statusConfig.pulseColor"></div>
            </div>
          </div>
        </div>
        <div class="compose-basic-info">
          <n-tooltip :delay="500">
            <template #trigger>
              <div class="compose-name">{{ project.name }}</div>
            </template>
            <span>{{ project.name }}</span>
          </n-tooltip>
          <div class="compose-path">
            <n-tooltip :delay="500">
              <template #trigger>
                <span class="truncate w-full block">{{ project.path }}</span>
              </template>
              <span>{{ project.path }}</span>
            </n-tooltip>
          </div>
        </div>
        <div class="compose-status">
          <ComposeStatusBadge :project="project" />
          <n-dropdown :options="dropdownOptions" @select="handleMenuSelect" trigger="click">
            <n-button quaternary circle>
              <template #icon>
                <n-icon><MenuIcon /></n-icon>
              </template>
            </n-button>
          </n-dropdown>
        </div>
      </div>

      <!-- 服务信息 -->
      <div class="compose-services">
        <div class="services-header">
          <span class="services-title">服务</span>
          <span class="services-count">{{ project.services.length }} 个</span>
        </div>
        <div class="services-list">
          <div v-for="service in project.services.slice(0, 3)" :key="service.name" class="service-item">
            <n-icon size="14">
              <ContainerIcon />
            </n-icon>
            <span class="service-name">{{ service.name }}</span>
            <div class="service-status" :class="service.status">
              <div class="status-dot"></div>
            </div>
          </div>
          <div v-if="project.services.length > 3" class="service-more">
            +{{ project.services.length - 3 }} 个更多服务
          </div>
        </div>
      </div>

      <!-- 网络和卷信息 -->
      <div class="compose-details">
        <div class="detail-row">
          <div class="detail-item">
            <div class="detail-label">
              <n-icon size="16"><NetworkIcon /></n-icon>
              网络
            </div>
            <div class="detail-label">
              <n-icon size="16"><VolumeIcon /></n-icon>
              卷
            </div>
          </div>
          <div class="detail-item">
            <div class="detail-value">{{ project.networks.length }}</div>
            <div class="detail-value">{{ project.volumes.length }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ComposeLogo from '@/assets/svg/composeLogo.svg?component'
import ContainerIcon from '@/assets/svg/containerLogo.svg?component'
import NetworkIcon from '@/assets/svg/network.svg?component'
import VolumeIcon from '@/assets/svg/volume.svg?component'
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import type { ComposeProject } from '@/store/compose'
import { useSettingStore } from '@/store/setting'
import { PlayCircleOutline, StopCircleOutline, RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import { NIcon, useThemeVars } from 'naive-ui'
import { computed, h } from 'vue'
import ComposeStatusBadge from './ComposeStatusBadge.vue'

const settingStore = useSettingStore()

interface Props {
  project: ComposeProject
  loading?: boolean
}

interface Emits {
  (e: 'start'): void
  (e: 'stop'): void  
  (e: 'restart'): void
  (e: 'delete'): void
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})
const theme = useThemeVars()
const emits = defineEmits<Emits>()

const isRunning = computed(() => props.project.status === 'running')

const statusConfig = computed(() => {
  switch (props.project.status) {
    case 'running':
      return {
        color: 'bg-emerald-500',
        pulseColor: 'bg-emerald-400',
      }
    case 'partial':
      return {
        color: 'bg-orange-500',  
        pulseColor: 'bg-orange-400',
      }
    case 'error':
      return {
        color: 'bg-red-500',
        pulseColor: 'bg-red-400',
      }
    default:
      return {
        color: 'bg-slate-500',
        pulseColor: 'bg-slate-400',
      }
  }
})

// 下拉菜单选项
const dropdownOptions = computed(() => [
  {
    key: isRunning.value ? 'stop' : 'start',
    label: isRunning.value ? '停止项目' : '启动项目',
    icon: () => h(NIcon, null, {
      default: () => h(isRunning.value ? StopCircleOutline : PlayCircleOutline)
    }),
    disabled: props.loading
  },
  {
    key: 'restart',
    label: '重新创建',
    icon: () => h(NIcon, null, {
      default: () => h(RefreshOutline)
    }),
    disabled: props.loading
  },
  {
    key: 'delete',
    label: '删除项目',
    icon: () => h(NIcon, {
      color: theme.value.errorColor
    }, {
      default: () => h(TrashOutline)
    }),
    disabled: props.loading,
  }
])

// 处理下拉菜单选择
const handleMenuSelect = (key: string) => {
  switch (key) {
    case 'start':
      emits('start')
      break
    case 'stop':
      emits('stop')
      break
    case 'restart':
      emits('restart')
      break
    case 'delete':
      emits('delete')
      break
  }
}
</script>

<style scoped lang="less">
.compose-card {
  position: relative;
  border-radius: 16px;
  transition: all 0.3s ease;
  overflow: hidden;
  color: var(--text-color-1);
  box-shadow: var(--box-shadow-1);
  min-width: 320px;

  &:hover {
    transform: translateY(-2px);
  }

  &:has(.status-bar.running) {
    border: 2px solid rgba(0, 188, 125, 0.2);
    background: linear-gradient(135deg, rgba(0, 188, 125, 0.05) 0%, rgba(0, 201, 80, 0.05) 100%);
  }

  &:has(.status-bar.partial) {
    border: 2px solid rgba(255, 165, 0, 0.2);
    background: linear-gradient(135deg, rgba(255, 165, 0, 0.05) 0%, rgba(255, 140, 0, 0.05) 100%);
  }

  &:has(.status-bar.error) {
    border: 2px solid rgba(239, 68, 68, 0.2);
    background: linear-gradient(135deg, rgba(239, 68, 68, 0.05) 0%, rgba(220, 38, 38, 0.05) 100%);
  }

  &:has(.status-bar.stopped) {
    background: linear-gradient(135deg, rgba(98, 116, 142, 0.05) 0%, rgba(106, 114, 130, 0.05) 100%);
    border-color: rgba(98, 116, 142, 0.2);
  }

  .status-bar {
    height: 4px;
    width: 100%;

    &.running {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #00bc7d;
    }

    &.partial {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #ffa500;
    }

    &.error {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #ef4444;
    }

    &.stopped {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #62748e;
    }
  }

  .card-content {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .compose-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;

    .compose-logo {
      position: relative;
      width: 48px;
      height: 48px;
      border-radius: 14px;
      display: flex;
      align-items: center;
      justify-content: center;
      border: 1px solid rgba(0, 188, 125, 0.2);
      background: linear-gradient(135deg, rgba(250, 250, 250, 0.1) 0%, rgba(250, 250, 250, 0.05) 100%);
    }

    .compose-basic-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 8px;
      overflow: hidden;

      .compose-name {
        font-weight: 600;
        font-size: 16px;
        line-height: 1.25;
        color: var(--text-base);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .compose-path {
        border: 1px solid var(--border-color);
        border-radius: 4px;
        padding: 4px 8px;
        font-size: 14px;
        color: var(--text-color-3);
        max-width: 100%;
      }
    }

    .compose-status {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
    }
  }

  .compose-services {
    border-top: 1px solid var(--divider-color);
    padding-top: 12px;

    .services-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 8px;

      .services-title {
        font-size: 14px;
        font-weight: 500;
        color: var(--text-color-3);
      }

      .services-count {
        font-size: 12px;
        color: var(--text-color-3);
      }
    }

    .services-list {
      display: flex;
      flex-direction: column;
      gap: 4px;

      .service-item {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 12px;

        .service-name {
          flex: 1;
          color: var(--text-color-2);
        }

        .service-status {
          .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
          }

          &.running .status-dot {
            background-color: #00bc7d;
          }

          &.stopped .status-dot {
            background-color: #62748e;
          }

          &.error .status-dot {
            background-color: #ef4444;
          }
        }
      }

      .service-more {
        font-size: 12px;
        color: var(--text-color-3);
        font-style: italic;
      }
    }
  }

  .compose-details {
    display: flex;
    flex-direction: column;
    gap: 8px;

    .detail-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      flex-direction: column;
      gap: 12px;

      .detail-item {
        display: flex;
        flex: 1;
        width: 100%;
        gap: 8px;
        align-items: center;

        .detail-label,
        .detail-value {
          flex: 0 1 50%;
          width: fit-content;
          display: flex;
          gap: 4px;
          align-items: center;
        }

        .detail-label {
          color: var(--text-color-3);
        }

        .detail-value {
          border-radius: 10px;
          border: 1px solid var(--border-color);
          padding: 8px 12px;
          font-size: 12px;
        }
      }
    }
  }
}
</style>
```

### 5.5 路由配置扩展 (`router/index.ts`)

```typescript
// 在现有路由中添加 Compose 路由
{
  path: '/compose',
  component: () => import('@/pages/ComposeView.vue'),
  meta: { title: 'Compose 管理', requiresAuth: true },
},
```

## 6. 部署配置

### 6.1 依赖管理

#### 后端依赖 (`go.mod`)

```go
require (
    github.com/compose-spec/compose-go/v2 v2.1.1
    // ... 其他现有依赖
)
```

#### 前端依赖 (`package.json`)

```json
{
  "dependencies": {
    // ... 现有依赖保持不变
  }
}
```

### 6.2 配置文件示例

```yaml
# config.yml
compose:
  enabled: true
  projectPaths:
    - "/var/lib/docker/compose"
    - "/opt/compose-projects"
  scanInterval: 30
```

## 7. 实施计划

### 7.1 开发阶段

1. **第一阶段** (1-2 周)
   - 集成 compose-go 库
   - 实现基础的项目扫描和状态获取
   - 开发后端基础 API

2. **第二阶段** (1-2 周)
   - 实现项目操作功能 (启动/停止/删除)
   - 开发前端 Compose 列表页面
   - 实现 Compose 项目卡片组件

3. **第三阶段** (1 周)
   - 添加项目导入功能
   - 实现日志查看功能
   - 优化用户体验和错误处理

### 7.2 测试阶段

1. **单元测试**: 测试 Compose 客户端核心功能
2. **集成测试**: 测试 API 接口和前后端集成
3. **用户测试**: 验证用户体验和功能完整性

## 8. 风险和限制

### 8.1 技术风险

1. **compose-go 库限制**: 需要深入了解库的能力边界
2. **Docker API 兼容性**: 确保与不同版本 Docker 的兼容性
3. **文件权限**: 需要适当的权限访问 Compose 文件

### 8.2 功能限制

1. **项目发现**: 依赖文件系统扫描，可能存在性能问题
2. **状态同步**: 需要定期刷新状态，实时性有限
3. **错误处理**: Compose 操作失败时需要详细的错误信息

## 9. 后续扩展

### 9.1 高级功能

1. **服务日志聚合**: 统一展示所有服务日志
2. **健康检查**: 集成服务健康检查状态
3. **性能监控**: 展示服务资源使用情况
4. **服务扩缩容**: 支持动态调整服务副本数

### 9.2 用户体验优化

1. **WebSocket 实时更新**: 实时推送项目状态变化
2. **批量操作**: 支持多选和批量操作
3. **操作历史**: 记录和展示操作历史
4. **模板管理**: 支持 Compose 模板的创建和管理

---

## 总结

本设计方案基于现有的 watch-docker 项目架构，充分利用了项目的技术栈和设计模式。通过集成 `compose-spec/compose-go/v2` 库，可以实现对 Docker Compose 项目的完整管理功能。

前端设计复用了现有的容器管理页面模式，保持了一致的用户体验。后端设计遵循了现有的 API 设计规范，确保了系统的整体一致性。

该方案具有良好的可扩展性，可以根据实际需求进行功能增强和性能优化。
