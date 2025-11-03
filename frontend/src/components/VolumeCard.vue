<template>
  <div
    class="volume-card"
    :data-theme="settingStore.setting.theme"
    :class="{ 'card-used': isUsed }"
    @click="handleCardClick"
  >
    <!-- 状态指示条 -->
    <div class="status-bar" :class="isUsed ? 'used' : 'unused'"></div>
    <div class="card-content">
      <!-- Volume头部信息 -->
      <div class="volume-header">
        <div class="volume-logo">
          <n-icon size="24">
            <DatabaseIcon />
          </n-icon>
          <div class="absolute -top-1 -right-1">
            <div
              class="w-4 h-4 rounded-full flex items-center justify-center"
              :class="statusConfig.color"
            >
              <div
                v-if="isUsed"
                class="w-2 h-2 rounded-full"
                :class="statusConfig.pulseColor"
              ></div>
            </div>
          </div>
        </div>
        <div class="volume-basic-info">
          <n-tooltip :delay="500">
            <template #trigger>
              <div class="volume-name">{{ volume.name }}</div>
            </template>
            <span>{{ volume.name }}</span>
          </n-tooltip>
          <div class="volume-driver">
            <n-tag :bordered="false" size="small" type="info">
              {{ volume.driver }}
            </n-tag>
          </div>
        </div>
        <div class="volume-status">
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

      <!-- Volume详细信息 -->
      <div class="volume-details">
        <div class="detail-row">
          <div class="detail-item">
            <div class="detail-label">
              <n-icon size="16">
                <TimeOutline />
              </n-icon>
              创建时间
            </div>
            <div class="detail-label">
              <n-icon size="16">
                <GlobeIcon />
              </n-icon>
              作用域
            </div>
          </div>
          <div class="detail-item">
            <div class="detail-value min-w-[152px]">
              {{ formatCreatedTime(volume.createdAt) }}
            </div>
            <div class="detail-value">
              <n-tag
                :bordered="false"
                size="small"
                :type="volume.scope === 'local' ? 'success' : 'info'"
              >
                {{ volume.scope === 'local' ? '本地' : '全局' }}
              </n-tag>
            </div>
          </div>
        </div>
      </div>

      <!-- 使用情况 -->
      <div class="volume-stats">
        <div class="flex flex-row justify-between items-center mb-2">
          <div class="stats-title">使用情况</div>
        </div>
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <CubeIcon />
              </n-icon>
              <span>容器数</span>
            </div>
            <div class="stat-value">{{ volume.usageData?.refCount || 0 }}</div>
          </div>

          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <ArchiveIcon />
              </n-icon>
              <span>大小</span>
            </div>
            <div class="stat-value">{{ formatBytes(volume.usageData?.size || 0) }}</div>
          </div>
          <!--
          <div class="stat-item">
            <div class="stat-header">
              <n-icon size="12">
                <FolderIcon />
              </n-icon>
              <span>挂载点</span>
            </div>
            <div class="mountpoint">{{ formatMountpoint(volume.mountpoint) }}</div>
          </div> -->
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import DatabaseIcon from '@/assets/svg/containerLogo.svg?component'
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import type { VolumeInfo } from '@/common/types'
import { formatBytes } from '@/common/utils'
import { useSettingStore } from '@/store/setting'
import {
  TimeOutline,
  TrashOutline,
  InformationCircleOutline,
  CubeOutline as CubeIcon,
  ArchiveOutline as ArchiveIcon,
  GlobeOutline as GlobeIcon,
} from '@vicons/ionicons5'
import dayjs from 'dayjs'
import { NIcon, useThemeVars } from 'naive-ui'
import { computed, h } from 'vue'

const settingStore = useSettingStore()

interface Props {
  volume: VolumeInfo
}

interface Emits {
  (e: 'delete'): void
  (e: 'detail'): void
}

const props = defineProps<Props>()
const theme = useThemeVars()
const emits = defineEmits<Emits>()

// 是否正在使用
const isUsed = computed(() => (props.volume.usageData?.refCount || 0) > 0)

const statusConfig = computed(() => {
  return {
    color: isUsed.value ? 'bg-emerald-500' : 'bg-slate-500',
    pulseColor: isUsed.value ? 'bg-emerald-400' : 'bg-slate-400',
  }
})

// 格式化创建时间
const formatCreatedTime = (createdAt: string): string => {
  if (!createdAt) {
    return '-'
  }
  return dayjs(createdAt).format('YYYY-MM-DD HH:mm')
}

// 格式化挂载点
// const formatMountpoint = (mountpoint: string): string => {
//   if (!mountpoint) {
//     return '-'
//   }
//   // 只显示最后两级目录
//   const parts = mountpoint.split('/')
//   if (parts.length > 2) {
//     return '.../' + parts.slice(-2).join('/')
//   }
//   return mountpoint
// }

// 下拉菜单选项
const dropdownOptions = computed(() => [
  {
    key: 'detail',
    label: '查看详情',
    icon: () =>
      h(NIcon, null, {
        default: () => h(InformationCircleOutline),
      }),
  },
  {
    key: 'delete',
    label: '删除Volume',
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
  },
])

// 处理下拉菜单选择
const handleMenuSelect = (key: string) => {
  switch (key) {
    case 'detail':
      emits('detail')
      break
    case 'delete':
      emits('delete')
      break
  }
}

// 处理卡片点击
const handleCardClick = () => {
  emits('detail')
}
</script>

<style scoped lang="less">
.volume-card {
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
  }

  &:has(.status-bar.used) {
    border: 2px solid rgba(0, 188, 125, 0.2);
    background: linear-gradient(135deg, rgba(0, 188, 125, 0.05) 0%, rgba(0, 201, 80, 0.05) 100%);
  }

  &[data-theme='light']:has(.status-bar.used) {
    border: 2px solid rgba(0, 188, 125, 0.2);
    background: linear-gradient(135deg, rgba(0, 188, 125, 0.05) 0%, rgba(0, 201, 80, 0.05) 100%);
  }

  &:has(.status-bar.unused) {
    background: linear-gradient(
      135deg,
      rgba(98, 116, 142, 0.05) 0%,
      rgba(106, 114, 130, 0.05) 100%
    );
    border-color: rgba(98, 116, 142, 0.2);
  }

  &[data-theme='light']:has(.status-bar.unused) {
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

    &.used {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #00bc7d;
    }

    &.unused {
      background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0) 100%), #62748e;
    }
  }

  .card-content {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .volume-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    white-space: nowrap;
    flex-wrap: nowrap;

    .volume-logo {
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

    .volume-basic-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 8px;
      overflow: hidden;

      .volume-name {
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

      .volume-driver {
        display: inline-block;
        width: fit-content;
      }
    }
  }

  &[data-theme='light'] .volume-header {
    .volume-logo {
      border: 1px solid rgba(0, 188, 125, 0.2);
      background: linear-gradient(135deg, rgba(3, 2, 19, 0.1) 0%, rgba(3, 2, 19, 0.05) 100%);
    }
  }

  .volume-details {
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

  .volume-status {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
}

.volume-stats {
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
    font-size: 12px;
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
      flex: 0 0 33.33%;
    }

    .stat-value {
      font-size: 14px;
      font-weight: 600;
      color: var(--text-color-1);
    }

    .mountpoint {
      font-size: 12px;
      color: var(--text-color-3);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 100%;
    }
  }
}
</style>
