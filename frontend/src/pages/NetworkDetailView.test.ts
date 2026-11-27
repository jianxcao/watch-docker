import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { IResData } from '@/common/axiosConfig'
import type { NetworkDetailResponse } from '@/common/types'
import { createNetworkDetailController } from './networkDetailController'
import type { DetailDialogHandle, DetailWarningDialogOptions } from './resourceDetailController'

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

function makeNetworkDetail(id: string, name: string, containerCount = 0): NetworkDetailResponse {
  return {
    network: {
      id,
      name,
      driver: 'bridge',
      scope: 'local',
      internal: false,
      attachable: true,
      ingress: false,
      enableIPv6: false,
      ipam: { driver: 'default' },
      created: '2026-11-26T00:00:00Z',
      labels: {},
      options: {},
      containerCount,
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

function setupNetworkHarness() {
  const currentId = ref('net-a')
  const pendingFetches: Array<{ id: string; deferred: Deferred<IResData<NetworkDetailResponse>> }> =
    []
  const deleteCalls: string[] = []
  const deleteMock = vi.fn(async (id: string) => {
    deleteCalls.push(id)
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

  const controller = createNetworkDetailController({
    getNetworkId: () => currentId.value,
    fetchNetwork: (id) => {
      const deferred = createDeferred<IResData<NetworkDetailResponse>>()
      pendingFetches.push({ id, deferred })
      return deferred.promise
    },
    deleteNetwork: deleteMock,
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

  const switchRoute = (nextId: string) => {
    const prev = currentId.value
    currentId.value = nextId
    controller.handleRouteChange(nextId, prev)
  }

  return {
    currentId,
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

describe('NetworkDetailView stale response & delete confirmation protection', () => {
  it('keeps B detail when B resolves before A resolves', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupNetworkHarness()

    const detailA = makeNetworkDetail('id-a', 'net-a')
    const detailB = makeNetworkDetail('id-b', 'net-b')

    const loadA = controller.fetchNetworkDetail()
    switchRoute('net-b')
    await Promise.resolve()

    expect(pendingFetches).toHaveLength(2)
    expect(controller.loading.value).toBe(true)

    pendingFetches[1].deferred.resolve(okRes(detailB))
    await Promise.resolve()
    await Promise.resolve()

    expect(controller.loading.value).toBe(false)
    expect(controller.isCurrentNetworkDetail.value).toBe(true)
    expect(controller.canOperate.value).toBe(true)
    expect(controller.networkDetail.value).toEqual(detailB)

    // A 迟到成功：不能覆盖 B 的详情，也不能重置加载/跳转状态
    pendingFetches[0].deferred.resolve(okRes(detailA))
    await loadA

    expect(controller.networkDetail.value).toEqual(detailB)
    expect(controller.loadedNetworkId.value).toBe('net-b')
    expect(controller.loading.value).toBe(false)
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()
  })

  it('keeps B detail and suppresses A error navigation when B succeeds before A fails', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupNetworkHarness()

    const detailB = makeNetworkDetail('id-b', 'net-b')

    const loadA = controller.fetchNetworkDetail()
    switchRoute('net-b')
    pendingFetches[1].deferred.resolve(okRes(detailB))
    await Promise.resolve()
    await Promise.resolve()

    // 迟到的 A 失败（Promise reject）：不能弹错、清空 B 或跳回列表
    pendingFetches[0].deferred.reject(new Error('network A disappeared'))
    await loadA

    expect(controller.networkDetail.value).toEqual(detailB)
    expect(controller.loadedNetworkId.value).toBe('net-b')
    expect(controller.loading.value).toBe(false)
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()

    // 迟到的非零 code 响应同样忽略
    const loadB2 = controller.fetchNetworkDetail()
    switchRoute('net-c')
    pendingFetches[3].deferred.resolve(okRes(makeNetworkDetail('id-c', 'net-c')))
    await Promise.resolve()
    await Promise.resolve()

    pendingFetches[2].deferred.resolve(errRes('stale non-zero error'))
    await loadB2

    expect(controller.networkDetail.value?.network.name).toBe('net-c')
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()
  })

  it('keeps B loading while A finishes early and blocks operations until B completes', async () => {
    const {
      pendingFetches,
      onError,
      navigateToList,
      switchRoute,
      controller,
    } = setupNetworkHarness()

    const loadA = controller.fetchNetworkDetail()
    switchRoute('net-b')

    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)
    expect(controller.networkDetail.value).toBeNull()

    // A 在 B 仍在加载时提前失败：不能提前结束 loading，也不能跳回列表
    pendingFetches[0].deferred.reject(new Error('early A error'))
    await loadA

    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)
    expect(controller.networkDetail.value).toBeNull()
    expect(onError).not.toHaveBeenCalled()
    expect(navigateToList).not.toHaveBeenCalled()

    // 同样地，C 加载时先加载完 B 后又快速切 C 期间提前返回成功也不得提前关闭 C 的 loading
    const detailB = makeNetworkDetail('id-b', 'net-b')
    pendingFetches[1].deferred.resolve(okRes(detailB))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.loading.value).toBe(false)
    expect(controller.canOperate.value).toBe(true)

    const refreshB = controller.fetchNetworkDetail()
    switchRoute('net-c')
    expect(controller.networkDetail.value).toBeNull()
    expect(controller.loading.value).toBe(true)
    expect(controller.canOperate.value).toBe(false)

    pendingFetches[2].deferred.resolve(okRes(detailB))
    await refreshB
    expect(controller.loading.value).toBe(true)
    expect(controller.networkDetail.value).toBeNull()
    expect(controller.canOperate.value).toBe(false)

    const detailC = makeNetworkDetail('id-c', 'net-c')
    pendingFetches[3].deferred.resolve(okRes(detailC))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.loading.value).toBe(false)
    expect(controller.networkDetail.value).toEqual(detailC)
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
    } = setupNetworkHarness()

    const loadA = controller.fetchNetworkDetail()
    pendingFetches[0].deferred.resolve(okRes(makeNetworkDetail('net-a', 'network-alpha')))
    await loadA

    // 在 A 打开删除确认弹窗，弹窗标题应展示 A 的名称
    controller.handleDelete()
    expect(openedDialogs).toHaveLength(1)
    expect(openedDialogs[0].options.content).toContain('network-alpha')

    // 切换到 B（先处于 B 加载中，再让 B 加载完成）
    switchRoute('net-b')
    expect(openedDialogs[0].destroySpy).toHaveBeenCalledTimes(1)
    expect(controller.networkDetail.value).toBeNull()
    expect(controller.canOperate.value).toBe(false)

    // B 加载前点击旧弹窗确认：绝不能发起删除
    await openedDialogs[0].options.onPositiveClick?.()
    expect(deleteMock).not.toHaveBeenCalled()

    // 让 B 加载完成并再点一次旧弹窗确认：仍绝不能删除 B 或 A
    pendingFetches[1].deferred.resolve(okRes(makeNetworkDetail('net-b', 'network-beta')))
    await Promise.resolve()
    await Promise.resolve()
    expect(controller.networkDetail.value?.network.name).toBe('network-beta')

    await openedDialogs[0].options.onPositiveClick?.()
    expect(deleteMock).not.toHaveBeenCalled()

    // 在 B 页面重新点击删除并确认：正确显示 B 并只删除 net-b
    controller.handleDelete()
    expect(openedDialogs).toHaveLength(2)
    expect(openedDialogs[1].options.content).toContain('network-beta')

    await openedDialogs[1].options.onPositiveClick?.()
    expect(deleteCalls).toEqual(['net-b'])
    expect(onSuccess).toHaveBeenCalledWith('网络删除成功')
    expect(navigateToList).toHaveBeenCalledTimes(1)
  })
})
