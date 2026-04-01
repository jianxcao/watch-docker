import { useDialog, useMessage } from 'naive-ui'
import { useComposeStore } from '@/store/compose'
import type { ComposeProject, ComposeAction } from '@/common/types'
import ComposeLogsModal from '@/components/ComposeLogsModal.vue'

export const useCompose = () => {
  const dialog = useDialog()
  const message = useMessage()
  const composeStore = useComposeStore()

  // 执行项目操作（阻塞式确认弹窗，用于破坏性操作）
  const executeActionWithConfirm = async (
    project: ComposeProject,
    action: ComposeAction,
    confirmMessage: string,
  ) => {
    return new Promise<void>((resolve, reject) => {
      const d = dialog.warning({
        title: '确认操作',
        content: confirmMessage,
        positiveText: '确认',
        negativeText: '取消',
        onPositiveClick: async () => {
          d.loading = true
          try {
            await composeStore.executeProjectAction(project, action)
            resolve()
          } catch (error) {
            reject(error)
          } finally {
            d.loading = false
          }
        },
        onNegativeClick: () => {
          reject(new Error('用户取消操作'))
        },
      })
    })
  }

  // 执行项目操作（非阻塞式确认弹窗，确认后立即关闭弹窗，卡片展示进度）
  const executeActionWithQuickConfirm = (
    project: ComposeProject,
    action: ComposeAction,
    confirmMessage: string,
  ) => {
    dialog.warning({
      title: '确认操作',
      content: confirmMessage,
      positiveText: '确认',
      negativeText: '取消',
      onPositiveClick: () => {
        composeStore.executeProjectAction(project, action)
      },
    })
  }

  // 直接执行操作（无确认弹窗，卡片展示进度）
  const executeAction = (project: ComposeProject, action: ComposeAction) => {
    composeStore.executeProjectAction(project, action)
  }

  // 启动项目（直接执行，卡片展示进度）
  const handleStart = (project: ComposeProject) => {
    executeAction(project, 'start')
  }

  // 停止项目（直接执行，卡片展示进度）
  const handleStop = (project: ComposeProject) => {
    executeAction(project, 'stop')
  }

  // 重启项目（直接执行，卡片展示进度）
  const handleRestart = (project: ComposeProject) => {
    executeAction(project, 'restart')
  }

  // 重新创建项目（确认后立即关闭弹窗，卡片展示进度）
  const handleCreate = (project: ComposeProject) => {
    executeActionWithQuickConfirm(
      project,
      'create',
      `确定要重新创建项目 "${project.name}" 吗？这将停止并删除所有容器，然后重新创建它们。`,
    )
  }

  // 删除项目（阻塞式确认，完成后关闭弹窗）
  const handleDelete = async (project: ComposeProject) => {
    try {
      const confirmMessage =
        project.status === 'draft' || project.status === 'created_stack'
          ? `确定要删除项目 "${project.name}" 吗？这将删除该项目的配置文件（草稿）。此操作不可撤销！`
          : `确定要删除项目 "${project.name}" 吗？这将停止并删除所有容器、网络和卷。此操作不可撤销！`

      await executeActionWithConfirm(project, 'delete', confirmMessage)
    } catch (error) {
      console.error('删除项目失败:', error)
    }
  }

  // 刷新项目列表
  const handleRefresh = async () => {
    try {
      await composeStore.fetchProjects(true)
      message.success('刷新成功')
    } catch (error) {
      console.error('刷新失败:', error)
    }
  }

  // 获取项目状态文本
  const getProjectStatusText = (status: string) => {
    switch (status) {
      case 'running':
        return '运行中'
      case 'stopped':
        return '已停止'
      case 'partial':
        return '部分运行'
      case 'error':
        return '错误'
      case 'unknown':
        return '未知'
      default:
        return '未知'
    }
  }

  return {
    // 操作方法
    handleStart,
    handleStop,
    handleRestart,
    handleCreate,
    handleDelete,
    handleRefresh,

    // 工具方法
    getProjectStatusText,

    // 通用操作
    executeAction,

    ComposeLogsModal,
  }
}
