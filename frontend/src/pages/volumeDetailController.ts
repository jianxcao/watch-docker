import type { IResData } from '@/common/axiosConfig'
import type { VolumeDetailResponse } from '@/common/types'
import {
  createResourceDetailController,
  type DetailDialogHandle,
  type DetailWarningDialogOptions,
} from './resourceDetailController'

export interface VolumeDetailControllerDeps {
  getVolumeName: () => string
  fetchVolume: (name: string) => Promise<IResData<VolumeDetailResponse>>
  deleteVolume: (name: string, force: boolean) => Promise<IResData<any>>
  openWarningDialog: (options: DetailWarningDialogOptions) => DetailDialogHandle | undefined
  onSuccess: (msg: string) => void
  onError: (msg: string) => void
  navigateToList: () => void
}

export function createVolumeDetailController(deps: VolumeDetailControllerDeps) {
  const controller = createResourceDetailController<VolumeDetailResponse>({
    getRouteId: deps.getVolumeName,
    fetchResource: deps.fetchVolume,
    deleteResource: (name) => deps.deleteVolume(name, false),
    getDisplayName: (detail) => detail.volume.name || deps.getVolumeName(),
    getInUseCount: (detail) => detail.volume.usageData?.refCount || 0,
    formatFetchErrorMsg: (msg) => `获取 Volume 详情失败：${msg}`,
    formatInUseContent: (count) =>
      `此 Volume 正在被 ${count} 个容器使用，无法删除。请先停止或删除使用该 Volume 的容器。`,
    formatDeleteConfirmContent: (name) => `确定要删除 Volume "${name}" 吗？此操作不可恢复。`,
    deleteSuccessMsg: 'Volume 删除成功',
    formatDeleteErrorMsg: (msg) => `删除失败：${msg}`,
    openWarningDialog: deps.openWarningDialog,
    onSuccess: deps.onSuccess,
    onError: deps.onError,
    navigateToList: deps.navigateToList,
  })

  return {
    volumeDetail: controller.detail,
    loadedVolumeName: controller.loadedId,
    loading: controller.loading,
    isCurrentVolumeDetail: controller.isCurrentDetail,
    canOperate: controller.canOperate,
    fetchVolumeDetail: controller.fetchDetail,
    handleRouteChange: controller.handleRouteChange,
    handleDelete: controller.handleDelete,
    dispose: controller.dispose,
  }
}
