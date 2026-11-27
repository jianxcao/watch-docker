import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { IResData } from '@/common/axiosConfig'
import type { VolumeDetailResponse } from '@/common/types'
import type { DetailDialogHandle, DetailWarningDialogOptions } from './resourceDetailController'
import { createVolumeDetailController } from './volumeDetailController'

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

function makeVolumeDetail(name: string, refCount = 0): VolumeDetailResponse {
  return {
    volume: {
      name,
      driver: 'local',
      mountpoint: `/var/lib/docker/volumes/${name}/_data`,
      createdAt: '2026-11-26T00:00:00Z',
      scope: 'local',
      status: {},
      labels: {},
      options: {},
      usageData: {
        size: 1024,
        refCount,
      },
    },
    containers: [],
  }
}

function okRes<T>(data: T): IResData<T> {
  return { code: 0, msg: 'ok', data }
}

function errRes<T>(msg: string): IResData<T> {
  return { code: 500, msg, data: null as unknown as T }
}

function setupVolumeHarness() {
  const currentName = ref('vol-a')
  const pendingFetches: Array<{ name: string; deferred: Deferred<IResData<VolumeDetailResponse>> }> =
    []
  const deleteCalls: Array<{ name: string; force: boolean }> = []
  const deleteMock = vi.fn(async (name: string, force: boolean) => {
    deleteCalls.push({ name, force })
    return okRes({ ok: true })
  })
  const openedDialogs: Array<{
    options: DetailWarningDialogOptions
    handle: DetailDialogHandle
    destroySpy: ReturnType<typeof vi.fn>
  }> = []
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const navigateToList = vi.fn()

  const controller = createVolumeDetailController({
    getVolumeName: () => currentName.value,
    fetchVolume: (name) => {
      const deferred = createDeferred<IResData<VolumeDetailResponse>>()
      pendingFetches.push({ name, deferred })
      return deferred.promise
    },
    deleteVolume: deleteMock,
    openWarningDialog: (options) => {
      const destroySpy = vi.fn()
      const handle: DetailDialogHandle = { loading: false, destroy: destroySpy }
      openedDialogs.push({ options, handle, destroySpy })
      return handle
    },
    onSuccess,
    onError,
    navigateToList,
  })

  const switchRoute = (nextName: string) => {
    const prev = currentName.value
    currentName.value = nextName
    controller.handleRouteChange(nextName, prev)
  }

  return {
    currentName,
    pendingFetches,
    deleteCalls,
    deleteMock,
    openedDialogs,
    onSuccess,
    onError,
    navigateToList,
    switchRoute,
    controller,
  }
}

