<template>
  <div class="compose-detail-page">
    <!-- 页面头部 -->
    <Teleport to="#header" defer>
      <div class="page-header">
        <div class="flex items-center gap-3">
          <n-button text circle @click="handleBack">
            <template #icon>
              <n-icon size="20">
                <ArrowBackOutline />
              </n-icon>
            </template>
          </n-button>
          <n-h2 class="m-0 text-lg">{{ projectName }}</n-h2>
          <div
            v-if="currentProject"
            class="status-badge"
            :class="'status-' + currentProject.status"
          >
            <span class="status-dot"></span>
            <span class="status-text">{{ getStatusText(currentProject.status) }}</span>
          </div>
        </div>
        <n-dropdown :options="dropdownOptions" @select="handleMenuSelect" trigger="click">
          <n-button text circle>
            <template #icon>
              <n-icon size="18">
                <EllipsisHorizontal />
              </n-icon>
            </template>
          </n-button>
        </n-dropdown>
      </div>
    </Teleport>

    <!-- 加载状态 -->
    <n-spin :show="loading" class="h-[300px] flex items-center justify-center" v-if="loading">
    </n-spin>
    <template v-else>
      <div v-if="!currentProject" class="empty-state">
        <n-empty description="项目不存在">
          <template #extra>
            <n-button @click="handleBack">返回列表</n-button>
          </template>
        </n-empty>
      </div>

      <!-- Tabs 内容 -->
      <n-tabs
        v-else
        type="line"
        animated
        pane-class="compose-detail-pane"
        :pane-style="{ height: tabHeight }"
        :default-value="activeTab"
        @update:value="handleTabChange"
      >
        <!-- Tab 1: 容器列表 -->
        <n-tab-pane name="containers" tab="容器">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <LayersOutline />
              </n-icon>
              <span>容器</span>
              <n-badge
                v-if="projectContainers.length > 0"
                :value="projectContainers.length"
                :max="99"
                show-zero
                type="info"
              />
            </div>
          </template>

          <div class="tab-content">
            <div v-if="projectContainers.length === 0" class="empty-container">
              <n-empty description="该项目下没有容器" />
            </div>
            <div
              v-else
              class="containers-grid"
              :class="{
                'grid-cols-1': isMobile,
                'grid-cols-2': isTablet || isLaptop,
                'grid-cols-3': isDesktop,
                'grid-cols-4': isDesktopLarge,
              }"
            >
              <ContainerCard
                v-for="container in projectContainers"
                :key="container.id"
                :container="container"
                @start="handleStartContainer(container)"
                @stop="handleStopContainer(container)"
                @restart="handleRestartContainer(container)"
                @update="handleUpdateContainer(container)"
                @delete="handleDeleteContainer(container)"
                @export="handleExportContainer(container)"
                @logs="handleViewContainerLogs(container)"
                @detail="handleViewContainerDetail(container)"
              />
            </div>
          </div>
        </n-tab-pane>

        <!-- Tab 2: YAML 编辑 -->
        <n-tab-pane name="yaml" tab="配置">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <DocumentTextOutline />
              </n-icon>
              <span>配置</span>
            </div>
          </template>

          <div class="tab-content yaml-tab">
            <n-spin :show="yamlLoading" class="h-full" v-if="yamlLoading"> </n-spin>
            <div v-else class="yaml-card">
              <div ref="yamlEditorContainerRef" :style="{ height: 'calc(100% - 42px)' }">
                <YamlEditor
                  v-model="yamlContent"
                  placeholder="请输入 docker-compose.yml 配置内容"
                  :min-height="yamlEditorMinHeight"
                  :max-height="yamlEditorMinHeight"
                  @change="handleYamlChange"
                />
              </div>
              <div class="flex justify-end gap-2 items-center px-2 h-[42px]">
                <n-text depth="3" class="text-xs" :style="{ color: theme.errorColor }">
                  {{ yamlValidationMessage }}
                </n-text>
                <n-button
                  type="primary"
                  size="small"
                  :disabled="!isYamlValid || deployLoading"
                  :loading="deployLoading"
                  @click="handleDeploy"
                >
                  <template #icon>
                    <n-icon>
                      <RefreshOutline />
                    </n-icon>
                  </template>
                  {{ isYamlModified ? '保存并重新部署' : '重新部署' }}
                </n-button>
              </div>
            </div>
          </div>
        </n-tab-pane>

        <!-- Tab 3: 日志 -->
        <n-tab-pane name="logs" tab="日志">
          <template #tab>
            <div class="flex items-center gap-2">
              <n-icon size="18">
                <DocumentIcon />
              </n-icon>
              <span>日志</span>
            </div>
          </template>

          <div class="tab-content logs-tab">
            <LogsStreamView
              class="logs-stream-view"
              v-if="activeTab === 'logs' && currentProject"
              :socket-url="logsSocketUrl"
              height="100%"
            />
          </div>
        </n-tab-pane>
      </n-tabs>
    </template>

    <!-- 容器日志弹窗 -->
    <ContainerLogsModal v-model:show="showContainerLogsModal" :container="currentContainer" />
    <!-- 重新部署进度组件 -->
    <ComposeCreateProgress
      ref="createProgressRef"
      :show="showProgress"
      :project-name="projectName"
      :yaml-content="yamlContent"
      :force="true"
      @success="handleDeploySuccess"
      @error="handleDeployError"
      @complete="handleDeployComplete"
    />
    <!-- Pull 日志弹窗 -->
    <ComposePullLogsModal v-model:show="showPullLogs" :project="currentProject || null" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage, useThemeVars, type DropdownOption } from 'naive-ui'
