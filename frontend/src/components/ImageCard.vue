<template>
  <div class="image-card">
    <!-- 头部：镜像名称和状态 -->
    <div class="card-header">
      <div class="image-name">
        {{ imageHooks.getImageNameOnly(image) }}
      </div>
      <div class="status-tags">
        <n-tag
          v-if="imageHooks.isImageInUse(image)"
          :bordered="false"
          round
          type="success"
          size="small"
        >
          使用中
        </n-tag>
      </div>
    </div>

    <!-- 版本信息 -->
    <div class="tag-section">
      <n-icon class="tag-icon">
        <TagIcon />
      </n-icon>
      <span class="tag-text">{{ imageHooks.getVersionDisplayText(image) }}</span>
    </div>

    <!-- 信息列表 -->
    <div class="info-list">
      <div class="info-item">
        <span class="info-label">ID</span>
        <span class="info-value">{{ imageStore.getDisplayId(image) }}</span>
      </div>

      <div class="info-item">
        <n-icon class="info-icon">
          <SizeIcon />
        </n-icon>
        <span class="info-label">大小</span>
        <span class="info-value">{{ formatBytes(image.size) }}</span>
      </div>

      <div class="info-item">
        <n-icon class="info-icon">
          <CalendarIcon />
        </n-icon>
        <span class="info-label">创建时间</span>
        <span class="info-value">{{ imageHooks.formatCreateTime(image.created) }}</span>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="card-footer">
      <n-button
        @click="() => handleDownload()"
        ghost
        size="small"
        class="download-btn"
        :loading="imageStore.isImageDownloading(image.id)"
      >
        <template #icon>
          <n-icon>
            <DownloadOutline />
          </n-icon>
        </template>
        下载
      </n-button>
      <n-button
        @click="() => handleDelete()"
        type="error"
        ghost
        size="small"
        class="delete-btn"
        :loading="imageStore.isImageDeleting(image.id)"
      >
        <template #icon>
          <n-icon>
            <TrashOutline />
          </n-icon>
        </template>
        删除
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useImageStore } from '@/store/image'
import { useImage } from '@/hooks/useImage'
import type { ImageInfo } from '@/common/types'
import { TrashOutline, DownloadOutline } from '@vicons/ionicons5'
import { formatBytes } from '@/common/utils'

// 导入 SVG 图标
import TagIcon from '@/assets/svg/tag.svg?component'
import SizeIcon from '@/assets/svg/size.svg?component'
import CalendarIcon from '@/assets/svg/calendar.svg?component'
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

// 下载处理函数
const handleDownload = async () => {
  await imageHooks.handleDownload(props.image)
}

// 删除处理函数
const handleDelete = () => {
  emit('delete', props.image)
}
</script>

<style scoped lang="less">
.image-card {
  background: var(--card-color);
  border-radius: 12px;
  border: 1px solid var(--border-color);
  padding: 16px;
  transition: all 0.3s ease;
  position: relative;
  overflow: hidden;
  color: var(--text-color-1);
  box-shadow: var(--box-shadow-1);

  &:hover {
    transform: translateY(-2px);
    background: linear-gradient(
      135deg,
      var(--card-color) 0%,
      color-mix(in srgb, var(--card-color) 10%, transparent) 100%
    );
    border-color: color-mix(in srgb, var(--border-color) 90%, var(--text-color-3) 100%);
  }

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 1px;
    background: linear-gradient(
      90deg,
      transparent 0%,
      var(--text-color-disabled) 50%,
      transparent 100%
    );
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 1px;

    .image-name {
      font-size: 16px;
      font-weight: 600;
      color: var(--text-base);
      line-height: 1.3;
      flex: 1;
      margin-right: 8px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .tag-section {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 16px;

    .tag-text {
      color: var(--text-color-3);
    }

    .tag-icon {
      color: var(--text-color-3);
      flex-shrink: 0;
    }
  }

  .info-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 8px;

    .info-item {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 2px 0;

      .info-icon {
        color: var(--text-color-3);
        flex-shrink: 0;
      }

      .info-label {
        color: var(--text-color-3);
        min-width: 60px;
        flex-shrink: 0;
      }

      .info-value {
        color: var(--text-color-2);
        font-weight: 500;
        flex: 1;
        text-align: right;
      }
    }
  }

  .card-footer {
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--divider-color);
    display: flex;
    justify-content: flex-end;
    gap: 8px;

    .download-btn {
      flex-shrink: 0;
    }

    .delete-btn {
      flex-shrink: 0;
    }
  }
}
</style>
