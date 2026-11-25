import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getImagesMock, fetchContainersMock, containerLoadingRef, containersRef } = vi.hoisted(
  () => ({
    getImagesMock: vi.fn(),
    fetchContainersMock: vi.fn(),
    containerLoadingRef: { value: false },
    containersRef: { value: [] as any[] },
  }),
)

vi.mock('@/common/api', () => ({
  imageApi: {
    getImages: getImagesMock,
  },
}))

vi.mock('@/common/axiosConfig', () => ({
  getToken: () => '',
}))

vi.mock('@/store/container', () => ({
  useContainerStore: () => ({
    get loading() {
      return containerLoadingRef.value
    },
    get containers() {
      return containersRef.value
    },
    fetchContainers: fetchContainersMock,
  }),
}))

import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { loadImagesViewOnMount } from './imagesViewMount'

describe('ImagesView re-entry refresh and imageStore deduplication', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getImagesMock.mockReset()
    fetchContainersMock.mockReset()
    containerLoadingRef.value = false
    containersRef.value = []
  })

  it('refreshes image list even when existing images are cached in store', async () => {
    const imageStore = useImageStore()
    const containerStore = useContainerStore()

    imageStore.images = [
      { id: 'sha256:old', repoTags: ['alpine:3.19'], repoDigests: [], size: 100, created: 1 },
    ]
    containersRef.value = [{ id: 'c1', name: 'web', image: 'alpine:3.19' }]

    getImagesMock.mockResolvedValueOnce({
      code: 0,
      msg: 'ok',
      data: {
        images: [
          {
            id: 'sha256:old',
            repoTags: ['alpine:3.19', 'alpine:newtag'],
            repoDigests: [],
            size: 100,
            created: 1,
          },
        ],
      },
    })

    const onImagesError = vi.fn()
    await loadImagesViewOnMount({
      fetchImages: () => imageStore.fetchImages(),
      fetchContainers: () => containerStore.fetchContainers(),
      hasContainerWork: () => !containerStore.loading && containerStore.containers.length === 0,
      onImagesError,
    })

    expect(getImagesMock).toHaveBeenCalledTimes(1)
    expect(fetchContainersMock).not.toHaveBeenCalled()
    expect(onImagesError).not.toHaveBeenCalled()
    expect(imageStore.images[0].repoTags).toEqual(['alpine:3.19', 'alpine:newtag'])
  })

  it('reuses in-flight fetchImages across rapid re-entry and retains existing cards + loads containers when image fetch fails', async () => {
    const imageStore = useImageStore()
    const containerStore = useContainerStore()

    const existing = [
      { id: 'sha256:keep', repoTags: ['busybox:latest'], repoDigests: [], size: 50, created: 1 },
    ]
    imageStore.images = existing

    let rejectImages!: (err: Error) => void
    getImagesMock.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectImages = reject
        }),
    )
    fetchContainersMock.mockImplementationOnce(async () => {
      containersRef.value = [{ id: 'c2', name: 'box', image: 'busybox:latest' }]
    })

    const onErr1 = vi.fn()
    const onErr2 = vi.fn()

    const mount1 = loadImagesViewOnMount({
      fetchImages: () => imageStore.fetchImages(),
      fetchContainers: () => containerStore.fetchContainers(),
      hasContainerWork: () => !containerStore.loading && containerStore.containers.length === 0,
      onImagesError: onErr1,
    })
    const mount2 = loadImagesViewOnMount({
      fetchImages: () => imageStore.fetchImages(),
      fetchContainers: () => containerStore.fetchContainers(),
      hasContainerWork: () => !containerStore.loading && containerStore.containers.length === 0,
      onImagesError: onErr2,
    })

    expect(getImagesMock).toHaveBeenCalledTimes(1)
    expect(imageStore.images).toEqual(existing)

    rejectImages(new Error('network down'))
    await Promise.all([mount1, mount2])

    expect(imageStore.images).toEqual(existing)
    expect(onErr1).toHaveBeenCalledWith('network down')
    expect(onErr2).toHaveBeenCalledWith('network down')
    expect(fetchContainersMock).toHaveBeenCalledTimes(1)
    expect(containerStore.containers).toHaveLength(1)
  })
})
