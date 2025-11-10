<template>
  <div class="container-create-view">
    <n-tabs
      v-model:value="activeTab"
      type="line"
      pane-class="container-create-pane"
      :pane-style="{ height: tabHeight }"
      animated
    >
      <n-tab-pane name="basic" tab="基础">
        <BasicTab v-model="formData" />
      </n-tab-pane>
      <n-tab-pane name="env" tab="环境变量">
        <EnvTab v-model="formData.env" />
      </n-tab-pane>
      <n-tab-pane name="port" tab="端口">
        <PortTab v-model="formData" />
      </n-tab-pane>
      <n-tab-pane name="volume" tab="数据挂载">
        <VolumeTab v-model:binds="formData.binds" />
      </n-tab-pane>
      <n-tab-pane name="network-security" tab="网络与安全">
        <NetworkSecurityTab v-model="formData" />
      </n-tab-pane>
      <n-tab-pane name="advanced" tab="高级">
        <AdvancedTab v-model="formData" />
      </n-tab-pane>
    </n-tabs>

    <Teleport to="#header" defer>
      <div class="welcome-card">
        <n-h2 class="m-0 text-lg"> 创建容器 </n-h2>
      </div>
    </Teleport>

    <Teleport to="#footer" defer>
      <n-space justify="end" class="pr-2">
        <n-button @click="handleCancel">取消</n-button>
        <n-button type="primary" :loading="creating" @click="handleCreate"> 创建容器 </n-button>
      </n-space>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { containerApi } from '@/common/api'
import type { ContainerCreateRequest } from '@/common/types'
import BasicTab from './BasicTab.vue'
import EnvTab from './EnvTab.vue'
import PortTab from './PortTab.vue'
import VolumeTab from './VolumeTab.vue'
import NetworkSecurityTab from './NetworkSecurityTab.vue'
import AdvancedTab from './AdvancedTab.vue'
import { useMessage } from 'naive-ui'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSettingStore } from '@/store/setting'

const router = useRouter()
const message = useMessage()
const settingStore = useSettingStore()
const activeTab = ref('basic')
const creating = ref(false)
const tabTitleHeight = computed(() => {
  return 42
})

const bottomHeight = computed(() => {
  return 51 + settingStore.contentSafeBottom
})

const tabHeight = computed(() => {
  return `calc(100vh - ${settingStore.contentSafeTop + tabTitleHeight.value + bottomHeight.value}px)`
})
const formData = ref<Partial<ContainerCreateRequest>>({
  name: '',
  image: '',
  cmd: [],
  entrypoint: [],
  workingDir: '',
  env: [],
  exposedPorts: {},
  labels: {},
  hostname: '',
  domainname: '',
  user: '',
  attachStdin: false,
  attachStdout: true,
  attachStderr: true,
  tty: false,
  openStdin: false,
  stdinOnce: false,
  binds: [],
  portBindings: {},
  restartPolicy: {
    name: 'no',
    maximumRetryCount: 0,
  },
  autoRemove: false,
  networkMode: 'bridge',
  privileged: false,
  publishAllPorts: false,
  readonlyRootfs: false,
  dns: [],
  dnsSearch: [],
  dnsOptions: [],
  extraHosts: [],
  capAdd: [],
  capDrop: [],
  securityOpt: [],
  cpuShares: 0,
  memory: 0,
  cpuQuota: 0,
  cpuPeriod: 0,
  cpusetCpus: '',
  cpusetMems: '',
  blkioWeight: 0,
  shmSize: 0,
  pidMode: '',
  ipcMode: '',
  utsMode: '',
  cgroup: '',
  runtime: '',
  devices: [],
  deviceRequests: [],
})

const handleCancel = () => {
  router.push('/containers')
}

