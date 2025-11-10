<template>
  <n-modal
    v-model:show="showModal"
    :icon="getIcon()"
    display-directive="if"
    preset="dialog"
    title="创建网络"
    titleClass="network-create-modal-title"
    class="network-create-modal"
    :style="{
      padding: '0px',
      width: '90vw',
      maxWidth: '900px',
      height: '90vh',
    }"
  >
    <div class="network-create-modal-content">
      <n-form
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-placement="top"
        require-mark-placement="right-hanging"
      >
        <!-- 基础配置 -->
        <n-h3 prefix="bar" class="mt-0">基础配置</n-h3>
        <n-form-item label="网络名称" path="name" required>
          <n-input
            v-model:value="formData.name"
            placeholder="请输入网络名称"
            :maxlength="50"
            show-count
          />
        </n-form-item>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="驱动类型" path="driver">
              <n-select
                v-model:value="formData.driver"
                :options="driverOptions"
                placeholder="选择驱动类型"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="作用域" path="scope">
              <n-select
                v-model:value="formData.scope"
                :options="scopeOptions"
                placeholder="选择作用域"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <!-- Macvlan/IPvlan Parent 接口 -->
        <n-form-item
          v-if="formData.driver === 'macvlan' || formData.driver === 'ipvlan'"
          label="父网络接口 (Parent)"
          path="parentInterface"
          required
        >
          <n-input v-model:value="formData.parentInterface" placeholder="例如：eth0, enp0s3" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              {{ formData.driver }} 驱动需要指定宿主机的物理网络接口作为父接口
            </n-text>
          </template>
        </n-form-item>

        <!-- IPAM 配置 -->
        <n-divider />
        <n-h3 prefix="bar">IPAM 配置</n-h3>

        <n-form-item label="启用自定义 IPAM 配置">
          <n-switch v-model:value="enableCustomIPAM" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              可配置 IPv4 和 IPv6 子网，多个配置项可同时存在
            </n-text>
          </template>
        </n-form-item>

        <template v-if="enableCustomIPAM">
          <n-form-item label="IPAM 驱动" path="ipam.driver">
            <n-input v-model:value="formData.ipam.driver" placeholder="default" />
          </n-form-item>

          <div
            v-for="(config, index) in formData.ipam.config"
            :key="index"
            class="ipam-config-item"
          >
            <div class="flex items-center justify-between mb-3">
              <n-text strong>子网配置 {{ index + 1 }}</n-text>
              <n-button
                v-if="formData.ipam.config.length > 1"
                size="small"
                tertiary
                type="error"
                @click="removeIPAMConfig(index)"
              >
                <template #icon>
                  <n-icon><TrashOutline /></n-icon>
                </template>
                删除
              </n-button>
            </div>

            <n-form-item
              :label="`子网 ${index + 1}`"
              :path="`ipam.config[${index}].subnet`"
              :rule="ipamSubnetRule"
            >
              <n-input
                v-model:value="config.subnet"
                placeholder="IPv4: 172.20.0.0/16 或 IPv6: fd00::/64"
                @blur="handleSubnetBlur(index)"
              />
            </n-form-item>

            <n-grid :cols="2" :x-gap="12">
              <n-gi>
                <n-form-item :label="`网关 ${index + 1}`" :path="`ipam.config[${index}].gateway`">
                  <n-input
                    v-model:value="config.gateway"
                    placeholder="IPv4: 172.20.0.1 或 IPv6: fd00::1"
                  />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item
                  :label="`IP 范围 ${index + 1}`"
                  :path="`ipam.config[${index}].ipRange`"
                >
                  <n-input
                    v-model:value="config.ipRange"
                    placeholder="IPv4: 172.20.10.0/24 或 IPv6: fd00:1::/80"
                  />
                </n-form-item>
              </n-gi>
            </n-grid>

            <n-divider v-if="index < formData.ipam.config.length - 1" />
          </div>

          <n-button dashed block @click="addIPAMConfig" class="mb-4">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加子网配置
          </n-button>
        </template>

        <!-- 高级选项 -->
        <n-divider />
        <n-collapse>
          <n-collapse-item title="高级选项" name="advanced">
            <n-grid cols="2 s:1" :x-gap="12" :y-gap="12">
              <!-- 启用 IPv6 -->
              <n-gi>
                <n-form-item label="启用 IPv6">
                  <n-switch v-model:value="formData.enableIPv6" />
                  <template #feedback>
                    <n-text depth="3" style="font-size: 12px">
                      启用后可在 IPAM 配置中添加 IPv6 子网
                    </n-text>
                  </template>
                </n-form-item>
              </n-gi>

              <!-- 内部网络 -->
              <n-gi>
                <n-form-item label="内部网络 (Isolate)">
                  <n-switch v-model:value="formData.internal" />
                  <template #feedback>
                    <n-text depth="3" style="font-size: 12px"> 内部网络无法访问外部网络 </n-text>
                  </template>
                </n-form-item>
              </n-gi>

              <!-- 可连接 -->
              <n-gi>
                <n-form-item label="可连接 (Attachable)">
                  <n-switch v-model:value="formData.attachable" :disabled="formData.ingress" />
                  <template #feedback>
                    <n-text depth="3" style="font-size: 12px">
                      {{
                        formData.ingress
                          ? 'Ingress 网络不支持 Attachable 选项'
                          : '允许容器动态连接到此网络'
                      }}
                    </n-text>
                  </template>
                </n-form-item>
              </n-gi>

              <!-- Ingress 网络 -->
              <n-gi>
                <n-form-item label="Ingress 网络">
                  <n-switch
                    v-model:value="formData.ingress"
                    :disabled="formData.scope !== 'global' || formData.driver !== 'overlay'"
                  />
                  <template #feedback>
                    <n-text depth="3">
                      {{
                        formData.driver !== 'overlay'
                          ? 'Ingress 网络仅支持 Overlay 驱动'
                          : formData.scope !== 'global'
                            ? 'Ingress 网络仅在 Global 作用域下可用'
                            : 'Swarm 模式下的 Ingress 网络'
                      }}
                    </n-text>
                  </template>
                </n-form-item>
              </n-gi>

              <!-- 驱动选项标题 -->
              <n-gi :span="2">
                <n-divider style="margin: 8px 0" />
                <n-text strong>驱动选项</n-text>
              </n-gi>

              <!-- 驱动选项列表 -->
              <n-gi :span="2">
                <n-space vertical size="small">
                  <div v-for="(option, index) in driverOptions_" :key="index" class="option-item">
                    <n-grid cols="3" :x-gap="8">
                      <n-gi>
                        <n-input v-model:value="option.key" placeholder="键" size="small" />
                      </n-gi>
                      <n-gi>
                        <n-input v-model:value="option.value" placeholder="值" size="small" />
                      </n-gi>
                      <n-gi>
                        <n-button
                          size="small"
                          tertiary
                          type="error"
                          @click="removeDriverOption(index)"
                          block
                        >
                          <template #icon>
                            <n-icon><CloseOutline /></n-icon>
                          </template>
                          删除
                        </n-button>
                      </n-gi>
                    </n-grid>
                  </div>
                  <n-button dashed block @click="addDriverOption" size="small">
                    <template #icon>
                      <n-icon><AddOutline /></n-icon>
                    </template>
                    添加驱动选项
                  </n-button>
                </n-space>
              </n-gi>

              <!-- 标签标题 -->
              <n-gi :span="2">
                <n-divider style="margin: 8px 0" />
                <n-text strong>标签</n-text>
              </n-gi>

              <!-- 标签列表 -->
              <n-gi :span="2">
                <n-space vertical size="small">
                  <div v-for="(label, index) in labels" :key="index" class="option-item">
                    <n-grid cols="3" :x-gap="8">
                      <n-gi>
                        <n-input v-model:value="label.key" placeholder="键" size="small" />
                      </n-gi>
                      <n-gi>
                        <n-input v-model:value="label.value" placeholder="值" size="small" />
                      </n-gi>
                      <n-gi>
                        <n-button
                          size="small"
                          tertiary
                          type="error"
                          @click="removeLabel(index)"
                          block
                        >
                          <template #icon>
                            <n-icon><CloseOutline /></n-icon>
                          </template>
                          删除
                        </n-button>
                      </n-gi>
                    </n-grid>
                  </div>
                  <n-button dashed block @click="addLabel" size="small">
                    <template #icon>
                      <n-icon><AddOutline /></n-icon>
                    </template>
                    添加标签
                  </n-button>
                </n-space>
              </n-gi>
            </n-grid>
          </n-collapse-item>
        </n-collapse>
      </n-form>
    </div>
    <div class="flex justify-end gap-2 p-[12px]">
      <n-button @click="handleCancel">取消</n-button>
      <n-button type="primary" @click="handleCreate" :loading="creating">创建</n-button>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import type { NetworkCreateRequest, NetworkIPAMConfigCreate } from '@/common/types'
