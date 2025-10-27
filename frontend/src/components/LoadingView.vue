<template>
  <div v-if="loading" class="loading-view" :class="customClass">
    <div class="loading-content">
      <v-progress-circular indeterminate size="64" :width="4" color="primary"></v-progress-circular>
      <h3 class="loading-text">{{ text }}</h3>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  loading: boolean
  text?: string
  customClass?: string
}

withDefaults(defineProps<Props>(), {
  text: '正在加载数据...',
})
</script>

<style scoped>
.loading-view {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 400px;
  width: 100%;
}

.loading-content {
  text-align: center;
}

.loading-text {
  margin-top: 1rem;
  color: rgb(var(--v-theme-on-surface));
}
</style>
