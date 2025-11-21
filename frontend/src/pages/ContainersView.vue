<template>
  <div class="containers-page">
    <!-- 页面头部 -->
    <n-space>
      <!-- 过滤器菜单 -->
      <n-dropdown :options="statusFilterMenuOptions" @select="handleFilterSelect">
        <n-button circle size="small" :type="statusFilter ? 'primary' : 'default'">
          <template #icon>
            <n-icon>
              <FunnelOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>
      <!-- 排序菜单 -->
      <n-dropdown :options="sortMenuOptions" @select="handleSortSelect">
        <n-button circle size="small" :type="isSortActive ? 'primary' : 'default'">
          <template #icon>
            <n-icon>
              <SwapVerticalOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>
      <!-- 搜索 -->
      <n-input
        v-model:value="searchKeyword"
        placeholder="名称、镜像或端口"
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
    <!-- 容器列表 -->
    <div class="containers-content">
      <n-spin :show="containerStore.loading && filteredContainers.length === 0">
        <div v-if="filteredContainers.length === 0 && !containerStore.loading" class="empty-state">
          <n-empty description="没有找到容器">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <div
          v-else
          class="containers-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet || isLaptop,
            'grid-cols-3': isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <ContainerCard
            v-for="container in filteredContainers"
            :key="container.id"
            :container="container"
            :loading="operationLoading"
            @start="() => handleStart(container)"
            @stop="() => handleStop(container)"
            @restart="() => handleRestart(container)"
            @update="() => handleUpdate(container)"
            @delete="() => handleDelete(container)"
            @export="() => handleExport(container)"
            @logs="() => handleLogs(container)"
            @detail="() => handleDetail(container)"
          />
        </div>
      </n-spin>
    </div>

    <Teleport to="body">
      <!-- 悬浮操作按钮 -->
      <div class="floating-actions">
        <!-- 批量更新提示 -->
        <n-badge
          v-if="containerStore.updateableContainers.length > 0"
          :value="containerStore.updateableContainers.length"
          class="update-badge"
          type="info"
        >
          <span></span>
        </n-badge>
        <n-space vertical class="relative">
          <!-- 批量更新按钮 -->
          <n-button
            v-if="containerStore.updateableContainers.length > 0"
            type="primary"
            size="large"
            circle
            @click="handleBatchUpdate"
            :loading="containerStore.batchUpdating"
            class="fab-button"
          >
            <template #icon>
              <n-icon size="20">
                <CloudDownloadOutline />
              </n-icon>
            </template>
          </n-button>
        </n-space>
      </div>
    </Teleport>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg"
            >容器管理<span class="text-xs pl-1">{{ connectionStatusType }}</span></n-h2
          >
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ containerStore.stats.total }} 个容器，
            {{ containerStore.stats.running }} 个运行中，
            {{ containerStore.stats.updateable }} 个可更新
          </n-text>
        </div>
        <div class="flex gap-2">
          <n-button type="primary" @click="handleCreateContainer" circle size="tiny">
            <template #icon>
              <n-icon>
                <AddOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 导入按钮 -->
          <n-button @click="showImportModal = true" circle size="tiny">
            <template #icon>
              <n-icon>
                <CloudUploadOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="containerStore.loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>

    <!-- 容器导入弹窗 -->
    <ContainerImportModal v-model:show="showImportModal" @success="handleImportSuccess" />
    <!-- 容器日志弹窗 -->
    <ContainerLogsModal v-model:show="showLogsModal" :container="currentContainer" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useContainerStore } from '@/store/container'
import { useContainer } from '@/hooks/useContainer'
import { useResponsive } from '@/hooks/useResponsive'
import { renderIcon } from '@/common/utils'
import ContainerCard from '@/components/ContainerCard.vue'
import ContainerImportModal from '@/components/ContainerImportModal.vue'
import ContainerLogsModal from '@/components/ContainerLogsModal.vue'
import type { ContainerStatus } from '@/common/types'
import {
  SearchOutline,
  RefreshOutline,
  CloudDownloadOutline,
  FunnelOutline,
  AppsOutline,
  PlayOutline,
  StopOutline,
  CloudUploadOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  RemoveCircleOutline,
  SwapVerticalOutline,
  CalendarOutline,
  TextOutline,
  RadioButtonOnOutline,
  AddOutline,
} from '@vicons/ionicons5'
import { useAppStore } from '@/store/app'

