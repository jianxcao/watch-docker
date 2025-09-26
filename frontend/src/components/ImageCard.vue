<template>
  <n-card hoverable class="image-card">
    <template #header>
      <n-space align="center" justify="space-between">
        <div class="image-title">
          <n-text strong>{{ imageStore.getImageDisplayTag(image) }}</n-text>
          <n-tag v-if="imageStore.isDanglingImage(image)" type="warning" size="small" style="margin-left: 8px;">
            悬空
          </n-tag>
        </div>
      </n-space>
    </template>

    <n-space vertical>
      <!-- 镜像信息 -->
      <n-descriptions :column="1" size="small">
        <n-descriptions-item label="ID">
          <n-tooltip>
            <template #trigger>
              <n-text code class="image-id cursor-pointer">{{ imageStore.getDigestDisplayText(image)
              }}</n-text>
            </template>
            {{ getFullDigestText(image) }}
          </n-tooltip>
        </n-descriptions-item>

        <n-descriptions-item label="标签">
          <n-text class="image-tags">{{ imageHooks.getTagsDisplayText(image) }}</n-text>
        </n-descriptions-item>

        <n-descriptions-item label="大小">
          <n-text>{{ imageStore.formatSize(image.size) }}</n-text>
        </n-descriptions-item>

        <n-descriptions-item label="创建时间">
          <n-text :depth="3">{{ imageHooks.formatCreateTime(image.created) }}</n-text>
        </n-descriptions-item>

        <n-descriptions-item label="持续时间">
          <n-text :depth="3">{{ imageHooks.getImageAge(image.created) }}</n-text>
        </n-descriptions-item>

        <n-descriptions-item label="使用状态">
          <n-space align="center" size="small">
            <n-tag :type="imageHooks.isImageInUse(image) ? 'info' : 'default'" size="small">
              {{ imageHooks.getImageUsageText(image) }}
            </n-tag>

            <!-- 如果有使用该镜像的容器，显示容器列表 -->
            <n-tooltip v-if="imageHooks.isImageInUse(image)">
              <template #trigger>
                <n-icon style="cursor: pointer; color: #999;">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                    <path
                      d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z" />
                  </svg>
                </n-icon>
              </template>
              <div>
                <div style="margin-bottom: 4px;"><strong>使用此镜像的容器：</strong></div>
                <div v-for="containerName in imageHooks.getImageUsageContainers(image)" :key="containerName">
                  • {{ containerName }}
                </div>
              </div>
            </n-tooltip>
          </n-space>
        </n-descriptions-item>
      </n-descriptions>

      <!-- 完整标签列表 -->
      <div v-if="image.repoTags && image.repoTags.length > 1" class="full-tags">
        <n-text strong style="font-size: 12px;">所有标签:</n-text>
        <n-space style="margin-top: 4px;" vertical size="small">
          <n-tag v-for="tag in validTags(image.repoTags)" :key="tag" size="tiny" type="info">
            {{ tag }}
          </n-tag>
        </n-space>
      </div>
    </n-space>

    <template #action>
      <n-space justify="end">
        <n-button @click="() => handleDelete()" type="error" size="small" ghost
          :loading="imageStore.isImageDeleting(image.id)">
          <template #icon>
            <n-icon>
              <TrashOutline />
            </n-icon>
          </template>
          删除
        </n-button>
      </n-space>
    </template>
  </n-card>
</template>

<script setup lang="ts">
import { useImageStore } from '@/store/image'
import { useImage } from '@/hooks/useImage'
import type { ImageInfo } from '@/common/types'
import { TrashOutline } from '@vicons/ionicons5'

// Props
interface Props {
  image: ImageInfo
}

const props = defineProps<Props>()

// Emits
const emit = defineEmits<{
  delete: [image: ImageInfo, force?: boolean]
}>()

// Store & Hooks
const imageStore = useImageStore()
const imageHooks = useImage()

// 获取有效标签（过滤掉 <none>:<none>）
const validTags = (tags: string[]): string[] => {
  return tags.filter(tag => tag !== '<none>:<none>')
}

// 获取完整的摘要文本（用于悬停提示）
const getFullDigestText = (image: ImageInfo): string => {
  // 优先显示 repoDigests 中的第一个摘要
  if (image.repoDigests && image.repoDigests.length > 0) {
    return image.repoDigests[0]
  }

  // 如果没有摘要，显示完整的镜像 ID
  return image.id
}

// 删除处理函数
const handleDelete = () => {
  emit('delete', props.image, false)
}
</script>

<style scoped lang="less">
.image-card {
  .image-title {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
  }

  .image-id {
    font-size: 11px;
    font-family: 'Monaco', 'Consolas', monospace;
  }

  .image-tags {
    word-break: break-all;
    font-family: 'Monaco', 'Consolas', monospace;
    font-size: 12px;
  }

  .full-tags {
    padding: 8px 0;
    border-top: 1px solid #f0f0f0;
  }
}
</style>
