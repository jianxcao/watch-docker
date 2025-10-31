<template>
  <div class="settings-page">
    <!-- 设置内容 -->
    <div class="settings-content">
      <n-space vertical size="large">
        <!-- 服务器设置 -->
        <!-- <n-card title="服务器设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="监听地址" disabled>
              <n-input v-model:value="configForm.server.addr" placeholder=":8080" />
            </n-form-item>
          </n-form>
        </n-card> -->

        <!-- 通知设置 -->
        <n-card title="通知设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="通知地址">
              <n-input v-model:value="configForm.notify.url" :placeholder="notifyUrlPlaceholder" />
            </n-form-item>
            <n-form-item label="请求方法">
              <n-select
                v-model:value="configForm.notify.method"
                :options="notifyMethodOptions"
                placeholder="选择请求方法"
              />
            </n-form-item>
            <n-form-item label="启用通知">
              <n-switch v-model:value="configForm.notify.isEnable" />
            </n-form-item>
            <n-alert title="占位符说明" type="info" class="mt-2">
              支持在查询参数或路径中使用 title={title}、content={content}、url={url}、image={image}
              占位符
            </n-alert>
          </n-form>
        </n-card>

        <!-- Docker 设置 -->
        <n-card title="Docker 设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="Docker 主机">
              <n-input v-model:value="configForm.docker.host" placeholder="留空使用默认设置" />
            </n-form-item>
            <n-form-item label="包含已停止容器">
              <n-switch v-model:value="configForm.docker.includeStopped" />
            </n-form-item>
          </n-form>
        </n-card>

        <!-- 扫描设置 -->
        <n-card title="扫描设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="Cron 表达式">
              <n-input v-model:value="configForm.scan.cron" placeholder="0 */10 * * * *" />
            </n-form-item>
            <n-form-item label="并发数">
              <n-input-number v-model:value="configForm.scan.concurrency" :min="1" :max="20" />
            </n-form-item>
            <n-form-item label="缓存TTL">
              <n-input-number v-model:value="configForm.scan.cacheTTL" :min="1" placeholder="5">
                <template #suffix>
                  <n-text depth="3">分钟</n-text>
                </template>
              </n-input-number>
            </n-form-item>
            <n-form-item label="启用自动更新">
              <n-switch v-model:value="configForm.scan.isUpdate" />
            </n-form-item>
            <n-form-item label="允许更新 Compose 容器">
              <n-switch v-model:value="configForm.scan.allowComposeUpdate" />
            </n-form-item>
          </n-form>
        </n-card>

        <!-- 策略设置 -->
        <n-card title="策略设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="跳过标签">
              <n-dynamic-tags v-model:value="configForm.policy.skipLabels" />
            </n-form-item>
            <n-form-item label="仅包含标签">
              <n-dynamic-tags v-model:value="configForm.policy.onlyLabels" />
            </n-form-item>
            <n-form-item label="跳过本地构建">
              <n-switch v-model:value="configForm.policy.skipLocalBuild" />
            </n-form-item>
            <n-form-item label="跳过固定摘要">
              <n-switch v-model:value="configForm.policy.skipPinnedDigest" />
            </n-form-item>
            <n-form-item label="跳过语义化版本">
              <n-switch v-model:value="configForm.policy.skipSemverPinned" />
            </n-form-item>
            <n-form-item label="浮动标签">
              <n-dynamic-tags v-model:value="configForm.policy.floatingTags" />
            </n-form-item>
          </n-form>
        </n-card>

        <!-- 仓库认证设置 -->
        <n-card title="仓库认证" embedded>
          <div class="registry-auth-section">
            <n-space vertical>
              <div v-for="(auth, index) in configForm.registry.auth" :key="index" class="auth-item">
                <n-card size="small">
                  <n-form :model="auth" label-placement="left" label-width="80px">
                    <n-form-item label="主机">
                      <n-select
                        v-model:value="auth.host"
                        :options="registryHostOptions"
                        filterable
                        tag
                        placeholder="选择或输入 registry 主机"
                      />
                    </n-form-item>
                    <n-form-item label="用户名">
                      <n-input v-model:value="auth.username" placeholder="username" />
                    </n-form-item>
                    <n-form-item label="令牌">
                      <n-input
                        v-model:value="auth.token"
                        type="password"
                        show-password-on="click"
                        placeholder="access token"
                      />
                    </n-form-item>
                  </n-form>

                  <template #action>
                    <n-button @click="removeAuth(index)" type="error" size="small" ghost>
                      删除
                    </n-button>
                  </template>
                </n-card>
              </div>

              <n-button @click="addAuth" type="primary" dashed>
                <template #icon>
                  <n-icon>
                    <AddOutline />
                  </n-icon>
                </template>
                添加仓库认证
              </n-button>
            </n-space>

            <n-alert title="仓库说明" type="info" class="mt-4">
              <div>
                <div><strong>内置支持：</strong></div>
                <ul class="registry-tips">
                  <li><strong>Docker Hub：</strong>选择 "dockerhub" 或 "docker.io"</li>
                  <li><strong>GitHub：</strong>选择 "ghcr.io"</li>
                  <li>
                    <strong>自定义：</strong>直接输入私有 registry 地址（如 registry.example.com）
                  </li>
                </ul>
              </div>
            </n-alert>
          </div>
        </n-card>

        <!-- 日志设置 -->
        <n-card title="日志设置" embedded>
          <n-form :model="configForm" label-placement="left" label-width="120px">
            <n-form-item label="日志级别">
              <n-select v-model:value="configForm.logging.level" :options="logLevelOptions" />
            </n-form-item>
          </n-form>
        </n-card>
      </n-space>
    </div>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg">系统设置</n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            配置 Watch Docker 的运行参数和策略
          </n-text>
        </div>
      </div>
    </Teleport>

    <!-- 底部保存按钮 -->
    <Teleport to="#footer" defer>
      <div class="save-button-container">
        <n-button type="primary" size="large" @click="handleSave" :loading="saving">
          <template #icon>
            <SaveOutline />
          </template>
          保存配置
        </n-button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import type { Config } from '@/common/types'
