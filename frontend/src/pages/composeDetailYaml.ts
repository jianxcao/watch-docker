import { computed, nextTick, ref } from 'vue'
import type { ComposeProject } from '@/common/types'

export interface YamlTargetIdentity {
  projectName: string
  composeFile: string
}

export interface DeployTargetSnapshot {
  projectName: string
  composeFile: string
  yamlContent: string
}

export interface ConfirmDialogHandle {
  destroy: () => void
}

export interface ComposeDetailYamlDeps {
  initialProjectName: string
  getCurrentProject: () => ComposeProject | undefined
  getProjectYaml: (projectName: string, composeFile: string) => Promise<string>
  validateYaml: (yaml: string) => { isValid: boolean; errorMessage: string }
  initProjectData: () => Promise<void>
  openConfirmDialog: (opts: {
    content: string
    onConfirm: () => Promise<void> | void
  }) => ConfirmDialogHandle
  startDeployProgress: () => void
}

export function createComposeDetailYamlController(deps: ComposeDetailYamlDeps) {
  const projectName = ref(deps.initialProjectName)
  const activeTab = ref('containers')
  const yamlLoading = ref(false)
  const deployLoading = ref(false)
  const showProgress = ref(false)
  const yamlContent = ref('')
  const originalYamlContent = ref('')
  const yamlValidationMessage = ref('')
  const isYamlValid = ref(true)
  const loadedYamlTarget = ref<YamlTargetIdentity | null>(null)
  const deploySnapshot = ref<DeployTargetSnapshot | null>(null)

  let yamlRequestSeq = 0
  let routeSwitchSeq = 0
  let activeConfirmDialog: { destroy: () => void; cancelled: boolean } | null = null

  const isCurrentProjectYamlLoaded = computed(() => {
    const project = deps.getCurrentProject()
    const loaded = loadedYamlTarget.value
    if (!project || !loaded) {
      return false
    }
    return (
      project.name === projectName.value &&
      loaded.projectName === project.name &&
      loaded.composeFile === project.composeFile
    )
  })

  const isYamlModified = computed(() => {
    return isCurrentProjectYamlLoaded.value && yamlContent.value !== originalYamlContent.value
  })

  const canSubmitYamlDeploy = computed(() => {
    return (
      !yamlLoading.value &&
      !deployLoading.value &&
      isCurrentProjectYamlLoaded.value &&
      isYamlValid.value
    )
  })

  const canDeploy = computed(() => {
    const project = deps.getCurrentProject()
    if (yamlLoading.value || deployLoading.value || !project || project.name !== projectName.value) {
      return false
    }
    if (activeTab.value === 'yaml') {
      return isCurrentProjectYamlLoaded.value && isYamlValid.value
    }
    return true
  })

  const deployTargetProjectName = computed(() => {
    return deploySnapshot.value?.projectName ?? projectName.value
  })

  const deployTargetComposeFile = computed(() => {
    return (
      deploySnapshot.value?.composeFile ??
      loadedYamlTarget.value?.composeFile ??
      deps.getCurrentProject()?.composeFile ??
      ''
    )
  })

  const deployTargetYamlContent = computed(() => {
    return deploySnapshot.value?.yamlContent ?? yamlContent.value
  })

  const cancelDeployConfirmation = () => {
    if (activeConfirmDialog) {
      activeConfirmDialog.cancelled = true
      activeConfirmDialog.destroy()
      activeConfirmDialog = null
    }
  }

  const handleYamlChange = () => {
    const result = deps.validateYaml(yamlContent.value)
    isYamlValid.value = result.isValid
    yamlValidationMessage.value = result.errorMessage
  }

  const loadYamlContent = async () => {
    const project = deps.getCurrentProject()
    if (!project || project.name !== projectName.value) {
      return
    }

    const reqSeq = ++yamlRequestSeq
    const reqProjectName = project.name
    const reqComposeFile = project.composeFile

    yamlLoading.value = true
    try {
      const content = await deps.getProjectYaml(reqProjectName, reqComposeFile)
      const cur = deps.getCurrentProject()
      if (
        reqSeq !== yamlRequestSeq ||
        projectName.value !== reqProjectName ||
        !cur ||
        cur.name !== reqProjectName ||
        cur.composeFile !== reqComposeFile
      ) {
        return
      }
      yamlContent.value = content
      originalYamlContent.value = content
      loadedYamlTarget.value = {
        projectName: reqProjectName,
        composeFile: reqComposeFile,
      }
      handleYamlChange()
    } catch (error) {
      console.error('加载 YAML 内容失败:', error)
    } finally {
      if (reqSeq === yamlRequestSeq) {
        yamlLoading.value = false
      }
    }
  }

  const handleDeploy = () => {
    if (yamlLoading.value || deployLoading.value) {
      return
    }
    const targetProject = deps.getCurrentProject()
    if (!targetProject || targetProject.name !== projectName.value) {
      return
    }
    if (activeTab.value === 'yaml' && (!isCurrentProjectYamlLoaded.value || !isYamlValid.value)) {
      return
    }

    cancelDeployConfirmation()

    const dialogEntry: { destroy: () => void; cancelled: boolean } = {
      destroy: () => {},
      cancelled: false,
    }
    activeConfirmDialog = dialogEntry

    const confirmedProjectName = targetProject.name
    const confirmedComposeFile = targetProject.composeFile
    const confirmedSwitchSeq = routeSwitchSeq

    const isConfirmationStillValid = () => {
      if (dialogEntry.cancelled || activeConfirmDialog !== dialogEntry) {
        return false
      }
      if (routeSwitchSeq !== confirmedSwitchSeq || projectName.value !== confirmedProjectName) {
        return false
      }
      const cur = deps.getCurrentProject()
      return Boolean(
        cur && cur.name === confirmedProjectName && cur.composeFile === confirmedComposeFile,
      )
    }

    const content = isYamlModified.value
      ? '保存 YAML 配置后将重新创建并启动所有服务，这可能会导致服务短暂中断。是否继续？'
      : '将重新创建并启动所有服务，这可能会导致服务短暂中断。是否继续？'

    const handle = deps.openConfirmDialog({
      content,
      onConfirm: async () => {
        try {
          if (!isConfirmationStillValid()) {
            return
          }
          if (!isCurrentProjectYamlLoaded.value) {
            await loadYamlContent()
            if (!isConfirmationStillValid()) {
              return
            }
          }
          const loaded = loadedYamlTarget.value
          if (
            yamlLoading.value ||
            !isConfirmationStillValid() ||
            !loaded ||
            loaded.projectName !== confirmedProjectName ||
            loaded.composeFile !== confirmedComposeFile ||
            !yamlContent.value ||
            !isYamlValid.value
          ) {
            return
          }

          deploySnapshot.value = {
            projectName: confirmedProjectName,
            composeFile: confirmedComposeFile,
            yamlContent: yamlContent.value,
          }
          await nextTick()
          if (!isConfirmationStillValid()) {
            deploySnapshot.value = null
            return
          }
          deployLoading.value = true
          showProgress.value = true
          deps.startDeployProgress()
        } finally {
          if (activeConfirmDialog === dialogEntry) {
            activeConfirmDialog = null
          }
        }
      },
    })
    dialogEntry.destroy = handle.destroy
  }

  const switchProject = async (newName: string) => {
    const switchSeq = ++routeSwitchSeq
    ++yamlRequestSeq
    cancelDeployConfirmation()

    loadedYamlTarget.value = null
    deploySnapshot.value = null
    yamlContent.value = ''
    originalYamlContent.value = ''
    yamlValidationMessage.value = ''
    isYamlValid.value = true
    projectName.value = newName
    yamlLoading.value = activeTab.value === 'yaml'

    await deps.initProjectData()
    if (switchSeq !== routeSwitchSeq) {
      return
    }
    if (activeTab.value === 'yaml' && deps.getCurrentProject()) {
      await loadYamlContent()
    } else {
      yamlLoading.value = false
    }
  }

  const handleDeploySuccess = () => {
    const snap = deploySnapshot.value
    const loaded = loadedYamlTarget.value
    if (
      snap &&
      loaded &&
      loaded.projectName === snap.projectName &&
      loaded.composeFile === snap.composeFile
    ) {
      originalYamlContent.value = yamlContent.value
    }
  }

  return {
    projectName,
    activeTab,
    yamlLoading,
    deployLoading,
    showProgress,
    yamlContent,
    originalYamlContent,
    yamlValidationMessage,
    isYamlValid,
    loadedYamlTarget,
    deploySnapshot,
    isCurrentProjectYamlLoaded,
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
    handleDeploySuccess,
  }
}
