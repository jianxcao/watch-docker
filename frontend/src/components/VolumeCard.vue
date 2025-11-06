<template>
  <div
    class="volume-card"
    :data-theme="settingStore.setting.theme"
    :class="{ 'card-used': isUsed }"
    @click="handleCardClick"
  >
    <!-- 顶部渐变条 -->
    <div v-if="isUsed" class="gradient-bar"></div>

    <div class="card-content">
      <!-- Volume头部信息 -->
      <div class="volume-header">
        <div class="volume-logo" :class="{ 'logo-used': isUsed }">
          <n-icon size="20"><VolumeIcon /></n-icon>
        </div>
        <div class="volume-info">
          <n-tooltip :delay="500">
            <template #trigger>
              <div class="volume-name">{{ volume.name }}</div>
            </template>
            <span>{{ volume.name }}</span>
          </n-tooltip>
          <div class="volume-metadata">
            <div class="metadata-item">
              <n-icon size="14">
                <CpuIcon />
              </n-icon>
              <span class="metadata-label whitespace-nowrap">驱动:</span>
              <n-tag round :type="isUsed ? 'success' : 'default'" size="small" :bordered="true">
                {{ volume.driver }}
              </n-tag>
            </div>
            <div class="metadata-divider"></div>
            <div class="metadata-item">
              <span class="metadata-label">作用域:</span>
              <n-tag round type="default" size="small" :bordered="true">
                {{ volume.scope === 'local' ? '本地' : '全局' }}
              </n-tag>
            </div>
          </div>
        </div>
        <div class="volume-menu">
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

      <!-- 信息卡片 -->
      <div class="info-cards">
        <div class="info-card">
          <div class="info-label">
            <n-icon size="14">
              <CalendarIcon />
            </n-icon>
            <span>创建时间</span>
          </div>
          <div class="info-value">{{ formatCreatedTime(volume.createdAt) }}</div>
        </div>
        <div class="info-card">
          <div class="info-label">
            <n-icon size="14">
              <CubeIcon />
            </n-icon>
            <span>使用容器</span>
          </div>
          <div class="info-value-container">
            <div class="info-value">{{ volume.usageData?.refCount || 0 }}</div>
            <div v-if="isUsed" class="usage-indicator"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import VolumeIcon from '@/assets/svg/volume.svg?component'
import CpuIcon from '@/assets/svg/cpu.svg?component'
import type { VolumeInfo } from '@/common/types'
import { useSettingStore } from '@/store/setting'
import {
  TrashOutline,
  CalendarOutline as CalendarIcon,
  InformationCircleOutline,
  CubeOutline as CubeIcon,
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

// 格式化创建时间
const formatCreatedTime = (createdAt: string): string => {
  if (!createdAt) {
    return '-'
  }
  return dayjs(createdAt).format('YYYY-MM-DD HH:mm')
}

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
  &[data-theme='light'] {
    --volume-card-background-color: #ffffff;
    --volume-info-background-color: rgba(251, 249, 250, 0.7);
    &.card-used {
      background-color: transparent;
      background-image: linear-gradient(
        to right bottom,
        oklch(0.979 0.021 166.113) 0%,
        rgb(255, 255, 255) 100%
      );
    }
  }

  --volume-card-background-color: #0a0a0a;
  --volume-geen-color: #00bc7d;
  --volume-info-background-color: oklab(0.269 0 0 / 0.3);
  position: relative;
  border-radius: 14px;
  padding: 12px;
  transition: all 0.3s ease;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid var(--border-color);
  min-width: 320px;
  background-color: var(--volume-card-background-color);
  &:hover {
    transform: translateY(-2px);
  }

  // 使用中的卡片样式
  &.card-used {
    border: 1px solid color-mix(in srgb, var(--volume-geen-color) 50%, transparent);
    box-shadow:
      0px 4px 6px -4px color-mix(in srgb, var(--volume-geen-color) 10%, transparent),
      0px 10px 15px -3px color-mix(in srgb, var(--volume-geen-color) 10%, transparent);
  }

  // 顶部渐变条
  .gradient-bar {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(
      90deg,
      var(--volume-geen-color) 0%,
      color-mix(in srgb, var(--volume-geen-color) 90%, transparent) 100%
    );
  }

  .card-content {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .volume-header {
    display: flex;
    gap: 12px;
    align-items: flex-start;

    .volume-logo {
      width: 40px;
      height: 40px;
      border-radius: 10px;
      display: flex;
      align-items: center;
      justify-content: center;
      border: 1px solid var(--border-color);
      &.logo-used {
        color: var(--volume-geen-color);
        background: color-mix(in srgb, var(--volume-geen-color) 10%, transparent);
        border: 1px solid color-mix(in srgb, var(--volume-geen-color) 10%, transparent);
      }
    }

    .volume-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 8px;
      overflow: hidden;
      min-width: 0;

      .volume-name {
        font-size: 16px;
        font-weight: 400;
        line-height: 1.5;
        color: var(--text-color-1);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        cursor: pointer;
      }

      .volume-metadata {
        display: flex;
        align-items: center;
        gap: 8px;
        height: 22px;

        .metadata-item {
          display: flex;
          align-items: center;
          gap: 6px;
        }
        .metadata-label {
          color: var(--text-color-3);
        }

        .metadata-divider {
          width: 1px;
          height: 12px;
          background: var(--border-color);
        }
      }
    }

    .volume-menu {
      flex-shrink: 0;
    }
  }

  .info-cards {
    display: flex;
    flex-direction: column;
    gap: 12px;

    .info-card {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 0 12px;
      background: var(--volume-info-background-color);
      border-radius: 10px;
      min-height: 40px;

      .info-label {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 14px;
        font-weight: 400;
        color: var(--text-color-3);
      }

      .info-value {
        font-size: 14px;
        font-weight: 400;
        color: var(--text-color-1);
      }

      .info-value-container {
        display: flex;
        align-items: center;
        gap: 6px;

        .usage-indicator {
          width: 6px;
          height: 6px;
          border-radius: 50%;
          background: var(--volume-geen-color);
          opacity: 0.8;
        }
      }
    }
  }
}
</style>
