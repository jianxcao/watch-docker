import { computed, ref } from 'vue'
import type { ContainerStats, ContainerStatus } from '@/common/types'

export interface ContainerDetailControllerDeps {
  getContainerId: () => string
  fetchContainerDetail: (id: string) => Promise<any>
  onLoadError: (error: unknown) => void
}

export function createContainerDetailController(deps: ContainerDetailControllerDeps) {
  const containerDetail = ref<any>(null)
  const loadedContainerId = ref<string | null>(null)
  const containerStats = ref<ContainerStats | null>(null)
  const loading = ref(false)
  let requestSeq = 0

  const isCurrentContainerDetail = computed(() => {
    const currentId = deps.getContainerId()
    return Boolean(
      currentId &&
        containerDetail.value &&
        loadedContainerId.value &&
        loadedContainerId.value === currentId,
    )
  })

  const canOperateMenu = computed(() => {
    return !loading.value && isCurrentContainerDetail.value
  })

  const containerName = computed(() => {
    if (!isCurrentContainerDetail.value || !containerDetail.value?.Name) {
      return ''
    }
    return String(containerDetail.value.Name).replace(/^\//, '')
  })

  const loadContainerDetail = async () => {
    const reqId = deps.getContainerId()
    if (!reqId) {
      ++requestSeq
      containerDetail.value = null
      loadedContainerId.value = null
      containerStats.value = null
      loading.value = false
      return
    }

    const reqSeq = ++requestSeq
    if (loadedContainerId.value !== reqId) {
      containerDetail.value = null
      loadedContainerId.value = null
      containerStats.value = null
    }
    loading.value = true

    const isCurrentRequest = () => reqSeq === requestSeq && deps.getContainerId() === reqId

    try {
      const detail = await deps.fetchContainerDetail(reqId)
      if (!isCurrentRequest()) {
        return
      }
      containerDetail.value = detail ?? null
      loadedContainerId.value = detail ? reqId : null
    } catch (error) {
      if (!isCurrentRequest()) {
        return
      }
      containerDetail.value = null
      loadedContainerId.value = null
      deps.onLoadError(error)
    } finally {
      if (isCurrentRequest()) {
        loading.value = false
      }
    }
  }

  const resolveActionContainer = (
    findContainerById: (id: string) => ContainerStatus | undefined,
  ): ContainerStatus | null => {
    if (!canOperateMenu.value || !containerDetail.value) {
      return null
    }
    const currentId = deps.getContainerId()
    return (
      findContainerById(currentId) || {
        id: currentId,
        name: containerName.value,
        image: containerDetail.value.Config?.Image || '',
        running: Boolean(containerDetail.value.State?.Running),
        status: '',
        skipped: false,
        skipReason: '',
        labels: containerDetail.value.Config?.Labels || {},
        lastCheckedAt: '',
        startedAt: containerDetail.value.State?.StartedAt || '',
        ports: [],
      }
    )
  }

  return {
    containerDetail,
    loadedContainerId,
    containerStats,
    loading,
    isCurrentContainerDetail,
    canOperateMenu,
    containerName,
    loadContainerDetail,
    resolveActionContainer,
  }
}
