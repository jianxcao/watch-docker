import { computed, ref } from 'vue'
import type { IResData } from '@/common/axiosConfig'

export interface DetailDialogHandle {
  loading?: boolean
  destroy?: () => void
}

export interface DetailWarningDialogOptions {
  title: string
  content: string
  positiveText: string
  negativeText?: string
  onPositiveClick?: () => Promise<void> | void
}

export interface ResourceDetailControllerDeps<TDetail> {
  getRouteId: () => string
  fetchResource: (id: string) => Promise<IResData<TDetail>>
  deleteResource: (id: string) => Promise<IResData<any>>
  getDisplayName: (detail: TDetail) => string
  getInUseCount: (detail: TDetail) => number
  canDelete?: (detail: TDetail) => boolean
  formatFetchErrorMsg: (detailMsg: string) => string
  formatInUseContent: (count: number) => string
  formatDeleteConfirmContent: (displayName: string) => string
  deleteSuccessMsg: string
  formatDeleteErrorMsg: (detailMsg: string) => string
  openWarningDialog: (options: DetailWarningDialogOptions) => DetailDialogHandle | undefined
  onSuccess: (msg: string) => void
  onError: (msg: string) => void
  navigateToList: () => void
}

export function createResourceDetailController<TDetail>(
  deps: ResourceDetailControllerDeps<TDetail>,
) {
  const detail = ref<TDetail | null>(null)
  const loadedId = ref<string | null>(null)
  const loading = ref(false)
  let requestSeq = 0
  let activeDialog: DetailDialogHandle | null = null

  const isCurrentDetail = computed(() => {
    const currentId = deps.getRouteId()
    return Boolean(currentId && detail.value && loadedId.value && loadedId.value === currentId)
  })

  const canOperate = computed(() => !loading.value && isCurrentDetail.value)

  const clearDetail = () => {
    detail.value = null
    loadedId.value = null
  }

  const closeActiveDialog = () => {
    if (activeDialog) {
      activeDialog.destroy?.()
      activeDialog = null
    }
  }

  const fetchDetail = async () => {
    const reqId = deps.getRouteId()
    if (!reqId) {
      ++requestSeq
      closeActiveDialog()
      clearDetail()
      loading.value = false
      return
    }

    const reqSeq = ++requestSeq
    if (loadedId.value !== reqId) {
      closeActiveDialog()
      clearDetail()
    }
    loading.value = true

    const isCurrentRequest = () => reqSeq === requestSeq && deps.getRouteId() === reqId

    try {
      const response = await deps.fetchResource(reqId)
      if (!isCurrentRequest()) {
        return
      }
      if (response.code === 0) {
        detail.value = response.data ?? null
        loadedId.value = response.data ? reqId : null
      } else {
        clearDetail()
        deps.onError(deps.formatFetchErrorMsg(response.msg))
        deps.navigateToList()
      }
    } catch (error: any) {
      if (!isCurrentRequest()) {
        return
      }
      clearDetail()
      deps.onError(deps.formatFetchErrorMsg(error?.message || '未知错误'))
      deps.navigateToList()
    } finally {
      if (isCurrentRequest()) {
        loading.value = false
      }
    }
  }

  const handleRouteChange = (newId: string, oldId?: string) => {
    closeActiveDialog()
    if (!newId) {
      ++requestSeq
      clearDetail()
      loading.value = false
      return
    }
    if (newId !== oldId) {
      clearDetail()
      void fetchDetail()
    }
  }

  const handleDelete = () => {
    if (!canOperate.value || !detail.value || !loadedId.value) {
      return
    }
    const currentRouteId = deps.getRouteId()
    if (!currentRouteId || loadedId.value !== currentRouteId) {
      return
    }
    if (deps.canDelete && !deps.canDelete(detail.value)) {
      return
    }

    const inUseCount = deps.getInUseCount(detail.value)
    if (inUseCount > 0) {
      closeActiveDialog()
      activeDialog =
        deps.openWarningDialog({
          title: '无法删除',
          content: deps.formatInUseContent(inUseCount),
          positiveText: '知道了',
        }) ?? null
      return
    }

    const targetId = loadedId.value
    const displayName = deps.getDisplayName(detail.value)

    closeActiveDialog()
    let handle: DetailDialogHandle | undefined
    handle = deps.openWarningDialog({
      title: '确认删除',
      content: deps.formatDeleteConfirmContent(displayName),
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        if (deps.getRouteId() !== targetId || loadedId.value !== targetId) {
          return
        }
        try {
          if (handle) {
            handle.loading = true
          }
          const response = await deps.deleteResource(targetId)
          if (response.code === 0) {
            deps.onSuccess(deps.deleteSuccessMsg)
            if (deps.getRouteId() === targetId) {
              deps.navigateToList()
            }
          } else {
            deps.onError(deps.formatDeleteErrorMsg(response.msg))
          }
        } catch (error: any) {
          deps.onError(deps.formatDeleteErrorMsg(error?.message || '未知错误'))
        } finally {
          if (handle) {
            handle.loading = false
          }
          if (activeDialog === handle) {
            activeDialog = null
          }
        }
      },
    })
    activeDialog = handle ?? null
  }

  const dispose = () => {
    ++requestSeq
    closeActiveDialog()
  }

  return {
    detail,
    loadedId,
    loading,
    isCurrentDetail,
    canOperate,
    fetchDetail,
    handleRouteChange,
    handleDelete,
    dispose,
  }
}
