import { h } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useComposeStore } from '@/store/compose'
import type { ComposeProject, ComposeAction } from '@/common/types'

export const useCompose = () => {
  const dialog = useDialog()
  const message = useMessage()
  const composeStore = useComposeStore()

  // 执行项目操作的通用处理函数
  const executeAction = async (
    project: ComposeProject,
    action: ComposeAction,
    confirmMessage?: string
  ) => {
    // 如果需要确认对话框
    if (confirmMessage) {
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
    } else {
      // 直接执行操作
      return composeStore.executeProjectAction(project, action)
    }
  }

  // 启动项目
  const handleStart = async (project: ComposeProject) => {
    try {
      await executeAction(project, 'start')
    } catch (error) {
      console.error('启动项目失败:', error)
    }
  }

  // 停止项目
  const handleStop = async (project: ComposeProject) => {
    try {
      await executeAction(project, 'stop')
    } catch (error) {
      console.error('停止项目失败:', error)
    }
  }
  // 重启项目
  const handleRestart = async (project: ComposeProject) => {
    try {
      await executeAction(project, 'restart')
    } catch (error) {
      console.error('重启项目失败:', error)
    }
  }

  // 重新创建项目
  const handleCreate = async (project: ComposeProject) => {
    try {
      await executeAction(
        project,
        'create',
        `确定要重新创建项目 "${project.name}" 吗？这将停止并删除所有容器，然后重新创建它们。`
      )
    } catch (error) {
      console.error('重新创建项目失败:', error)
    }
  }

  // 删除项目
  const handleDelete = async (project: ComposeProject) => {
    try {
      await executeAction(
        project,
        'delete',
        `确定要删除项目 "${project.name}" 吗？这将停止并删除所有容器、网络和卷。此操作不可撤销！`
      )
    } catch (error) {
      console.error('删除项目失败:', error)
    }
  }

  // 查看项目日志
  const handleViewLogs = async (project: ComposeProject, lines = 100) => {
    try {
      const logs = await composeStore.getProjectLogs(project.name, lines)

      // 显示日志对话框
      dialog.info({
        title: `项目日志 - ${project.name}`,
        content: () => {
          return h(
            'pre',
            {
              style: {
                maxHeight: '400px',
                overflow: 'auto',
                background: '#1e1e1e',
                color: '#d4d4d4',
                padding: '16px',
                borderRadius: '4px',
                fontSize: '12px',
                fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
              },
            },
            logs || '暂无日志'
          )
        },
        style: {
          width: '80vw',
          maxWidth: '800px',
        },
        positiveText: '关闭',
      })
    } catch (error) {
      console.error('获取日志失败:', error)
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
    handleViewLogs,
    handleRefresh,

    // 工具方法
    getProjectStatusText,

    // 通用操作
    executeAction,
  }
}
