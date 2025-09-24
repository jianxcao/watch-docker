import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { imageApi } from '@/common/api'
import type { ImageInfo } from '@/common/types'
import { formatSize } from '@/common/utils'
export const useImageStore = defineStore('image', () => {
  // 状态
  const images = ref<ImageInfo[]>([])
  const loading = ref(false)
  const deleting = ref(new Set<string>())

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
  const formattedTotalSize = computed(() => formatSize(totalSize.value))

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
  const getDigestDisplayText = (image: ImageInfo): string => {
    // 优先显示 repoDigests 中的第一个摘要
    if (image.repoDigests && image.repoDigests.length > 0) {
      const digest = image.repoDigests[0]
      if (digest.includes('@sha256:')) {
        // 提取 sha256 部分并截断显示
        const sha256Part = digest.split('@sha256:')[1]
        return sha256Part.slice(0, 12) + '...'
      }
      return digest.slice(0, 12) + '...'
    }

    // 如果没有摘要，显示镜像 ID
    if (image.id.startsWith('sha256:')) {
      return image.id.slice(7, 19) + '...'
    }
    return image.id.slice(0, 12) + '...'
  }

  return {
    // 状态
    images,
    normalImages,
    loading,
    deleting,

    // 计算属性
    totalImages,
    totalSize,
    formattedTotalSize,
    stats,
    danglingImages,

    // 方法
    fetchImages,
    deleteImage,
    findImageById,
    findImagesByTag,
    isImageDeleting,
    getImageDisplayTag,
    getDigestDisplayText,
    isDanglingImage,
    formatSize,
  }
})
