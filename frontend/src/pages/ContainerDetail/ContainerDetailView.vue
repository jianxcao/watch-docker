<template>
  <div class="container-detail-page">
    <!-- 加载状态 -->
    <n-spin :show="loading" class="h-[300px] flex items-center justify-center" v-if="loading">
    </n-spin>
    <template v-else>
      <div v-if="!containerDetail" class="empty-state">
        <n-empty description="容器不存在">
          <template #extra>
            <n-button @click="handleBack">返回列表</n-button>
          </template>
        </n-empty>
      </div>

      <!-- Tabs 内容 -->
      <n-tabs
        v-else
        type="line"
        animated
        pane-class="container-detail-pane"
        :pane-style="{ height: tabHeight }"
        v-model:value="activeTab"
      >
        <!-- Tab 1: 概览 -->
        <n-tab-pane name="overview" tab="概览">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <InformationCircleOutline />
              </n-icon>
              <span>概览</span>
            </div>
          </template>
          <OverviewTab :container-detail="containerDetail" :container-name="containerName" />
        </n-tab-pane>

        <!-- Tab 2: 仪表盘 -->
        <n-tab-pane name="dashboard" tab="仪表盘">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <StatsChartOutline />
              </n-icon>
              <span>仪表盘</span>
            </div>
          </template>
          <DashboardTab :is-running="containerDetail.State.Running" :container-id="containerId" />
        </n-tab-pane>

        <!-- Tab 3: 日志 -->
        <n-tab-pane name="logs" tab="日志">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <DocumentIcon />
              </n-icon>
              <span>日志</span>
            </div>
          </template>
          <LogsTab v-if="activeTab === 'logs'" :logs-socket-url="logsSocketUrl" />
        </n-tab-pane>

        <!-- Tab 4: Shell -->
        <n-tab-pane name="shell" tab="Shell">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <TerminalOutline />
              </n-icon>
              <span>Shell</span>
            </div>
          </template>
          <ShellTab
            v-if="activeTab === 'shell'"
            :container-id="containerId"
            :container-name="containerName"
            :is-running="containerDetail.State.Running"
          />
        </n-tab-pane>

        <!-- Tab 5: 配置 -->
        <n-tab-pane name="config" tab="配置">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <SettingsOutline />
              </n-icon>
              <span>配置</span>
            </div>
          </template>
          <ConfigTab :container-detail="containerDetail" />
        </n-tab-pane>

        <!-- Tab 6: 网络 -->
        <n-tab-pane name="network" tab="网络">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <NetworkIcon class="network-icon" />
              </n-icon>
              <span>网络</span>
            </div>
          </template>
          <NetworkTab :container-detail="containerDetail" />
        </n-tab-pane>

        <!-- Tab 7: 存储 -->
        <n-tab-pane name="storage" tab="存储">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <VolumeIcon class="volume-icon" />
              </n-icon>
              <span>存储</span>
            </div>
          </template>
          <StorageTab :container-detail="containerDetail" @volume-click="handleVolumeClick" />
        </n-tab-pane>
      </n-tabs>
    </template>

    <!-- Teleport 到页面头部 -->
    <Teleport to="#header" defer>
      <div class="page-header" v-if="containerDetail">
        <div class="flex items-center gap-3">
          <n-button text circle @click="handleBack">
            <template #icon>
              <n-icon size="20">
                <ArrowBackOutline />
              </n-icon>
            </template>
          </n-button>
          <n-h2 class="m-0 text-lg">{{ containerName }}</n-h2>
          <div
            class="status-badge"
            :class="'status-' + (containerDetail.State.Running ? 'running' : 'stopped')"
          >
            <span class="status-dot"></span>
            <span class="status-text">{{
              containerDetail.State.Running ? '运行中' : '已停止'
            }}</span>
          </div>
        </div>
        <n-dropdown :options="dropdownOptions" @select="handleMenuSelect" trigger="click">
          <n-button text circle>
            <template #icon>
              <n-icon size="18">
                <EllipsisHorizontal />
              </n-icon>
            </template>
          </n-button>
        </n-dropdown>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage, useThemeVars, type DropdownOption } from 'naive-ui'
import {
  ArrowBackOutline,
  InformationCircleOutline,
  StatsChartOutline,
  TerminalOutline,
  SettingsOutline,
  EllipsisHorizontal,
  PlayOutline,
  StopOutline,
  TrashOutline,
  RefreshOutline,
  SyncOutline,
} from '@vicons/ionicons5'
import DocumentIcon from '@/assets/svg/log.svg?component'
import NetworkIcon from '@/assets/svg/networkIO.svg?component'
import VolumeIcon from '@/assets/svg/volume.svg?component'
import { useContainerStore } from '@/store/container'
import { useSettingStore } from '@/store/setting'
import { useContainer } from '@/hooks/useContainer'
import { renderIcon } from '@/common/utils'
import statsEmitter from '@/evt/containerStats'
import type { ContainerStats } from '@/common/types'

// 导入 Tab 组件
import OverviewTab from './OverviewTab.vue'
import DashboardTab from './DashboardTab.vue'
import LogsTab from './LogsTab.vue'
import ShellTab from './ShellTab.vue'
import ConfigTab from './ConfigTab.vue'
import NetworkTab from './NetworkTab.vue'
import StorageTab from './StorageTab.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const containerStore = useContainerStore()
const settingStore = useSettingStore()
const { handleStart, handleStop, handleRestart, handleUpdate, handleDelete } = useContainer()
const theme = useThemeVars()
// 状态
const containerId = ref(route.params.id as string)
const containerDetail = ref<any>(null)
const containerStats = ref<ContainerStats | null>(null)
const loading = ref(false)
const activeTab = defineModel<string>('activeTab', { default: 'overview' })