const router = useRouter()
const containerStore = useContainerStore()
const containerHooks = useContainer()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const appStore = useAppStore()

// 搜索和过滤
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const sortBy = ref<string>('name') // 默认按名称排序
const sortOrder = ref<'asc' | 'desc'>('asc') // 排序方向，默认升序
const operationLoading = ref(false)
const showImportModal = ref(false)
const showLogsModal = ref(false)
const currentContainer = ref<ContainerStatus | null>(null)
// WebSocket 连接状态
const wsConnectionState = computed(() => containerStore.wsConnectionState)

// 状态过滤菜单选项
const statusFilterMenuOptions = computed(() => [
  {
    label: '全部',
    key: null,
    icon: renderIcon(AppsOutline),
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
    label: '可更新',
    key: 'updateable',
    icon: renderIcon(CloudUploadOutline),
  },
  {
    label: '最新',
    key: 'uptodate',
    icon: renderIcon(CheckmarkCircleOutline),
  },
  {
    label: '错误',
    key: 'error',
    icon: renderIcon(CloseCircleOutline),
  },
  {
    label: '跳过更新',
    key: 'skipped',
    icon: renderIcon(RemoveCircleOutline),
  },
])

// 排序菜单选项
const sortMenuOptions = computed(() => [
  {
    label: `名称 ${sortBy.value === 'name' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'name',
    icon: renderIcon(TextOutline),
  },
  {
    label: `启动时间 ${sortBy.value === 'created' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'created',
    icon: renderIcon(CalendarOutline),
  },
  {
    label: `状态 ${sortBy.value === 'status' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'status',
    icon: renderIcon(RadioButtonOnOutline),
  },
])

// 处理过滤器菜单选择
const handleFilterSelect = (key: string | null) => {
  statusFilter.value = key
}

// 判断排序按钮是否应该显示为主色（激活状态）
const isSortActive = computed(() => {
  // 如果不是默认排序设置（名称升序），则显示为激活状态
  return sortBy.value !== 'name' || sortOrder.value !== 'asc'
})

// 处理排序菜单选择
const handleSortSelect = (key: string) => {
  if (sortBy.value === key) {
    // 如果选择的是相同字段，切换升序/降序
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    // 如果选择的是不同字段，设置新字段并默认为升序
    sortBy.value = key
    sortOrder.value = 'asc'
  }
}

// 过滤和排序后的容器列表
const filteredContainers = computed(() => {
  let containers = containerStore.containers

  // 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    containers = containers.filter((container) => {
      // 搜索容器名称或镜像
      const matchesNameOrImage =
        container.name.toLowerCase().includes(keyword) ||
        container.image.toLowerCase().includes(keyword)

      // 搜索端口（支持搜索公共端口和私有端口）
      const matchesPort = container.ports?.some(
        (port) =>
          port.publicPort?.toString().includes(keyword) ||
          port.privatePort?.toString().includes(keyword),
      )
      return matchesNameOrImage || matchesPort
    })
  }

  // 状态过滤
  if (statusFilter.value) {
    containers = containers.filter((container) => {
      switch (statusFilter.value) {
        case 'running':
          return container.running
        case 'stopped':
          return !container.running
        case 'updateable':
          return container.status === 'UpdateAvailable' && !container.skipped
        case 'uptodate':
          return container.status === 'UpToDate'
        case 'error':
          return container.status === 'Error'
        case 'skipped':
          return container.skipped
        default:
          return true
      }
    })
  }

  // 排序
  return containers.sort((a, b) => {
    let result = 0

    switch (sortBy.value) {
      case 'name':
        result = a.name.localeCompare(b.name)
        break
      case 'created':
        result = new Date(a.startedAt).getTime() - new Date(b.startedAt).getTime()
        break
      case 'status':
        // 按状态排序：运行中 > 已停止 > 其他
        const getStatusPriority = (container: any) => {
          if (container.running) {
            return 0
          }
          if (!container.running) {
            return 1
          }
          return 2
        }
        result = getStatusPriority(a) - getStatusPriority(b)
        break
      default:
        return 0
    }

    // 根据排序方向调整结果
    return sortOrder.value === 'asc' ? result : -result
  })
})

// 操作处理函数
const handleStart = async (container: ContainerStatus) => {
  operationLoading.value = true
  try {
    await containerHooks.handleStart(container)
  } finally {
    operationLoading.value = false
  }
}

const handleStop = async (container: ContainerStatus) => {
  operationLoading.value = true
  try {
    await containerHooks.handleStop(container)
  } finally {
    operationLoading.value = false
  }
}

const handleRestart = async (container: ContainerStatus) => {
  operationLoading.value = true
  try {
    await containerHooks.handleRestart(container)
  } finally {
    operationLoading.value = false
  }
}

const handleUpdate = async (container: ContainerStatus) => {
  await containerHooks.handleUpdate(container)
}

const handleDelete = async (container: ContainerStatus) => {
  operationLoading.value = true
  try {
    await containerHooks.handleDelete(container)
  } finally {
    operationLoading.value = false
  }
}

// 处理导出容器
const handleExport = async (container: ContainerStatus) => {
  await containerHooks.handleExport(container)
}

const handleLogs = async (container: ContainerStatus) => {
  currentContainer.value = container
  showLogsModal.value = true
}

const handleDetail = (container: ContainerStatus) => {
  router.push({ name: 'container-detail', params: { id: container.id } })
}

const handleCreateContainer = () => {
  router.push({ name: 'container-create' })
}

const handleBatchUpdate = async () => {
  await containerHooks.handleBatchUpdate()
}

const handleRefresh = async () => {
  if (appStore.systemHealth === 'unhealthy') {
    await appStore.checkHealth()
  }
  if (!containerStore.statsWebSocket.isConnected) {
    containerStore.statsWebSocket.connect()
  }
  await containerStore.fetchContainers(true, true)
}

// 处理导入成功
const handleImportSuccess = async () => {
  showImportModal.value = false
  // 刷新容器列表（这里会显示新导入的镜像）
  await containerStore.fetchContainers(true, false)
}

// WebSocket 连接状态指示器

const connectionStatusType = computed(() => {
  switch (wsConnectionState.value) {
    case 'connected':
      return '🟢'
    case 'connecting':
      return '🟡'
    case 'disconnected':
      return '🔴'
    default:
      return '🟡'
  }
})

// 页面初始化
onMounted(async () => {
  containerStore.fetchContainers(true, true)
  // 启动 WebSocket 统计监听
  containerStore.statsWebSocket.connect()
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

.containers-page {
  width: 100%;

  .containers-content {
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

  .containers-grid {
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

.floating-actions {
  position: fixed;
  bottom: 40px;
  right: 40px;
  z-index: 100;

  .fab-button {
    box-shadow: 0 4px 12px rgba(24, 144, 255, 0.4);

    &:hover {
      box-shadow: 0 6px 16px rgba(24, 144, 255, 0.5);
    }
  }

  .update-badge {
    position: absolute;
    top: -1px;
    right: 6px;
    z-index: 101;
  }
}

// 响应式调整
@media (max-width: 768px) {
  .containers-page {
    .containers-grid {
      gap: 8px;
    }
  }

  .floating-actions {
    bottom: 16px;
    right: 16px;
  }
}

@media (max-width: 640px) {
  .containers-page {
    .page-header {
      .n-space {
        flex-direction: column;
        align-items: stretch !important;

        & > div:last-child {
          .n-space {
            flex-wrap: wrap;

            .n-input {
              width: 100% !important;
              min-width: 200px;
            }
          }
        }
      }
    }
  }
}
</style>
