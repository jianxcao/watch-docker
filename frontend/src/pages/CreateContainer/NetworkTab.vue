<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="network-tab">
      <n-space vertical size="large">
        <!-- 第一部分：网络配置模式选择 -->
        <div>
          <n-h3 prefix="bar" class="mt-0">网络配置模式</n-h3>
          <n-form-item>
            <n-radio-group v-model:value="formValue.configMode">
              <n-space vertical>
                <n-radio value="default">
                  <div>
                    <div>默认 Bridge 网络</div>
                    <n-text depth="3" style="font-size: 12px">
                      自动创建 "容器名_default" 网络，适合简单场景
                    </n-text>
                  </div>
                </n-radio>
                <n-radio value="custom">
                  <div>
                    <div>自定义网络配置</div>
                    <n-text depth="3" style="font-size: 12px">
                      连接到已有网络或创建新网络，可配置静态 IP、子网等
                    </n-text>
                  </div>
                </n-radio>
              </n-space>
            </n-radio-group>
          </n-form-item>
        </div>

        <n-divider />

        <!-- 第二部分：根据模式显示不同配置区域 -->

        <!-- 模式 A：默认 bridge -->
        <div v-if="formValue.configMode === 'default'">
          <n-h3 prefix="bar">默认网络配置</n-h3>
          <n-alert type="info" :bordered="false">
            将自动创建一个名为 "容器名_default" 的 bridge 网络，容器将连接到该网络并自动分配 IP 地址
          </n-alert>

          <n-form-item label="发布所有已曝光的端口" style="margin-top: 16px">
            <n-switch v-model:value="formValue.publishAllPorts" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                自动将容器的所有曝光端口映射到主机的随机端口
              </n-text>
            </template>
          </n-form-item>
        </div>

        <!-- 模式 B：自定义网络配置 -->
        <div v-if="formValue.configMode === 'custom'">
          <n-h3 prefix="bar">自定义网络配置</n-h3>

          <n-form-item label="发布所有已曝光的端口" style="margin-bottom: 16px">
            <n-switch v-model:value="formValue.publishAllPorts" />
            <template #feedback>
              <n-text depth="3" style="font-size: 12px">
                自动将容器的所有曝光端口映射到主机的随机端口
              </n-text>
            </template>
          </n-form-item>

          <n-form-item label="网络配置列表">
            <template #feedback>
              <n-text depth="3">
                添加一个或多个网络，容器将连接到这些网络。如果网络不存在，可以选择自动创建
              </n-text>
            </template>
            <n-space vertical class="w-full">
              <n-card
                v-for="(network, index) in formValue.customNetworks"
                :key="index"
                size="small"
                :bordered="true"
              >
                <template #header>
                  <n-space justify="space-between" align="center">
                    <n-text>网络 {{ index + 1 }}</n-text>
                    <n-button size="small" type="error" text @click="removeCustomNetwork(index)">
                      删除
                    </n-button>
                  </n-space>
                </template>

                <n-space vertical class="w-full">
                  <!-- 基础配置 -->
                  <n-h4 style="margin-top: 0">基础配置</n-h4>

                  <n-form-item label="网络名称" required>
                    <n-select
                      v-model:value="network.name"
                      :options="networkOptions"
                      filterable
                      tag
                      placeholder="选择已有网络或输入新网络名称"
                      @update:value="handleNetworkNameChange(index)"
                    />
                    <template #feedback>
                      <n-text
                        v-if="network.exists"
                        depth="3"
                        style="font-size: 12px; color: #18a058"
                      >
                        ✓ 网络已存在，将直接连接到该网络
                      </n-text>
                      <n-text
                        v-else-if="network.name"
                        depth="3"
                        style="font-size: 12px; color: #f0a020"
                      >
                        ⚠ 网络不存在，将自动创建该网络
                      </n-text>
                      <n-text v-else depth="3" style="font-size: 12px">
                        选择已有网络或输入新网络名称
                      </n-text>
                    </template>
                  </n-form-item>

                  <!-- 网络创建配置（仅在网络不存在时显示） -->
                  <template v-if="!network.exists && network.name">
                    <n-divider />
                    <n-h4>网络创建配置</n-h4>
                    <n-alert type="info" :bordered="false" style="margin-bottom: 12px">
                      以下配置仅在网络不存在时用于创建网络。如果网络已存在，这些配置将被忽略
                    </n-alert>

                    <n-form-item label="驱动类型" :show-feedback="false">
                      <n-select
                        v-model:value="network.driver"
                        :options="driverOptions"
                        placeholder="选择驱动类型"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          bridge: 默认网络驱动 | overlay: 跨主机通信 | macvlan: 物理网络集成
                        </n-text>
                      </template>
                    </n-form-item>

                    <!-- Macvlan 父网络接口 -->
                    <n-form-item
                      v-if="network.driver === 'macvlan'"
                      label="父网络接口 (Parent)"
                      :show-feedback="false"
                    >
                      <n-input
                        v-model:value="network.parentInterface"
                        placeholder="例如：eth0, enp0s3"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          macvlan 驱动必须指定宿主机的物理网络接口作为父接口
                        </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="启用 IPv6" :show-feedback="false">
                      <n-switch v-model:value="network.enableIPv6" />
                    </n-form-item>

                    <!-- IPv4 配置 -->
                    <n-divider style="margin: 12px 0" />
                    <n-text strong>IPv4 配置（可选）</n-text>

                    <n-form-item label="IPv4 子网" :show-feedback="false" style="margin-top: 12px">
                      <n-input
                        v-model:value="network.ipv4Subnet"
                        placeholder="例如: 172.20.0.0/16"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          CIDR 格式的子网地址，留空则自动分配
                        </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="IPv4 网关" :show-feedback="false">
                      <n-input v-model:value="network.ipv4Gateway" placeholder="例如: 172.20.0.1" />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          网关地址，留空则使用子网的第一个地址
                        </n-text>
                      </template>
                    </n-form-item>

                    <!-- IPv6 配置 -->
                    <template v-if="network.enableIPv6">
                      <n-divider style="margin: 12px 0" />
                      <n-text strong>IPv6 配置（可选）</n-text>

                      <n-form-item
                        label="IPv6 子网"
                        :show-feedback="false"
                        style="margin-top: 12px"
                      >
                        <n-input
                          v-model:value="network.ipv6Subnet"
                          placeholder="例如: 2001:db8::/64"
                        />
                        <template #feedback>
                          <n-text depth="3" style="font-size: 12px">
                            CIDR 格式的 IPv6 子网地址
                          </n-text>
                        </template>
                      </n-form-item>

                      <n-form-item label="IPv6 网关" :show-feedback="false">
                        <n-input
                          v-model:value="network.ipv6Gateway"
                          placeholder="例如: 2001:db8::1"
                        />
                        <template #feedback>
                          <n-text depth="3" style="font-size: 12px"> IPv6 网关地址 </n-text>
                        </template>
                      </n-form-item>
                    </template>

                    <!-- 其他选项 -->
                    <n-divider style="margin: 12px 0" />
                    <n-text strong>其他选项</n-text>

                    <n-form-item label="内部网络" :show-feedback="false" style="margin-top: 12px">
                      <n-switch v-model:value="network.internal" />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px"> 内部网络限制外部访问 </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="可附加" :show-feedback="false">
                      <n-switch v-model:value="network.attachable" />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          允许独立容器附加到此网络
                        </n-text>
                      </template>
                    </n-form-item>
                  </template>

                  <!-- 容器连接配置（当有网络名称时显示） -->
                  <template v-if="network.name">
                    <n-divider />
                    <n-h4>容器连接配置</n-h4>
                    <n-alert type="info" :bordered="false" style="margin-bottom: 12px">
                      配置容器连接到此网络时的参数。留空则自动分配
                    </n-alert>

                    <n-form-item label="容器 IPv4 地址" :show-feedback="false">
                      <n-input
                        v-model:value="network.containerIPv4Address"
                        placeholder="例如: 172.20.0.10"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          为容器指定静态 IPv4 地址，留空则自动分配
                        </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="容器 IPv6 地址" :show-feedback="false">
                      <n-input
                        v-model:value="network.containerIPv6Address"
                        placeholder="例如: 2001:db8::10"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          为容器指定静态 IPv6 地址（需要网络支持 IPv6）
                        </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="MAC 地址" :show-feedback="false">
                      <n-input
                        v-model:value="network.macAddress"
                        placeholder="例如: 02:42:ac:11:00:02"
                      />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          为容器指定自定义 MAC 地址，留空则自动生成
                        </n-text>
                      </template>
                    </n-form-item>

                    <n-form-item label="网络别名" :show-feedback="false">
                      <n-dynamic-tags v-model:value="network.aliases" />
                      <template #feedback>
                        <n-text depth="3" style="font-size: 12px">
                          容器在此网络中的别名，可用于网络内的 DNS 解析
                        </n-text>
                      </template>
                    </n-form-item>
                  </template>
                </n-space>
              </n-card>

              <n-button type="primary" dashed block @click="addCustomNetwork">
                + 添加网络
              </n-button>
            </n-space>
          </n-form-item>
        </div>

        <!-- 第三部分：通用网络配置（所有模式共享） -->
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
import type { NetworkFormValue, CustomNetworkConfig } from './types'
import { useNetworkStore } from '@/store/network'
import { onMounted, computed } from 'vue'

