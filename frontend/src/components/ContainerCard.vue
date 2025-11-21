<template>
  <div
    class="container-card"
    :data-theme="settingStore.setting.theme"
    :class="{ 'card-updating': isUpdating }"
    @click="handleCardClick"
  >
    <!-- 状态指示条 -->
    <div class="status-bar" :class="container.running ? 'running' : 'stopped'"></div>
    <div class="card-content">
      <!-- 容器头部信息 -->
      <div class="container-header">
        <div class="container-logo">
          <n-icon size="24">
            <ContainerLogo />
          </n-icon>
          <div class="absolute -top-1 -right-1">
            <div
              class="w-4 h-4 rounded-full flex items-center justify-center"
              :class="statusConfig.color"
            >
              <div
                class="w-2 h-2 rounded-full"
                v-if="container.running"
                :class="statusConfig.pulseColor"
              ></div>
            </div>
          </div>
        </div>
        <div class="container-basic-info">
          <n-tooltip :delay="500">
            <template #trigger>
              <div class="container-name">{{ container.name }}</div>
            </template>
            <span>{{ container.name }}</span>
          </n-tooltip>
          <div class="container-image">
            <n-tooltip :delay="500">
              <template #trigger>
                <span class="truncate w-full block">{{ container.image }}</span>
              </template>
              <span>{{ container.image }}</span>
            </n-tooltip>
            <n-tooltip :delay="500" v-if="container.status === 'UpdateAvailable'">
              <template #trigger>
                <div
                  class="absolute -top-1 -right-1 w-3 h-3 bg-orange-500 rounded-full cursor-help"
                >
                  <div class="w-full h-full bg-orange-400 rounded-full animate-ping"></div>
                </div>
              </template>
              <span>可更新</span>
            </n-tooltip>
          </div>
        </div>
        <div class="container-status">
          <RunningStatusBadge :container="container" />
          <n-dropdown :options="dropdownOptions" @select="handleMenuSelect" trigger="click">
            <n-button quaternary circle @click.stop>
              <template #icon>
                <n-icon>
                  <MenuIcon />
                </n-icon>
              </template>
            </n-button>
          </n-dropdown>
        </div>
      </div>

      <!-- 容器详细信息 -->
      <div class="container-details">
        <div class="detail-row">
          <div class="detail-item">
            <div class="detail-label">
              <n-icon size="16">
                <CreateTimeIcon />
              </n-icon>
              创建时间
            </div>
            <div class="detail-label">
              <n-icon size="16">
                <HeartLineIcon />
              </n-icon>
              端口映射
            </div>
          </div>
          <div class="detail-item">
            <div class="detail-value min-w-[152px]">
              {{ formatCreatedTime(container.startedAt) }}
            </div>
            <div class="detail-value">{{ formatPorts(container.ports) }}</div>
          </div>
        </div>
      </div>

      <!-- 资源使用情况 -->
      <div class="container-stats">
        <div class="flex flex-row justify-between items-center mb-2">
          <div class="stats-title">资源使用情况</div>
          <div class="flex flex-row gap-2" v-if="container.running">
            <div class="stat-header">
              <n-icon size="12">
                <TimeOutline />
              </n-icon>
              <span>运行时长</span>
            </div>
            <div class="time-value" :class="container.running ? 'running' : 'stopped'">
              {{ container.running && container.startedAt ? formatTime(container.startedAt) : '-' }}
            </div>
          </div>
        </div>
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <CpuIcon />
              </n-icon>
              <span>CPU</span>
            </div>
            <div class="stat-value">{{ formatPercent(stats.cpuPercent) }}</div>
          </div>

          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <MemoryIcon />
              </n-icon>
              <span>内存</span>
            </div>
            <div class="stat-value">{{ formatBytes(stats.memoryUsage) }}</div>
          </div>

          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <CloudDownloadOutline />
              </n-icon>
              <span>下载</span>
            </div>
            <div class="network-rate">{{ formatBytesPerSecond(stats.networkRxRate) }}</div>
          </div>
          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <CloudUploadOutline />
              </n-icon>
              <span>上传</span>
            </div>
            <div class="network-rate">{{ formatBytesPerSecond(stats.networkTxRate) }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ContainerLogo from '@/assets/svg/containerLogo.svg?component'
import CpuIcon from '@/assets/svg/cpu.svg?component'
import CreateTimeIcon from '@/assets/svg/createTime.svg?component'
import MemoryIcon from '@/assets/svg/memory.svg?component'
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import HeartLineIcon from '@/assets/svg/hartLine.svg?component'
import type { ContainerStatus } from '@/common/types'
import { formatBytes, formatBytesPerSecond, formatPercent, formatTime } from '@/common/utils'
import { useContainerStore } from '@/store/container'
import { useSettingStore } from '@/store/setting'
import {
  CloudDownloadOutline,
  CloudUploadOutline,
  PlayCircleOutline,
  StopCircleOutline,
  TimeOutline,
  TrashOutline,
  DownloadOutline,
  InformationCircleOutline,
  RefreshOutline,
  SyncOutline,
} from '@vicons/ionicons5'
import LogIcon from '@/assets/svg/log.svg?component'
import dayjs from 'dayjs'
import { NIcon, useThemeVars } from 'naive-ui'
import { computed, h } from 'vue'
import RunningStatusBadge from './RunningStatusBadge.vue'
const settingStore = useSettingStore()

interface Props {
  container: ContainerStatus
  loading?: boolean
}

interface Emits {
  (e: 'start'): void
  (e: 'stop'): void
  (e: 'restart'): void
  (e: 'update'): void
  (e: 'delete'): void
  (e: 'export'): void
  (e: 'logs'): void
  (e: 'detail'): void
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})
const theme = useThemeVars()
const emits = defineEmits<Emits>()

