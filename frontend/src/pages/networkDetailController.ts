import { computed } from 'vue'
import type { IResData } from '@/common/axiosConfig'
import type { NetworkDetailResponse } from '@/common/types'
import {
  createResourceDetailController,
  type DetailDialogHandle,
  type DetailWarningDialogOptions,
} from './resourceDetailController'

const BUILT_IN_NETWORK_NAMES = ['bridge', 'host', 'none']

export interface NetworkDetailControllerDeps {
  getNetworkId: () => string
  fetchNetwork: (id: string) => Promise<IResData<NetworkDetailResponse>>
  deleteNetwork: (id: string) => Promise<IResData<any>>
  openWarningDialog: (options: DetailWarningDialogOptions) => DetailDialogHandle | undefined
  onSuccess: (msg: string) => void
  onError: (msg: string) => void
  navigateToList: () => void
}

export function createNetworkDetailController(deps: NetworkDetailControllerDeps) {
  const controller = createResourceDetailController<NetworkDetailResponse>({
    getRouteId: deps.getNetworkId,
    fetchResource: deps.fetchNetwork,
    deleteResource: deps.deleteNetwork,
    getDisplayName: (detail) => detail.network.name,
    getInUseCount: (detail) => detail.network.containerCount || 0,
    canDelete: (detail) => !BUILT_IN_NETWORK_NAMES.includes(detail.network.name),
    formatFetchErrorMsg: (msg) => `获取网络详情失败：${msg}`,
    formatInUseContent: (count) =>
      `此网络正在被 ${count} 个容器使用，无法删除。请先断开所有容器的连接。`,
    formatDeleteConfirmContent: (name) => `确定要删除网络 "${name}" 吗？此操作不可恢复。`,
    deleteSuccessMsg: '网络删除成功',
    formatDeleteErrorMsg: (msg) => `删除失败：${msg}`,
    openWarningDialog: deps.openWarningDialog,
    onSuccess: deps.onSuccess,
    onError: deps.onError,
    navigateToList: deps.navigateToList,
  })

  const isBuiltInNetwork = computed(() => {
    if (!controller.isCurrentDetail.value || !controller.detail.value) {
      return false
    }
    return BUILT_IN_NETWORK_NAMES.includes(controller.detail.value.network.name)
  })

  return {
    networkDetail: controller.detail,
    loadedNetworkId: controller.loadedId,
    loading: controller.loading,
    isCurrentNetworkDetail: controller.isCurrentDetail,
    canOperate: controller.canOperate,
    isBuiltInNetwork,
    fetchNetworkDetail: controller.fetchDetail,
    handleRouteChange: controller.handleRouteChange,
    handleDelete: controller.handleDelete,
    dispose: controller.dispose,
  }
}
