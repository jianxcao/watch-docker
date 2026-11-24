import { imageApi } from '@/common/api'
import { doesContainerUseImage } from '@/common/imageRef'
import type { ContainerStatus, ImageInfo } from '@/common/types'
import { formatBytes } from '@/common/utils'
import { getToken } from '@/common/axiosConfig'
import { useContainerStore } from '@/store/container'
import { buildImageDownloadUrl, triggerBrowserDownload } from '@/utils/download'
import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
export const useImageStore = defineStore('image', () => {
  const containerStore = useContainerStore()
  // 状态
  const images = ref<ImageInfo[]>([])
  const loading = ref(false)
  const deleting = ref(new Set<string>())
  const downloading = ref(new Set<string>())
  const importing = ref(false)
  let inFlightFetch: Promise<void> | null = null
  let latestFetchSeq = 0

  // 方法：检查镜像是否为悬空镜像（dangling image）
  const isDanglingImage = (image: ImageInfo): boolean => {
    return (
      !image.repoTags ||
      image.repoTags.length === 0 ||
      image.repoTags.every((tag) => tag === '<none>:<none>')
    )
  }

  const normalImages = computed(() => images.value.filter((image) => !isDanglingImage(image)))

  // 计算属性
  const totalImages = computed(() => normalImages.value.length)

  const totalSize = computed(() => normalImages.value.reduce((sum, img) => sum + img.size, 0))

  // 格式化的总大小
  const formattedTotalSize = computed(() => formatBytes(totalSize.value))

  // 统计信息
  const stats = computed(() => ({
    total: totalImages.value,
    totalSize: totalSize.value,
    formattedTotalSize: formattedTotalSize.value,
  }))

  // 仅在容器身份或镜像引用发生变化时刷新列表，避免每秒 stats 心跳引发所有 ImageCard 重计算
  const stableContainerRefs = ref<ContainerStatus[]>([])
  watch(
    () =>
      containerStore.containers
        .map((c) => `${c.id}|${c.name}|${c.image}|${c.imageID || ''}`)
        .join('\n'),
    () => {
      stableContainerRefs.value = containerStore.containers.slice()
    },
    { immediate: true },
  )

  const imageUsageMap = computed(() => {
    const map = new Map<string, ContainerStatus[]>()
    const containers = stableContainerRefs.value
    for (const image of images.value) {
      map.set(
        image.id,
        containers.filter((ct) => doesContainerUseImage(ct, image)),
      )
    }
    return map
  })

  // 方法：获取镜像列表（非强制时并发去重；force=true 强制发起新请求且避免旧响应覆盖新结果）
  const fetchImages = async (force = false) => {
    if (!force && inFlightFetch) {
      return inFlightFetch
    }
    const seq = ++latestFetchSeq
    loading.value = true
    const reqPromise: Promise<void> = (async () => {
      try {
        const data = await imageApi.getImages()
        if (data.code === 0) {
          if (seq === latestFetchSeq) {
            images.value = data.data.images
          }
        } else {
          console.error('获取镜像列表失败:', data.msg)
          throw new Error(data.msg)
        }
      } catch (error) {
        console.error('获取镜像列表失败:', error)
        throw error
      }
    })().finally(() => {
      if (seq === latestFetchSeq) {
        loading.value = false
      }
      if (inFlightFetch === reqPromise) {
        inFlightFetch = null
      }
    })
    inFlightFetch = reqPromise
    return reqPromise
  }

  // 方法：删除镜像
  const deleteImage = async (ref: string, force: boolean = false): Promise<boolean> => {
    deleting.value.add(ref)
    try {
      const data = await imageApi.deleteImage(ref, force)
      if (data.code === 0) {
        await fetchImages(true) // 重新获取列表
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('删除镜像失败:', error)
      throw error
    } finally {
      deleting.value.delete(ref)
    }
  }

  // 方法：根据ID查找镜像
  const findImageById = (id: string) => {
    return images.value.find((img) => img.id === id)
  }

  // 方法：根据标签查找镜像
  const findImagesByTag = (tag: string) => {
    return images.value.filter((img) => img.repoTags.some((repoTag) => repoTag.includes(tag)))
  }

  // 方法：检查镜像是否正在删除
  const isImageDeleting = (ref: string) => {
    return deleting.value.has(ref)
  }

  // 方法：检查镜像是否正在下载
  const isImageDownloading = (ref: string) => {
    return downloading.value.has(ref)
  }

  // 方法：下载镜像
  const downloadImage = async (id: string): Promise<boolean> => {
    downloading.value.add(id)
    try {
      const token = getToken()

      triggerBrowserDownload(buildImageDownloadUrl(id, token))

      // 设置一个短暂的延迟来清除下载状态
      setTimeout(() => {
        downloading.value.delete(id)
      }, 1000)

      return true
    } catch (error: any) {
      console.error('下载镜像失败:', error)
      downloading.value.delete(id)
      throw error
    }
  }

  // 方法：导入镜像
  const importImage = async (file: File): Promise<boolean> => {
    importing.value = true
    try {
      const response = await imageApi.importImage(file)
      if (response.code === 0) {
        await fetchImages(true) // 重新获取镜像列表
        return true
      } else {
        throw new Error(response.msg || '导入失败')
      }
    } catch (error: any) {
      console.error('导入镜像失败:', error)
      if (error.response?.data?.error) {
        throw new Error(error.response.data.error)
      }
      throw error
    } finally {
      importing.value = false
    }
  }

  // 方法：获取镜像的主要标签（用于显示）
  const getImageDisplayTag = (image: ImageInfo): string => {
    if (image.repoTags && image.repoTags.length > 0) {
      // 过滤掉 <none>:<none> 标签
      const validTags = image.repoTags.filter((tag) => tag !== '<none>:<none>')
      if (validTags.length > 0) {
        return validTags[0]
      }
    }
    // 如果没有有效标签，返回短ID
    return image.id.startsWith('sha256:') ? image.id.slice(7, 19) : image.id.slice(0, 12)
  }

  // 计算悬空镜像
  const danglingImages = computed(() => images.value.filter((img) => isDanglingImage(img)))

  // 方法：获取镜像的摘要显示文本（用于显示）
  const getDisplayId = (image: ImageInfo): string => {
    // 如果没有摘要，显示镜像 ID
    if (image.id.startsWith('sha256:')) {
      return image.id.slice(7, 19)
    }
    return image.id.slice(0, 12)
  }

  return {
    // 状态
    images,
    normalImages,
    loading,
    deleting,
    downloading,
    importing,

    // 计算属性
    totalImages,
    totalSize,
    formattedTotalSize,
    stats,
    danglingImages,
    imageUsageMap,

    // 方法
    fetchImages,
    deleteImage,
    downloadImage,
    importImage,
    findImageById,
    findImagesByTag,
    isImageDeleting,
    isImageDownloading,
    getImageDisplayTag,
    getDisplayId,
    isDanglingImage,
  }
})