const stats = computed(() => {
  return (
    props.container.stats || {
      cpuPercent: 0,
      memoryUsage: 0,
      memoryPercent: 0,
      networkRxRate: 0,
      networkTxRate: 0,
    }
  )
})

const containerStore = useContainerStore()

// 是否正在更新
const isUpdating = computed(() => containerStore.isContainerUpdating(props.container.id))

const statusConfig = computed(() => {
  return {
    color: props.container.running ? 'bg-emerald-500' : 'bg-slate-500',
    pulseColor: props.container.running ? 'bg-emerald-400' : 'bg-slate-400',
  }
})

// 格式化创建时间
const formatCreatedTime = (createdAt: string): string => {
  if (!createdAt) {
    return '-'
  }
  return dayjs(createdAt).format('YYYY-MM-DD HH:mm')
}

// 格式化端口映射
const formatPorts = (ports: any[]): string => {
  if (!ports || ports.length === 0) {
    return '-'
  }
  // return ports
  //   .map((port) => {
  //     if (port.publicPort) {
  //       return `${port.publicPort}:${port.privatePort}`
  //     } else {
  //       return `${port.privatePort}/${port.type}`
  //     }
  //   })
  //   .join(', ')
  return (
    ports
      .filter((port) => port.publicPort)
      .map((port) => `${port.publicPort}:${port.privatePort}`)[0] || '-'
  )
}

// 下拉菜单选项
const dropdownOptions = computed(() => {
  const options: any[] = [
    {
      key: 'detail',
      label: '查看详情',
      icon: () =>
        h(NIcon, null, {
          default: () => h(InformationCircleOutline),
        }),
    },
    {
      key: 'logs',
      label: '查看日志',
      icon: () =>
        h(NIcon, null, {
          default: () => h(LogIcon),
        }),
    },
    {
      type: 'divider',
      key: 'divider1',
    },
  ]

  // 如果有可用更新，添加更新选项
  if (props.container.status === 'UpdateAvailable') {
    options.push({
      key: 'update',
      label: '更新容器',
      icon: () =>
        h(NIcon, null, {
          default: () => h(SyncOutline),
        }),
      disabled: props.loading,
    })
  }

  // 启动/停止/重启选项
  if (props.container.running) {
    options.push(
      {
        key: 'restart',
        label: '重启容器',
        icon: () =>
          h(NIcon, null, {
            default: () => h(RefreshOutline),
          }),
        disabled: props.loading,
      },
      {
        key: 'stop',
        label: '停止容器',
        icon: () =>
          h(NIcon, null, {
            default: () => h(StopCircleOutline),
          }),
        disabled: props.loading,
      },
    )
  } else {
    options.push({
      key: 'start',
      label: '启动容器',
      icon: () =>
        h(NIcon, null, {
          default: () => h(PlayCircleOutline),
        }),
      disabled: props.loading,
    })
  }

  options.push(
    {
      key: 'export',
      label: '导出容器',
      icon: () =>
        h(NIcon, null, {
          default: () => h(DownloadOutline),
        }),
    },
    {
      type: 'divider',
      key: 'divider2',
    },
    {
      key: 'delete',
      label: '删除容器',
      icon: () =>
        h(
          NIcon,
          {
            color: theme.value.errorColor,
          },
          {
            default: () => h(TrashOutline),
          },
        ),
      disabled: props.loading,
    },
  )

  return options
})

