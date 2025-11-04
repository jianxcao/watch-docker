<template>
  <div class="volumes-page">
    <!-- 页面头部：搜索、过滤、排序 -->
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
        placeholder="名称、驱动或挂载点"
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

    <!-- Volume 列表 -->
    <div class="volumes-content">
      <n-spin :show="volumeStore.loading && filteredVolumes.length === 0">
        <!-- 空状态 -->
        <div v-if="filteredVolumes.length === 0 && !volumeStore.loading" class="empty-state">
          <n-empty description="没有找到 Volume">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <!-- Volume 卡片网格 -->
        <div
          v-else
          class="volumes-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet,
            'grid-cols-3': isLaptop || isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <VolumeCard
            v-for="volume in filteredVolumes"
            :key="volume.name"
            :volume="volume"
            @delete="() => handleDelete(volume)"
            @detail="() => handleDetail(volume)"
          />
        </div>
      </n-spin>
    </div>

    <!-- Teleport 到页面头部 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">Volume 管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ volumeStore.stats.total }} 个 Volume， 总大小
            {{ volumeStore.stats.formattedTotalSize }}， 使用中 {{ volumeStore.stats.used }} 个
          </n-text>
        </div>
        <div class="flex gap-2">
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="volumeStore.loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useVolumeStore } from '@/store/volume'
import { useResponsive } from '@/hooks/useResponsive'
import { renderIcon } from '@/common/utils'
import VolumeCard from '@/components/VolumeCard.vue'
import type { VolumeInfo } from '@/common/types'
import {
  SearchOutline,
  RefreshOutline,
  FunnelOutline,
  SwapVerticalOutline,
  AppsOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  HomeOutline,
  GlobeOutline,
  TextOutline,
  CalendarOutline,
  ArchiveOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { useDialog, useMessage } from 'naive-ui'

const volumeStore = useVolumeStore()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const router = useRouter()
const dialog = useDialog()
const message = useMessage()

// 搜索和过滤状态
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const sortBy = ref<string>('name') // 默认按名称排序
const sortOrder = ref<'asc' | 'desc'>('asc') // 排序方向，默认升序

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
    label: '本地作用域',
    key: 'local',
    icon: renderIcon(HomeOutline),
  },
  {
    label: '全局作用域',
    key: 'global',
    icon: renderIcon(GlobeOutline),
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
  {
    label: `大小 ${sortBy.value === 'size' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'size',
    icon: renderIcon(ArchiveOutline),
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

// 过滤和排序后的 Volume 列表
const filteredVolumes = computed(() => {
  let volumes = volumeStore.volumes

  // 1. 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    volumes = volumes.filter((volume) => {
      // 搜索 Volume 名称
      const matchesName = volume.name.toLowerCase().includes(keyword)

      // 搜索驱动类型
      const matchesDriver = volume.driver.toLowerCase().includes(keyword)

      // 搜索挂载点
      const matchesMountpoint = volume.mountpoint.toLowerCase().includes(keyword)

      return matchesName || matchesDriver || matchesMountpoint
    })
  }

  // 2. 状态过滤
  if (statusFilter.value) {
    volumes = volumes.filter((volume) => {
      switch (statusFilter.value) {
        case 'used':
          return volume.usageData && volume.usageData.refCount > 0
        case 'unused':
          return !volume.usageData || volume.usageData.refCount === 0
        case 'local':
          return volume.scope === 'local'
        case 'global':
          return volume.scope === 'global'
        default:
          return true
      }
    })
  }

  // 3. 排序
  return volumes.sort((a, b) => {
    let result = 0

    switch (sortBy.value) {
      case 'name':
        result = a.name.localeCompare(b.name)
        break
      case 'created':
        result = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
        break
      case 'size':
        const sizeA = a.usageData?.size || 0
        const sizeB = b.usageData?.size || 0
        result = sizeA - sizeB
        break
      default:
        result = 0
    }

    // 根据排序方向调整结果
    return sortOrder.value === 'asc' ? result : -result
  })
})

// 处理删除Volume
const handleDelete = async (volume: VolumeInfo) => {
  const refCount = volume.usageData?.refCount || 0

  if (refCount > 0) {
    dialog.warning({
      title: '无法删除',
      content: `此 Volume 正在被 ${refCount} 个容器使用，无法删除。请先停止或删除使用该 Volume 的容器。`,
      positiveText: '知道了',
    })
    return
  }

  const d = dialog.warning({
    title: '确认删除',
    content: `确定要删除 Volume "${volume.name}" 吗？此操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        d.loading = true
        await volumeStore.deleteVolume(volume.name, false)
        message.success('Volume 删除成功')
      } catch (error: any) {
        message.error(`删除失败：${error.message || '未知错误'}`)
      } finally {
        d.loading = false
      }
    },
  })
}

// 处理查看详情
const handleDetail = (volume: VolumeInfo) => {
  router.push(`/volumes/${volume.name}`)
}

// 处理刷新
const handleRefresh = async () => {
  try {
    await volumeStore.fetchVolumes()
  } catch (error: any) {
    message.error(`刷新失败：${error.message || '未知错误'}`)
  }
}

// 处理清理未使用的Volume
const handlePrune = () => {
  const unusedCount = volumeStore.unusedVolumes.length

  if (unusedCount === 0) {
    message.info('没有未使用的 Volume 需要清理')
    return
  }

  const d = dialog.warning({
    title: '确认清理',
    content: `确定要清理 ${unusedCount} 个未使用的 Volume 吗？此操作不可恢复。`,
    positiveText: '清理',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        d.loading = true
        const result = await volumeStore.pruneVolumes()
        const deletedCount = result.volumesDeleted?.length || 0
        message.success(`清理成功，删除了 ${deletedCount} 个 Volume`)
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
  await volumeStore.fetchVolumes()
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

.volumes-page {
  width: 100%;

  .volumes-content {
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

  .volumes-grid {
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
  .volumes-page {
    .volumes-grid {
      gap: 8px;
    }
  }
}
</style>
