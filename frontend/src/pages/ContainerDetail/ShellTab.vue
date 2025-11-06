<template>
  <div class="tab-content shell-tab">
    <ContainerShellView
      :container-id="containerId"
      :container-name="containerName"
      :is-running="isRunning"
    />
  </div>
</template>

<script setup lang="ts">
import ContainerShellView from '@/components/ContainerShellView.vue'

interface Props {
  containerId: string
  containerName: string
  isRunning: boolean
}

defineProps<Props>()
</script>

<style scoped lang="less">
@import './styles.less';
</style>
