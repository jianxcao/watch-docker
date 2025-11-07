<template>
  <div class="advanced-tab">
    <n-space vertical size="large">
      <!-- 重启策略 -->
      <div>
        <n-h3 prefix="bar" class="mt-0">重启策略</n-h3>
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="策略" path="restartPolicy.name">
              <n-select
                v-model:value="formData.restartPolicy.name"
                :options="restartPolicyOptions"
                @update:value="updateRestartPolicy"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item
              label="最大重试次数"
              path="restartPolicy.maximumRetryCount"
              :show-label="formData.restartPolicy.name === 'on-failure'"
            >
              <n-input-number
                v-model:value="formData.restartPolicy.maximumRetryCount"
                :disabled="formData.restartPolicy.name !== 'on-failure'"
                :min="0"
                placeholder="0 表示无限制"
                style="width: 100%"
                @blur="updateRestartPolicy"
              />
            </n-form-item>
          </n-gi>
        </n-grid>
      </div>

      <n-divider />

      <!-- 资源限制 -->
      <div>
        <n-h3 prefix="bar">资源限制</n-h3>
        
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="内存限制 (MB)">
              <n-input-number
                v-model:value="memoryMB"
                :min="0"
                placeholder="0 表示不限制"
                style="width: 100%"
                @blur="updateMemory"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="CPU 份额">
              <n-input-number
                v-model:value="formData.cpuShares"
                :min="0"
                placeholder="默认 1024"
                style="width: 100%"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="CPU 配额">
              <n-input-number
                v-model:value="formData.cpuQuota"
                :min="0"
                placeholder="微秒"
                style="width: 100%"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="CPU 周期">
              <n-input-number
                v-model:value="formData.cpuPeriod"
                :min="0"
                placeholder="微秒"
                style="width: 100%"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="CPU 集 (CPUs)">
              <n-input
                v-model:value="formData.cpusetCpus"
                placeholder="例如: 0-3, 0,1"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="内存节点">
              <n-input
                v-model:value="formData.cpusetMems"
                placeholder="例如: 0-1"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="块 I/O 权重">
              <n-input-number
                v-model:value="formData.blkioWeight"
                :min="10"
                :max="1000"
                placeholder="10-1000"
                style="width: 100%"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="共享内存大小 (MB)">
              <n-input-number
                v-model:value="shmSizeMB"
                :min="0"
                placeholder="默认 64"
                style="width: 100%"
                @blur="updateShmSize"
              />
            </n-form-item>
          </n-gi>
        </n-grid>
      </div>

      <n-divider />

      <!-- 设备分配 -->
      <div>
        <n-h3 prefix="bar">设备分配</n-h3>
        <n-space vertical size="small">
          <div v-for="(device, index) in deviceList" :key="index" class="device-item">
            <n-grid :cols="24" :x-gap="8">
              <n-gi :span="8">
                <n-input
                  v-model:value="device.pathOnHost"
                  placeholder="主机设备路径"
                  size="small"
                  @blur="updateDevices"
                />
              </n-gi>
              <n-gi :span="8">
                <n-input
                  v-model:value="device.pathInContainer"
                  placeholder="容器设备路径"
                  size="small"
                  @blur="updateDevices"
                />
              </n-gi>
              <n-gi :span="5">
                <n-input
                  v-model:value="device.cgroupPermissions"
                  placeholder="权限 (rwm)"
                  size="small"
                  @blur="updateDevices"
                />
              </n-gi>
              <n-gi :span="2">
                <n-button
                  size="small"
                  tertiary
                  type="error"
                  @click="removeDevice(index)"
                  block
                >
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                </n-button>
              </n-gi>
            </n-grid>
          </div>
          <n-button dashed block @click="addDevice" size="small">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加设备映射
          </n-button>
        </n-space>

        <n-divider style="margin: 16px 0" />

        <n-h4>GPU 配置</n-h4>
        <n-space vertical size="small">
          <div v-for="(gpu, index) in gpuList" :key="index" class="gpu-item">
            <n-grid :cols="24" :x-gap="8">
              <n-gi :span="6">
                <n-input
                  v-model:value="gpu.driver"
                  placeholder="驱动 (nvidia)"
                  size="small"
                  @blur="updateGPUs"
                />
              </n-gi>
              <n-gi :span="4">
                <n-input-number
                  v-model:value="gpu.count"
                  placeholder="数量"
                  :min="-1"
                  size="small"
                  style="width: 100%"
                  @blur="updateGPUs"
                />
              </n-gi>
              <n-gi :span="11">
                <n-input
                  v-model:value="gpu.deviceIDsStr"
                  placeholder="设备 ID (逗号分隔)"
                  size="small"
                  @blur="updateGPUs"
                />
              </n-gi>
              <n-gi :span="2">
                <n-button
                  size="small"
                  tertiary
                  type="error"
                  @click="removeGPU(index)"
                  block
                >
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                </n-button>
              </n-gi>
            </n-grid>
          </div>
          <n-button dashed block @click="addGPU" size="small">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
            添加 GPU 请求
          </n-button>
        </n-space>
      </div>

      <n-divider />

      <!-- 能力配置 -->
      <div>
        <n-h3 prefix="bar">能力配置</n-h3>
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="添加能力 (CapAdd)">
              <n-select
                v-model:value="formData.capAdd"
                :options="capabilityOptions"
                multiple
                filterable
                tag
                placeholder="选择或输入能力"
                @update:value="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="移除能力 (CapDrop)">
              <n-select
                v-model:value="formData.capDrop"
                :options="capabilityOptions"
                multiple
                filterable
                tag
                placeholder="选择或输入能力"
                @update:value="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>
      </div>

      <n-divider />

      <!-- 其他高级选项 -->
      <div>
        <n-h3 prefix="bar">其他高级选项</n-h3>
        
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="PID 模式">
              <n-input
                v-model:value="formData.pidMode"
                placeholder="例如: host"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="IPC 模式">
              <n-input
                v-model:value="formData.ipcMode"
                placeholder="例如: host"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="UTS 模式">
              <n-input
                v-model:value="formData.utsMode"
                placeholder="例如: host"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="Cgroup">
              <n-input
                v-model:value="formData.cgroup"
                placeholder="Cgroup 路径"
                @blur="updateField"
              />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="Runtime">
          <n-input
            v-model:value="formData.runtime"
            placeholder="例如: nvidia"
            @blur="updateField"
          />
        </n-form-item>

        <n-form-item label="DNS 服务器">
          <n-dynamic-tags v-model:value="formData.dns" @update:value="updateField" />
        </n-form-item>

        <n-form-item label="DNS 搜索域">
          <n-dynamic-tags v-model:value="formData.dnsSearch" @update:value="updateField" />
        </n-form-item>

        <n-form-item label="DNS 选项">
          <n-dynamic-tags v-model:value="formData.dnsOptions" @update:value="updateField" />
        </n-form-item>

        <n-form-item label="Extra Hosts">
          <n-dynamic-tags v-model:value="formData.extraHosts" @update:value="updateField" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              格式: hostname:ip
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="安全选项">
          <n-dynamic-tags v-model:value="formData.securityOpt" @update:value="updateField" />
        </n-form-item>
      </div>

      <n-divider />

      <!-- 标签 -->
      <div>
        <n-h3 prefix="bar">标签</n-h3>
        <n-space vertical size="small">
          <div v-for="(label, index) in labelList" :key="index" class="label-item">
            <n-grid :cols="12" :x-gap="8">
              <n-gi :span="5">
                <n-input
                  v-model:value="label.key"
                  placeholder="键"
                  size="small"
                  @blur="updateLabels"
                />
              </n-gi>
              <n-gi :span="1" class="flex items-center justify-center">
                <span>=</span>
              </n-gi>
              <n-gi :span="5">
                <n-input
                  v-model:value="label.value"
                  placeholder="值"
                  size="small"
                  @blur="updateLabels"
                />
              </n-gi>
              <n-gi :span="1">
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
      </div>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import { ref, watch } from 'vue'
