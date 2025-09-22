<template>
  <div class="containers-page">
    <!-- 页面头部 -->
    <n-card class="page-header">
      <n-space align="center" justify="space-between">
        <div>
          <n-h2 style="margin: 0;">容器管理</n-h2>
          <n-text depth="3">
            共 {{ containerStore.stats.total }} 个容器，
            {{ containerStore.stats.running }} 个运行中，
            {{ containerStore.stats.updateable }} 个可更新
          </n-text>
        </div>

        <n-space>
          <!-- 过滤器 -->
          <n-select v-model:value="statusFilter" :options="statusFilterOptions" placeholder="状态过滤" style="width: 120px;"
            clearable />

          <!-- 搜索 -->
          <n-input v-model:value="searchKeyword" placeholder="搜索容器名称或镜像" style="width: 200px;" clearable>
            <template #prefix>
              <n-icon>
                <SearchOutline />
              </n-icon>
            </template>
          </n-input>

          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="containerStore.loading" circle>
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
        </n-space>
      </n-space>
    </n-card>

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

        <div v-else class="containers-grid" :class="{
          'grid-cols-1': isMobile,
          'grid-cols-2': isTablet,
          'grid-cols-3': isLaptop || isDesktop,
          'grid-cols-4': isDesktopLarge,
        }">
          <ContainerCard v-for="container in filteredContainers" :key="container.id" :container="container"
            :loading="operationLoading" @start="() => handleStart(container)" @stop="() => handleStop(container)"
            @update="() => handleUpdate(container)" @delete="() => handleDelete(container)" />
        </div>
      </n-spin>
    </div>

    <!-- 悬浮操作按钮 -->
    <div class="floating-actions">
      <n-space vertical>
        <!-- 批量更新按钮 -->
        <n-button v-if="containerStore.updateableContainers.length > 0" type="primary" size="large" circle
          @click="handleBatchUpdate" :loading="containerStore.batchUpdating" class="fab-button">
          <template #icon>
            <n-icon size="20">
              <CloudDownloadOutline />
            </n-icon>
          </template>
        </n-button>

        <!-- 返回顶部按钮 -->
        <n-back-top :bottom="80" />
      </n-space>
    </div>

    <!-- 批量更新提示 -->
    <n-badge v-if="containerStore.updateableContainers.length > 0" :value="containerStore.updateableContainers.length"
      class="update-badge" type="info">
      <span></span>
    </n-badge>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useContainerStore } from '@/store/container'
import { useContainer } from '@/hooks/useContainer'
import { useResponsive } from '@/hooks/useResponsive'
import ContainerCard from '@/components/ContainerCard.vue'
import type { ContainerStatus } from '@/common/types'
import {
  SearchOutline,
  RefreshOutline,
  CloudDownloadOutline,
} from '@vicons/ionicons5'

const containerStore = useContainerStore()
const containerHooks = useContainer()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()


// 搜索和过滤
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const operationLoading = ref(false)

// 状态过滤选项
const statusFilterOptions = [
  { label: '全部', value: null },
  { label: '运行中', value: 'running' },
  { label: '已停止', value: 'stopped' },
  { label: '可更新', value: 'updateable' },
  { label: '最新', value: 'uptodate' },
  { label: '错误', value: 'error' },
  { label: '跳过', value: 'skipped' },
]

// 过滤后的容器列表
const filteredContainers = computed(() => {
  let containers = containerStore.containers

  // 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    containers = containers.filter(container =>
      container.name.toLowerCase().includes(keyword) ||
      container.image.toLowerCase().includes(keyword)
    )
  }

  // 状态过滤
  if (statusFilter.value) {
    containers = containers.filter(container => {
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

  return containers
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

const handleBatchUpdate = async () => {
  await containerHooks.handleBatchUpdate()
}

const handleRefresh = async () => {
  await containerHooks.handleRefresh()
}

// 页面初始化
onMounted(async () => {
  if (containerStore.containers.length === 0) {
    await containerStore.fetchContainers()
  }
})
</script>

<style scoped lang="less">
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

  .floating-actions {
    position: fixed;
    bottom: 20px;
    right: 20px;
    z-index: 100;
  }

  .fab-button {
    box-shadow: 0 4px 12px rgba(24, 144, 255, 0.4);

    &:hover {
      box-shadow: 0 6px 16px rgba(24, 144, 255, 0.5);
    }
  }

  .update-badge {
    position: fixed;
    bottom: 75px;
    right: 35px;
    z-index: 101;
  }
}

// 响应式调整
@media (max-width: 768px) {
  .containers-page {
    .containers-grid {
      gap: 8px;
    }

    .floating-actions {
      bottom: 16px;
      right: 16px;
    }

    .update-badge {
      bottom: 71px;
      right: 31px;
    }
  }
}

@media (max-width: 640px) {
  .containers-page {
    .page-header {
      .n-space {
        flex-direction: column;
        align-items: stretch !important;

        &>div:last-child {
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
