<template>
  <div class="tab-content storage-tab">
    <div class="detail-container">
      <n-card title="挂载点" class="info-card" size="small">
        <div v-if="containerDetail.Mounts.length === 0" class="empty-container">
          <n-empty description="没有挂载点" />
        </div>
        <div v-else class="mount-list">
          <div v-for="(mount, index) in containerDetail.Mounts" :key="index" class="mount-item">
            <div class="mount-header">
              <n-tag :type="getMountTypeColor(mount.Type)" size="small">{{ mount.Type }}</n-tag>
              <n-tag v-if="mount.RW" type="success" size="small">读写</n-tag>
              <n-tag v-else type="warning" size="small">只读</n-tag>
            </div>
            <div class="mount-details">
              <div class="mount-detail-item" v-if="mount.Name">
                <span class="label">名称:</span>
                <n-button
                  text
                  type="primary"
                  @click="emit('volumeClick', mount.Name)"
                  v-if="mount.Type === 'volume'"
                >
                  {{ mount.Name }}
                </n-button>
                <n-text v-else>{{ mount.Name }}</n-text>
              </div>
              <div class="mount-detail-item">
                <span class="label">源路径:</span>
                <n-text code>{{ mount.Source }}</n-text>
              </div>
              <div class="mount-detail-item">
                <span class="label">目标路径:</span>
                <n-text code>{{ mount.Destination }}</n-text>
              </div>
              <div class="mount-detail-item" v-if="mount.Mode">
                <span class="label">模式:</span>
                <n-text>{{ mount.Mode }}</n-text>
              </div>
            </div>
          </div>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  containerDetail: any
}

interface Emits {
  (e: 'volumeClick', volumeName: string): void
}

defineProps<Props>()
const emit = defineEmits<Emits>()

// 获取挂载类型颜色
const getMountTypeColor = (type: string) => {
  const colorMap: Record<string, any> = {
    volume: 'info',
    bind: 'success',
    tmpfs: 'warning',
  }
  return colorMap[type] || 'default'
}
</script>

<style scoped lang="less">
@import './styles.less';
</style>