import type { ContainerCreateRequest, DeviceMapping, DeviceRequest, RestartPolicyType } from '@/common/types'

interface Props {
  modelValue: Partial<ContainerCreateRequest>
}

interface Emits {
  (e: 'update:modelValue', value: Partial<ContainerCreateRequest>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const formData = ref({
  restartPolicy: {
    name: (props.modelValue.restartPolicy?.name || 'no') as RestartPolicyType,
    maximumRetryCount: props.modelValue.restartPolicy?.maximumRetryCount || 0,
  },
  cpuShares: props.modelValue.cpuShares || 0,
  cpuQuota: props.modelValue.cpuQuota || 0,
  cpuPeriod: props.modelValue.cpuPeriod || 0,
  cpusetCpus: props.modelValue.cpusetCpus || '',
  cpusetMems: props.modelValue.cpusetMems || '',
  blkioWeight: props.modelValue.blkioWeight || 0,
  pidMode: props.modelValue.pidMode || '',
  ipcMode: props.modelValue.ipcMode || '',
  utsMode: props.modelValue.utsMode || '',
  cgroup: props.modelValue.cgroup || '',
  runtime: props.modelValue.runtime || '',
  dns: props.modelValue.dns || [],
  dnsSearch: props.modelValue.dnsSearch || [],
  dnsOptions: props.modelValue.dnsOptions || [],
  extraHosts: props.modelValue.extraHosts || [],
  securityOpt: props.modelValue.securityOpt || [],
  capAdd: props.modelValue.capAdd || [],
  capDrop: props.modelValue.capDrop || [],
})

const memoryMB = ref(props.modelValue.memory ? props.modelValue.memory / (1024 * 1024) : 0)
const shmSizeMB = ref(props.modelValue.shmSize ? props.modelValue.shmSize / (1024 * 1024) : 0)

const deviceList = ref<DeviceMapping[]>(props.modelValue.devices || [])

interface GPUItem {
  driver: string
  count: number
  deviceIDsStr: string
}

const gpuList = ref<GPUItem[]>(
  props.modelValue.deviceRequests?.map((req) => ({
    driver: req.driver,
    count: req.count,
    deviceIDsStr: req.deviceIDs.join(','),
  })) || [],
)

interface LabelItem {
  key: string
  value: string
}

const labelList = ref<LabelItem[]>(
  props.modelValue.labels
    ? Object.entries(props.modelValue.labels).map(([key, value]) => ({ key, value }))
    : [],
)

const restartPolicyOptions = [
  { label: '不适用 (no)', value: 'no' },
  { label: '总是显示 (always)', value: 'always' },
  { label: '除非停止 (unless-stopped)', value: 'unless-stopped' },
  { label: '失败时 (on-failure)', value: 'on-failure' },
]

const capabilityOptions = [
  { label: 'SYS_ADMIN', value: 'SYS_ADMIN' },
  { label: 'NET_ADMIN', value: 'NET_ADMIN' },
  { label: 'SYS_TIME', value: 'SYS_TIME' },
  { label: 'SYS_MODULE', value: 'SYS_MODULE' },
  { label: 'SYS_RAWIO', value: 'SYS_RAWIO' },
  { label: 'SYS_PTRACE', value: 'SYS_PTRACE' },
  { label: 'NET_RAW', value: 'NET_RAW' },
  { label: 'IPC_LOCK', value: 'IPC_LOCK' },
]

const updateField = () => {
  const data: Partial<ContainerCreateRequest> = {
    ...props.modelValue,
    ...formData.value,
  }
  
  // 清理空值
  if (!data.cpuShares) delete data.cpuShares
  if (!data.cpuQuota) delete data.cpuQuota
  if (!data.cpuPeriod) delete data.cpuPeriod
  if (!data.cpusetCpus) delete data.cpusetCpus
  if (!data.cpusetMems) delete data.cpusetMems
  if (!data.blkioWeight) delete data.blkioWeight
  if (!data.pidMode) delete data.pidMode
  if (!data.ipcMode) delete data.ipcMode
  if (!data.utsMode) delete data.utsMode
  if (!data.cgroup) delete data.cgroup
  if (!data.runtime) delete data.runtime
  if (!data.dns || data.dns.length === 0) delete data.dns
  if (!data.dnsSearch || data.dnsSearch.length === 0) delete data.dnsSearch
  if (!data.dnsOptions || data.dnsOptions.length === 0) delete data.dnsOptions
  if (!data.extraHosts || data.extraHosts.length === 0) delete data.extraHosts
  if (!data.securityOpt || data.securityOpt.length === 0) delete data.securityOpt
  if (!data.capAdd || data.capAdd.length === 0) delete data.capAdd
  if (!data.capDrop || data.capDrop.length === 0) delete data.capDrop

  emit('update:modelValue', data)
}

const updateRestartPolicy = () => {
  updateField()
}

const updateMemory = () => {
  const memory = memoryMB.value > 0 ? memoryMB.value * 1024 * 1024 : 0
  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
    memory,
  })
}