// 容器名称
const containerName = computed(() => {
  if (!containerDetail.value) {
    return ''
  }
  return containerDetail.value.Name.replace(/^\//, '')
})

// 下拉菜单选项
const dropdownOptions = computed<DropdownOption[]>(() => {
  if (!containerDetail.value) {
    return []
  }

  const options: DropdownOption[] = []
  const isRunning = containerDetail.value.State.Running

  // 从容器列表中获取容器状态信息（包含更新状态）
  const containerStatus = containerStore.findContainerById(containerId.value)
  const hasUpdate = containerStatus?.status === 'UpdateAvailable'

  // 如果有可用更新，添加更新选项
  if (hasUpdate) {
    options.push({
      label: '更新容器',
      key: 'update',
      icon: renderIcon(SyncOutline),
    })
  }

  if (!isRunning) {
    options.push({
      label: '启动',
      key: 'start',
      icon: renderIcon(PlayOutline),
    })
  } else {
    options.push({
      label: '停止',
      key: 'stop',
      icon: renderIcon(StopOutline),
    })
    options.push({
      label: '重启',
      key: 'restart',
      icon: renderIcon(RefreshOutline),
    })
  }

  options.push(
    {
      type: 'divider',
      key: 'divider',
    },
    {
      label: '删除容器',
      key: 'delete',
      icon: renderIcon(TrashOutline),
      props: {
        style: `color: ${theme.value.errorColor}`,
      },
    },
  )

  return options
})

// Tab 高度
const tabTitleHeight = computed(() => 42)
const tabHeight = computed(() => {
  return `calc(100vh - ${settingStore.contentSafeTop + tabTitleHeight.value + settingStore.contentSafeBottom}px)`
})

// 日志 WebSocket URL
const logsSocketUrl = computed(() => {
  if (!containerDetail.value) {
    return ''
  }

  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1/containers/logs/${containerId.value}/ws?token=${token}&projectName=${encodeURIComponent(containerName.value)}`
})

// 加载容器详情
const loadContainerDetail = async () => {
  loading.value = true
  try {
    const detail = await containerStore.getContainerDetail(containerId.value)
    containerDetail.value = detail
  } catch (error: any) {
    console.error('加载容器详情失败:', error)
    message.error('加载容器详情失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

// 处理统计数据更新
const handleStatsUpdate = (containers: any[]) => {
  const container = containers.find((c) => c.id === containerId.value)
  if (container && container.stats) {
    containerStats.value = container.stats
  }
}

// 操作菜单处理
const handleMenuSelect = async (key: string) => {
  if (!containerDetail.value) {
    return
  }

  // 从容器列表中找到对应的容器对象
  const container = containerStore.findContainerById(containerId.value)
  if (!container) {
    message.error('找不到容器信息')
    return
  }

  switch (key) {
    case 'start':
      await handleStart(container)
      await loadContainerDetail()
      break
    case 'stop':
      await handleStop(container)
      await loadContainerDetail()
      break
    case 'restart':
      await handleRestart(container)
      await loadContainerDetail()
      break
    case 'update':
      await handleUpdate(container)
      await loadContainerDetail()
      break
    case 'delete':
      await handleDelete(container)
      handleBack()
      break
  }
}

// 返回
const handleBack = () => {
  router.push({ path: '/containers' })
}

// 处理 Volume 点击
const handleVolumeClick = (volumeName: string) => {
  router.push({ name: 'volume-detail', params: { name: volumeName } })
}

// 初始化
onMounted(async () => {
  await loadContainerDetail()
  // 订阅统计数据
  statsEmitter.on('containers', handleStatsUpdate)
})

// 清理
onUnmounted(() => {
  statsEmitter.off('containers', handleStatsUpdate)
})
</script>

<style lang="less">
.layout-container-detail {
  .n-layout-scroll-container {
    .n-layout-content {
      padding-top: 0;
    }
  }
}
</style>

<style scoped lang="less">
.page-header {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  gap: 16px;

  .status-badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 10px;
    height: 28px;
    border-radius: 8px;
    font-size: 13px;
    line-height: 1.428;
    letter-spacing: -0.01em;
    box-shadow: var(--box-shadow-1);

    .status-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
    }

    &.status-running {
      background-color: color-mix(in srgb, var(--success-color) 10%, transparent);
      border: 1px solid color-mix(in srgb, var(--success-color) 10%, transparent);

      .status-dot {
        background: var(--success-color);
        opacity: 0.9;
      }

      .status-text {
        color: var(--success-color);
      }
    }

    &.status-stopped {
      background-color: color-mix(in srgb, var(--text-color-3) 10%, transparent);
      border: 1px solid color-mix(in srgb, var(--text-color-3) 10%, transparent);

      .status-dot {
        background: var(--text-color-3);
      }

      .status-text {
        color: var(--text-color-3);
      }
    }
  }
}

.container-detail-page {
  width: 100%;
  height: 100%;

  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 400px;
  }

  .container-detail-pane {
    padding-top: 4px;
  }
}
</style>
