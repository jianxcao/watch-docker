import { useContainerStore } from '@/store/container'
import { useMessage, useDialog } from 'naive-ui'
import type { ContainerStatus } from '@/common/types'

export function useContainer() {
  const store = useContainerStore()
  const message = useMessage()
  const dialog = useDialog()

  // 启动容器
  const handleStart = async (container: ContainerStatus) => {
    try {
      await store.startContainer(container.id)
      message.success(`容器 ${container.name} 启动成功`)
    } catch (error: any) {
      message.error(`启动容器失败: ${error.message}`)
    }
  }

  // 停止容器
  const handleStop = async (container: ContainerStatus) => {
    try {
      await store.stopContainer(container.id)
      message.success(`容器 ${container.name} 停止成功`)
    } catch (error: any) {
      message.error(`停止容器失败: ${error.message}`)
    }
  }

  // 更新容器
  const handleUpdate = async (container: ContainerStatus, image?: string) => {
    try {
      await store.updateContainer(container.id, image)
      message.success(`容器 ${container.name} 更新成功`)
    } catch (error: any) {
      message.error(`更新容器失败: ${error.message}`)
    }
  }

  // 删除容器（带确认）
  const handleDelete = (container: ContainerStatus) => {
    const d = dialog.warning({
      title: '确认删除',
      content: `确定要删除容器 "${container.name}" 吗？此操作不可撤销。`,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          d.loading = true
          await store.deleteContainer(container.id)
          message.success(`容器 ${container.name} 删除成功`)
        } catch (error: any) {
          message.error(`删除容器失败: ${error.message}`)
        } finally {
          d.loading = false
        }
      },
    })
  }

  // 批量更新（带确认）
  const handleBatchUpdate = () => {
    const updateableCount = store.updateableContainers.length

    if (updateableCount === 0) {
      message.info('当前没有可更新的容器')
      return
    }

    const d = dialog.info({
      title: '批量更新确认',
      content: `发现 ${updateableCount} 个可更新的容器，确定要批量更新吗？`,
      positiveText: '确认更新',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          d.loading = true
          const result = await store.batchUpdate()

          if (result.updated.length > 0) {
            message.success(`成功更新 ${result.updated.length} 个容器`)
          }

          if (Object.keys(result.failed).length > 0) {
            const failedNames = Object.keys(result.failed)
            message.warning(`${failedNames.length} 个容器更新失败: ${failedNames.join(', ')}`)
          }
        } catch (error: any) {
          message.error(`批量更新失败: ${error.message}`)
        } finally {
          d.loading = false
        }
      },
    })
  }

  // 刷新容器列表
  const handleRefresh = async () => {
    try {
      await store.fetchContainers()
      message.success('容器列表刷新成功')
    } catch (error: any) {
      message.error(`刷新失败: ${error.message}`)
    }
  }

  // 获取容器状态颜色
  const getStatusColor = (container: ContainerStatus): string => {
    if (!container.running) return 'warning'

    switch (container.status) {
      case 'UpToDate':
        return 'success'
      case 'UpdateAvailable':
        return 'info'
      case 'Error':
        return 'error'
      case 'Skipped':
        return 'default'
      default:
        return 'default'
    }
  }

  // 获取容器状态文本
  const getStatusText = (container: ContainerStatus): string => {
    if (!container.running) return '已停止'

    switch (container.status) {
      case 'UpToDate':
        return '最新'
      case 'UpdateAvailable':
        return '可更新'
      case 'Error':
        return '错误'
      case 'Skipped':
        return '跳过'
      default:
        return '未知'
    }
  }

  // 获取运行状态颜色
  const getRunningStatusColor = (running: boolean): string => {
    return running ? 'success' : 'default'
  }

  // 获取运行状态文本
  const getRunningStatusText = (running: boolean): string => {
    return running ? '运行中' : '已停止'
  }

  return {
    // 操作方法
    handleStart,
    handleStop,
    handleUpdate,
    handleDelete,
    handleBatchUpdate,
    handleRefresh,

    // 工具方法
    getStatusColor,
    getStatusText,
    getRunningStatusColor,
    getRunningStatusText,
  }
}
