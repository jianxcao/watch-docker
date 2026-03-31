<template>
  <n-modal
    v-model:show="modalVisible"
    preset="dialog"
    :title="modalTitle"
    :closable="canClose"
    :close-on-esc="canClose"
    :mask-closable="false"
    style="width: 640px; max-width: 90vw"
    :data-theme="settingStore.setting.theme"
  >
    <div class="batch-update-content">
      <!-- 总进度 -->
      <div class="overall-progress">
        <div class="progress-label">
          <span>{{ progressLabel }}</span>
          <span class="progress-percent" v-if="totalContainers > 0">
            {{ completedCount }}/{{ totalContainers }}
          </span>
        </div>
        <n-progress
          :percentage="overallPercent"
          :status="overallStatus"
          :height="8"
          :border-radius="4"
          :show-indicator="false"
        />
      </div>

      <!-- 容器列表 -->
      <div class="container-list" v-if="containerList.length > 0">
        <div
          v-for="item in containerList"
          :key="item.id"
          class="container-item"
          :class="'item-' + item.status"
        >
          <div class="item-header">
            <div class="item-icon">
              <n-icon v-if="item.status === 'success'" color="#00bc7d" :size="16">
                <CheckmarkCircleOutline />
              </n-icon>
              <n-icon v-else-if="item.status === 'error'" color="#ef4444" :size="16">
                <CloseCircleOutline />
              </n-icon>
              <div v-else-if="item.status === 'updating'" class="item-spinner"></div>
              <div v-else class="item-dot"></div>
            </div>
            <div class="item-info">
              <span class="item-name">{{ item.name }}</span>
              <span class="item-image">{{ item.image }}</span>
            </div>
            <n-tag :bordered="false" size="small" :type="getTagType(item.status)">
              {{ getStatusLabel(item.status) }}
            </n-tag>
          </div>

          <!-- 更新中的步骤详情 -->
          <div v-if="item.status === 'updating' && item.stepMessage" class="item-step">
            {{ item.stepMessage }}
          </div>

          <!-- 拉取进度条 -->
          <div
            v-if="item.status === 'updating' && item.step === 'pulling' && item.pullTotal > 0"
            class="item-pull-progress"
          >
            <n-progress
              :percentage="Math.min(100, Math.round((item.pullCurrent / item.pullTotal) * 100))"
              :height="4"
              :border-radius="2"
              :show-indicator="false"
              type="line"
            />
            <span class="pull-size"
              >{{ formatBytes(item.pullCurrent) }} / {{ formatBytes(item.pullTotal) }}</span
            >
          </div>

          <!-- 错误信息 -->
          <div v-if="item.status === 'error' && item.error" class="item-error">
            {{ item.error }}
          </div>
        </div>
      </div>
    </div>

    <template #action>
      <n-space justify="end">
        <n-button v-if="canClose" @click="handleClose" :type="isAllDone ? 'primary' : 'default'">
          关闭
        </n-button>
        <n-button v-if="!isStarted" type="primary" @click="startUpdate" :loading="isConnecting">
          确认更新
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, reactive, watch } from 'vue'
import { useWebSocket } from '@vueuse/core'
import { useSettingStore } from '@/store/setting'
import { useContainerStore } from '@/store/container'
import { CheckmarkCircleOutline, CloseCircleOutline } from '@vicons/ionicons5'

interface ContainerUpdateItem {
  id: string
  name: string
  image: string
  status: 'pending' | 'updating' | 'success' | 'error'
  step?: string
  stepMessage?: string
  pullCurrent: number
  pullTotal: number
  error?: string
  pullLayers: Map<string, { current: number; total: number }>
}

const modalVisible = defineModel<boolean>('show')
const settingStore = useSettingStore()
const containerStore = useContainerStore()

const containerList = reactive<ContainerUpdateItem[]>([])
const totalContainers = ref(0)
const isStarted = ref(false)
const isConnecting = ref(false)
const isAllDone = ref(false)
const phase = ref<'idle' | 'scanning' | 'updating' | 'done'>('idle')

const wsUrl = computed(() => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const token = settingStore.getToken()
  return `${protocol}//${host}/api/v1/updates/batch/ws?token=${encodeURIComponent(token)}`
})

const { open, close: wsClose } = useWebSocket(wsUrl, {
  immediate: false,
  autoReconnect: false,
  onConnected() {
    isConnecting.value = false
  },
  onMessage(_ws, event) {
    if (typeof event.data === 'string') {
      try {
        handleMessage(JSON.parse(event.data))
      } catch (e) {
        console.error('Failed to parse batch update message:', e)
      }
    }
  },
  onError() {
    isConnecting.value = false
  },
  onDisconnected() {
    isConnecting.value = false
  },
})

const completedCount = computed(
  () => containerList.filter((c) => c.status === 'success' || c.status === 'error').length,
)

const overallPercent = computed(() => {
  if (totalContainers.value === 0) {
    return 0
  }
  return Math.round((completedCount.value / totalContainers.value) * 100)
})

const overallStatus = computed(() => {
  if (isAllDone.value) {
    return containerList.some((c) => c.status === 'error') ? 'warning' : 'success'
  }
  return 'default'
})

const canClose = computed(() => !isStarted.value || isAllDone.value)

const modalTitle = computed(() => {
  if (isAllDone.value) {
    return '批量更新完成'
  }
  if (phase.value === 'scanning') {
    return '批量更新 - 扫描中'
  }
  if (phase.value === 'updating') {
    return `批量更新 - ${completedCount.value}/${totalContainers.value}`
  }
  return '批量更新'
})

