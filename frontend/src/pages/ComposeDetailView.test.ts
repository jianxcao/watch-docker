import { describe, expect, it, vi } from 'vitest'
import type { ComposeProject } from '@/common/types'
import { validateComposeYaml } from '@/common/utils'
import { createComposeDetailYamlController } from './composeDetailYaml'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (err: Error) => void
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (err: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const projectA: ComposeProject = {
  name: 'project-a',
  composeFile: '/apps/project-a/compose.yaml',
  status: 'running',
  runningCount: 1,
  exitedCount: 0,
  createdCount: 0,
}

const projectB: ComposeProject = {
  name: 'project-b',
  composeFile: '/apps/project-b/compose.yaml',
  status: 'running',
  runningCount: 1,
  exitedCount: 0,
  createdCount: 0,
}

const yamlA = 'services:\n  a:\n    image: alpine:3.19\n'
const yamlB = 'services:\n  b:\n    image: nginx:1.27\n'

function setupTestHarness(options?: {
  initImpl?: (projectName: string) => Promise<void>
}) {
  const projectsMap: Record<string, ComposeProject> = {
    [projectA.name]: projectA,
    [projectB.name]: projectB,
  }

  const pendingYamlCalls: Array<{
    projectName: string
    composeFile: string
    deferred: Deferred<string>
  }> = []

  const dialogDestroySpies: Array<ReturnType<typeof vi.fn>> = []
  const openedDialogs: Array<{
    content: string
    onConfirm: () => Promise<void> | void
    destroySpy: ReturnType<typeof vi.fn>
  }> = []
  const startDeployProgress = vi.fn()

  const controller = createComposeDetailYamlController({
    initialProjectName: projectA.name,
    getCurrentProject: () => projectsMap[controller.projectName.value],
    getProjectYaml: (name, composeFile) => {
      const deferred = createDeferred<string>()
      pendingYamlCalls.push({ projectName: name, composeFile, deferred })
      return deferred.promise
    },
    validateYaml: validateComposeYaml,
    initProjectData: async () => {
      if (options?.initImpl) {
        await options.initImpl(controller.projectName.value)
      }
    },
    openConfirmDialog: ({ content, onConfirm }) => {
      const destroySpy = vi.fn()
      dialogDestroySpies.push(destroySpy)
      openedDialogs.push({ content, onConfirm, destroySpy })
      return { destroy: destroySpy }
    },
    startDeployProgress,
  })

  return {
    controller,
    pendingYamlCalls,
    dialogDestroySpies,
    openedDialogs,
    startDeployProgress,
  }
}

describe('ComposeDetailView YAML project switching & deploy race protection', () => {
  it('keeps B loading and blocks deploy when A resolves before B', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'yaml'

    const loadAPromise = controller.loadYamlContent()
    expect(pendingYamlCalls).toHaveLength(1)
    expect(pendingYamlCalls[0].projectName).toBe(projectA.name)
    expect(controller.yamlLoading.value).toBe(true)

    const switchPromise = controller.switchProject(projectB.name)
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(2)
    expect(pendingYamlCalls[1].projectName).toBe(projectB.name)
    expect(controller.yamlLoading.value).toBe(true)
    expect(controller.canSubmitYamlDeploy.value).toBe(false)
    expect(controller.canDeploy.value).toBe(false)

    // A 先返回：不得写入 B 编辑器，也不得提前结束 B 的加载状态或放开部署
    pendingYamlCalls[0].deferred.resolve(yamlA)
    await loadAPromise

    expect(controller.yamlContent.value).toBe('')
    expect(controller.originalYamlContent.value).toBe('')
    expect(controller.loadedYamlTarget.value).toBeNull()
    expect(controller.yamlLoading.value).toBe(true)
    expect(controller.canSubmitYamlDeploy.value).toBe(false)
    expect(controller.canDeploy.value).toBe(false)

    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(0)
    expect(startDeployProgress).not.toHaveBeenCalled()

    // B 后返回：恢复可部署，提交信息全属于 B
    pendingYamlCalls[1].deferred.resolve(yamlB)
    await switchPromise

    expect(controller.yamlLoading.value).toBe(false)
    expect(controller.yamlContent.value).toBe(yamlB)
    expect(controller.loadedYamlTarget.value).toEqual({
      projectName: projectB.name,
      composeFile: projectB.composeFile,
    })
    expect(controller.canSubmitYamlDeploy.value).toBe(true)

    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    await openedDialogs[0].onConfirm()
    expect(startDeployProgress).toHaveBeenCalledTimes(1)
    expect(controller.deployTargetProjectName.value).toBe(projectB.name)
    expect(controller.deployTargetComposeFile.value).toBe(projectB.composeFile)
    expect(controller.deployTargetYamlContent.value).toBe(yamlB)
  })

  it('prevents late A response from overwriting B when B resolves before A', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'yaml'

    const loadAPromise = controller.loadYamlContent()
    const switchPromise = controller.switchProject(projectB.name)
    await Promise.resolve()

    // B 先返回
    pendingYamlCalls[1].deferred.resolve(yamlB)
    await switchPromise

    const editedYamlB = 'services:\n  b:\n    image: nginx:1.28-alpine\n'
    controller.yamlContent.value = editedYamlB
    controller.handleYamlChange()

    // A 迟到返回：绝不能覆盖 B 的 YAML 或校验状态
    pendingYamlCalls[0].deferred.resolve(yamlA)
    await loadAPromise

    expect(controller.yamlLoading.value).toBe(false)
    expect(controller.yamlContent.value).toBe(editedYamlB)
    expect(controller.originalYamlContent.value).toBe(yamlB)
    expect(controller.loadedYamlTarget.value).toEqual({
      projectName: projectB.name,
      composeFile: projectB.composeFile,
    })

    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    await openedDialogs[0].onConfirm()
    expect(startDeployProgress).toHaveBeenCalledTimes(1)
    expect(controller.deployTargetProjectName.value).toBe(projectB.name)
    expect(controller.deployTargetComposeFile.value).toBe(projectB.composeFile)
    expect(controller.deployTargetYamlContent.value).toBe(editedYamlB)
  })

  it('ignores A resolving while B initProjectData is still pending', async () => {
    const initBDeferred = createDeferred<void>()
    const { controller, pendingYamlCalls } = setupTestHarness({
      initImpl: () => initBDeferred.promise,
    })
    controller.activeTab.value = 'yaml'

    const loadAPromise = controller.loadYamlContent()
    const switchPromise = controller.switchProject(projectB.name)
    expect(controller.yamlLoading.value).toBe(true)

    pendingYamlCalls[0].deferred.resolve(yamlA)
    await loadAPromise

    expect(controller.yamlContent.value).toBe('')
    expect(controller.yamlLoading.value).toBe(true)
    expect(controller.canDeploy.value).toBe(false)

    initBDeferred.resolve()
    await Promise.resolve()
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(2)

    pendingYamlCalls[1].deferred.resolve(yamlB)
    await switchPromise
    expect(controller.yamlLoading.value).toBe(false)
    expect(controller.yamlContent.value).toBe(yamlB)
  })

  it('cancels open deploy confirmation dialog if route switches to another project', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'yaml'

    const loadAPromise = controller.loadYamlContent()
    pendingYamlCalls[0].deferred.resolve(yamlA)
    await loadAPromise

    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    const confirmDialogA = openedDialogs[0]
    expect(confirmDialogA.destroySpy).not.toHaveBeenCalled()

    const switchPromise = controller.switchProject(projectB.name)
    expect(confirmDialogA.destroySpy).toHaveBeenCalledTimes(1)

    // 即使旧确认框回调被触发，也不可启动部署
    await confirmDialogA.onConfirm()
    expect(startDeployProgress).not.toHaveBeenCalled()
    expect(controller.showProgress.value).toBe(false)

    await Promise.resolve()
    pendingYamlCalls[1].deferred.resolve(yamlB)
    await switchPromise
    expect(controller.loadedYamlTarget.value).toEqual({
      projectName: projectB.name,
      composeFile: projectB.composeFile,
    })
  })

  it('ignores confirmed A deploy when switched to B while A YAML is still pending', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'containers'

    // 在 A 的容器页点击重新部署并确认，触发 A 的按需 loadYamlContent（保持等待）
    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    const confirmAPromise = Promise.resolve(openedDialogs[0].onConfirm())
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(1)
    expect(pendingYamlCalls[0].projectName).toBe(projectA.name)

    // 切换到 B 并打开 YAML 页
    controller.activeTab.value = 'yaml'
    const switchToBPromise = controller.switchProject(projectB.name)
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(2)
    expect(pendingYamlCalls[1].projectName).toBe(projectB.name)

    // 先返回 B，再返回 A
    pendingYamlCalls[1].deferred.resolve(yamlB)
    await switchToBPromise
    pendingYamlCalls[0].deferred.resolve(yamlA)
    await confirmAPromise

    expect(startDeployProgress).not.toHaveBeenCalled()
    expect(controller.showProgress.value).toBe(false)
    expect(controller.deployLoading.value).toBe(false)
    expect(controller.deploySnapshot.value).toBeNull()
    expect(controller.yamlContent.value).toBe(yamlB)
    expect(controller.loadedYamlTarget.value).toEqual({
      projectName: projectB.name,
      composeFile: projectB.composeFile,
    })
  })

  it('blocks A -> B -> A stale async confirm and preserves later confirmation on revisited A', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'containers'

    // 在第一次停留在 A 时打开确认并进入等待 A1 YAML
    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    const firstConfirmA = Promise.resolve(openedDialogs[0].onConfirm())
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(1)
    const firstACall = pendingYamlCalls[0]

    // 快速切换 A -> B -> A
    await controller.switchProject(projectB.name)
    await controller.switchProject(projectA.name)
    controller.activeTab.value = 'yaml'
    const loadARevisitPromise = controller.loadYamlContent()
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(2)
    pendingYamlCalls[1].deferred.resolve(yamlA)
    await loadARevisitPromise

    // 再次进入 A 后重新打开确认框，不应被旧回调的 finally 误清空
    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(2)

    // 此时旧的 A1 YAML 返回：必须因 routeSwitchSeq 不一致而被拒绝，也不得清除第二次确认记录
    firstACall.deferred.resolve(yamlA)
    await firstConfirmA
    expect(startDeployProgress).not.toHaveBeenCalled()

    // 验证第二次在 A 上打开的新确认框仍可正常确认并恰好部署 A 一次
    await openedDialogs[1].onConfirm()
    expect(startDeployProgress).toHaveBeenCalledTimes(1)
    expect(controller.deployTargetProjectName.value).toBe(projectA.name)
    expect(controller.deployTargetComposeFile.value).toBe(projectA.composeFile)
    expect(controller.deployTargetYamlContent.value).toBe(yamlA)
  })

  it('deploys A exactly once when staying on A until async confirmation finishes', async () => {
    const { controller, pendingYamlCalls, openedDialogs, startDeployProgress } = setupTestHarness()
    controller.activeTab.value = 'containers'

    controller.handleDeploy()
    expect(openedDialogs).toHaveLength(1)
    const confirmAPromise = Promise.resolve(openedDialogs[0].onConfirm())
    await Promise.resolve()
    expect(pendingYamlCalls).toHaveLength(1)

    pendingYamlCalls[0].deferred.resolve(yamlA)
    await confirmAPromise

    expect(startDeployProgress).toHaveBeenCalledTimes(1)
    expect(controller.showProgress.value).toBe(true)
    expect(controller.deployLoading.value).toBe(true)
    expect(controller.deployTargetProjectName.value).toBe(projectA.name)
    expect(controller.deployTargetComposeFile.value).toBe(projectA.composeFile)
    expect(controller.deployTargetYamlContent.value).toBe(yamlA)
  })

  it('blocks deploy actions when current project has managedConfig=false', () => {
    const startDeployProgress = vi.fn()
    const openConfirmDialog = vi.fn(() => ({ destroy: vi.fn() }))
    const externalProject: ComposeProject = {
      ...projectA,
      managedConfig: false,
    }
    const controller = createComposeDetailYamlController({
      initialProjectName: externalProject.name,
      getCurrentProject: () => externalProject,
      getProjectYaml: vi.fn(),
      validateYaml: validateComposeYaml,
      initProjectData: async () => {},
      openConfirmDialog,
      startDeployProgress,
    })

    expect(controller.canDeploy.value).toBe(false)
    expect(controller.canSubmitYamlDeploy.value).toBe(false)
    controller.handleDeploy()
    expect(openConfirmDialog).not.toHaveBeenCalled()
    expect(startDeployProgress).not.toHaveBeenCalled()
  })
})