import { renderIcon } from '@/common/utils'
import { useNetworkStore } from '@/store/network'
import { AddOutline, CloseOutline, GitNetworkOutline, TrashOutline } from '@vicons/ionicons5'
import {
  useMessage,
  useThemeVars,
  type FormInst,
  type FormRules,
  NForm,
  NGrid,
  NGi,
} from 'naive-ui'
import { ref, watch } from 'vue'
const theme = useThemeVars()
interface Emits {
  (e: 'created'): void
}
const getIcon = () => {
  return renderIcon(GitNetworkOutline, {
    color: theme.value.primaryColor,
    size: 20,
  })
}
const showModal = defineModel<boolean>('show')
const emits = defineEmits<Emits>()

const networkStore = useNetworkStore()
const message = useMessage()

const formRef = ref<FormInst | null>(null)
const creating = ref(false)
const enableCustomIPAM = ref(false)

// 表单数据
const formData = ref<
  NetworkCreateRequest & {
    ipam: { driver: string; config: NetworkIPAMConfigCreate[] }
    parentInterface?: string
  }
>({
  name: '',
  driver: 'bridge',
  scope: 'local',
  internal: false,
  attachable: false,
  ingress: false,
  enableIPv6: false,
  parentInterface: '',
  ipam: {
    driver: 'default',
    config: [
      {
        subnet: '',
        gateway: '',
        ipRange: '',
      },
    ],
  },
  options: {},
  labels: {},
})

