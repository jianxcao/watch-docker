<template>
  <n-tag :type="statusConfig.type" :color="statusConfig.color" size="small" round>
    <template #icon>
      <n-icon :component="statusConfig.icon" />
    </template>
    {{ statusConfig.text }}
  </n-tag>
</template>

<script setup lang="ts">
import { computed, h } from 'vue'
import { NTag, NIcon } from 'naive-ui'
import type { ComposeProject } from '@/common/types'

interface Props {
  project: ComposeProject
}

const props = defineProps<Props>()

// 状态图标组件
const PlayCircleIcon = () => h('div', { style: { fontSize: '12px' } }, '▶️')
const StopCircleIcon = () => h('div', { style: { fontSize: '12px' } }, '⏸️')
const WarningIcon = () => h('div', { style: { fontSize: '12px' } }, '⚠️')
const ErrorIcon = () => h('div', { style: { fontSize: '12px' } }, '❌')
const QuestionIcon = () => h('div', { style: { fontSize: '12px' } }, '❓')

// 状态配置
const statusConfig = computed(() => {
  switch (props.project.status) {
    case 'running':
      return {
        type: 'success' as const,
        text: '运行中',
        icon: PlayCircleIcon,
        color: { color: '#52c41a', borderColor: '#52c41a' }
      }
    case 'stopped':
      return {
        type: 'default' as const,
        text: '已停止',
        icon: StopCircleIcon,
        color: { color: '#8c8c8c', borderColor: '#8c8c8c' }
      }
    case 'partial':
      return {
        type: 'warning' as const,
        text: '部分运行',
        icon: WarningIcon,
        color: { color: '#faad14', borderColor: '#faad14' }
      }
    case 'error':
      return {
        type: 'error' as const,
        text: '错误',
        icon: ErrorIcon,
        color: { color: '#ff4d4f', borderColor: '#ff4d4f' }
      }
    default:
      return {
        type: 'default' as const,
        text: '未知',
        icon: QuestionIcon,
        color: { color: '#d9d9d9', borderColor: '#d9d9d9' }
      }
  }
})
</script>