const updateShmSize = () => {
  const shmSize = shmSizeMB.value > 0 ? shmSizeMB.value * 1024 * 1024 : 0
  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
    shmSize,
  })
}

const addDevice = () => {
  deviceList.value.push({ pathOnHost: '', pathInContainer: '', cgroupPermissions: 'rwm' })
}

const removeDevice = (index: number) => {
  deviceList.value.splice(index, 1)
  updateDevices()
}

const updateDevices = () => {
  const devices = deviceList.value.filter(
    (d) => d.pathOnHost.trim() && d.pathInContainer.trim(),
  )
  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
    devices: devices.length > 0 ? devices : undefined,
  })
}

const addGPU = () => {
  gpuList.value.push({ driver: 'nvidia', count: -1, deviceIDsStr: '' })
}

const removeGPU = (index: number) => {
  gpuList.value.splice(index, 1)
  updateGPUs()
}

const updateGPUs = () => {
  const deviceRequests: DeviceRequest[] = gpuList.value
    .filter((g) => g.driver.trim())
    .map((g) => ({
      driver: g.driver,
      count: g.count,
      deviceIDs: g.deviceIDsStr ? g.deviceIDsStr.split(',').map((id) => id.trim()) : [],
      capabilities: [['gpu']],
      options: {},
    }))

  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
    deviceRequests: deviceRequests.length > 0 ? deviceRequests : undefined,
  })
}

