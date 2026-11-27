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
        <n-dropdown
          :options="dropdownOptions"
          :render-option="renderComposeDropdownOption"
          @select="handleMenuSelect"
          trigger="click"
        >
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
        v-model:value="activeTab"
        type="line"
        animated
        pane-class="compose-detail-pane"
        :pane-style="{ height: tabHeight }"
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
                  :disabled="!canSubmitYamlDeploy"
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
      v-model:show="showProgress"
      :project-name="deployTargetProjectName"
      :compose-file="deployTargetComposeFile"
      :yaml-content="deployTargetYamlContent"
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
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage, useThemeVars } from 'naive-ui'
import {
  ArrowBackOutline,
  LayersOutline,
  DocumentTextOutline,
  RefreshOutline,
  EllipsisHorizontal,
} from '@vicons/ionicons5'
import DocumentIcon from '@/assets/svg/log.svg?component'
import { useComposeStore } from '@/store/compose'
import { useContainerStore } from '@/store/container'
import { useSettingStore } from '@/store/setting'
import { useAuthStore } from '@/store/auth'
import { useResponsive } from '@/hooks/useResponsive'
import { useContainer } from '@/hooks/useContainer'
import type { ComposeProject, ContainerStatus } from '@/common/types'
import { validateComposeYaml } from '@/common/utils'
import ContainerCard from '@/components/ContainerCard.vue'
import ContainerLogsModal from '@/components/ContainerLogsModal.vue'
import LogsStreamView from '@/components/LogsStreamView.vue'
import YamlEditor from '@/components/YamlEditor/index.vue'
import ComposeCreateProgress from '@/components/ComposeCreateProgress.vue'
import ComposePullLogsModal from '@/components/ComposePullLogsModal.vue'
import {
  buildComposeDetailDropdownOptions,
  dispatchComposeDetailAction,
  renderComposeDropdownOption,
} from '@/components/composeActionMenus'
import { useCompose } from '@/hooks/useCompose'
import { buildComposeListQuery, parseComposeListQuery } from './composeListQuery'
import { createComposeDetailYamlController } from './composeDetailYaml'
import { canNavigateBackInApp } from './listQuery'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const theme = useThemeVars()
const composeStore = useComposeStore()
const containerStore = useContainerStore()
const settingStore = useSettingStore()
const authStore = useAuthStore()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()
const { handleStart, handleStop, handleRestart, handleUpdate, handleDelete, handleExport } = useContainer()
const {
  handleStart: handleComposeStart,
  handleStop: handleComposeStop,
  handleDelete: handleComposeDelete,
  handleRestart: handleComposeRestart,
} = useCompose()

// 状态
const loading = ref(false)
const showContainerLogsModal = ref(false)
const currentContainer = ref<ContainerStatus | null>(null)
const yamlEditorContainerRef = ref<HTMLElement | null>(null)
const createProgressRef = ref<InstanceType<typeof ComposeCreateProgress>>()
const showPullLogs = ref(false)

