<template>
  <n-card :title="container.name" hoverable class="container-card" :class="{ 'card-updating': isUpdating }">
    <template #header-extra>
      <n-space>
        <StatusBadge :container="container" show-running-status />
        <StatusBadge :container="container" />
      </n-space>
    </template>

    <n-space vertical>
      <!-- 容器信息 -->
      <div class="container-info">
        <n-descriptions :column="1" size="small">
          <n-descriptions-item label="镜像">
            <n-text class="image-text" :depth="2">{{ container.image }}</n-text>
          </n-descriptions-item>

          <n-descriptions-item label="当前摘要" v-if="container.currentDigest">
            <n-tooltip>
              <template #trigger>
                <n-text code class="digest-text cursor-pointer">{{ formatDigest(container.currentDigest) }}</n-text>
              </template>
              {{ container.currentDigest }}
            </n-tooltip>
          </n-descriptions-item>

          <n-descriptions-item label="远程摘要" v-if="container.remoteDigest">
            <n-tooltip>
              <template #trigger>
                <n-text code class="digest-text cursor-pointer">{{ formatDigest(container.remoteDigest) }}</n-text>
              </template>
              {{ container.remoteDigest }}
            </n-tooltip>
          </n-descriptions-item>

          <n-descriptions-item label="最后检查">
            <n-text :depth="3">{{ formatTime(container.lastCheckedAt) }}</n-text>
          </n-descriptions-item>
        </n-descriptions>
      </div>

      <!-- 跳过原因 -->
      <n-alert v-if="container.skipped && container.skipReason" type="warning" :show-icon="false" size="small">
        跳过原因: {{ container.skipReason }}
      </n-alert>

      <!-- 标签 -->
      <div v-if="hasLabels" class="container-labels">
        <n-text strong style="font-size: 12px;">标签:</n-text>
        <div class="flex flex-col gap-2">
          <n-tooltip v-for="(value, key) in visibleLabels" :key="key" :disabled="!isLabelTruncated(key, value)">
            <template #trigger>
              <n-tag type="info" class="container-label-tag flex-1 w-full">
                {{ key }}={{ value }}
              </n-tag>
            </template>
            {{ key }}={{ value }}
          </n-tooltip>
          <n-button v-if="hiddenLabelsCount > 0" text size="tiny" @click="showAllLabels = !showAllLabels"
            class="self-end">
            {{ showAllLabels ? '收起' : `+${hiddenLabelsCount}` }}
          </n-button>
        </div>
      </div>
    </n-space>

    <template #action>
      <n-space justify="space-between">
        <!-- 基础操作 -->
        <n-button-group>
          <n-button v-if="!container.running" @click="$emit('start')" type="primary" size="small" :loading="loading">
            <template #icon>
              <n-icon>
                <PlayCircleOutline />
              </n-icon>
            </template>
            启动
          </n-button>

          <n-button v-else @click="$emit('stop')" type="warning" size="small" :loading="loading">
            <template #icon>
              <n-icon>
                <StopCircleOutline />
              </n-icon>
            </template>
            停止
          </n-button>

          <n-button v-if="container.status === 'UpdateAvailable' && !container.skipped" @click="$emit('update')"
            type="info" size="small" :loading="isUpdating">
            <template #icon>
              <n-icon>
                <CloudDownloadOutline />
              </n-icon>
            </template>
            更新
          </n-button>
        </n-button-group>

        <!-- 危险操作 -->
        <n-button @click="$emit('delete')" type="error" size="small" ghost :loading="loading">
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
import { computed, ref } from 'vue'
import { useContainerStore } from '@/store/container'
import StatusBadge from './StatusBadge.vue'
import dayjs from 'dayjs'
import type { ContainerStatus } from '@/common/types'
import {
  PlayCircleOutline,
  StopCircleOutline,
  CloudDownloadOutline,
  TrashOutline,
} from '@vicons/ionicons5'

interface Props {
  container: ContainerStatus
  loading?: boolean
}

interface Emits {
  (e: 'start'): void
  (e: 'stop'): void
  (e: 'update'): void
  (e: 'delete'): void
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})

defineEmits<Emits>()

const containerStore = useContainerStore()
const showAllLabels = ref(false)

// 是否正在更新
const isUpdating = computed(() =>
  containerStore.isContainerUpdating(props.container.id)
)

// 格式化摘要显示
const formatDigest = (digest: string): string => {
  if (digest.startsWith('sha256:')) {
    return digest.slice(7, 19) + '...'
  }
  return digest.slice(0, 12) + '...'
}

// 格式化时间显示
const formatTime = (timeStr: string): string => {
  return dayjs(timeStr).format('MM-DD HH:mm')
}

// 标签相关计算
const hasLabels = computed(() => {
  return props.container.labels && Object.keys(props.container.labels).length > 0
})

const maxVisibleLabels = 3

const visibleLabels = computed(() => {
  if (!props.container.labels) return {}

  const entries = Object.entries(props.container.labels)
  if (showAllLabels.value) {
    return props.container.labels
  }

  return Object.fromEntries(entries.slice(0, maxVisibleLabels))
})

const hiddenLabelsCount = computed(() => {
  if (!props.container.labels) return 0
  const totalCount = Object.keys(props.container.labels).length
  return Math.max(0, totalCount - maxVisibleLabels)
})

// 标签文本截断相关
const maxLabelLength = 30



const isLabelTruncated = (key: string, value: string): boolean => {
  const fullText = `${key}=${value}`
  return fullText.length > maxLabelLength
}
</script>

<style scoped lang="less">
@import '@/styles/mix.less';

.container-card {
  transition: all 0.3s ease;
  overflow-x: auto;
  position: relative;
  .scrollbar();

  &.card-updating {
    border-color: #1890ff;
    box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.2);
  }
}

.container-info {
  .image-text {
    word-break: break-all;
    font-family: 'Monaco', 'Consolas', monospace;
    font-size: 12px;
  }

  .digest-text {
    font-size: 11px;
  }
}

.container-labels {
  padding: 8px 0;
  border-top: 1px solid var(--border-color);
}

.container-label-tag {
  display: inline-block;
  padding: 4px 8px;
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
  cursor: pointer;
}

// 响应式调整
@media (max-width: 768px) {
  .container-card {
    margin-bottom: 8px;
  }

  .container-label-tag {
    max-width: 120px;
  }
}
</style>
