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
import { PlayCircleOutline, StopCircleOutline } from '@vicons/ionicons5'
import type { ContainerStatus } from '@/common/types'

interface Props {
  container: ContainerStatus
}

const props = defineProps<Props>()

// 运行状态类型
const badgeType = computed(() => {
  return props.container.running ? 'success' : 'default'
})

// 运行状态文本
const statusText = computed(() => {
  return props.container.running ? '运行中' : '已停止'
})

// 运行状态图标
const statusIcon = computed(() => {
  return props.container.running ? PlayCircleOutline : StopCircleOutline
})
</script>
