<template>
  <div class="volume-tab">
    <n-space vertical size="large">
      <div>
        <n-h3 prefix="bar" class="mt-0">数据卷挂载</n-h3>
        <n-space vertical size="small">
          <div v-for="(volume, index) in volumeList" :key="index" class="volume-item">
            <n-grid :cols="24" :x-gap="8">
              <n-gi :span="8">
                <n-input
                  v-model:value="volume.source"
                  placeholder="源路径或卷名"
                  size="small"
                  @blur="updateVolumes"
                />
              </n-gi>
              <n-gi :span="1" class="flex items-center justify-center">
                <span>:</span>
              </n-gi>
              <n-gi :span="8">
                <n-input
                  v-model:value="volume.target"
                  placeholder="容器路径"
                  size="small"
                  @blur="updateVolumes"
                />
              </n-gi>
              <n-gi :span="3">
                <n-checkbox
                  v-model:checked="volume.readonly"
                  size="small"
                  @update:checked="updateVolumes"
                >
                  只读
                </n-checkbox>
              </n-gi>
              <n-gi :span="2">
                <n-button size="small" tertiary type="error" @click="removeVolume(index)" block>
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                </n-button>
              </n-gi>
            </n-grid>
          </div>
          <n-button dashed block @click="addVolume" size="small">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加数据卷挂载
          </n-button>
        </n-space>
      </div>

      <n-divider />

      <div>
        <n-h3 prefix="bar">文本格式</n-h3>
        <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">
          每行一个挂载,格式: /host/path:/container/path 或 /host/path:/container/path:ro
        </n-text>
        <n-input
          v-model:value="volumeText"
          type="textarea"
          placeholder="/host/path:/container/path&#10;volume_name:/container/path:ro"
          :rows="6"
          @blur="handleVolumeTextChange"
        />
      </div>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import { ref, watch } from 'vue'

const binds = defineModel<string[]>({ default: [] })

interface VolumeItem {
  source: string
  target: string
  readonly: boolean
}

const volumeList = ref<VolumeItem[]>([])
const volumeText = ref('')

// 初始化数据卷列表
const initVolumeList = () => {
  if (binds.value && binds.value.length > 0) {
    volumeList.value = binds.value.map((bind) => {
      const parts = bind.split(':')
      return {
        source: parts[0] || '',
        target: parts[1] || '',
        readonly: parts[2] === 'ro',
      }
    })
    volumeText.value = binds.value.join('\n')
  } else {
    volumeList.value = []
    volumeText.value = ''
  }
}

initVolumeList()

// 处理文本格式变化（仅在失去焦点时）
const handleVolumeTextChange = () => {
  const lines = volumeText.value.split('\n').filter((line) => line.trim())
  const newBinds: string[] = []
  const newVolumeList: VolumeItem[] = []

  lines.forEach((line) => {
    const trimmedLine = line.trim()
    if (trimmedLine && trimmedLine.includes(':')) {
      newBinds.push(trimmedLine)
      const parts = trimmedLine.split(':')
      newVolumeList.push({
        source: parts[0] || '',
        target: parts[1] || '',
        readonly: parts[2] === 'ro',
      })
    }
  })

  volumeList.value = newVolumeList
  binds.value = newBinds.length > 0 ? newBinds : []

  // 更新文本为格式化后的结果
  volumeText.value = newBinds.join('\n')
}

const addVolume = () => {
  volumeList.value.push({ source: '', target: '', readonly: false })
}

const removeVolume = (index: number) => {
  volumeList.value.splice(index, 1)
  updateVolumes()
}

const updateVolumes = () => {
  const newBinds = volumeList.value
    .filter((item) => item.source.trim() && item.target.trim())
    .map((item) => {
      let bind = `${item.source}:${item.target}`
      if (item.readonly) {
        bind += ':ro'
      }
      return bind
    })

  binds.value = newBinds.length > 0 ? newBinds : []
  volumeText.value = newBinds.join('\n')
}

// 监听外部 binds 变化
watch(
  () => binds.value,
  (newVal) => {
    const currentBinds = volumeList.value
      .filter((item) => item.source.trim() && item.target.trim())
      .map((item) => {
        let bind = `${item.source}:${item.target}`
        if (item.readonly) {
          bind += ':ro'
        }
        return bind
      })

    if (JSON.stringify(newVal) !== JSON.stringify(currentBinds)) {
      initVolumeList()
    }
  },
)
</script>

<style scoped>
.volume-tab {
  padding: 0;
}

.volume-item {
  margin-bottom: 8px;
}
</style>
