<template>
  <div class="tab-content logs-tab">
    <LogsStreamView class="logs-stream-view" :socket-url="logsSocketUrl" height="100%" />
  </div>
</template>

<script setup lang="ts">
import LogsStreamView from '@/components/LogsStreamView.vue'

interface Props {
  logsSocketUrl: string
}

defineProps<Props>()
</script>

<style scoped lang="less">
@import './styles.less';
</style>
