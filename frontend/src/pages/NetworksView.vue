<template>
  <div class="networks-page">
    <!-- 页面头部：搜索、过滤、排序、创建 -->
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

      <!-- 搜索框 -->
      <n-input
        v-model:value="searchKeyword"
        placeholder="名称、驱动或子网"
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

    <!-- 网络列表 -->
    <div class="networks-content">
      <n-spin :show="networkStore.loading && filteredNetworks.length === 0">
        <!-- 空状态 -->
        <div v-if="filteredNetworks.length === 0 && !networkStore.loading" class="empty-state">
          <n-empty description="没有找到网络">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <!-- 网络卡片网格 -->
        <div
          v-else
          class="networks-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet,
            'grid-cols-3': isLaptop || isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <NetworkCard
            v-for="network in filteredNetworks"
            :key="network.id"
            :network="network"
            @delete="() => handleDelete(network)"
            @detail="() => handleDetail(network)"
          />
        </div>
      </n-spin>
    </div>

    <!-- Teleport 到页面头部 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">网络管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ networkStore.stats.total }} 个网络， 使用中 {{ networkStore.stats.used }} 个，
            自定义 {{ networkStore.stats.custom }} 个
          </n-text>
        </div>
        <div class="flex gap-2">
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="networkStore.loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 创建网络按钮 -->
          <n-button circle size="tiny" @click="showCreateModal = true">
            <template #icon>
              <n-icon>
                <AddOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 清理按钮 -->
          <n-button @click="handlePrune" circle tertiary size="tiny">
            <template #icon>
              <n-icon>
                <TrashOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>

    <!-- 创建网络弹窗 -->
    <NetworkCreateModal v-model:show="showCreateModal"/>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useNetworkStore } from '@/store/network'
import { useResponsive } from '@/hooks/useResponsive'
import { renderIcon } from '@/common/utils'
import NetworkCard from '@/components/NetworkCard.vue'
import NetworkCreateModal from '@/components/NetworkCreateModal.vue'
import type { NetworkInfo } from '@/common/types'
import {
  SearchOutline,
  RefreshOutline,
  FunnelOutline,
  SwapVerticalOutline,
  AppsOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  TextOutline,
  CalendarOutline,
  TrashOutline,
  AddOutline,
  GitNetworkOutline,
  GridOutline,
} from '@vicons/ionicons5'
import { useDialog, useMessage } from 'naive-ui'

const networkStore = useNetworkStore()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const router = useRouter()
const dialog = useDialog()
const message = useMessage()

// 搜索和过滤状态
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const sortBy = ref<string>('name') // 默认按名称排序
const sortOrder = ref<'asc' | 'desc'>('asc') // 排序方向，默认升序
const showCreateModal = ref(false)