const addLabel = () => {
  labelList.value.push({ key: '', value: '' })
}

const removeLabel = (index: number) => {
  labelList.value.splice(index, 1)
  updateLabels()
}

const updateLabels = () => {
  const labels: Record<string, string> = {}
  labelList.value.forEach((label) => {
    if (label.key.trim()) {
      labels[label.key] = label.value
    }
  })

  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
    labels: Object.keys(labels).length > 0 ? labels : undefined,
  })
}

watch(
  () => props.modelValue,
  (newVal) => {
    formData.value = {
      restartPolicy: {
        name: (newVal.restartPolicy?.name || 'no') as RestartPolicyType,
        maximumRetryCount: newVal.restartPolicy?.maximumRetryCount || 0,
      },
      cpuShares: newVal.cpuShares || 0,
      cpuQuota: newVal.cpuQuota || 0,
      cpuPeriod: newVal.cpuPeriod || 0,
      cpusetCpus: newVal.cpusetCpus || '',
      cpusetMems: newVal.cpusetMems || '',
      blkioWeight: newVal.blkioWeight || 0,
      pidMode: newVal.pidMode || '',
      ipcMode: newVal.ipcMode || '',
      utsMode: newVal.utsMode || '',
      cgroup: newVal.cgroup || '',
      runtime: newVal.runtime || '',
      dns: newVal.dns || [],
      dnsSearch: newVal.dnsSearch || [],
      dnsOptions: newVal.dnsOptions || [],
      extraHosts: newVal.extraHosts || [],
      securityOpt: newVal.securityOpt || [],
      capAdd: newVal.capAdd || [],
      capDrop: newVal.capDrop || [],
    }
    memoryMB.value = newVal.memory ? newVal.memory / (1024 * 1024) : 0
    shmSizeMB.value = newVal.shmSize ? newVal.shmSize / (1024 * 1024) : 0
    deviceList.value = newVal.devices || []
    gpuList.value =
      newVal.deviceRequests?.map((req) => ({
        driver: req.driver,
        count: req.count,
        deviceIDsStr: req.deviceIDs.join(','),
      })) || []
    labelList.value = newVal.labels
      ? Object.entries(newVal.labels).map(([key, value]) => ({ key, value }))
      : []
  },
  { deep: true },
)
</script>

<style scoped>
.advanced-tab {
  padding: 0;
}

.device-item,
.gpu-item,
.label-item {
  margin-bottom: 8px;
}
</style>