const progressLabel = computed(() => {
  if (phase.value === 'scanning') {
    return '正在扫描可更新容器...'
  }
  if (phase.value === 'updating') {
    return '更新进度'
  }
  if (isAllDone.value) {
    const successCount = containerList.filter((c) => c.status === 'success').length
    const failCount = containerList.filter((c) => c.status === 'error').length
    if (failCount > 0) {
      return `完成：${successCount} 成功，${failCount} 失败`
    }
    return `全部更新完成`
  }
  return `发现 ${containerStore.updateableContainers.length} 个可更新容器`
})

function handleMessage(msg: any) {
  switch (msg.type) {
    case 'scan_start':
      phase.value = 'scanning'
      break

    case 'scan_complete':
      phase.value = 'updating'
      totalContainers.value = msg.total
      containerList.length = 0
      if (msg.containers) {
        for (const c of msg.containers) {
          containerList.push({
            id: c.id,
            name: c.name,
            image: c.image,
            status: 'pending',
            pullCurrent: 0,
            pullTotal: 0,
            pullLayers: new Map(),
          })
        }
      }
      break

    case 'container_start': {
      const item = containerList.find((c) => c.id === msg.containerId)
      if (item) {
        item.status = 'updating'
        item.pullLayers = new Map()
        item.pullCurrent = 0
        item.pullTotal = 0
      }
      break
    }

    case 'step': {
      const item = containerList.find((c) => c.id === msg.containerId)
      if (item) {
        item.step = msg.step
        item.stepMessage = msg.message
      }
      break
    }

    case 'pull_progress': {
      const item = containerList.find((c) => c.id === msg.containerId)
      if (item && msg.layerId) {
        item.pullLayers.set(msg.layerId, {
          current: msg.current || 0,
          total: msg.totalBytes || 0,
        })
        // aggregate across layers
        let totalCurrent = 0
        let totalSize = 0
        item.pullLayers.forEach((v) => {
          totalCurrent += v.current
          totalSize += v.total
        })
        item.pullCurrent = totalCurrent
        item.pullTotal = totalSize
      }
      break
    }

    case 'container_complete': {
      const item = containerList.find((c) => c.id === msg.containerId)
      if (item) {
        item.status = msg.success ? 'success' : 'error'
        item.error = msg.error
        item.stepMessage = undefined
      }
      break
    }

    case 'complete':
      isAllDone.value = true
      phase.value = 'done'
      containerStore.fetchContainers()
      break

    case 'error':
      isAllDone.value = true
      phase.value = 'done'
      break
  }
}

function startUpdate() {
  isStarted.value = true
  isConnecting.value = true
  open()
}

function handleClose() {
  modalVisible.value = false
}

function formatBytes(bytes: number): string {
  if (bytes === 0) {
    return '0 B'
  }
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function getTagType(status: string) {
  switch (status) {
    case 'success':
      return 'success'
    case 'error':
      return 'error'
    case 'updating':
      return 'info'
    default:
      return 'default'
  }
}

function getStatusLabel(status: string) {
  switch (status) {
    case 'pending':
      return '等待中'
    case 'updating':
      return '更新中'
    case 'success':
      return '已完成'
    case 'error':
      return '失败'
    default:
      return status
  }
}

// reset state when modal opens
watch(modalVisible, (val) => {
  if (val) {
    containerList.length = 0
    totalContainers.value = 0
    isStarted.value = false
    isConnecting.value = false
    isAllDone.value = false
    phase.value = 'idle'
  } else {
    wsClose()
  }
})
</script>

<style scoped lang="less">
.batch-update-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.overall-progress {
  .progress-label {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
    font-size: 14px;
    color: var(--text-color-2);

    .progress-percent {
      font-weight: 600;
      color: var(--text-color-1);
    }
  }
}

.container-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 400px;
  overflow-y: auto;
  padding-right: 4px;
}

.container-item {
  padding: 12px;
  border-radius: 10px;
  border: 1px solid var(--divider-color);
  transition: all 0.2s ease;

  &.item-updating {
    border-color: rgba(59, 130, 246, 0.3);
    background: rgba(59, 130, 246, 0.04);
  }

  &.item-success {
    border-color: rgba(0, 188, 125, 0.2);
    background: rgba(0, 188, 125, 0.04);
  }

  &.item-error {
    border-color: rgba(239, 68, 68, 0.2);
    background: rgba(239, 68, 68, 0.04);
  }

  .item-header {
    display: flex;
    align-items: center;
    gap: 10px;

    .item-icon {
      flex-shrink: 0;
      width: 16px;
      height: 16px;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .item-spinner {
      width: 14px;
      height: 14px;
      border: 2px solid rgba(59, 130, 246, 0.2);
      border-top-color: #3b82f6;
      border-radius: 50%;
      animation: batch-spin 0.8s linear infinite;
    }

    .item-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--divider-color);
    }

    .item-info {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 2px;

      .item-name {
        font-size: 14px;
        font-weight: 500;
        color: var(--text-color-1);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .item-image {
        font-size: 12px;
        color: var(--text-color-3);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
  }

  .item-step {
    margin-top: 8px;
    padding-left: 26px;
    font-size: 12px;
    color: #3b82f6;
  }

  .item-pull-progress {
    margin-top: 6px;
    padding-left: 26px;
    display: flex;
    align-items: center;
    gap: 8px;

    :deep(.n-progress) {
      flex: 1;
    }

    .pull-size {
      font-size: 11px;
      color: var(--text-color-3);
      white-space: nowrap;
      min-width: 100px;
      text-align: right;
    }
  }

  .item-error {
    margin-top: 6px;
    padding-left: 26px;
    font-size: 12px;
    color: #ef4444;
    word-break: break-all;
  }
}

@keyframes batch-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