const networkStore = useNetworkStore()

const formValue = defineModel<NetworkFormValue>({
  default: () => ({
    configMode: 'default',
    publishAllPorts: false,
    dns: [],
    dnsSearch: [],
    dnsOptions: [],
    extraHosts: [],
    customNetworks: [],
  }),
})

const formRef = ref<FormInst | null>(null)

const driverOptions = [
  { label: 'Bridge (默认)', value: 'bridge' },
  { label: 'Overlay (跨主机)', value: 'overlay' },
  { label: 'Macvlan (物理网络)', value: 'macvlan' },
]

// 计算网络选项列表
const networkOptions = computed(() => {
  return networkStore.networks.map((network) => ({
    label: network.name,
    value: network.name,
  }))
})

// 组件加载时获取网络列表
onMounted(async () => {
  try {
    await networkStore.fetchNetworks()
  } catch (error) {
    console.error('获取网络列表失败:', error)
  }
})

// 监听配置模式变化，清空不适用的配置
watch(
  () => formValue.value.configMode,
  (newMode) => {
    if (newMode === 'default') {
      // 默认模式：清空所有自定义配置
      formValue.value.customNetworks = []
    }
  },
)

// 检查网络是否存在
const checkNetworkExists = (networkName: string): boolean => {
  if (!networkName || !networkName.trim()) {
    return false
  }
  const network = networkStore.findNetworkByName(networkName.trim())
  return !!network
}

// 监听网络名称变化，自动检测是否存在
const handleNetworkNameChange = (index: number) => {
  if (!formValue.value.customNetworks || !formValue.value.customNetworks[index]) {
    return
  }
  const network = formValue.value.customNetworks[index]
  network.exists = checkNetworkExists(network.name)
}

// 添加自定义网络
const addCustomNetwork = () => {
  const newNetwork: CustomNetworkConfig = {
    name: '',
    exists: false,
    driver: 'bridge',
    enableIPv6: false,
    ipv4Subnet: '',
    ipv4Gateway: '',
    ipv6Subnet: '',
    ipv6Gateway: '',
    internal: false,
    attachable: false,
    containerIPv4Address: '',
    containerIPv6Address: '',
    macAddress: '',
    aliases: [],
  }
  if (!formValue.value.customNetworks) {
    formValue.value.customNetworks = []
  }
  formValue.value.customNetworks.push(newNetwork)
}

// 删除自定义网络
const removeCustomNetwork = (index: number) => {
  if (!formValue.value.customNetworks) {
    return
  }
  formValue.value.customNetworks.splice(index, 1)
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
