<template>
  <div class="home-page">
    <!-- 统计卡片 -->
    <div class="stats-grid">
      <!-- 容器统计 -->
      <div class="stat-card container-card">
        <div class="card-header">
          <div class="icon-container">
            <LayersOutline />
          </div>
          <div class="card-title">容器状态</div>
        </div>
        <div class="card-content">
          <div class="stat-item">
            <span class="stat-label">总容器</span>
            <span class="stat-value value-blue">{{ containerStore.stats.total }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">运行中</span>
            <span class="stat-value value-green">{{ containerStore.stats.running }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">已停止</span>
            <span class="stat-value value-gray">{{ containerStore.stats.stopped }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">正在更新</span>
            <span class="stat-value value-orange">{{ containerStore.stats.updateable }}</span>
          </div>
        </div>
      </div>

      <!-- 镜像统计 -->
      <div class="stat-card image-card">
        <div class="card-header">
          <div class="icon-container">
            <ArchiveOutline />
          </div>
          <div class="card-title">镜像信息</div>
        </div>
        <div class="card-content">
          <div class="stat-item">
            <span class="stat-label">总镜像</span>
            <span class="stat-value value-purple">{{ imageStore.stats.total }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">最大的</span>
            <span class="stat-value value-pink">{{ imageStore.stats.formattedTotalSize }}</span>
          </div>
        </div>
      </div>

      <!-- 系统信息 -->
      <div class="stat-card system-card">
        <div class="card-header">
          <div class="icon-container">
            <SystemIcon />
          </div>
          <div class="card-title">系统信息</div>
        </div>
        <div class="card-content">
          <div class="stat-item">
            <span class="stat-label">后端版本</span>
            <span class="stat-value value-teal">{{ version }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">前端版本</span>
            <span class="stat-value value-teal">{{ appVersion }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">刷新时间</span>
            <span class="stat-value value-teal">{{ lastRefreshText }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">系统状态</span>
            <span class="system-badge">{{ systemHealthText }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 快速操作 -->
    <div class="quick-actions">
      <div class="card-header">
        <HeartLineIcon class="header-icon" />
        <div class="card-title">快速操作</div>
      </div>
      <div class="actions-content">
        <n-button
          v-if="containerStore.updateableContainers.length > 0"
          class="action-warning"
          size="large"
          @click="handleBatchUpdate"
          :loading="containerStore.batchUpdating"
        >
          <template #icon>
            <n-icon>
              <CloudDownloadOutline />
            </n-icon>
          </template>
          批量更新容器 ({{ containerStore.updateableContainers.length }})
        </n-button>

        <n-button class="action-primary" size="large" @click="handleCreateApp">
          <template #icon>
            <n-icon>
              <AddCircleOutline />
            </n-icon>
          </template>
          创建应用
        </n-button>

        <n-button
          class="action-primary"
          size="large"
          @click="handleRefreshAll"
          :loading="appStore.globalLoading"
        >
          <template #icon>
            <n-icon>
              <RefreshOutline />
            </n-icon>
          </template>
          刷新所有数据
        </n-button>

        <n-tooltip trigger="hover" :delay="500">
          <template #trigger>
            <n-button
              class="action-warning"
              size="large"
              @click="handlePruneSystem"
              :loading="isPruning"
            >
              <template #icon>
                <n-icon>
                  <TrashBinOutline />
                </n-icon>
              </template>
              系统清理
            </n-button>
          </template>
          清理悬空镜像、网络和数据卷
        </n-tooltip>
      </div>
    </div>

    <!-- 最近容器 -->
    <div class="recent-containers" v-if="recentContainers.length > 0">
      <div class="card-header">
        <LayersOutline class="header-icon" />
        <div class="card-title">最近检测的容器</div>
      </div>
      <div class="container-list">
        <div
          v-for="container in recentContainers"
          :key="container.id"
          class="container-item"
          @click="handleContainerClick(container.id)"
        >
          <div class="item-left">
            <div
              class="container-icon"
              :class="{
                'status-running': container.running,
                'status-stopped': !container.running,
              }"
            >
              <LayersOutline />
            </div>
            <div class="container-info">
              <div class="container-name">{{ container.name }}</div>
              <div class="container-image">{{ container.image }}</div>
            </div>
          </div>
          <div class="item-right">
            <span
              class="container-badge"
              :class="{
                'badge-running': container.running,
                'badge-stopped': !container.running,
              }"
            >
              {{ container.running ? '运行中' : '已停止' }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg"
            >首页<span class="text-xs pl-1">{{ systemHealthIcon }}</span></n-h2
          >
          <n-text depth="3" class="text-xs max-md:hidden">
            Docker 容器和镜像管理工具，自动检测更新并管理您的容器
          </n-text>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { useContainer } from '@/hooks/useContainer'
import { useSettingStore } from '@/store/setting'
import { api } from '@/common/api'
import SystemIcon from '@/assets/svg/system.svg?component'
import dayjs from 'dayjs'
import {
  LayersOutline,
  ArchiveOutline,
  CloudDownloadOutline,
  RefreshOutline,
  TrashBinOutline,
  AddCircleOutline,
} from '@vicons/ionicons5'
import HeartLineIcon from '@/assets/svg/hartLine.svg?component'

const router = useRouter()
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()
const containerHooks = useContainer()
const settingStore = useSettingStore()
const message = useMessage()

// 系统清理状态
const isPruning = ref(false)

// 版本信息
const appVersion = 'v' + __APP_VERSION__
const version = computed(() => settingStore.systemInfo?.version.version)

const systemHealthText = computed(() => {
  switch (appStore.systemHealth) {
    case 'healthy':
      return '系统正常'
    case 'unhealthy':
      return '系统异常'
    default:
      return '状态未知'
  }
})

const systemHealthIcon = computed(() => {
  switch (appStore.systemHealth) {
    case 'healthy':
      return '🟢'
    case 'unhealthy':
      return '🔴'
    default:
      return '🟡'
  }
})

// 最后刷新时间
const lastRefreshText = computed(() => {
  if (!appStore.lastRefreshTime) {
    return '从未'
  }
  return dayjs(appStore.lastRefreshTime).format('MM-DD HH:mm:ss')
})

// 最近检查的容器（最多5个）
const recentContainers = computed(() => {
  return containerStore.containers
    .slice()
    .sort((a, b) => new Date(b.lastCheckedAt).getTime() - new Date(a.lastCheckedAt).getTime())
    .slice(0, 5)
})

// 快速操作处理函数
const handleBatchUpdate = async () => {
  await containerHooks.handleBatchUpdate()
}

const handleRefreshAll = async () => {
  appStore.setGlobalLoading(true)
  try {
    await Promise.all([containerStore.fetchContainers(), imageStore.fetchImages()])
    appStore.updateRefreshTime()
  } finally {
    appStore.setGlobalLoading(false)
  }
}

// 系统清理处理函数
const handlePruneSystem = async () => {
  isPruning.value = true
  try {
    const data = await api.container.pruneSystem()
    if (data.code === 0) {
      message.success(data.data.message || '系统清理完成')
      // 清理完成后刷新数据
      await Promise.all([containerStore.fetchContainers(), imageStore.fetchImages()])
    } else {
      message.error(data.msg || '系统清理失败')
    }
  } catch (error: any) {
    message.error(error.message || '系统清理失败')
  } finally {
    isPruning.value = false
  }
}

// 创建应用处理函数
const handleCreateApp = () => {
  router.push('/compose/create')
}

// 处理容器点击
const handleContainerClick = (containerId: string) => {
  router.push(`/containers/${containerId}`)
}

// 页面初始化
onMounted(async () => {
  // 如果没有数据，先加载数据
  if (containerStore.containers.length === 0) {
    await containerStore.fetchContainers(true, false)
  }

  if (imageStore.images.length === 0) {
    await imageStore.fetchImages()
  }
})
</script>

<style scoped lang="less">
@import './HomeView.less';

.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}
</style>
