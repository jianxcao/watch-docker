<template>
  <n-tag :type="badgeType" size="small" round>
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
  PlayCircleOutline,
  StopCircleOutline,
} from '@vicons/ionicons5'
import MinusCircleOutline from '@/assets/svg/minusCircleOutline.svg?component'
import type { ContainerStatus } from '@/common/types'

interface Props {
  container: ContainerStatus
  showRunningStatus?: boolean // 是否显示运行状态而非更新状态
}

const props = withDefaults(defineProps<Props>(), {
  showRunningStatus: false,
})

// 状态类型
const badgeType = computed(() => {
  if (props.showRunningStatus) {
    return props.container.running ? 'success' : 'default'
  }

  if (!props.container.running) {
    return 'warning'
  }

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

// 状态文本
const statusText = computed(() => {
  if (props.showRunningStatus) {
    return props.container.running ? '运行中' : '已停止'
  }

  if (!props.container.running) {
    return '已停止'
  }

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

// 状态图标
const statusIcon = computed(() => {
  if (props.showRunningStatus) {
    return props.container.running ? PlayCircleOutline : StopCircleOutline
  }

  if (!props.container.running) {
    return MinusCircleOutline
  }

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
