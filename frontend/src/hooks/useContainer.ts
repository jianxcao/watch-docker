import { useContainerStore } from '@/store/container'
import { useMessage, useDialog, NSpace } from 'naive-ui'
import type { ContainerStatus } from '@/common/types'
import { h, ref } from 'vue'
import { NCheckbox } from 'naive-ui'

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

  // 重启容器
  const handleRestart = async (container: ContainerStatus) => {
    try {
      await store.restartContainer(container.id)
      message.success(`容器 ${container.name} 重启成功`)
    } catch (error: any) {
      message.error(`重启容器失败: ${error.message}`)
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
    // 根据容器运行状态设置默认的强制删除值
    const forceDefault = container.running
    const forceValue = ref(forceDefault)
    const removeVolumesValue = ref(false)
    const removeNetworksValue = ref(false)

    const d = dialog.warning({
      title: '确认删除容器',
      content: () =>
        h(NSpace, { vertical: true }, () => [
          h('div', {}, `确定要删除容器 "${container.name}" 吗？此操作不可撤销。`),
          container.running
            ? h('div', { class: 'text-orange-500 text-sm' }, '注意：容器正在运行中')
            : h('div', { class: 'text-gray-500 text-sm' }, '容器已停止'),
          h(
            NCheckbox,
            {
              checked: forceValue.value,
              'onUpdate:checked': (value: boolean) => {
                forceValue.value = value
              },
            },
            {
              default: () => (container.running ? '强制删除 (停止并删除运行中的容器)' : '强制删除'),
            },
          ),
          h(
            NCheckbox,
            {
              checked: removeVolumesValue.value,
              'onUpdate:checked': (value: boolean) => {
                removeVolumesValue.value = value
              },
            },
            {
              default: () => '删除关联的卷 (Volume)',
            },
          ),
          h(
            NCheckbox,
            {
              checked: removeNetworksValue.value,
              'onUpdate:checked': (value: boolean) => {
                removeNetworksValue.value = value
              },
            },
            {
              default: () => '删除关联的网络 (Network)',
            },
          ),
        ]),
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          d.loading = true
          await store.deleteContainer(
            container.id,
            forceValue.value,
            removeVolumesValue.value,
            removeNetworksValue.value,
          )
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

  // 获取容器状态颜色
  const getStatusColor = (container: ContainerStatus): string => {
    if (!container.running) {
      return 'warning'
    }

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
    if (!container.running) {
      return '已停止'
    }

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

  // 导出容器
  const handleExport = async (container: ContainerStatus) => {
    try {
      await store.exportContainer(container.id)
      message.success(`容器 ${container.name} 导出成功`)
    } catch (error: any) {
      message.error(`导出容器失败: ${error.message}`)
    }
  }

  return {
    // 操作方法
    handleStart,
    handleStop,
    handleRestart,
    handleUpdate,
    handleDelete,
    handleBatchUpdate,
    handleExport,

    // 工具方法
    getStatusColor,
    getStatusText,
    getRunningStatusColor,
    getRunningStatusText,
  }
}