const handleCreate = async () => {
  // 验证必填字段
  if (!formData.value.image) {
    message.error('请输入镜像名称')
    activeTab.value = 'basic'
    return
  }

  try {
    creating.value = true

    // 构建请求数据,移除空值
    const requestData: ContainerCreateRequest = {
      name: formData.value.name || '',
      image: formData.value.image,
    }

    // 添加可选字段
    if (formData.value.cmd && formData.value.cmd.length > 0) {
      requestData.cmd = formData.value.cmd
    }
    if (formData.value.entrypoint && formData.value.entrypoint.length > 0) {
      requestData.entrypoint = formData.value.entrypoint
    }
    if (formData.value.workingDir) {
      requestData.workingDir = formData.value.workingDir
    }
    if (formData.value.env && formData.value.env.length > 0) {
      requestData.env = formData.value.env
    }
    if (formData.value.exposedPorts && Object.keys(formData.value.exposedPorts).length > 0) {
      requestData.exposedPorts = formData.value.exposedPorts
    }
    if (formData.value.labels && Object.keys(formData.value.labels).length > 0) {
      requestData.labels = formData.value.labels
    }
    if (formData.value.hostname) {
      requestData.hostname = formData.value.hostname
    }
    if (formData.value.domainname) {
      requestData.domainname = formData.value.domainname
    }
    if (formData.value.user) {
      requestData.user = formData.value.user
    }

    // I/O 设置
    requestData.attachStdin = formData.value.attachStdin || false
    requestData.attachStdout = formData.value.attachStdout !== false
    requestData.attachStderr = formData.value.attachStderr !== false
    requestData.tty = formData.value.tty || false
    requestData.openStdin = formData.value.openStdin || false
    requestData.stdinOnce = formData.value.stdinOnce || false

    // 数据卷
    if (formData.value.binds && formData.value.binds.length > 0) {
      requestData.binds = formData.value.binds
    }

    // 端口
    if (formData.value.portBindings && Object.keys(formData.value.portBindings).length > 0) {
      requestData.portBindings = formData.value.portBindings
    }

    // 重启策略
    if (formData.value.restartPolicy) {
      requestData.restartPolicy = formData.value.restartPolicy
    }

    // 网络和安全
    requestData.autoRemove = formData.value.autoRemove || false
    requestData.networkMode = formData.value.networkMode || 'bridge'
    requestData.privileged = formData.value.privileged || false
    requestData.publishAllPorts = formData.value.publishAllPorts || false
    requestData.readonlyRootfs = formData.value.readonlyRootfs || false

    // DNS 配置
    if (formData.value.dns && formData.value.dns.length > 0) {
      requestData.dns = formData.value.dns
    }
    if (formData.value.dnsSearch && formData.value.dnsSearch.length > 0) {
      requestData.dnsSearch = formData.value.dnsSearch
    }
    if (formData.value.dnsOptions && formData.value.dnsOptions.length > 0) {
      requestData.dnsOptions = formData.value.dnsOptions
    }
    if (formData.value.extraHosts && formData.value.extraHosts.length > 0) {
      requestData.extraHosts = formData.value.extraHosts
    }

    // 能力
    if (formData.value.capAdd && formData.value.capAdd.length > 0) {
      requestData.capAdd = formData.value.capAdd
    }
    if (formData.value.capDrop && formData.value.capDrop.length > 0) {
      requestData.capDrop = formData.value.capDrop
    }
    if (formData.value.securityOpt && formData.value.securityOpt.length > 0) {
      requestData.securityOpt = formData.value.securityOpt
    }

    // 资源限制
    if (formData.value.cpuShares && formData.value.cpuShares > 0) {
      requestData.cpuShares = formData.value.cpuShares
    }
    if (formData.value.memory && formData.value.memory > 0) {
      requestData.memory = formData.value.memory
    }
    if (formData.value.cpuQuota && formData.value.cpuQuota > 0) {
      requestData.cpuQuota = formData.value.cpuQuota
    }
    if (formData.value.cpuPeriod && formData.value.cpuPeriod > 0) {
      requestData.cpuPeriod = formData.value.cpuPeriod
    }
    if (formData.value.cpusetCpus) {
      requestData.cpusetCpus = formData.value.cpusetCpus
    }
    if (formData.value.cpusetMems) {
      requestData.cpusetMems = formData.value.cpusetMems
    }
    if (formData.value.blkioWeight && formData.value.blkioWeight > 0) {
      requestData.blkioWeight = formData.value.blkioWeight
    }
    if (formData.value.shmSize && formData.value.shmSize > 0) {
      requestData.shmSize = formData.value.shmSize
    }

    // 其他高级选项
    if (formData.value.pidMode) {
      requestData.pidMode = formData.value.pidMode
    }
    if (formData.value.ipcMode) {
      requestData.ipcMode = formData.value.ipcMode
    }
    if (formData.value.utsMode) {
      requestData.utsMode = formData.value.utsMode
    }
    if (formData.value.cgroup) {
      requestData.cgroup = formData.value.cgroup
    }
    if (formData.value.runtime) {
      requestData.runtime = formData.value.runtime
    }

    // 设备
    if (formData.value.devices && formData.value.devices.length > 0) {
      requestData.devices = formData.value.devices
    }
    if (formData.value.deviceRequests && formData.value.deviceRequests.length > 0) {
      requestData.deviceRequests = formData.value.deviceRequests
    }

    // 调用 API 创建容器
    const response = await containerApi.createContainer(requestData)
    console.debug(response)
    message.success('容器创建成功')

    // 跳转到容器详情页
    router.push(`/containers`)
  } catch (error: any) {
    console.error('创建容器失败:', error)
    message.error(`创建容器失败: ${error.message || '未知错误'}`)
  } finally {
    creating.value = false
  }
}
</script>

<style scoped lang="less">
@import '@/styles/mix.less';
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.container-create-view {
  padding-inline: 8px;
  box-sizing: border-box;
  max-width: 1200px;
  margin: 0 auto;
  .container-create-pane {
    overflow: auto;
    .scrollbar();
    .tab-content {
      height: 100%;
    }
  }
}
</style>
