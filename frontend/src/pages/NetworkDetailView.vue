<template>
  <div class="network-detail-page">
    <n-spin :show="loading">
      <div v-if="networkDetail" class="detail-container">
        <!-- 基本信息 -->
        <n-card title="基本信息" class="info-card">
          <div class="info-grid">
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16">
                  <NetworkIcon class="network-icon" />
                </n-icon>
                网络名称
              </div>
              <div class="info-value">{{ networkDetail.network.name }}</div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16"> <HashtagIcon class="id-icon" /> </n-icon>网络ID
              </div>
              <div class="info-value">
                <n-text code>{{ networkDetail.network.id.substring(0, 12) }}</n-text>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16"> <GitNetworkOutline class="driver-icon" /> </n-icon>驱动类型
              </div>
              <div class="info-value">
                <n-tag :bordered="false" :type="getDriverType(networkDetail.network.driver)" round>
                  {{ networkDetail.network.driver }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16"> <GlobeOutline class="scope-icon" /> </n-icon>作用域
              </div>
              <div class="info-value">
                <n-tag :bordered="false" type="default" round>
                  {{ getScopeLabel(networkDetail.network.scope) }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="14"> <CalendarIcon class="calendar-icon" /> </n-icon>创建时间
              </div>
              <div class="info-value">{{ formatCreatedTime(networkDetail.network.created) }}</div>
            </div>
            <div class="info-item">
              <div class="info-label">连接容器数</div>
              <div class="info-value">
                <n-tag :bordered="true" type="warning" round>
                  {{ networkDetail.network.containerCount || 0 }} 个容器
                </n-tag>
              </div>
            </div>
          </div>

          <n-divider />

          <div class="info-grid">
            <div class="info-item">
              <div class="info-label">内部网络</div>
              <div class="info-value">
                <n-tag :type="networkDetail.network.internal ? 'info' : 'default'" size="small">
                  {{ networkDetail.network.internal ? '是' : '否' }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">可连接</div>
              <div class="info-value">
                <n-tag
                  :type="networkDetail.network.attachable ? 'success' : 'default'"
                  size="small"
                >
                  {{ networkDetail.network.attachable ? '是' : '否' }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">Ingress</div>
              <div class="info-value">
                <n-tag :type="networkDetail.network.ingress ? 'info' : 'default'" size="small">
                  {{ networkDetail.network.ingress ? '是' : '否' }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">IPv6</div>
              <div class="info-value">
                <n-tag
                  :type="networkDetail.network.enableIPv6 ? 'success' : 'default'"
                  size="small"
                >
                  {{ networkDetail.network.enableIPv6 ? '已启用' : '未启用' }}
                </n-tag>
              </div>
            </div>
          </div>
        </n-card>

        <!-- IPAM 配置 -->
        <n-card v-if="hasIPAMConfig" title="IPAM 配置" class="info-card">
          <div class="info-grid">
            <div class="info-item">
              <div class="info-label">IPAM 驱动</div>
              <div class="info-value">{{ networkDetail.network.ipam?.driver || 'default' }}</div>
            </div>
          </div>

          <n-divider v-if="networkDetail.network.ipam?.config" />

          <div
            v-for="(config, index) in networkDetail.network.ipam?.config"
            :key="index"
            class="ipam-config-section"
          >
            <n-h4 class="my-2">配置 {{ index + 1 }}</n-h4>
            <div class="info-grid">
              <div v-if="config.subnet" class="info-item info-item-full">
                <div class="info-label">
                  <n-icon size="16"> <GlobeOutline /> </n-icon>子网
                </div>
                <div class="info-value">
                  <n-text code>{{ config.subnet }}</n-text>
                </div>
              </div>
              <div v-if="config.gateway" class="info-item">
                <div class="info-label">
                  <n-icon size="16"> <RouterIcon /> </n-icon>网关
                </div>
                <div class="info-value">
                  <n-text code>{{ config.gateway }}</n-text>
                </div>
              </div>
              <div v-if="config.ipRange" class="info-item">
                <div class="info-label">IP 范围</div>
                <div class="info-value">
                  <n-text code>{{ config.ipRange }}</n-text>
                </div>
              </div>
            </div>
          </div>
        </n-card>

        <!-- 驱动选项 -->
        <n-card v-if="hasOptions" title="驱动选项" class="info-card">
          <n-space vertical>
            <div
              v-for="(value, key) in networkDetail.network.options"
              :key="key"
              class="option-item"
            >
              <n-text strong>{{ key }}:</n-text>
              <n-text code>{{ value }}</n-text>
            </div>
          </n-space>
        </n-card>

        <!-- 标签信息 -->
        <n-card v-if="hasLabels" title="标签" class="info-card">
          <n-space>
            <n-tag
              v-for="(value, key) in networkDetail.network.labels"
              :key="key"
              :bordered="false"
              type="info"
            >
              {{ key }}: {{ value }}
            </n-tag>
          </n-space>
        </n-card>

        <!-- 已连接的容器 -->
        <n-card title="已连接的容器" class="info-card">
          <div
            v-if="!networkDetail.containers || networkDetail.containers.length === 0"
            class="empty-container"
          >
            <n-empty description="没有容器连接到此网络" />
          </div>
          <div v-else class="container-list">
            <div
              v-for="container in networkDetail.containers"
              :key="container.id"
              class="container-item"
              @click="handleContainerClick(container)"
            >
              <div class="container-info">
                <div class="container-name">
                  <n-icon size="20" class="mr-2">
                    <CubeOutline />
                  </n-icon>
                  {{ container.name }}
                  <n-tag
                    :bordered="false"
                    size="small"
                    :type="container.running ? 'success' : 'default'"
                    class="ml-2"
                  >
                    {{ container.running ? '运行中' : '已停止' }}
                  </n-tag>
                </div>
                <div v-if="container.image" class="container-image">
                  <n-text depth="3">{{ container.image }}</n-text>
                </div>
                <div class="container-network-info">
                  <div v-if="container.ipv4Address" class="network-info-item">
                    <n-icon size="16" class="mr-1">
                      <GlobeOutline />
                    </n-icon>
                    <n-text depth="3" code class="text-xs">
                      IPv4: {{ container.ipv4Address }}
                    </n-text>
                  </div>
                  <div v-if="container.ipv6Address" class="network-info-item">
                    <n-icon size="16" class="mr-1">
                      <GlobeOutline />
                    </n-icon>
                    <n-text depth="3" code class="text-xs">
                      IPv6: {{ container.ipv6Address }}
                    </n-text>
                  </div>
                  <div v-if="container.macAddress" class="network-info-item">
                    <n-text depth="3" code class="text-xs">
                      MAC: {{ container.macAddress }}
                    </n-text>
                  </div>
                </div>
              </div>
              <div class="container-action">
                <n-icon size="20">
                  <ChevronForwardOutline />
                </n-icon>
              </div>
            </div>
          </div>
        </n-card>
      </div>
    </n-spin>

    <!-- Teleport 到页面头部 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div class="flex items-center gap-3">
          <!-- 返回按钮 -->
          <n-button @click="handleBack" text circle>
            <template #icon>
              <n-icon size="20">
                <ArrowBackOutline />
              </n-icon>
            </template>
          </n-button>
          <div>
            <n-h2 class="m-0 text-lg">网络详情</n-h2>
            <n-text depth="3" class="text-xs max-md:hidden">
              {{ networkName }}
            </n-text>
          </div>
        </div>
        <div class="flex gap-2">
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 删除按钮 -->
          <n-button
            v-if="!isBuiltInNetwork"
            @click="handleDelete"
            circle
            size="tiny"
            tertiary
            type="error"
          >
            <template #icon>
              <n-icon>
                <TrashOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { networkApi } from '@/common/api'
import type { NetworkDetailResponse, NetworkContainer } from '@/common/types'
import NetworkIcon from '@/assets/svg/network.svg?component'
import HashtagIcon from '@/assets/svg/hashtag.svg?component'
import RouterIcon from '@/assets/svg/router.svg?component'
import {
  RefreshOutline,
  TrashOutline,
  ArrowBackOutline,
  CubeOutline,
  ChevronForwardOutline,
  GlobeOutline,
  CalendarOutline as CalendarIcon,
  GitNetworkOutline,
} from '@vicons/ionicons5'
import { useDialog, useMessage } from 'naive-ui'
import dayjs from 'dayjs'

const route = useRoute()
const router = useRouter()
const dialog = useDialog()
const message = useMessage()

const loading = ref(false)
const networkDetail = ref<NetworkDetailResponse | null>(null)

const networkName = computed(() => route.params.id as string)

// 是否是内置网络
const isBuiltInNetwork = computed(() => {
  if (!networkDetail.value) {
    return false
  }
  const builtInNames = ['bridge', 'host', 'none']
  return builtInNames.includes(networkDetail.value.network.name)
})

const hasLabels = computed(() => {
  return networkDetail.value && Object.keys(networkDetail.value.network.labels || {}).length > 0
})

const hasOptions = computed(() => {
  return networkDetail.value && Object.keys(networkDetail.value.network.options || {}).length > 0
})

const hasIPAMConfig = computed(() => {
  return networkDetail.value && networkDetail.value.network.ipam
})

// 格式化创建时间
const formatCreatedTime = (created: string): string => {
  if (!created) {
    return '-'
  }
  return dayjs(created).format('YYYY-MM-DD HH:mm:ss')
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

// 获取网络详情
const fetchNetworkDetail = async () => {
  loading.value = true
  try {
    const response = await networkApi.getNetwork(networkName.value)
    if (response.code === 0) {
      networkDetail.value = response.data
    } else {
      message.error(`获取网络详情失败：${response.msg}`)
      router.push('/networks')
    }
  } catch (error: any) {
    message.error(`获取网络详情失败：${error.message || '未知错误'}`)
    router.push('/networks')
  } finally {
    loading.value = false
  }
}

// 处理返回
const handleBack = () => {
  router.push('/networks')
}

// 处理刷新
const handleRefresh = async () => {
  await fetchNetworkDetail()
}

// 处理删除
const handleDelete = () => {
  if (!networkDetail.value) {
    return
  }

  if (networkDetail.value.network.containerCount > 0) {
    dialog.warning({
      title: '无法删除',
      content: `此网络正在被 ${networkDetail.value.network.containerCount} 个容器使用，无法删除。请先断开所有容器的连接。`,
      positiveText: '知道了',
    })
    return
  }

  dialog.warning({
    title: '确认删除',
    content: `确定要删除网络 "${networkDetail.value.network.name}" 吗？此操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const response = await networkApi.deleteNetwork(networkName.value)
        if (response.code === 0) {
          message.success('网络删除成功')
          router.push('/networks')
        } else {
          message.error(`删除失败：${response.msg}`)
        }
      } catch (error: any) {
        message.error(`删除失败：${error.message || '未知错误'}`)
      }
    },
  })
}

// 处理容器点击
const handleContainerClick = (container: NetworkContainer) => {
  router.push(`/containers/${container.id}`)
}

// 页面初始化
onMounted(async () => {
  await fetchNetworkDetail()
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

.network-detail-page {
  width: 100%;

  .detail-container {
    display: flex;
    flex-direction: column;
    gap: 16px;

    .info-card {
      box-shadow: var(--box-shadow-1);
    }

    .info-grid {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 24px 32px;
      .info-item {
        display: flex;
        flex-direction: column;
        gap: 8px;

        &.info-item-full {
          grid-column: 1 / -1;
        }
        .info-label {
          font-size: 14px;
          line-height: 20px;
          color: var(--text-color-3);
          font-weight: 500;
          white-space: nowrap;
          display: inline-flex;
          align-items: center;
          gap: 8px;
          .network-icon,
          .scope-icon,
          .calendar-icon,
          .id-icon {
            color: var(--primary-color);
          }
          .driver-icon {
            color: #2b7fff;
          }
        }

        .info-value {
          font-size: 14px;
          color: var(--n-text-color-1);
          word-break: break-all;
        }
      }
    }

    .ipam-config-section {
      margin-top: 16px;
      padding: 16px;
      border-radius: 8px;
      background: var(--n-color-embedded);
    }

    .option-item {
      display: flex;
      gap: 8px;
      align-items: center;
      padding: 4px 0;
    }

    .empty-container {
      padding: 40px 0;
    }

    .container-list {
      display: flex;
      flex-direction: column;
      gap: 12px;

      .container-item {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 16px;
        border-radius: 8px;
        background: var(--n-color-embedded);
        cursor: pointer;
        transition: all 0.3s ease;

        &:hover {
          background: var(--n-color-embedded-popover);
          transform: translateX(4px);
        }

        .container-info {
          flex: 1;
          display: flex;
          flex-direction: column;
          gap: 8px;

          .container-name {
            font-weight: 600;
            font-size: 16px;
            display: flex;
            align-items: center;
          }

          .container-image {
            font-size: 14px;
          }

          .container-network-info {
            display: flex;
            flex-wrap: wrap;
            gap: 12px;
            font-size: 12px;

            .network-info-item {
              display: flex;
              align-items: center;
            }
          }
        }

        .container-action {
          display: flex;
          align-items: center;
          color: var(--text-color-3);
        }
      }
    }
  }
}

@media (max-width: 768px) {
  .network-detail-page {
    .detail-container {
      gap: 12px;

      .info-grid {
        grid-template-columns: 1fr;
        gap: 20px;

        .info-item {
          &.info-item-full {
            grid-column: 1;
          }
        }
      }
    }
  }
}
</style>