import {
  ArrowBackOutline,
  LayersOutline,
  DocumentTextOutline,
  RefreshOutline,
  ReloadOutline,
  TrashOutline,
  PlayOutline,
  StopOutline,
  EllipsisHorizontal,
  CloudDownloadOutline,
} from '@vicons/ionicons5'
import DocumentIcon from '@/assets/svg/log.svg?component'
import { useComposeStore } from '@/store/compose'
import { useContainerStore } from '@/store/container'
import { useSettingStore } from '@/store/setting'
import { useResponsive } from '@/hooks/useResponsive'
import { useContainer } from '@/hooks/useContainer'
import type { ComposeProject, ContainerStatus } from '@/common/types'
import { renderIcon, validateComposeYaml } from '@/common/utils'
import ContainerCard from '@/components/ContainerCard.vue'
import ContainerLogsModal from '@/components/ContainerLogsModal.vue'
import LogsStreamView from '@/components/LogsStreamView.vue'
import YamlEditor from '@/components/YamlEditor/index.vue'
import ComposeCreateProgress from '@/components/ComposeCreateProgress.vue'
import ComposePullLogsModal from '@/components/ComposePullLogsModal.vue'
import { useCompose } from '@/hooks/useCompose'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const theme = useThemeVars()
const composeStore = useComposeStore()
const containerStore = useContainerStore()
const settingStore = useSettingStore()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const { handleStart, handleStop, handleRestart, handleUpdate, handleDelete, handleExport } = useContainer()
const {
  handleStart: handleComposeStart,
  handleStop: handleComposeStop,
  handleDelete: handleComposeDelete,
  handleRestart: handleComposeRestart,
} = useCompose()

// 状态
const projectName = ref(route.params.projectName as string)
const activeTab = ref('containers')
const loading = ref(false)
const yamlLoading = ref(false)
const deployLoading = ref(false)
const yamlContent = ref('')
const originalYamlContent = ref('')
const yamlValidationMessage = ref('')
const isYamlValid = ref(true)
const showContainerLogsModal = ref(false)
const currentContainer = ref<ContainerStatus | null>(null)
const yamlEditorContainerRef = ref<HTMLElement | null>(null)
const createProgressRef = ref<InstanceType<typeof ComposeCreateProgress>>()
const showProgress = ref(false)
const showPullLogs = ref(false)

// 下拉菜单选项
const dropdownOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = []
  const status = currentProject.value?.status
  // 根据项目状态显示不同的操作选项
  if (status === 'exited' || status === 'partial') {
    options.push({
      label: '启动',
      key: 'start',
      icon: renderIcon(PlayOutline),
    })
  }

  if (status === 'running' || status === 'partial') {
    options.push(
      {
        label: '停止',
        key: 'stop',
        icon: renderIcon(StopOutline),
      },
      {
        label: '重启',
        key: 'restart',
        icon: renderIcon(ReloadOutline),
      },
    )
  }

  options.push(
    {
      label: '重新部署',
      key: 'deploy',
      icon: renderIcon(RefreshOutline),
    },
    {
      type: 'divider',
      key: 'divider1',
    },
    {
      label: '拉取镜像',
      key: 'pull',
      icon: renderIcon(CloudDownloadOutline),
    },
  )
  if (status !== 'unknown') {
    let label = '删除应用'
    if (status === 'draft' || status === 'created_stack') {
      label = '删除配置'
    }
    options.push(
      {
        type: 'divider',
        key: 'divider2',
      },
      {
        label: label,
        key: 'delete',
        icon: renderIcon(TrashOutline),
        props: {
          style: `color: ${theme.value.errorColor}`,
        },
      },
    )
  }
  return options
})

