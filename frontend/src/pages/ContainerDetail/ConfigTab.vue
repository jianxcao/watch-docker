<template>
  <div class="tab-content config-tab">
    <div class="detail-container">
      <!-- 环境变量 -->
      <n-card title="环境变量" class="info-card" size="small">
        <n-input v-model:value="envSearchKeyword" placeholder="搜索环境变量" clearable class="mb-4">
          <template #prefix>
            <n-icon>
              <SearchOutline />
            </n-icon>
          </template>
        </n-input>
        <div v-if="filteredEnvVars.length === 0" class="empty-container">
          <n-empty description="没有环境变量" />
        </div>
        <div v-else class="env-list">
          <div v-for="(env, index) in filteredEnvVars" :key="index" class="env-item">
            <div class="env-key">{{ env.key }}</div>
            <div class="env-value">
              <n-text code>{{ env.value }}</n-text>
            </div>
          </div>
        </div>
      </n-card>

      <!-- 标签 -->
      <n-card title="标签" class="info-card" size="small">
        <div
          v-if="Object.keys(containerDetail.Config.Labels || {}).length === 0"
          class="empty-container"
        >
          <n-empty description="没有标签" />
        </div>
        <div v-else class="env-list">
          <div v-for="(value, key) in containerDetail.Config.Labels" :key="key" class="env-item">
            <div class="env-key">{{ key }}</div>
            <div class="env-value">
              <n-text code>{{ value }}</n-text>
            </div>
          </div>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { SearchOutline } from '@vicons/ionicons5'

interface Props {
  containerDetail: any
}

const props = defineProps<Props>()

const envSearchKeyword = ref('')

// 环境变量列表
const envVars = computed(() => {
  if (!props.containerDetail?.Config?.Env) {
    return []
  }

  return props.containerDetail.Config.Env.map((env: string) => {
    const [key, ...valueParts] = env.split('=')
    return {
      key,
      value: valueParts.join('='),
    }
  })
})

// 过滤后的环境变量
const filteredEnvVars = computed(() => {
  if (!envSearchKeyword.value) {
    return envVars.value
  }

  const keyword = envSearchKeyword.value.toLowerCase()
  return envVars.value.filter(
    (env: any) =>
      env.key.toLowerCase().includes(keyword) || env.value.toLowerCase().includes(keyword),
  )
})
</script>

<style scoped lang="less">
@import './styles.less';
</style>
