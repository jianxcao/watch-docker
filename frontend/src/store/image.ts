import { imageApi } from '@/common/api'
import type { ImageInfo } from '@/common/types'
import { formatBytes } from '@/common/utils'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
export const useImageStore = defineStore('image', () => {
  // 状态
  const images = ref<ImageInfo[]>([])
  const loading = ref(false)
  const deleting = ref(new Set<string>())
  const downloading = ref(new Set<string>())

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

  // 方法：获取镜像列表
  const fetchImages = async () => {
    loading.value = true
    try {
      const data = await imageApi.getImages()
      if (data.code === 0) {
        images.value = data.data.images
      } else {
        console.error('获取镜像列表失败:', data.msg)
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('获取镜像列表失败:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  // 方法：定时获取镜像列表
  const { pause: stopImagesPolling, resume: startImagesPolling } = useIntervalFn(
    fetchImages,
    10000,
    {
      immediate: false,
    }
  )

  // 方法：删除镜像
  const deleteImage = async (ref: string, force: boolean = false): Promise<boolean> => {
    deleting.value.add(ref)
    try {
      const data = await imageApi.deleteImage(ref, force)
      if (data.code === 0) {
        await fetchImages() // 重新获取列表
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
      // 获取当前的token
      const { getToken } = await import('@/common/axiosConfig')
      const token = getToken()

      // 构建下载URL，将token作为查询参数
      const baseUrl = '/api/v1'
      const downloadUrl = `${baseUrl}/images/${id}/download?token=${encodeURIComponent(
        token
      )}&_t=${Date.now()}`

      // 使用window.open直接下载，浏览器会处理大文件和进度
      const downloadWindow = window.open(downloadUrl, '_blank')

      // 检查是否成功打开了下载窗口
      if (!downloadWindow) {
        throw new Error('浏览器阻止了弹窗，请允许弹窗后重试')
      }

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

    // 计算属性
    totalImages,
    totalSize,
    formattedTotalSize,
    stats,
    danglingImages,

    // 方法
    fetchImages,
    deleteImage,
    downloadImage,
    findImageById,
    findImagesByTag,
    isImageDeleting,
    isImageDownloading,
    getImageDisplayTag,
    getDisplayId,
    isDanglingImage,
    stopImagesPolling,
    startImagesPolling,
  }
})