const handleMenuSelect = async (key: string) => {
  if (!currentProject.value) {
    return
  }
  switch (key) {
    case 'deploy':
      handleDeploy()
      break
    case 'start':
      await handleComposeStart(currentProject.value)
      await containerStore.fetchContainers(true, false)
      break
    case 'stop':
      await handleComposeStop(currentProject.value)
      await containerStore.fetchContainers(true, false)
      break
    case 'restart':
      await handleComposeRestart(currentProject.value)
      await containerStore.fetchContainers(true, false)
      break
    case 'delete':
      handleComposeDelete(currentProject.value).then(() => {
        handleBack()
      })
      break
    case 'pull':
      showPullLogs.value = true
      break
  }
}
// 计算属性
const currentProject = computed<ComposeProject | undefined>(() => {
  return composeStore.projects.find((p) => p.name === projectName.value)
})

const projectContainers = computed(() => {
  return containerStore.getProjectContainers(projectName.value).value
})

const isYamlModified = computed(() => {
  return yamlContent.value !== originalYamlContent.value
})

const tabTitleHeight = computed(() => {
  return 42
})
const yamlEditorOptHeight = computed(() => {
  return 42
})

const tabHeight = computed(() => {
  return `calc(100vh - ${settingStore.contentSafeTop + tabTitleHeight.value + settingStore.contentSafeBottom}px)`
})

const yamlEditorMinHeight = computed(() => {
  return `${document.documentElement.clientHeight - settingStore.contentSafeTop - tabTitleHeight.value - yamlEditorOptHeight.value - settingStore.contentSafeBottom}px`
})

