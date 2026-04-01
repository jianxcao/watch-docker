<template>
  <n-tooltip v-if="shouldShowBadge && tooltipText" trigger="hover">
    <template #trigger>
      <n-tag :type="badgeType" size="small" round>
        <template #icon>
          <n-icon :component="statusIcon" />
        </template>
        {{ statusText }}
      </n-tag>
    </template>
    {{ tooltipText }}
  </n-tooltip>
  <n-tag v-else-if="shouldShowBadge" :type="badgeType" size="small" round>
    <template #icon>
      <n-icon :component="statusIcon" />
    </template>
    {{ statusText }}
  </n-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  CheckmarkCircleOutline,
  AlertCircleOutline,
  CloseCircleOutline,
  BanOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import MinusCircleOutline from '@/assets/svg/minusCircleOutline.svg?component'
import type { ContainerStatus } from '@/common/types'

interface Props {
  container: ContainerStatus
}

const props = defineProps<Props>()

const shouldShowBadge = computed(() => {
  if (!props.container.running) {
    return false
  }
  if (!props.container.status) {
    return false
  }
  return true
})

const badgeType = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return 'success'
    case 'UpdateAvailable':
      return 'info'
    case 'Error':
      if (props.container.errorType === 'rate_limited') {
        return 'warning'
      }
      if (props.container.errorType === 'not_found') {
        return 'default'
      }
      return 'error'
    case 'Skipped':
      return 'default'
    default:
      return 'default'
  }
})

const statusText = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return '最新'
    case 'UpdateAvailable':
      return '可更新'
    case 'Error':
      if (props.container.errorType === 'rate_limited') {
        return '限流'
      }
      if (props.container.errorType === 'not_found') {
        return '未找到'
      }
      return '错误'
    case 'Skipped':
      return '跳过'
    default:
      return '未知'
  }
})

const tooltipText = computed(() => {
  if (props.container.status !== 'Error') {
    return ''
  }
  if (props.container.errorType === 'rate_limited') {
    return 'Docker Hub 请求频率超限，将在冷却后自动重试'
  }
  if (props.container.errorType === 'not_found') {
    return '远程镜像或标签不存在，请检查镜像名称'
  }
  if (props.container.skipReason) {
    return props.container.skipReason
  }
  return ''
})

const statusIcon = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return CheckmarkCircleOutline
    case 'UpdateAvailable':
      return AlertCircleOutline
    case 'Error':
      if (props.container.errorType === 'rate_limited') {
        return TimeOutline
      }
      if (props.container.errorType === 'not_found') {
        return BanOutline
      }
      return CloseCircleOutline
    case 'Skipped':
      return MinusCircleOutline
    default:
      return MinusCircleOutline
  }
})
</script>
