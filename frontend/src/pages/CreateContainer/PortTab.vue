<template>
  <div class="port-tab">
    <n-space vertical size="large">
      <div>
        <n-h3 prefix="bar" class="mt-0">端口映射</n-h3>
        <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">
          将主机端口映射到容器端口
        </n-text>
        <n-space vertical size="small">
          <div v-for="(port, index) in portList" :key="index" class="port-item">
            <n-grid :cols="24" :x-gap="8">
              <n-gi :span="1" class="flex items-center">
                <n-text>主机</n-text>
              </n-gi>
              <n-gi :span="5">
                <n-input-number
                  v-model:value="port.hostPort"
                  placeholder="8080"
                  :min="1"
                  :max="65535"
                  size="small"
                  style="width: 100%"
                  @blur="updatePorts"
                />
              </n-gi>
              <n-gi :span="1" class="flex items-center justify-center">
                <n-icon><ArrowForwardOutline /></n-icon>
              </n-gi>
              <n-gi :span="1" class="flex items-center">
                <n-text>容器</n-text>
              </n-gi>
              <n-gi :span="5">
                <n-input-number
                  v-model:value="port.containerPort"
                  placeholder="80"
                  :min="1"
                  :max="65535"
                  size="small"
                  style="width: 100%"
                  @blur="updatePorts"
                />
              </n-gi>
              <n-gi :span="4">
                <n-select
                  v-model:value="port.protocol"
                  :options="protocolOptions"
                  size="small"
                  multiple
                  @update:value="updatePorts"
                />
              </n-gi>
              <n-gi :span="2">
                <n-button size="small" tertiary type="error" @click="removePort(index)" block>
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                </n-button>
              </n-gi>
            </n-grid>
          </div>
          <n-button dashed block @click="addPort" size="small">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加端口映射
          </n-button>
        </n-space>
      </div>

      <n-divider />

      <div v-if="exposedPortsString">
        <n-h3 prefix="bar">预制端口列表</n-h3>
        <n-alert title="预制端口列表" type="info">
          <template #icon>
            <n-icon><InformationCircleOutline /></n-icon>
          </template>
          <div>
            <n-text>
              {{ Object.keys(exposedPortsString).join(', ') }}
            </n-text>
          </div>
        </n-alert>
      </div>

      <n-form-item label="映射所有预制端口端口">
        <n-switch v-model:value="publishAllPorts" @update:value="updatePublishAllPorts" />
      </n-form-item>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import {
  AddOutline,
  ArrowForwardOutline,
  CloseOutline,
  InformationCircleOutline,
} from '@vicons/ionicons5'
import { ref, watch } from 'vue'
import type { ContainerCreateRequest, PortBinding } from '@/common/types'

interface Props {
  modelValue: Partial<ContainerCreateRequest>
}

interface Emits {
  (e: 'update:modelValue', value: Partial<ContainerCreateRequest>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

interface PortItem {
  hostPort: number | null
  containerPort: number | null
  protocol: ('tcp' | 'udp')[]
}

const portList = ref<PortItem[]>([])
const publishAllPorts = ref(false)

const exposedPortsString = computed(() => {
  return props.modelValue.exposedPorts ? Object.keys(props.modelValue.exposedPorts).join(', ') : ''
})

const protocolOptions = [
  { label: 'TCP', value: 'tcp' },
  { label: 'UDP', value: 'udp' },
]

// 初始化端口列表
const init = () => {
  if (props.modelValue.portBindings) {
    const portsMap = new Map<string, PortItem>()

    Object.entries(props.modelValue.portBindings).forEach(([containerPort, bindings]) => {
      const [port, protocol] = containerPort.split('/')
      bindings.forEach((binding: PortBinding) => {
        const hostPort = binding.hostPort ? parseInt(binding.hostPort) : null
        const key = `${hostPort}-${port}`

        if (portsMap.has(key)) {
          // 如果已存在相同的主机端口和容器端口，添加协议
          const existingPort = portsMap.get(key)!
          if (!existingPort.protocol.includes(protocol as 'tcp' | 'udp')) {
            existingPort.protocol.push(protocol as 'tcp' | 'udp')
          }
        } else {
          // 创建新的端口项
          portsMap.set(key, {
            hostPort: hostPort,
            containerPort: parseInt(port),
            protocol: [(protocol || 'tcp') as 'tcp' | 'udp'],
          })
        }
      })
    })

    portList.value = Array.from(portsMap.values())
  }
  if (props.modelValue.publishAllPorts) {
    publishAllPorts.value = true
  }
}

onBeforeMount(() => {
  init()
})

const addPort = () => {
  portList.value.push({
    hostPort: null,
    containerPort: null,
    protocol: ['tcp', 'udp'] as ('tcp' | 'udp')[],
  })
}

const removePort = (index: number) => {
  portList.value.splice(index, 1)
  updatePorts()
}

const updatePorts = () => {
  const portBindings: Record<string, PortBinding[]> = {}

  portList.value.forEach((port) => {
    if (port.containerPort && port.protocol.length > 0) {
      // 为每个选择的协议创建端口绑定
      port.protocol.forEach((protocol) => {
        const key = `${port.containerPort}/${protocol}`
        // 如果有主机端口,添加到端口绑定
        if (port.hostPort) {
          if (!portBindings[key]) {
            portBindings[key] = []
          }
          portBindings[key].push({
            hostIP: '',
            hostPort: port.hostPort.toString(),
          })
        }
      })
    }
  })

  emit('update:modelValue', {
    ...props.modelValue,
    portBindings: Object.keys(portBindings).length > 0 ? portBindings : undefined,
  })
}

const updatePublishAllPorts = () => {
  emit('update:modelValue', {
    ...props.modelValue,
    publishAllPorts: publishAllPorts.value,
  })
}

watch(
  () => props.modelValue.publishAllPorts,
  (newVal) => {
    publishAllPorts.value = newVal ?? false
  },
)
</script>

<style scoped>
.port-tab {
  padding: 0;
}

.port-item {
  margin-bottom: 8px;
}
</style>
