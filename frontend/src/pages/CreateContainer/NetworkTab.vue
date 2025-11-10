<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="network-tab">
      <n-space vertical size="large">
        <div>
          <n-h3 prefix="bar" class="mt-0">网络设置</n-h3>

          <n-form-item label="网络模式" path="networkMode">
            <n-select
              v-model:value="formValue.networkMode"
              :options="networkModeOptions"
              placeholder="选择网络模式"
            />
          </n-form-item>

          <n-form-item label="发布所有已曝光的端口">
            <n-switch v-model:value="formValue.publishAllPorts" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                自动将容器的所有曝光端口映射到主机的随机端口
              </n-text>
            </template>
          </n-form-item>
        </div>

        <n-divider />

        <div>
          <n-h3 prefix="bar">网络端点配置</n-h3>

          <n-form-item label="网络连接配置">
            <template #feedback>
              <n-text v-if="canUseNetworkConfig" depth="3">
                配置容器连接到特定网络时的端点设置，如静态 IP、网络别名等
              </n-text>
              <n-text v-else depth="3" type="warning">
                {{ networkConfigDisabledReason }}
              </n-text>
            </template>
            <n-space vertical class="w-full">
              <n-card
                v-for="(endpoint, index) in formValue.networkEndpoints"
                :key="index"
                size="small"
                :bordered="true"
              >
                <template #header>
                  <n-space justify="space-between" align="center">
                    <n-text>网络端点 {{ index + 1 }}</n-text>
                    <n-button size="small" type="error" text @click="removeNetworkEndpoint(index)">
                      删除
                    </n-button>
                  </n-space>
                </template>

                <n-space vertical class="w-full">
                  <n-form-item label="网络名称" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.networkName"
                      placeholder="例如: my-network"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="IPv4 地址" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.ipv4Address"
                      placeholder="例如: 172.20.0.10"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="IPv4 网关" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.ipv4Gateway"
                      placeholder="例如: 172.20.0.1"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="IPv6 地址" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.ipv6Address"
                      placeholder="例如: 2001:db8::10"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="IPv6 网关" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.ipv6Gateway"
                      placeholder="例如: 2001:db8::1"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="MAC 地址" :show-feedback="false">
                    <n-input
                      v-model:value="endpoint.macAddress"
                      placeholder="例如: 02:42:ac:11:00:02"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>

                  <n-form-item label="网络别名" :show-feedback="false">
                    <n-dynamic-tags
                      v-model:value="endpoint.aliases"
                      :disabled="!canUseNetworkConfig"
                    />
                  </n-form-item>
                </n-space>
              </n-card>

              <n-button
                type="primary"
                dashed
                block
                :disabled="!canUseNetworkConfig"
                @click="addNetworkEndpoint"
              >
                + 添加网络端点
              </n-button>
            </n-space>
          </n-form-item>
        </div>

        <n-divider />

        <div>
          <n-h3 prefix="bar">DNS 配置</n-h3>

          <n-form-item label="DNS 服务器">
            <n-dynamic-tags v-model:value="formValue.dns" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                自定义 DNS 服务器,覆盖容器的默认 DNS 配置,例如: 8.8.8.8
              </n-text>
            </template>
          </n-form-item>

          <n-form-item label="DNS 搜索域">
            <n-dynamic-tags v-model:value="formValue.dnsSearch" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                设置 DNS 搜索域,用于域名解析,例如: example.com
              </n-text>
            </template>
          </n-form-item>

          <n-form-item label="DNS 选项">
            <n-dynamic-tags v-model:value="formValue.dnsOptions" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                设置 DNS 解析器的选项,例如: ndots:2
              </n-text>
            </template>
          </n-form-item>
        </div>

        <n-divider />

        <div>
          <n-h3 prefix="bar">Hosts 记录</n-h3>

          <n-form-item label="额外的 Hosts">
            <n-dynamic-tags v-model:value="formValue.extraHosts" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                添加自定义的 /etc/hosts 记录,格式: hostname:ip (例如: myhost:192.168.1.100)
              </n-text>
            </template>
          </n-form-item>
        </div>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import type { FormInst } from 'naive-ui'
import type { NetworkFormValue, NetworkEndpointItem } from './types'

const formValue = defineModel<NetworkFormValue>({
  default: () => ({
    networkMode: 'bridge',
    publishAllPorts: false,
    dns: [],
    dnsSearch: [],
    dnsOptions: [],
    extraHosts: [],
    networkEndpoints: [],
  }),
})

const formRef = ref<FormInst | null>(null)

const networkModeOptions = [
  { label: 'Bridge (默认)', value: 'bridge' },
  { label: 'Host', value: 'host' },
  { label: 'None', value: 'none' },
  { label: 'Container', value: 'container' },
]

// 判断是否可以使用网络端点配置
const canUseNetworkConfig = computed(() => {
  const mode = formValue.value.networkMode
  // host 和 none 模式不支持网络端点配置
  return mode !== 'host' && mode !== 'none'
})

// 网络端点配置禁用原因
const networkConfigDisabledReason = computed(() => {
  const mode = formValue.value.networkMode
  if (mode === 'host') {
    return '在 Host 网络模式下，容器直接使用主机网络栈，无法配置独立的网络端点'
  }
  if (mode === 'none') {
    return '在 None 网络模式下，容器没有网络接口，无法配置网络端点'
  }
  return ''
})

// 监听网络模式变化，清空不适用的配置
watch(
  () => formValue.value.networkMode,
  (newMode) => {
    if (newMode === 'host' || newMode === 'none') {
      // 清空网络端点配置
      if (formValue.value.networkEndpoints && formValue.value.networkEndpoints.length > 0) {
        formValue.value.networkEndpoints = []
      }
    }
  },
)

// 添加网络端点
const addNetworkEndpoint = () => {
  const newEndpoint: NetworkEndpointItem = {
    networkName: '',
    ipv4Address: '',
    ipv4Gateway: '',
    ipv6Address: '',
    ipv6Gateway: '',
    macAddress: '',
    aliases: [],
  }
  if (!formValue.value.networkEndpoints) {
    formValue.value.networkEndpoints = []
  }
  formValue.value.networkEndpoints.push(newEndpoint)
}

// 删除网络端点
const removeNetworkEndpoint = (index: number) => {
  if (!formValue.value.networkEndpoints) {
    return
  }
  formValue.value.networkEndpoints.splice(index, 1)
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.network-tab {
  padding: 0;
}
</style>