const logsSocketUrl = computed(() => {
  if (!currentProject.value) {
    return undefined
  }
  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1/compose/logs/${currentProject.value.name}/ws?token=${token}&composeFile=${encodeURIComponent(currentProject.value.composeFile)}&projectName=${encodeURIComponent(currentProject.value.name)}`
})

// 获取状态文本
const getStatusText = (status: string) => {
  const statusMap: Record<string, string> = {
    running: '运行中',
    partial: '部分运行',
    exited: '已停止',
    paused: '暂停',
    draft: '草稿',
    created_stack: '创建中',
    unknown: '未知',
  }
  return statusMap[status] || '未知'
}

// 加载项目数据
const loadProjectData = async () => {
  try {
    // 加载项目列表
    await composeStore.fetchProjects(true)

    // 加载容器列表
    await containerStore.fetchContainers(true, false)

    // 检查项目是否存在
    if (!currentProject.value) {
      message.error('项目不存在')
      return
    }
  } catch (error) {
    console.error('加载项目数据失败:', error)
    message.error('加载项目数据失败')
  }
}

const Init = async () => {
  loading.value = true
  try {
    await loadProjectData()
  } catch (error) {
    console.error('初始化失败:', error)
  } finally {
    loading.value = false
  }
}

// 加载 YAML 内容
const loadYamlContent = async () => {
  if (!currentProject.value) {
    return
  }

  yamlLoading.value = true
  try {
    const content = await composeStore.getProjectYaml(
      currentProject.value.name,
      currentProject.value.composeFile,
    )
    yamlContent.value = content
    originalYamlContent.value = content
    handleYamlChange()
  } catch (error) {
    console.error('加载 YAML 内容失败:', error)
  } finally {
    yamlLoading.value = false
  }
}

// YAML 变化时验证
const handleYamlChange = () => {
  const result = validateComposeYaml(yamlContent.value)
  isYamlValid.value = result.isValid
  yamlValidationMessage.value = result.errorMessage
}

// 重新部署
const handleDeploy = () => {
  const content = isYamlModified.value
    ? '保存 YAML 配置后将重新创建并启动所有服务，这可能会导致服务短暂中断。是否继续？'
    : '将重新创建并启动所有服务，这可能会导致服务短暂中断。是否继续？'

  dialog.warning({
    title: '确认重新部署',
    content,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => {
      deployLoading.value = true
      showProgress.value = true
      createProgressRef.value?.start()
    },
  })
}

// 部署成功
const handleDeploySuccess = () => {
  const msg = isYamlModified.value ? '保存并重新部署成功' : '重新部署成功'
  message.success(msg)
  originalYamlContent.value = yamlContent.value
}

// 部署失败
const handleDeployError = (errorMessage: string) => {
  const prefix = isYamlModified.value ? '保存并重新部署' : '重新部署'
  message.error(`${prefix}失败: ${errorMessage}`)
  deployLoading.value = false
}

// 部署完成
const handleDeployComplete = () => {
  deployLoading.value = false
  // 延迟刷新数据
  setTimeout(() => {
    loadProjectData()
  }, 1000)
}

// Tab 切换
const handleTabChange = (value: string) => {
  activeTab.value = value

  // 切换到 YAML tab 时加载内容
  if (value === 'yaml' && !yamlContent.value) {
    loadYamlContent()
  }
}

// 容器操作
const handleStartContainer = async (container: ContainerStatus) => {
  await handleStart(container)
  await Promise.all([containerStore.fetchContainers(true, false), loadProjectData()])
}

const handleStopContainer = async (container: ContainerStatus) => {
  await handleStop(container)
  await Promise.all([containerStore.fetchContainers(true, false), loadProjectData()])
}

const handleRestartContainer = async (container: ContainerStatus) => {
  await handleRestart(container)
  await Promise.all([containerStore.fetchContainers(true, false), loadProjectData()])
}

const handleUpdateContainer = async (container: ContainerStatus) => {
  await handleUpdate(container)
  await Promise.all([containerStore.fetchContainers(true, false), loadProjectData()])
}

const handleDeleteContainer = async (container: ContainerStatus) => {
  await handleDelete(container)
  await Promise.all([containerStore.fetchContainers(true, false), loadProjectData()])
}

const handleExportContainer = async (container: ContainerStatus) => {
  await handleExport(container)
}

const handleViewContainerDetail = (container: ContainerStatus) => {
  router.push({ name: 'container-detail', params: { id: container.id } })
}

const handleViewContainerLogs = (container: ContainerStatus) => {
  currentContainer.value = container
  showContainerLogsModal.value = true
}

// 返回
const handleBack = () => {
  router.push({ name: 'compose' })
}

// 监听项目名称变化
watch(
  () => route.params.projectName,
  (newName) => {
    if (newName && typeof newName === 'string') {
      projectName.value = newName
      Init()
    }
  },
)

// 初始化
onMounted(async () => {
  Init()
})
</script>

<style lang="less">
.layout-compose-detail {
  .n-layout-scroll-container {
    .n-layout-content {
      padding-top: 0;
    }
  }
}
</style>

<style scoped lang="less">
.page-header {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  gap: 16px;
  .status-badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 10px;
    height: 28px;
    border-radius: 8px;
    font-size: 13px;
    line-height: 1.428;
    letter-spacing: -0.01em;
    box-shadow:
      0px 1px 2px -1px rgba(0, 0, 0, 0.1),
      0px 1px 3px 0px rgba(0, 0, 0, 0.1);

    .status-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
    }

    &.status-running,
    &.status-partial {
      background: rgba(0, 188, 125, 0.1);
      border: 1px solid rgba(0, 188, 125, 0.2);

      .status-dot {
        background: #00bc7d;
        opacity: 0.5;
      }

      .status-text {
        color: #00d492;
      }
    }

    &.status-exited,
    &.status-draft,
    &.status-created_stack,
    &.status-unknown {
      background: rgba(98, 116, 142, 0.1);
      border: 1px solid rgba(98, 116, 142, 0.2);

      .status-dot {
        background: #62748e;
      }

      .status-text {
        color: #90a1b9;
      }
    }

    &.status-paused {
      background: rgba(254, 154, 0, 0.1);
      border: 1px solid rgba(254, 154, 0, 0.2);

      .status-dot {
        background: #fe9a00;
      }

      .status-text {
        color: #ffb900;
      }
    }
  }
}

.compose-detail-page {
  width: 100%;
  height: 100%;

  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 400px;
  }
  .compose-detail-pane {
    padding-top: 4px;
    .tab-content {
      height: 100%;
      .logs-stream-view {
        padding-block: 0;
        height: 100%;
      }
      .yaml-card {
        border-radius: 8px;
        height: 100%;
      }
    }
  }
  .empty-container {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 300px;
  }

  .containers-grid {
    display: grid;
    gap: 12px;

    &.grid-cols-1 {
      grid-template-columns: 1fr;
    }

    &.grid-cols-2 {
      grid-template-columns: repeat(2, minmax(1fr, 50%));
    }

    &.grid-cols-3 {
      grid-template-columns: repeat(3, minmax(1fr, 33.33%));
    }

    &.grid-cols-4 {
      grid-template-columns: repeat(4, minmax(1fr, 25%));
    }
  }
}
</style>