// 过滤菜单选项
const statusFilterMenuOptions = computed(() => [
  {
    label: '全部',
    key: null,
    icon: renderIcon(AppsOutline),
  },
  {
    label: '使用中',
    key: 'used',
    icon: renderIcon(CheckmarkCircleOutline),
  },
  {
    label: '未使用',
    key: 'unused',
    icon: renderIcon(CloseCircleOutline),
  },
  {
    label: '内置网络',
    key: 'builtin',
    icon: renderIcon(GitNetworkOutline),
  },
  {
    label: '自定义网络',
    key: 'custom',
    icon: renderIcon(GridOutline),
  },
  {
    type: 'divider',
    key: 'd1',
  },
  {
    label: 'Bridge 驱动',
    key: 'driver-bridge',
    icon: renderIcon(GitNetworkOutline),
  },
  {
    label: 'Overlay 驱动',
    key: 'driver-overlay',
    icon: renderIcon(GitNetworkOutline),
  },
  {
    label: 'Host 驱动',
    key: 'driver-host',
    icon: renderIcon(GitNetworkOutline),
  },
  {
    label: 'Macvlan 驱动',
    key: 'driver-macvlan',
    icon: renderIcon(GitNetworkOutline),
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
    label: `创建时间 ${sortBy.value === 'created' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'created',
    icon: renderIcon(CalendarOutline),
  },
])

// 处理过滤器菜单选择
const handleFilterSelect = (key: string | null) => {
  statusFilter.value = key
}

// 判断排序按钮是否应该显示为主色（激活状态）
const isSortActive = computed(() => {
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

// 过滤和排序后的网络列表
const filteredNetworks = computed(() => {
  let networks = networkStore.networks

  // 内置网络列表
  const builtInNames = ['bridge', 'host', 'none']

  // 1. 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    networks = networks.filter((network) => {
      // 搜索网络名称
      const matchesName = network.name.toLowerCase().includes(keyword)

      // 搜索驱动类型
      const matchesDriver = network.driver.toLowerCase().includes(keyword)

      // 搜索子网
      const matchesSubnet =
        network.ipam?.config?.some((cfg) => cfg.subnet?.toLowerCase().includes(keyword)) || false

      return matchesName || matchesDriver || matchesSubnet
    })
  }

  // 2. 状态过滤
  if (statusFilter.value) {
    networks = networks.filter((network) => {
      switch (statusFilter.value) {
        case 'used':
          return network.containerCount > 0
        case 'unused':
          return network.containerCount === 0
        case 'builtin':
          return builtInNames.includes(network.name)
        case 'custom':
          return !builtInNames.includes(network.name)
        case 'driver-bridge':
          return network.driver === 'bridge'
        case 'driver-overlay':
          return network.driver === 'overlay'
        case 'driver-host':
          return network.driver === 'host'
        case 'driver-macvlan':
          return network.driver === 'macvlan'
        default:
          return true
      }
    })
  }

  // 3. 排序
  return networks.sort((a, b) => {
    let result = 0

    switch (sortBy.value) {
      case 'name':
        result = a.name.localeCompare(b.name)
        break
      case 'created':
        result = new Date(a.created).getTime() - new Date(b.created).getTime()
        break
      default:
        result = 0
    }

    // 根据排序方向调整结果
    return sortOrder.value === 'asc' ? result : -result
  })
})

// 处理删除网络
const handleDelete = async (network: NetworkInfo) => {
  // 内置网络不允许删除
  const builtInNames = ['bridge', 'host', 'none']
  if (builtInNames.includes(network.name)) {
    message.warning('内置网络不能删除')
    return
  }

  if (network.containerCount > 0) {
    dialog.warning({
      title: '无法删除',
      content: `此网络正在被 ${network.containerCount} 个容器使用，无法删除。请先断开所有容器的连接。`,
      positiveText: '知道了',
    })
    return
  }

  const d = dialog.warning({
    title: '确认删除',
    content: `确定要删除网络 "${network.name}" 吗？此操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        d.loading = true
        await networkStore.deleteNetwork(network.id)
        message.success('网络删除成功')
      } catch (error: any) {
        message.error(`删除失败：${error.message || '未知错误'}`)
      } finally {
        d.loading = false
      }
    },
  })
}

// 处理查看详情
const handleDetail = (network: NetworkInfo) => {
  router.push(`/networks/${network.id}`)
}

// 处理刷新
const handleRefresh = async () => {
  try {
    await networkStore.fetchNetworks()
  } catch (error: any) {
    message.error(`刷新失败：${error.message || '未知错误'}`)
  }
}

// 处理清理未使用的网络
const handlePrune = () => {
  const unusedCount = networkStore.unusedNetworks.length

  if (unusedCount === 0) {
    message.info('没有未使用的网络需要清理')
    return
  }

  // 计算可清理的网络数量（排除内置网络和使用中的网络）
  const builtInNames = ['bridge', 'host', 'none']
  const prunableCount = networkStore.networks.filter(
    (n) => n.containerCount === 0 && !builtInNames.includes(n.name),
  ).length

  if (prunableCount === 0) {
    message.info('没有可清理的网络（内置网络不会被清理）')
    return
  }

  const d = dialog.warning({
    title: '确认清理',
    content: `确定要清理 ${prunableCount} 个未使用的网络吗？此操作不可恢复。（内置网络不会被清理）`,
    positiveText: '清理',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        d.loading = true
        const result = await networkStore.pruneNetworks()
        const deletedCount = result.networksDeleted?.length || 0
        message.success(`清理成功，删除了 ${deletedCount} 个网络`)
      } catch (error: any) {
        message.error(`清理失败：${error.message || '未知错误'}`)
      } finally {
        d.loading = false
      }
    },
  })
}



// 页面初始化
onMounted(async () => {
  await networkStore.fetchNetworks()
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

.networks-page {
  width: 100%;

  .networks-content {
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

  .networks-grid {
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

// 响应式调整
@media (max-width: 768px) {
  .networks-page {
    .networks-grid {
      gap: 8px;
    }
  }
}
</style>
