<template>
  <div
    class="network-card"
    :data-theme="settingStore.setting.theme"
    :class="{ 'card-used': isUsed, 'card-builtin': isBuiltIn }"
    @click="handleCardClick"
  >
    <!-- 顶部渐变条 -->
    <div v-if="isUsed" class="gradient-bar"></div>

    <div class="card-content">
      <!-- 网络头部信息 -->
      <div class="network-header">
        <div class="network-logo" :class="{ 'logo-used': isUsed }">
          <n-icon size="20"><NetworkIcon /></n-icon>
        </div>
        <div class="network-info">
          <n-tooltip :delay="500">
            <template #trigger>
              <div class="network-name">{{ network.name }}</div>
            </template>
            <span>{{ network.name }}</span>
          </n-tooltip>
          <div class="network-id">
            <n-text depth="3" class="text-xs">ID: {{ shortId }}</n-text>
          </div>
        </div>
        <div class="network-menu">
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

      <!-- 网络元数据 -->
      <div class="network-metadata">
        <div class="metadata-item">
          <n-icon size="14">
            <CpuIcon />
          </n-icon>
          <span class="metadata-label whitespace-nowrap">驱动:</span>
          <n-tag round :type="getDriverType(network.driver)" size="small" :bordered="true">
            {{ network.driver }}
          </n-tag>
        </div>
        <div class="metadata-divider"></div>
        <div class="metadata-item">
          <span class="metadata-label">作用域:</span>
          <n-tag round type="default" size="small" :bordered="true">
            {{ getScopeLabel(network.scope) }}
          </n-tag>
        </div>
      </div>

      <!-- 子网信息 -->
      <div v-if="hasSubnet" class="subnet-info">
        <div class="subnet-label">
          <n-icon size="14">
            <GlobeOutline />
          </n-icon>
          <span>子网</span>
        </div>
        <div class="subnet-value">
          <n-text code class="text-xs">{{ subnetDisplay }}</n-text>
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
          <div class="info-value">{{ formatCreatedTime(network.created) }}</div>
        </div>
        <div class="info-card">
          <div class="info-label">
            <n-icon size="14">
              <CubeIcon />
            </n-icon>
            <span>连接容器</span>
          </div>
          <div class="info-value-container">
            <div class="info-value">{{ network.containerCount || 0 }}</div>
            <div v-if="isUsed" class="usage-indicator"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import NetworkIcon from '@/assets/svg/network.svg?component'
import CpuIcon from '@/assets/svg/cpu.svg?component'
import type { NetworkInfo } from '@/common/types'
import { useSettingStore } from '@/store/setting'
import {
  TrashOutline,
  CalendarOutline as CalendarIcon,
  InformationCircleOutline,
  CubeOutline as CubeIcon,
  GlobeOutline,
} from '@vicons/ionicons5'
import dayjs from 'dayjs'
import { NIcon, useThemeVars } from 'naive-ui'
import { computed, h } from 'vue'

const settingStore = useSettingStore()

interface Props {
  network: NetworkInfo
}

interface Emits {
  (e: 'delete'): void
  (e: 'detail'): void
}

const props = defineProps<Props>()
const theme = useThemeVars()
const emits = defineEmits<Emits>()

// 是否正在使用
const isUsed = computed(() => (props.network.containerCount || 0) > 0)

// 是否是内置网络
const isBuiltIn = computed(() => {
  const builtInNames = ['bridge', 'host', 'none']
  return builtInNames.includes(props.network.name)
})

// 短ID
const shortId = computed(() => {
  return props.network.id.substring(0, 12)
})

// 是否有子网配置
const hasSubnet = computed(() => {
  return props.network.ipam?.config && props.network.ipam.config.length > 0
})

// 子网显示
const subnetDisplay = computed(() => {
  if (!hasSubnet.value) {
    return '-'
  }
  const config = props.network.ipam.config![0]
  return config.subnet || '-'
})

// 格式化创建时间
const formatCreatedTime = (created: string): string => {
  if (!created) {
    return '-'
  }
  return dayjs(created).format('YYYY-MM-DD HH:mm')
}

// 获取驱动类型标签类型
const getDriverType = (driver: string) => {
  const typeMap: Record<string, any> = {
    bridge: 'success',
    overlay: 'info',
    host: 'warning',
    macvlan: 'default',
    none: 'default',
  }
  return typeMap[driver] || 'default'
}

// 获取作用域标签
const getScopeLabel = (scope: string): string => {
  const labelMap: Record<string, string> = {
    local: '本地',
    swarm: 'Swarm',
    global: '全局',
  }
  return labelMap[scope] || scope
}

// 下拉菜单选项
const dropdownOptions = computed(() => {
  const options = [
    {
      key: 'detail',
      label: '查看详情',
      icon: () =>
        h(NIcon, null, {
          default: () => h(InformationCircleOutline),
        }),
    },
  ]

  // 内置网络不允许删除
  if (!isBuiltIn.value) {
    options.push({
      key: 'delete',
      label: '删除网络',
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
    })
  }

  return options
})

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
.network-card {
  &[data-theme='light'] {
    --network-card-background-color: #ffffff;
    --network-info-background-color: rgba(251, 249, 250, 0.7);
    &.card-used {
      background-color: transparent;
      background-image: linear-gradient(
        to right bottom,
        oklch(0.979 0.021 166.113) 0%,
        rgb(255, 255, 255) 100%
      );
    }
  }

  --network-card-background-color: #0a0a0a;
  --network-green-color: #00bc7d;
  --network-info-background-color: oklab(0.269 0 0 / 0.3);
  position: relative;
  border-radius: 14px;
  padding: 12px;
  transition: all 0.3s ease;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid var(--border-color);
  min-width: 320px;
  background-color: var(--network-card-background-color);
  &:hover {
    transform: translateY(-2px);
  }

  // 使用中的卡片样式
  &.card-used {
    border: 1px solid color-mix(in srgb, var(--network-green-color) 50%, transparent);
    box-shadow:
      0px 4px 6px -4px color-mix(in srgb, var(--network-green-color) 10%, transparent),
      0px 10px 15px -3px color-mix(in srgb, var(--network-green-color) 10%, transparent);
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
      var(--network-green-color) 0%,
      color-mix(in srgb, var(--network-green-color) 90%, transparent) 100%
    );
  }

  .card-content {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .network-header {
    display: flex;
    gap: 12px;
    align-items: flex-start;

    .network-logo {
      width: 40px;
      height: 40px;
      border-radius: 10px;
      display: flex;
      align-items: center;
      justify-content: center;
      border: 1px solid var(--border-color);
      &.logo-used {
        color: var(--network-green-color);
        background: color-mix(in srgb, var(--network-green-color) 10%, transparent);
        border: 1px solid color-mix(in srgb, var(--network-green-color) 10%, transparent);
      }
    }

    .network-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 4px;
      overflow: hidden;
      min-width: 0;

      .network-name {
        font-size: 16px;
        font-weight: 400;
        line-height: 1.5;
        color: var(--text-color-1);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        cursor: pointer;
      }

      .network-id {
        line-height: 1.2;
      }
    }

    .network-menu {
      flex-shrink: 0;
    }
  }

  .network-metadata {
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

  .subnet-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: var(--network-info-background-color);
    border-radius: 8px;

    .subnet-label {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 13px;
      color: var(--text-color-3);
    }

    .subnet-value {
      font-size: 13px;
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
      background: var(--network-info-background-color);
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
          background: var(--network-green-color);
          opacity: 0.8;
        }
      }
    }
  }
}
</style>
