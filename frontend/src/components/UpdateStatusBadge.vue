<template>
  <n-tag v-if="shouldShowBadge" :type="badgeType" size="small" round>
    <template #icon>
      <n-icon :component="statusIcon" />
    </template>
    {{ statusText }}
  </n-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CheckmarkCircleOutline, AlertCircleOutline, CloseCircleOutline } from '@vicons/ionicons5'
import MinusCircleOutline from '@/assets/svg/minusCircleOutline.svg?component'
import type { ContainerStatus } from '@/common/types'

interface Props {
  container: ContainerStatus
}

const props = defineProps<Props>()

// 是否显示更新状态标签
const shouldShowBadge = computed(() => {
  // 如果容器没有运行，不显示更新状态
  if (!props.container.running) {
    return false
  }

  // 如果没有状态信息或状态为空，不显示
  if (!props.container.status) {
    return false
  }

  return true
})

// 更新状态类型
const badgeType = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return 'success'
    case 'UpdateAvailable':
      return 'info'
    case 'Error':
      return 'error'
    case 'Skipped':
      return 'default'
    default:
      return 'default'
  }
})

// 更新状态文本
const statusText = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return '最新'
    case 'UpdateAvailable':
      return '可更新'
    case 'Error':
      return '错误'
    case 'Skipped':
      return '跳过'
    default:
      return '未知'
  }
})

// 更新状态图标
const statusIcon = computed(() => {
  switch (props.container.status) {
    case 'UpToDate':
      return CheckmarkCircleOutline
    case 'UpdateAvailable':
      return AlertCircleOutline
    case 'Error':
      return CloseCircleOutline
    case 'Skipped':
      return MinusCircleOutline
    default:
      return MinusCircleOutline
  }
})
</script>
