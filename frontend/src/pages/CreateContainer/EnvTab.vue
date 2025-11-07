<template>
  <div class="env-tab">
    <n-space vertical size="large">
      <div>
        <n-h3 prefix="bar" class="mt-0">环境变量</n-h3>
        <n-space vertical size="small">
          <div v-for="(envItem, index) in envList" :key="index" class="env-item">
            <n-grid :cols="12" :x-gap="8">
              <n-gi :span="5">
                <n-input
                  v-model:value="envItem.key"
                  placeholder="KEY"
                  size="small"
                  @blur="updateEnv(index)"
                />
              </n-gi>
              <n-gi :span="1" class="flex items-center justify-center">
                <span>=</span>
              </n-gi>
              <n-gi :span="5">
                <n-input
                  v-model:value="envItem.value"
                  placeholder="value"
                  size="small"
                  @blur="updateEnv(index)"
                />
              </n-gi>
              <n-gi :span="1">
                <n-button size="small" tertiary type="error" @click="removeEnv(index)" block>
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                </n-button>
              </n-gi>
            </n-grid>
          </div>
          <n-button dashed block @click="addEnv" size="small">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加环境变量
          </n-button>
        </n-space>
      </div>

      <n-divider />

      <div>
        <n-h3 prefix="bar">文本格式</n-h3>
        <n-text depth="3" class="text-sm mb-4 block"> 每行一个环境变量,格式: KEY=value </n-text>
        <n-input
          v-model:value="envText"
          type="textarea"
          placeholder="KEY=value&#10;ANOTHER_KEY=另一个值"
          :rows="8"
          @blur="handleEnvTextChange"
        />
      </div>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import { ref } from 'vue'

const envs = defineModel<string[]>({ default: [] })

interface EnvItem {
  key: string
  value: string
}

const envList = ref<EnvItem[]>([])
const envText = ref('')

// 初始化环境变量列表
const initEnvList = () => {
  if (envs.value && envs.value.length > 0) {
    envList.value = envs.value.map((item) => {
      const [key, ...valueParts] = item.split('=')
      return {
        key: key || '',
        value: valueParts.join('=') || '',
      }
    })
    envText.value = envs.value.join('\n')
  }
}

onBeforeMount(() => {
  initEnvList()
})

const addEnv = () => {
  envList.value.push({ key: '', value: '' })
}

const removeEnv = (index: number) => {
  envs.value.splice(index, 1)
  envList.value.splice(index, 1)
}

const updateEnv = (index: number) => {
  envs.value[index] = `${envList.value[index].key}=${envList.value[index].value}`
  envText.value = envs.value.join('\n')
}

const handleEnvTextChange = () => {
  nextTick(() => {
    console.debug(envText.value)
    const lines = envText.value.split('\n').filter((line) => line.trim())
    console.debug(lines)
    const newEnvList = lines.reduce((acc, line) => {
      let [key, value] = line.split('=')
      key = (key || '').trim()
      value = (value || '').trim()
      if (key) {
        acc.push(`${key}=${value}`)
      }
      return acc
    }, [] as string[])
    envs.value = newEnvList
    envList.value = newEnvList.map((item) => {
      const [key, ...valueParts] = item.split('=')
      return {
        key: key || '',
        value: valueParts.join('=') || '',
      }
    })
  })
}
</script>

<style scoped>
.env-tab {
  padding: 0;
}

.env-item {
  margin-bottom: 8px;
}
</style>