// 驱动选项（键值对数组）
const driverOptions_ = ref<Array<{ key: string; value: string }>>([])

// 标签（键值对数组）
const labels = ref<Array<{ key: string; value: string }>>([])

// 驱动类型选项
const driverOptions = [
  { label: 'Bridge', value: 'bridge' },
  { label: 'Overlay', value: 'overlay' },
  { label: 'Macvlan', value: 'macvlan' },
  { label: 'IPvlan', value: 'ipvlan' },
  { label: 'Host', value: 'host' },
  { label: 'None', value: 'none' },
]

// 作用域选项
const scopeOptions = [
  { label: '本地 (Local)', value: 'local' },
  { label: 'Swarm', value: 'swarm' },
  { label: '全局 (Global)', value: 'global' },
]

// 表单验证规则
const rules: FormRules = {
  name: [
    { required: true, message: '请输入网络名称', trigger: 'blur' },
    {
      pattern: /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/,
      message: '网络名称只能包含字母、数字、下划线、点和连字符，且必须以字母或数字开头',
      trigger: 'blur',
    },
  ],
  parentInterface: [
    {
      required: true,
      trigger: 'blur',
      validator: (_rule: any, value: string) => {
        if (formData.value.driver === 'macvlan' || formData.value.driver === 'ipvlan') {
          if (!value || value.trim() === '') {
            return new Error(`${formData.value.driver} 驱动必须指定父网络接口`)
          }
        }
        return true
      },
    },
  ],
}

// IPAM 子网验证规则（支持 IPv4 和 IPv6）
const ipamSubnetRule = {
  trigger: 'blur',
  validator: (_rule: any, value: string) => {
    if (!value) {
      return true
    } // 允许为空
    // IPv4 CIDR 格式验证
    const ipv4CidrPattern = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/
    // IPv6 CIDR 格式验证（简化版）
    const ipv6CidrPattern = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\/\d{1,3}$/

    if (!ipv4CidrPattern.test(value) && !ipv6CidrPattern.test(value)) {
      return new Error('请输入有效的 CIDR 格式，例如 IPv4: 172.20.0.0/16 或 IPv6: fd00::/64')
    }
    return true
  },
}

// 添加 IPAM 配置
const addIPAMConfig = () => {
  formData.value.ipam.config.push({
    subnet: '',
    gateway: '',
    ipRange: '',
  })
}

// 删除 IPAM 配置
const removeIPAMConfig = (index: number) => {
  formData.value.ipam.config.splice(index, 1)
}

// 处理子网输入失焦事件，自动生成网关（支持 IPv4 和 IPv6）
const handleSubnetBlur = (index: number) => {
  const config = formData.value.ipam.config[index]
  if (config.subnet && !config.gateway) {
    // IPv4: 尝试自动生成网关（取子网的第一个IP）
    const ipv4Match = config.subnet.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.)\d{1,3}\/\d{1,2}$/)
    if (ipv4Match) {
      config.gateway = `${ipv4Match[1]}1`
      return
    }

    // IPv6: 尝试自动生成网关
    const ipv6Match = config.subnet.match(/^([0-9a-fA-F:]+)\/\d{1,3}$/)
    if (ipv6Match) {
      // 简单处理：如果是压缩格式（如 fd00::/64），生成 fd00::1
      const prefix = ipv6Match[1]
      if (prefix.endsWith('::')) {
        config.gateway = `${prefix}1`
      } else if (prefix.endsWith(':')) {
        config.gateway = `${prefix}:1`
      } else {
        config.gateway = `${prefix}::1`
      }
    }
  }
}

// 添加驱动选项
const addDriverOption = () => {
  driverOptions_.value.push({ key: '', value: '' })
}

// 删除驱动选项
const removeDriverOption = (index: number) => {
  driverOptions_.value.splice(index, 1)
}