// 处理下拉菜单选择
const handleMenuSelect = (key: string) => {
  switch (key) {
    case 'detail':
      emits('detail')
      break
    case 'start':
      emits('start')
      break
    case 'stop':
      emits('stop')
      break
    case 'restart':
      emits('restart')
      break
    case 'update':
      emits('update')
      break
    case 'export':
      emits('export')
      break
    case 'delete':
      emits('delete')
      break
    case 'logs':
      emits('logs')
      break
  }
}

// 处理卡片点击
const handleCardClick = () => {
  emits('detail')
}
</script>

<style scoped lang="less">
.container-card {
  position: relative;
  border-radius: 16px;
  transition: all 0.3s ease;
  position: relative;
  overflow: hidden;
  color: var(--text-color-1);
  box-shadow: var(--box-shadow-1);
  min-width: 320px;
  cursor: pointer;

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--box-shadow-2);
  }

  &:has(.status-bar.running) {
    border: 2px solid rgba(0, 188, 125, 0.2);
    background: linear-gradient(135deg, rgba(0, 188, 125, 0.05) 0%, rgba(0, 201, 80, 0.05) 100%);
  }

  &[data-theme='light']:has(.status-bar.running) {
    border: 2px solid rgba(0, 188, 125, 0.2);
    background: linear-gradient(135deg, rgba(0, 188, 125, 0.05) 0%, rgba(0, 201, 80, 0.05) 100%);
  }

  &:has(.status-bar.stopped) {
    background: linear-gradient(
      135deg,
      rgba(98, 116, 142, 0.05) 0%,
      rgba(106, 114, 130, 0.05) 100%
    );
    border-color: rgba(98, 116, 142, 0.2);
  }

  &[data-theme='light']:has(.status-bar.stopped) {
    border: 2px solid rgba(98, 116, 142, 0.2);
    background: linear-gradient(
      135deg,
      rgba(98, 116, 142, 0.05) 0%,
      rgba(106, 114, 130, 0.05) 100%
    );
  }

  .status-bar {
    height: 4px;
    width: 100%;

    &.running {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #00bc7d;
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

  .container-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    white-space: nowrap;
    flex-wrap: nowrap;

    .container-logo {
      position: relative;
      width: 48px;
      height: 48px;
      border-radius: 14px;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 14px;
      align-self: center;
      border: 1px solid rgba(0, 188, 125, 0.2);
      background: linear-gradient(
        135deg,
        rgba(250, 250, 250, 0.1) 0%,
        rgba(250, 250, 250, 0.05) 100%
      );
    }

    .container-basic-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 8px;
      overflow: hidden;

      .container-name {
        font-weight: 600;
        font-size: 16px;
        line-height: 1.25;
        color: var(--text-base);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        max-width: 100%;
        width: fit-content;
      }

      .container-image {
        border: 1px solid var(--border-color);
        border-radius: 4px;
        padding: 4px 8px;
        padding-right: 16px;
        font-size: 14px;
        color: var(--text-color-3);
        position: relative;
        display: inline-block;
        width: fit-content;
        max-width: calc(100% - 8px);
        margin-right: 8px;
        overflow: visible;
      }
    }
  }

  &[data-theme='light'] .container-header {
    .container-logo {
      border: 1px solid rgba(0, 188, 125, 0.2);
      background: linear-gradient(135deg, rgba(3, 2, 19, 0.1) 0%, rgba(3, 2, 19, 0.05) 100%);
    }
  }

  .container-details {
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
        flex: 0;
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
        }
      }
    }
  }

  .container-status {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
}

.container-stats {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--divider-color);

  .stats-title {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-color-3);
  }

  .stat-header {
    display: flex;
    flex-direction: row;
    gap: 4px;
    align-items: center;
    color: var(--text-color-3);
  }

  .stats-grid {
    display: flex;
    flex-direction: row;
    gap: 8px;
    justify-content: space-between;

    .stat-item {
      display: flex;
      flex-direction: column;
      gap: 8px;
      justify-content: center;
      align-items: flex-start;
      flex: 0 0 25%;
    }

    .stat-status {
      .time-value,
      .time-status,
      .network-rate {
        color: var(--primary-color);

        &.stopped {
          color: var(--text-color-3);
        }
      }
    }
  }
}
</style>
