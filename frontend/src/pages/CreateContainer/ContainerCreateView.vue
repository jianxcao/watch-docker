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
        <BasicTab v-model="basicForm" />
      </n-tab-pane>
      <n-tab-pane name="env" tab="环境变量">
        <EnvTab v-model="envForm" />
      </n-tab-pane>
      <n-tab-pane name="port" tab="端口">
        <PortTab v-model="portForm" />
      </n-tab-pane>
      <n-tab-pane name="volume" tab="数据挂载">
        <VolumeTab v-model="volumeForm" />
      </n-tab-pane>
      <n-tab-pane name="network" tab="网络">
        <NetworkTab v-model="networkForm" />
      </n-tab-pane>
      <n-tab-pane name="runtime-resource" tab="运行&资源">
        <RuntimeResourceTab v-model="runtimeResourceForm" />
      </n-tab-pane>
      <n-tab-pane name="label" tab="标签">
        <LabelTab v-model="labelForm" />
      </n-tab-pane>
      <n-tab-pane name="advanced" tab="高级">
        <AdvancedTab v-model="advancedForm" />
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
import { showErrorWithNewlines } from '@/common/utils'
import BasicTab from './BasicTab.vue'
import EnvTab from './EnvTab.vue'
import PortTab from './PortTab.vue'
import VolumeTab from './VolumeTab.vue'
import NetworkTab from './NetworkTab.vue'
import RuntimeResourceTab from './RuntimeResourceTab.vue'
import LabelTab from './LabelTab.vue'
import AdvancedTab from './AdvancedTab.vue'
import type {
  BasicFormValue,
  EnvFormValue,
  PortFormValue,
  VolumeFormValue,
  NetworkFormValue,
  RuntimeResourceFormValue,
  LabelFormValue,
  AdvancedFormValue,
} from './types'
import { mergeFormData } from './transformer'
import { useMessage } from 'naive-ui'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSettingStore } from '@/store/setting'
import { useContainerStore } from '@/store/container'

const router = useRouter()
const message = useMessage()
const settingStore = useSettingStore()
const containerStore = useContainerStore()
const activeTab = ref('basic')
const creating = ref(false)

const tabTitleHeight = computed(() => 42)
const bottomHeight = computed(() => 51 + settingStore.contentSafeBottom)
const tabHeight = computed(() => {
  return `calc(100vh - ${settingStore.contentSafeTop + tabTitleHeight.value + bottomHeight.value}px)`
})

// 各个 Tab 的表单数据
const basicForm = ref<BasicFormValue>({
  name: '',
  image: '',
  cmdString: '',
  entrypointString: '',
  workingDir: '',
  user: '',
  hostname: '',
  domainname: '',
  tty: false,
  openStdin: false,
  stdinOnce: false,
})

const envForm = ref<EnvFormValue>({
  env: [],
  envList: [],
  envText: '',
})

const portForm = ref<PortFormValue>({
  portList: [],
  portBindings: {},
  publishAllPorts: false,
  exposedPorts: {},
})

const volumeForm = ref<VolumeFormValue>({
  binds: [],
  volumeList: [],
  volumeText: '',
})

const networkForm = ref<NetworkFormValue>({
  configMode: 'default',
  publishAllPorts: false,
  dns: [],
  dnsSearch: [],
  dnsOptions: [],
  extraHosts: [],
  customNetworks: [],
})

const runtimeResourceForm = ref<RuntimeResourceFormValue>({
  privileged: false,
  readonlyRootfs: false,
  autoRemove: false,
  restartPolicyName: 'unless-stopped',
  restartPolicyMaxRetry: 0,
  memoryMB: 0,
  memoryReservationMB: 0,
  cpusetCpus: '',
  shmSizeMB: 0,
})

const labelForm = ref<LabelFormValue>({
  labelList: [],
  labels: {},
})

const advancedForm = ref<AdvancedFormValue>({
  capAdd: [],
  capDrop: [],
  pidMode: '',
  ipcMode: '',
  utsMode: '',
  cgroup: '',
  runtime: '',
  securityOpt: [],
})

const handleCancel = () => {
  router.push('/containers')
}

const handleCreate = async () => {
  // 校验基础表单 - 直接校验数据而不依赖表单组件
  if (!basicForm.value.image || !basicForm.value.image.trim()) {
    activeTab.value = 'basic'
    message.error('请输入镜像名称')
    return
  }

  try {
    creating.value = true

    // 合并所有表单数据为 ContainerCreateRequest
    const requestData = mergeFormData(
      basicForm.value,
      envForm.value,
      portForm.value,
      volumeForm.value,
      networkForm.value,
      runtimeResourceForm.value,
      labelForm.value,
      advancedForm.value,
    )
    console.debug('api', containerApi)
    console.debug('submitData', requestData)
    // 调用 API 创建容器
    const response = await containerApi.createContainer(requestData)
    if (response.code !== 0) {
      showErrorWithNewlines(message, response.msg || '容器创建失败')
      return
    }
    message.success(response.msg || '容器创建成功')
    await containerStore.fetchContainers(true, false)
    router.push(`/containers`)
  } catch (error: any) {
    console.error('创建容器失败:', error)
    showErrorWithNewlines(message, `创建容器失败: ${error.message || '未知错误'}`)
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
  box-sizing: border-box;
  max-width: 1200px;
  margin: 0 auto;
  .container-create-pane {
    overflow: auto;
    padding-inline: 8px;
    .scrollbar();
    .tab-content {
      height: 100%;
    }
  }
}
</style>
