<template>
  <div class="images-page">
    <!-- 页面头部 -->
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
      <!-- 搜索 -->
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索镜像标签或ID"
        style="width: 200px"
        clearable
      >
        <template #prefix>
          <n-icon>
            <SearchOutline />
          </n-icon>
        </template>
      </n-input>
    </n-space>

    <!-- 镜像列表 -->
    <div class="images-content">
      <n-spin :show="imageStore.loading && filteredImages.length === 0">
        <div v-if="filteredImages.length === 0 && !imageStore.loading" class="empty-state">
          <n-empty description="没有找到镜像">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <div
          v-else
          class="images-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet,
            'grid-cols-3': isLaptop || isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <ImageCard
            v-for="image in filteredImages"
            :key="image.id"
            :image="image"
            @delete="handleDelete"
          />
        </div>
      </n-spin>
    </div>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">镜像管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ imageStore.stats.total }} 个镜像， 总大小
            {{ imageStore.stats.formattedTotalSize }}，
          </n-text>
        </div>
        <n-space size="small">
          <n-button @click="handleRefresh" :loading="imageStore.loading" circle size="tiny">
            <template #icon>
              <RefreshOutline />
            </template>
          </n-button>
          <n-tooltip trigger="hover" :delay="500">
            <template #trigger>
              <n-button @click="showImportModal = true" circle size="tiny">
                <template #icon>
                  <CloudUploadOutline />
                </template>
              </n-button>
            </template>
            镜像导入
          </n-tooltip>
        </n-space>
      </div>
    </Teleport>

    <!-- 镜像导入弹窗 -->
    <ImageImportModal v-model:show="showImportModal" @success="handleImportSuccess" />
  </div>
</template>

<script setup lang="ts">
import type { ImageInfo } from '@/common/types'
import { renderIcon } from '@/common/utils'
import ImageCard from '@/components/ImageCard.vue'
import ImageImportModal from '@/components/ImageImportModal.vue'
import { useImage } from '@/hooks/useImage'
import { useResponsive } from '@/hooks/useResponsive'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import {
  AppsOutline,
  CalendarOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  CloudUploadOutline,
  FunnelOutline,
  RefreshOutline,
  ResizeOutline,
  SearchOutline,
  SwapVerticalOutline,
  TextOutline,
} from '@vicons/ionicons5'
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useAppStore } from '@/store/app'

const imageStore = useImageStore()
const containerStore = useContainerStore()
const imageHooks = useImage()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const message = useMessage()
const appStore = useAppStore()
// 弹窗状态
const showImportModal = ref(false)

// 搜索关键词
const searchKeyword = ref('')
const statusFilter = ref<string | null>(null)
const sortBy = ref<string>('created') // 默认按创建时间排序
const sortOrder = ref<'asc' | 'desc'>('desc') // 默认降序

// 状态过滤菜单选项
const statusFilterMenuOptions = computed(() => [
  {
    label: '全部',
    key: null,
    icon: renderIcon(AppsOutline),
  },
  {
    label: '使用中',
    key: 'in-use',
    icon: renderIcon(CheckmarkCircleOutline),
  },
  {
    label: '未使用',
    key: 'unused',
    icon: renderIcon(CloseCircleOutline),
  },
])

// 判断排序按钮是否应该显示为主色（激活状态）
const isSortActive = computed(() => {
  // 如果不是默认排序设置（创建时间降序），则显示为激活状态
  return sortBy.value !== 'created' || sortOrder.value !== 'desc'
})

// 排序菜单选项
const sortMenuOptions = computed(() => [
  {
    label: `名称 ${sortBy.value === 'name' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'name',
    icon: renderIcon(TextOutline),
  },
  {
    label: `大小 ${sortBy.value === 'size' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'size',
    icon: renderIcon(ResizeOutline),
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

// 过滤和排序后的镜像列表
const filteredImages = computed(() => {
  let images = [...imageStore.normalImages]

  // 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    images = images.filter((image) => {
      const displayTag = imageStore.getImageDisplayTag(image).toLowerCase()
      const id = image.id.toLowerCase()
      const tags = image.repoTags?.join(' ').toLowerCase() || ''

      return displayTag.includes(keyword) || id.includes(keyword) || tags.includes(keyword)
    })
  }

  // 状态过滤
  if (statusFilter.value) {
    images = images.filter((image) => {
      const isUse = imageHooks.isImageInUse(image)
      // console.debug('isUse', image.repoTags, isUse)
      switch (statusFilter.value) {
        case 'in-use':
          return isUse
        case 'unused':
          return !isUse
        default:
          return false
      }
    })
  }

  // 排序
  const sortedImages = images.sort((a, b) => {
    let result = 0
    switch (sortBy.value) {
      case 'name':
        const nameA = imageStore.getImageDisplayTag(a).toLowerCase()
        const nameB = imageStore.getImageDisplayTag(b).toLowerCase()
        result = nameA.localeCompare(nameB)
        break
      case 'size':
        result = a.size - b.size
        break
      case 'created':
        result = a.created - b.created
        break
      default:
        return 0
    }
    // 根据排序方向调整结果
    return sortOrder.value === 'asc' ? result : -result
  })
  return sortedImages
})

// 操作处理函数
const handleDelete = async (image: ImageInfo) => {
  await imageHooks.handleDelete(image)
}

// const handleDeleteDangling = async () => {
//   await imageHooks.handleDeleteDangling()
// }

const handleRefresh = async () => {
  if (appStore.systemHealth === 'unhealthy') {
    await appStore.checkHealth()
  }
  await imageHooks.handleRefresh()
  // 同时刷新容器数据以确保使用状态是最新的
  await containerStore.fetchContainers(true, true)
}

// 处理导入成功
const handleImportSuccess = async () => {
  // 刷新镜像列表
  await imageStore.fetchImages()
  // 刷新容器数据以确保使用状态是最新的
  await containerStore.fetchContainers(true, false)
  message.success('镜像导入成功')
}

// 页面初始化
onMounted(async () => {
  if (imageStore.images.length === 0) {
    await imageStore.fetchImages()
  }
  // 加载容器数据以便检查镜像使用情况
  if (containerStore.containers.length === 0) {
    await containerStore.fetchContainers()
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

.images-page {
  .images-content {
    margin-block: 16px;
    position: relative;
  }

  .empty-state {
    padding: 60px 0;
    text-align: center;
  }

  .images-grid {
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
  .images-page {
    .images-grid {
      gap: 8px;
    }
  }
}
</style>