describe('VolumeDetailView stale response & delete confirmation protection', () => {
  it('keeps B detail when B resolves before A resolves', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupVolumeHarness()

    const detailA = makeVolumeDetail('vol-a')
    const detailB = makeVolumeDetail('vol-b')

    const loadA = controller.fetchVolumeDetail()
    switchRoute('vol-b')
    await Promise.resolve()

    expect(pendingFetches).toHaveLength(2)
    expect(controller.loading.value).toBe(true)

    pendingFetches[1].deferred.resolve(okRes(detailB))
    await Promise.resolve()
    await Promise.resolve()

    expect(controller.loading.value).toBe(false)
    expect(controller.isCurrentVolumeDetail.value).toBe(true)
    expect(controller.canOperate.value).toBe(true)
    expect(controller.volumeDetail.value).toEqual(detailB)

    // A 迟到成功：不得覆盖 B 的详情
    pendingFetches[0].deferred.resolve(okRes(detailA))
    await loadA

    expect(controller.volumeDetail.value).toEqual(detailB)
    expect(controller.loadedVolumeName.value).toBe('vol-b')
    expect(controller.loading.value).toBe(false)
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()
  })

  it('keeps B detail and suppresses A error navigation when B succeeds before A rejects', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupVolumeHarness()

    const detailB = makeVolumeDetail('vol-b')

    const loadA = controller.fetchVolumeDetail()
    switchRoute('vol-b')
    pendingFetches[1].deferred.resolve(okRes(detailB))
    await Promise.resolve()
    await Promise.resolve()

    pendingFetches[0].deferred.reject(new Error('volume A vanished'))
    await loadA

    expect(controller.volumeDetail.value).toEqual(detailB)
    expect(controller.loadedVolumeName.value).toBe('vol-b')
    expect(controller.loading.value).toBe(false)
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()

    const loadB2 = controller.fetchVolumeDetail()
    switchRoute('vol-c')
    pendingFetches[3].deferred.resolve(okRes(makeVolumeDetail('vol-c')))
    await Promise.resolve()
    await Promise.resolve()

    pendingFetches[2].deferred.resolve(errRes('stale error code'))
    await loadB2

    expect(controller.volumeDetail.value?.volume.name).toBe('vol-c')
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()
  })

  it('keeps B loading while A finishes early and blocks delete until B completes', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupVolumeHarness()

    const loadA = controller.fetchVolumeDetail()
    switchRoute('vol-b')

    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)
    expect(controller.volumeDetail.value).toBeNull()

    pendingFetches[0].deferred.resolve(okRes(makeVolumeDetail('vol-a')))
    await loadA

    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)
    expect(controller.volumeDetail.value).toBeNull()
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()

    // B 完成后再切换至 C，并让旧的 B 失败提前返回，C 仍需保持加载态
    pendingFetches[1].deferred.resolve(okRes(makeVolumeDetail('vol-b')))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.loading.value).toBe(false)
    expect(controller.canOperate.value).toBe(true)

    const refreshB = controller.fetchVolumeDetail()
    switchRoute('vol-c')
    expect(controller.volumeDetail.value).toBeNull()
    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)

    pendingFetches[2].deferred.reject(new Error('stale B failed'))
    await refreshB

    expect(controller.loading.value).toBe(true)
    expect(controller.volumeDetail.value).toBeNull()
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()

    const detailC = makeVolumeDetail('vol-c')
    pendingFetches[3].deferred.resolve(okRes(detailC))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.loading.value).toBe(false)
    expect(controller.volumeDetail.value).toEqual(detailC)
    expect(controller.canOperate.value).toBe(true)
  })

  it('destroys stale A confirmation dialog on route switch and refuses to delete B or stale A', async () => {
    const {
      pendingFetches,
      deleteCalls,
      deleteMock,
      openedDialogs,
      onSuccess,
      navigateToList,
      switchRoute,
      controller,
    } = setupVolumeHarness()

    const loadA = controller.fetchVolumeDetail()
    pendingFetches[0].deferred.resolve(okRes(makeVolumeDetail('vol-a')))
    await loadA

    controller.handleDelete()
    expect(openedDialogs).toHaveLength(1)
    expect(openedDialogs[0].options.content).toContain('vol-a')

    switchRoute('vol-b')
    expect(openedDialogs[0].destroySpy).toHaveBeenCalledTimes(1)
    expect(controller.volumeDetail.value).toBeNull()
    expect(controller.canOperate.value).toBe(false)

    await openedDialogs[0].options.onPositiveClick?.()
    expect(deleteMock).not.toHaveBeenCalled()

    pendingFetches[1].deferred.resolve(okRes(makeVolumeDetail('vol-b')))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.volumeDetail.value?.volume.name).toBe('vol-b')

    await openedDialogs[0].options.onPositiveClick?.()
    expect(deleteMock).not.toHaveBeenCalled()

    controller.handleDelete()
    expect(openedDialogs).toHaveLength(2)
    expect(openedDialogs[1].options.content).toContain('vol-b')

    await openedDialogs[1].options.onPositiveClick?.()
    expect(deleteCalls).toEqual([{ name: 'vol-b', force: false }])
    expect(onSuccess).toHaveBeenCalledWith('Volume 删除成功')
    expect(navigateToList).toHaveBeenCalledTimes(1)
  })
})