import { AddOutline, SaveOutline } from '@vicons/ionicons5'
import { configApi } from '@/common/api'

const message = useMessage()

// 保存状态
const saving = ref(false)

const notifyUrlPlaceholder = computed(() => {
  const host = 'http://127.0.0.1:8080'
  if (configForm.notify.method == 'GET') {
    return `${host}/notify?title={title}&content={content}&url={url}&image={image}`
  } else {
    return `${host}/notify`
  }
})

// 表单数据
const configForm = reactive<Config>({
  server: {
    addr: ':8080',
  },
  notify: {
    url: '',
    method: 'GET',
    isEnable: true,
  },
  docker: {
    host: '',
    includeStopped: false,
  },
  scan: {
    cron: '',
    concurrency: 3,
    cacheTTL: 10,
    isUpdate: true,
    allowComposeUpdate: false,
  },
  policy: {
    skipLabels: ['watchdocker.skip=true'],
    onlyLabels: [],
    skipLocalBuild: true,
    skipPinnedDigest: true,
    skipSemverPinned: true,
    floatingTags: ['latest', 'main', 'stable'],
  },
  registry: {
    auth: [],
  },
  logging: {
    level: 'info',
  },
})

// 选项配置
const logLevelOptions = [
  { label: 'Debug', value: 'debug' },
  { label: 'Info', value: 'info' },
  { label: 'Warn', value: 'warn' },
  { label: 'Error', value: 'error' },
]

const notifyMethodOptions = [
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
]

// Registry 主机选项
const registryHostOptions = [
  {
    label: 'docker.io',
    value: 'docker.io',
    description: 'Docker Hub 官方域名',
  },
  {
    label: 'GitHub Container Registry',
    value: 'ghcr.io',
    description: 'GitHub 容器镜像仓库',
  },
]

// 添加仓库认证
const addAuth = () => {
  configForm.registry.auth.push({
    host: '',
    username: '',
    token: '',
  })
}

// 删除仓库认证
const removeAuth = (index: number) => {
  configForm.registry.auth.splice(index, 1)
}

// 保存配置
const handleSave = async () => {
  if (saving.value) {
    return
  }

  saving.value = true
  try {
    const response = await configApi.saveConfig(configForm)
    if (response.code === 0) {
      message.success('配置保存成功')
    } else {
      throw new Error(response.msg || '保存失败')
    }
  } catch (error: any) {
    message.error(`保存失败: ${error.message || '未知错误'}`)
  } finally {
    saving.value = false
  }
}

// 加载配置
const loadConfig = async () => {
  try {
    const response = await configApi.getConfig()

    if (response.code === 0 && response.data?.config) {
      // 将服务器返回的配置合并到表单中
      Object.assign(configForm, response.data.config)
      message.success('配置加载成功')
    } else {
      throw new Error(response.msg || '获取配置失败')
    }
  } catch (error: any) {
    message.error(`加载配置失败: ${error.message || '未知错误'}`)
  }
}

// 页面初始化
onMounted(async () => {
  await loadConfig()
})
</script>

<style scoped lang="less">
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.settings-page {
  .page-header {
    margin-bottom: 16px;
  }

  .settings-content {
    max-width: 800px;
  }

  .registry-auth-section {
    .auth-item {
      margin-bottom: 16px;
    }
  }
}

.registry-tips {
  margin: 8px 0 0 0;
  padding-left: 20px;

  li {
    margin: 4px 0;
    color: var(--n-text-color);
  }
}

.save-button-container {
  width: 100%;
  text-align: right;
  padding-inline: 16px;
}

// 响应式调整
@media (max-width: 768px) {
  .settings-page {
    .settings-content {
      max-width: 100%;
    }
  }
}
</style>
