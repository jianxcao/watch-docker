<template>
  <div class="files-tab">
    <FileBrowserPanel :container-id="props.containerId" />
  </div>
</template>

<script setup lang="ts">
import FileBrowserPanel from '@/components/FileBrowser/FileBrowserPanel.vue'

interface Props {
  containerId: string
}

const props = defineProps<Props>()
</script>

<style scoped lang="less">
.files-tab {
  height: 100%;
  border-radius: 4px;
  overflow: hidden;
}
</style>