// 添加标签
const addLabel = () => {
  labels.value.push({ key: '', value: '' })
}

// 删除标签
const removeLabel = (index: number) => {
  labels.value.splice(index, 1)
}

// 处理取消
const handleCancel = () => {
  showModal.value = false
}

// 处理创建
const handleCreate = async () => {
  try {
    await formRef.value?.validate()

    creating.value = true

    // 构建请求数据
    const requestData: NetworkCreateRequest = {
      name: formData.value.name,
      driver: formData.value.driver,
      scope: formData.value.scope,
      internal: formData.value.internal,
      // Ingress 和 Attachable 互斥
      attachable: formData.value.ingress ? false : formData.value.attachable,
      // Ingress 网络只能在 overlay 驱动 + global 作用域下使用
      ingress:
        formData.value.driver === 'overlay' && formData.value.scope === 'global'
          ? formData.value.ingress
          : false,
      enableIPv6: formData.value.enableIPv6,
    }

    // 添加 IPAM 配置
    if (enableCustomIPAM.value) {
      const ipamConfig = formData.value.ipam.config.filter((c) => c.subnet)

      if (ipamConfig.length > 0) {
        requestData.ipam = {
          driver: formData.value.ipam.driver || 'default',
          config: ipamConfig,
        }
      }
    }

    // 添加驱动选项
    const options: Record<string, string> = {}

    // 如果是 macvlan 或 ipvlan，添加 parent 参数
    if (
      (formData.value.driver === 'macvlan' || formData.value.driver === 'ipvlan') &&
      formData.value.parentInterface
    ) {
      options.parent = formData.value.parentInterface
    }

    driverOptions_.value.forEach((opt) => {
      if (opt.key && opt.value) {
        options[opt.key] = opt.value
      }
    })
    if (Object.keys(options).length > 0) {
      requestData.options = options
    }

    // 添加标签
    const labelsObj: Record<string, string> = {}
    labels.value.forEach((label) => {
      if (label.key && label.value) {
        labelsObj[label.key] = label.value
      }
    })
    if (Object.keys(labelsObj).length > 0) {
      requestData.labels = labelsObj
    }

    await networkStore.createNetwork(requestData)

    message.success('网络创建成功')
    showModal.value = false
    emits('created')
    resetForm()
  } catch (error: any) {
    if (error?.errors) {
      // 表单验证错误
      return
    }
    message.error(`创建失败：${error.message || '未知错误'}`)
  } finally {
    creating.value = false
  }
}

// 重置表单
const resetForm = () => {
  formData.value = {
    name: '',
    driver: 'bridge',
    scope: 'local',
    internal: false,
    attachable: false,
    ingress: false,
    enableIPv6: false,
    parentInterface: '',
    ipam: {
      driver: 'default',
      config: [
        {
          subnet: '',
          gateway: '',
          ipRange: '',
        },
      ],
    },
    options: {},
    labels: {},
  }
  enableCustomIPAM.value = false
  driverOptions_.value = []
  labels.value = []
  formRef.value?.restoreValidation()
}

// 监听驱动变化，清空 parentInterface（仅对非 macvlan/ipvlan 驱动）
watch(
  () => formData.value.driver,
  (newDriver) => {
    if (newDriver !== 'macvlan' && newDriver !== 'ipvlan') {
      formData.value.parentInterface = ''
    }
  },
)

// 监听驱动和作用域变化，自动禁用 Ingress（Ingress 仅在 overlay 驱动 + global 作用域下可用）
watch(
  () => [formData.value.driver, formData.value.scope],
  ([newDriver, newScope]) => {
    if (formData.value.ingress && (newDriver !== 'overlay' || newScope !== 'global')) {
      formData.value.ingress = false
    }
  },
)

// 监听 Ingress 变化，自动禁用 Attachable（Ingress 和 Attachable 互斥）
watch(
  () => formData.value.ingress,
  (newIngress) => {
    if (newIngress && formData.value.attachable) {
      formData.value.attachable = false
    }
  },
)

// 监听弹窗关闭，重置表单
watch(showModal, (newVal) => {
  if (!newVal) {
    setTimeout(() => {
      resetForm()
    }, 300)
  }
})
</script>

<style lang="less">
@import '@/styles/mix.less';

.network-create-modal {
  .network-create-modal-title {
    padding: 20px;
  }
  .network-create-modal-content {
    .scrollbar();
    overflow: auto;
    height: calc(90vh - 136px);
    padding: 20px;
    position: relative;
    will-change: scroll-position;
  }
  .ipam-config-item {
    padding: 16px;
    margin-bottom: 16px;
    border-radius: 8px;
    background: var(--n-color-embedded);
  }
  .option-item {
    margin-bottom: 8px;
  }
}
</style>
