<template>
  <div class="home-page">

    <!-- 统计卡片 -->
    <div class="stats-grid">
      <!-- 容器统计 -->
      <n-card title="容器状态" hoverable>
        <n-space vertical>
          <n-statistic label="总容器数" :value="containerStore.stats.total">
            <template #prefix>
              <n-icon color="#18a058">
                <LayersOutline />
              </n-icon>
            </template>
          </n-statistic>

          <n-descriptions :column="2" size="small">
            <n-descriptions-item label="运行中">
              <n-text type="success">{{ containerStore.stats.running }}</n-text>
            </n-descriptions-item>
            <n-descriptions-item label="已停止">
              <n-text type="warning">{{ containerStore.stats.stopped }}</n-text>
            </n-descriptions-item>
            <n-descriptions-item label="可更新">
              <n-text type="info">{{ containerStore.stats.updateable }}</n-text>
            </n-descriptions-item>
            <n-descriptions-item label="错误">
              <n-text type="error">{{ containerStore.stats.error }}</n-text>
            </n-descriptions-item>
          </n-descriptions>
        </n-space>

        <template #action>
          <n-button type="primary" @click="$router.push('/containers')">
            管理容器
          </n-button>
        </template>
      </n-card>

      <!-- 镜像统计 -->
      <n-card title="镜像信息" hoverable>
        <n-space vertical>
          <n-statistic label="总镜像数" :value="imageStore.stats.total">
            <template #prefix>
              <n-icon color="#2080f0">
                <ArchiveOutline />
              </n-icon>
            </template>
          </n-statistic>

          <n-descriptions :column="1" size="small">
            <n-descriptions-item label="总大小">
              <n-text>{{ imageStore.stats.formattedTotalSize }}</n-text>
            </n-descriptions-item>
            <!-- <n-descriptions-item label="悬空镜像">
              <n-text type="warning">{{ imageStore.danglingImages.length }}</n-text>
            </n-descriptions-item> -->
          </n-descriptions>
        </n-space>

        <template #action>
          <n-button type="primary" @click="$router.push('/images')">
            管理镜像
          </n-button>
        </template>
      </n-card>

      <!-- 系统信息 -->
      <n-card title="系统信息" hoverable>
        <n-space vertical>
          <n-descriptions :column="1" size="small">
            <n-descriptions-item label="版本">
              <n-text>{{ version }}</n-text>
            </n-descriptions-item>
            <n-descriptions-item label="最后刷新">
              <n-text :depth="3">{{ lastRefreshText }}</n-text>
            </n-descriptions-item>
            <n-descriptions-item label="系统状态">
              <n-tag :type="systemHealthType" size="small">
                {{ systemHealthText }}
              </n-tag>
            </n-descriptions-item>
          </n-descriptions>
        </n-space>

        <template #action>
          <n-button type="primary" @click="$router.push('/settings')">
            系统设置
          </n-button>
        </template>
      </n-card>
    </div>

    <!-- 快速操作 -->
    <n-card title="快速操作" class="quick-actions">
      <n-space>
        <n-button v-if="containerStore.updateableContainers.length > 0" type="info" size="large"
          @click="handleBatchUpdate" :loading="containerStore.batchUpdating">
          <template #icon>
            <n-icon>
              <CloudDownloadOutline />
            </n-icon>
          </template>
          批量更新容器 ({{ containerStore.updateableContainers.length }})
        </n-button>

        <!-- <n-button v-if="imageStore.danglingImages.length > 0" type="warning" size="large"
          @click="handleCleanDanglingImages">
          <template #icon>
            <n-icon>
              <TrashOutline />
            </n-icon>
          </template>
          清理悬空镜像 ({{ imageStore.danglingImages.length }})
        </n-button> -->

        <n-tooltip trigger="hover" :delay="500">
          <template #trigger>
            <n-button type="warning" size="large" @click="handlePruneSystem" :loading="isPruning">
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

        <n-button type="primary" size="large" @click="handleRefreshAll" :loading="appStore.globalLoading">
          <template #icon>
            <n-icon>
              <RefreshOutline />
            </n-icon>
          </template>
          刷新所有数据
        </n-button>
      </n-space>
    </n-card>

    <!-- 最近容器 -->
    <n-card title="最近检查的容器" v-if="recentContainers.length > 0">
      <n-list>
        <n-list-item v-for="container in recentContainers" :key="container.id">
          <n-space align="center" justify="space-between">
            <div>
              <n-text strong>{{ container.name }}</n-text>
              <br>
              <n-text depth="3" class="text-xs">{{ container.image }}</n-text>
            </div>

            <n-space>
              <StatusBadge :container="container" show-running-status />
              <StatusBadge :container="container" />
            </n-space>
          </n-space>
        </n-list-item>
      </n-list>

      <template #action>
        <n-button text type="primary" @click="$router.push('/containers')">
          查看全部容器 →
        </n-button>
      </template>
    </n-card>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">首页<span class="text-xs pl-1">{{ systemHealthIcon }}</span></n-h2>
          <n-text depth="3" class="text-xs max-md:hidden ">
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
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { useContainer } from '@/hooks/useContainer'
import { useSettingStore } from '@/store/setting'
import StatusBadge from '@/components/StatusBadge.vue'
import { api } from '@/common/api'
import dayjs from 'dayjs'
import {
  LayersOutline,
  ArchiveOutline,
  CloudDownloadOutline,
  RefreshOutline,
  TrashBinOutline,
} from '@vicons/ionicons5'

const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()
const containerHooks = useContainer()
const settingStore = useSettingStore()
const message = useMessage()

// 系统清理状态
const isPruning = ref(false)

// 版本信息
// 版本信息
const version = computed(() => settingStore.systemInfo?.version)

// 系统健康状态
const systemHealthType = computed(() => {
  switch (appStore.systemHealth) {
    case 'healthy':
      return 'success'
    case 'unhealthy':
      return 'error'
    default:
      return 'default'
  }
})

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
  if (!appStore.lastRefreshTime) return '从未'
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

// const handleCleanDanglingImages = async () => {
//   await imageHooks.handleDeleteDangling()
// }

const handleRefreshAll = async () => {
  appStore.setGlobalLoading(true)
  try {
    await Promise.all([
      containerStore.fetchContainers(),
      imageStore.fetchImages(),
    ])
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
      await Promise.all([
        containerStore.fetchContainers(),
        imageStore.fetchImages(),
      ])
    } else {
      message.error(data.msg || '系统清理失败')
    }
  } catch (error: any) {
    message.error(error.message || '系统清理失败')
  } finally {
    isPruning.value = false
  }
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
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.home-page {
  .stats-grid {
    display: grid;
    gap: 16px;
    margin-bottom: 24px;

    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  }

  .quick-actions {
    margin-bottom: 24px;
  }
}

// 响应式调整
@media (max-width: 768px) {
  .home-page {
    .stats-grid {
      grid-template-columns: 1fr;
      gap: 12px;
    }

    .quick-actions {
      .n-space {
        flex-direction: column;
        align-items: stretch;

        .n-button {
          width: 100%;
        }
      }
    }
  }
}
</style>
