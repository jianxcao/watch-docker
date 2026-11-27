import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createContainerDetailController } from './containerDetailController'

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

function makeDetail(id: string, name: string, running: boolean) {
  return {
    Id: id,
    Name: `/${name}`,
    Config: {
      Image: `example/${name}:latest`,
      Labels: {},
    },
    State: {
      Running: running,
      StartedAt: '2026-11-26T00:00:00Z',
    },
  }
}

function setupHarness() {
  const currentId = ref('container-a')
  const pendingRequests: Array<{ id: string; deferred: Deferred<any> }> = []
  const onLoadError = vi.fn()

  const controller = createContainerDetailController({
    getContainerId: () => currentId.value,
    fetchContainerDetail: (id) => {
      const deferred = createDeferred<any>()
      pendingRequests.push({ id, deferred })
      return deferred.promise
    },
    onLoadError,
  })

  return {
    currentId,
    pendingRequests,
    onLoadError,
    controller,
  }
}

describe('ContainerDetailView stale response protection', () => {
  it('keeps B detail when B resolves before A resolves', async () => {
    const { currentId, pendingRequests, onLoadError, controller } = setupHarness()
    const detailA = makeDetail('container-a', 'service-a', false)
    const detailB = makeDetail('container-b', 'service-b', true)

    const loadA = controller.loadContainerDetail()
    currentId.value = 'container-b'
    const loadB = controller.loadContainerDetail()
    expect(pendingRequests).toHaveLength(2)

    pendingRequests[1].deferred.resolve(detailB)
    await loadB

    expect(controller.loading.value).toBe(false)
    expect(controller.isCurrentContainerDetail.value).toBe(true)
    expect(controller.canOperateMenu.value).toBe(true)
    expect(controller.containerDetail.value).toEqual(detailB)
    expect(controller.containerName.value).toBe('service-b')

    // A 迟到成功：不得覆盖 B 的名称、状态或详情
    pendingRequests[0].deferred.resolve(detailA)
    await loadA

    expect(controller.containerDetail.value).toEqual(detailB)
    expect(controller.containerName.value).toBe('service-b')
    expect(controller.loading.value).toBe(false)
    expect(onLoadError).not.toHaveBeenCalled()
    expect(controller.resolveActionContainer(() => undefined)?.id).toBe('container-b')
  })

  it('keeps B detail and suppresses A error toast when B succeeds before A rejects', async () => {
    const { currentId, pendingRequests, onLoadError, controller } = setupHarness()
    const detailB = makeDetail('container-b', 'service-b', true)

    const loadA = controller.loadContainerDetail()
    currentId.value = 'container-b'
    const loadB = controller.loadContainerDetail()

    pendingRequests[1].deferred.resolve(detailB)
    await loadB

    // A 迟到失败：不得清空 B 的详情，也不得弹出 A 的错误提示
    pendingRequests[0].deferred.reject(new Error('container A vanished'))
    await loadA

    expect(controller.containerDetail.value).toEqual(detailB)
    expect(controller.containerName.value).toBe('service-b')
    expect(controller.loading.value).toBe(false)
    expect(controller.canOperateMenu.value).toBe(true)
    expect(onLoadError).not.toHaveBeenCalled()
    expect(controller.resolveActionContainer(() => undefined)?.id).toBe('container-b')
  })

  it('keeps loading spinner until B completes when A finishes first (success or error)', async () => {
    const { currentId, pendingRequests, onLoadError, controller } = setupHarness()
    const detailA = makeDetail('container-a', 'service-a', true)
    const detailB = makeDetail('container-b', 'service-b', true)

    const loadA = controller.loadContainerDetail()
    currentId.value = 'container-b'
    const loadB = controller.loadContainerDetail()
    expect(controller.loading.value).toBe(true)
    expect(controller.canOperateMenu.value).toBe(false)
    expect(controller.resolveActionContainer(() => undefined)).toBeNull()

    // B 仍在等待时 A 先完成：loading 必须保持 true 且不允许菜单操作
    pendingRequests[0].deferred.resolve(detailA)
    await loadA

    expect(controller.loading.value).toBe(true)
    expect(controller.containerDetail.value).toBeNull()
    expect(controller.canOperateMenu.value).toBe(false)
    expect(controller.resolveActionContainer(() => undefined)).toBeNull()
    expect(onLoadError).not.toHaveBeenCalled()

    // 再测试 A 先失败的情况（新切换到 container-c），同样不能提前结束 loading
    pendingRequests[1].deferred.resolve(detailB)
    await loadB
    expect(controller.loading.value).toBe(false)
    expect(controller.resolveActionContainer(() => undefined)?.id).toBe('container-b')

    const loadB2 = controller.loadContainerDetail()
    currentId.value = 'container-c'
    const loadC = controller.loadContainerDetail()
    pendingRequests[2].deferred.reject(new Error('stale B error'))
    await loadB2

    expect(controller.loading.value).toBe(true)
    expect(onLoadError).not.toHaveBeenCalled()

    const detailC = makeDetail('container-c', 'service-c', false)
    pendingRequests[3].deferred.resolve(detailC)
    await loadC
    expect(controller.loading.value).toBe(false)
    expect(controller.containerDetail.value).toEqual(detailC)
    expect(controller.resolveActionContainer(() => undefined)?.id).toBe('container-c')
  })
})
