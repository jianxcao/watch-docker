import { composeApi } from '@/common/api'
import type { ComposeAction, ComposeProject } from '@/common/types'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useComposeStore = defineStore('compose', () => {
  const message = window.$message

  // 状态
  const projects = ref<ComposeProject[]>([])
  const loading = ref(false)
  const operationLoading = ref<Record<string, boolean>>({})

  const stats = computed(() => ({
    total: projects.value.length,
    running: projects.value.filter((p) => p.status === 'running').length,
    exited: projects.value.filter((p) => p.status === 'exited').length,
    draft: projects.value.filter((p) => p.status === 'draft').length,
    createdStack: projects.value.filter((p) => p.status === 'created_stack').length,
  }))

  // 检查项目是否正在执行操作
  const isProjectOperating = (projectName: string) => {
    return computed(() => operationLoading.value[projectName] || false)
  }

  // 设置项目操作状态
  const setProjectOperating = (projectName: string, isOperating: boolean) => {
    operationLoading.value[projectName] = isOperating
  }

  // 获取项目列表
  const fetchProjects = async (force = false) => {
    if (loading.value && !force) {
      return
    }

    loading.value = true
    try {
      const response = await composeApi.getProjects()
      if (response.code === 0) {
        projects.value = response.data.projects || []
      } else {
        throw new Error(response.msg || '获取项目列表失败')
      }
    } catch (error) {
      console.error('获取 Compose 项目列表失败:', error)
      message.error(`获取 Compose 项目列表失败: ${(error as Error).message}`)
      throw error
    } finally {
      loading.value = false
    }
  }

  // 执行项目操作
  const executeProjectAction = async (
    project: ComposeProject,
    action: ComposeAction,
    options?: { refreshAfter?: boolean },
  ) => {
    const { refreshAfter = true } = options || {}

    setProjectOperating(project.name, true)

    try {
      let response
      let actionText = ''

      switch (action) {
        case 'start':
          response = await composeApi.startProject(project)
          actionText = '启动'
          break
        case 'stop':
          response = await composeApi.stopProject(project)
          actionText = '停止'
          break
        case 'restart':
          response = await composeApi.restartProject(project)
          actionText = '重启'
          break
        case 'delete':
          response = await composeApi.deleteProject(project)
          actionText = '删除'
          break
        case 'create':
          response = await composeApi.createProject(project)
          actionText = '创建'
          break
        default:
          throw new Error(`未知操作: ${action}`)
      }

      if (response.code === 0) {
        message.success(`${actionText}项目成功`)
        if (refreshAfter) {
          await fetchProjects(true)
        }
      } else {
        throw new Error(response.msg || `${actionText}项目失败`)
      }
    } catch (error) {
      console.error(`执行项目操作失败:`, error)
      const actionText = {
        start: '启动',
        stop: '停止',
        restart: '重新创建',
        delete: '删除',
        create: '创建',
      }[action]
      message.error(`${actionText}项目失败: ${(error as Error).message}`)
      throw error
    } finally {
      setProjectOperating(project.name, false)
    }
  }

  // 启动项目
  const startProject = async (app: ComposeProject) => {
    return executeProjectAction(app, 'start')
  }

  // 停止项目
  const stopProject = async (app: ComposeProject) => {
    return executeProjectAction(app, 'stop')
  }

  // 重新创建项目
  const restartProject = async (app: ComposeProject) => {
    return executeProjectAction(app, 'restart')
  }

  // 删除项目
  const deleteProject = async (app: ComposeProject) => {
    return executeProjectAction(app, 'delete')
  }

  const createProject = async (app: ComposeProject) => {
    return executeProjectAction(app, 'create')
  }

  // 保存新项目（创建目录和 YAML 文件）
  const saveNewProject = async (name: string, yamlContent: string) => {
    try {
      const response = await composeApi.saveNewProject(name, yamlContent)
      if (response.code === 0) {
        message.success('项目保存成功')
        return response.data
      } else {
        throw new Error(response.msg || '保存项目失败')
      }
    } catch (error) {
      console.error('保存项目失败:', error)
      message.error(`保存项目失败: ${(error as Error).message}`)
      throw error
    }
  }

  // 获取项目日志
  const getProjectLogs = async (projectName: string, lines = 100) => {
    try {
      const response = await composeApi.getProjectLogs(projectName, lines)
      if (response.code === 0) {
        return response.data.logs || ''
      } else {
        throw new Error(response.msg || '获取日志失败')
      }
    } catch (error) {
      console.error('获取项目日志失败:', error)
      message.error(`获取项目日志失败: ${(error as Error).message}`)
      throw error
    }
  }

  // 获取项目的 YAML 内容
  const getProjectYaml = async (projectName: string, composeFile: string) => {
    try {
      const response = await composeApi.getProjectYaml(projectName, composeFile)
      if (response.code === 0) {
        return response.data.yamlContent || ''
      } else {
        throw new Error(response.msg || '获取 YAML 内容失败')
      }
    } catch (error) {
      console.error('获取项目 YAML 失败:', error)
      message.error(`获取项目 YAML 失败: ${(error as Error).message}`)
      throw error
    }
  }

  // 根据名称查找项目
  const findProject = (name: string) => {
    return computed(() => projects.value.find((p) => p.name === name))
  }

  // 清空所有状态
  const clearAll = () => {
    projects.value = []
    operationLoading.value = {}
    loading.value = false
  }

  return {
    // 状态
    projects,
    loading,
    operationLoading,
    stats,

    // 工具函数
    isProjectOperating,
    setProjectOperating,
    findProject,

    // 操作方法
    fetchProjects,
    executeProjectAction,
    startProject,
    stopProject,
    restartProject,
    deleteProject,
    createProject,
    saveNewProject,
    getProjectLogs,
    getProjectYaml,
    clearAll,
  }
})
