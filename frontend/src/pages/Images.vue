<template>
  <div class="images-page">
    <!-- 页面头部 -->
    <n-input v-model:value="searchKeyword" placeholder="搜索镜像标签或ID" style="width: 200px;" clearable>
      <template #prefix>
        <n-icon>
          <SearchOutline />
        </n-icon>
      </template>
    </n-input>

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

        <div v-else class="images-grid" :class="{
          'grid-cols-1': isMobile,
          'grid-cols-2': isTablet,
          'grid-cols-3': isLaptop || isDesktop,
          'grid-cols-4': isDesktopLarge,
        }">
          <ImageCard v-for="image in filteredImages" :key="image.id" :image="image" @delete="handleDelete" />
        </div>
      </n-spin>
    </div>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">镜像管理</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden ">
            共 {{ imageStore.stats.total }} 个镜像，
            总大小 {{ imageStore.stats.formattedTotalSize }}，
          </n-text>
        </div>
        <n-button @click="handleRefresh" :loading="imageStore.loading" circle size="tiny">
          <template #icon>
            <RefreshOutline />
          </template>
        </n-button>
      </div>
    </Teleport>
  </div>


</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useImageStore } from '@/store/image'
import { useContainerStore } from '@/store/container'
import { useImage } from '@/hooks/useImage'
import { useResponsive } from '@/hooks/useResponsive'
import type { ImageInfo } from '@/common/types'
import ImageCard from '@/components/ImageCard.vue'
import {
  SearchOutline,
  RefreshOutline,
} from '@vicons/ionicons5'

const imageStore = useImageStore()
const containerStore = useContainerStore()
const imageHooks = useImage()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()

// 搜索关键词
const searchKeyword = ref('')

// 过滤后的镜像列表
const filteredImages = computed(() => {
  let images = imageStore.normalImages

  // 搜索过滤
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    images = images.filter(image => {
      const displayTag = imageStore.getImageDisplayTag(image).toLowerCase()
      const id = image.id.toLowerCase()
      const tags = image.repoTags?.join(' ').toLowerCase() || ''

      return displayTag.includes(keyword) ||
        id.includes(keyword) ||
        tags.includes(keyword)
    })
  }

  // 按创建时间排序（最新的在前）
  return images.sort((a, b) => b.created - a.created)
})


// 操作处理函数
const handleDelete = async (image: ImageInfo) => {
  await imageHooks.handleDelete(image)
}

// const handleDeleteDangling = async () => {
//   await imageHooks.handleDeleteDangling()
// }

const handleRefresh = async () => {
  await imageHooks.handleRefresh()
  // 同时刷新容器数据以确保使用状态是最新的
  await containerStore.fetchContainers()
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