// 加载项目数据
const loadProjectData = async () => {
  if (settingStore.systemInfo?.isComposeEnabled === false) {
    return
  }
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

const {
  projectName,
  activeTab,
  yamlLoading,
  deployLoading,
  showProgress,
  yamlContent,
  yamlValidationMessage,
  isYamlModified,
  canSubmitYamlDeploy,
  canDeploy,
  deployTargetProjectName,
  deployTargetComposeFile,
  deployTargetYamlContent,
  cancelDeployConfirmation,
  handleYamlChange,
  loadYamlContent,
  handleDeploy,
  switchProject,
  handleDeploySuccess: markYamlDeployed,
} = createComposeDetailYamlController({
  initialProjectName: route.params.projectName as string,
  getCurrentProject: () => currentProject.value,
  getProjectYaml: (name, composeFile) => composeStore.getProjectYaml(name, composeFile),
  validateYaml: validateComposeYaml,
  initProjectData: Init,
  openConfirmDialog: ({ content, onConfirm }) => {
    const inst = dialog.warning({
      title: '确认重新部署',
      content,
      positiveText: '确认',
      negativeText: '取消',
      onPositiveClick: onConfirm,
    })
    return {
      destroy: () => {
        inst.destroy()
      },
    }
  },
  startDeployProgress: () => {
    createProgressRef.value?.start()
  },
})

// 下拉菜单选项
const dropdownOptions = computed(() =>
  buildComposeDetailDropdownOptions({
    project: currentProject.value,
    canDeploy: canDeploy.value,
    errorColor: theme.value.errorColor,
  }),
)

const handleMenuSelect = async (key: string) => {
  await dispatchComposeDetailAction(key, currentProject.value, {
    onDeploy: () => handleDeploy(),
    onStart: async (project) => {
      await handleComposeStart(project)
      await containerStore.fetchContainers(true, false)
    },
    onStop: async (project) => {
      await handleComposeStop(project)
      await containerStore.fetchContainers(true, false)
    },
    onRestart: async (project) => {
      await handleComposeRestart(project)
      await containerStore.fetchContainers(true, false)
    },
    onDelete: async (project) => {
      const deleted = await handleComposeDelete(project)
      if (deleted) {
        handleBack()
      }
    },
    onPull: () => {
      showPullLogs.value = true
    },
    onUnsupported: (msg) => {
      message.warning(msg)
    },
  })
}
// 计算属性
const currentProject = computed<ComposeProject | undefined>(() => {
  return composeStore.projects.find((p) => p.name === projectName.value)
})

const projectContainers = computed(() => {
  return containerStore.getProjectContainers(projectName.value).value
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
  if (authStore.authEnabled && !token) {
    return undefined
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const base = `${protocol}//${host}/api/v1/compose/logs/${encodeURIComponent(currentProject.value.name)}/ws?composeFile=${encodeURIComponent(currentProject.value.composeFile)}&projectName=${encodeURIComponent(currentProject.value.name)}`
  return token ? `${base}&token=${encodeURIComponent(token)}` : base
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

// 部署成功
const handleDeploySuccess = () => {
  const msg = isYamlModified.value ? '保存并重新部署成功' : '重新部署成功'
  message.success(msg)
  markYamlDeployed()
}

// 部署失败
const handleDeployError = (errorMessage: string) => {
  const prefix = isYamlModified.value ? '保存并重新部署' : '重新部署'
  message.error(`${prefix}失败: ${errorMessage}`)
  deployLoading.value = false
}

let deployRefreshTimer: ReturnType<typeof setTimeout> | null = null

// 部署完成
const handleDeployComplete = () => {
  deployLoading.value = false
  if (deployRefreshTimer) {
    clearTimeout(deployRefreshTimer)
  }
  // 延迟刷新数据
  deployRefreshTimer = setTimeout(() => {
    deployRefreshTimer = null
    loadProjectData()
  }, 1000)
}

watch(activeTab, (value) => {
  // 切换到 YAML tab 时加载内容
  if (value === 'yaml' && !yamlContent.value) {
    void loadYamlContent()
  }
})

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
  if (canNavigateBackInApp()) {
    router.back()
    return
  }
  void router.push({
    name: 'compose',
    query: buildComposeListQuery({}, parseComposeListQuery(route.query)),
  })
}

// 监听项目名称变化
watch(
  () => route.params.projectName,
  async (newName) => {
    if (newName && typeof newName === 'string') {
      await switchProject(newName)
    }
  },
)

// 初始化
onMounted(async () => {
  Init()
})

onUnmounted(() => {
  cancelDeployConfirmation()
  if (deployRefreshTimer) {
    clearTimeout(deployRefreshTimer)
    deployRefreshTimer = null
  }
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
      grid-template-columns: minmax(0, 1fr);
    }

    &.grid-cols-2 {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    &.grid-cols-3 {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    &.grid-cols-4 {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }
}
</style>
