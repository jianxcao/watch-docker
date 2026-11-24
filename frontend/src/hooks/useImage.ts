import { useImageStore } from '@/store/image'
import { parseImageRef } from '@/common/imageRef'
import { useMessage, useDialog } from 'naive-ui'
import { h, ref } from 'vue'
import { NCheckbox, NSpace, NText } from 'naive-ui'
import type { ImageInfo, ContainerStatus } from '@/common/types'

export function useImage() {
  const store = useImageStore()
  const message = useMessage()
  const dialog = useDialog()

  // 删除镜像（带确认）
  const handleDelete = (image: ImageInfo) => {
    const displayTag = store.getImageDisplayTag(image)
    const isDangling = store.isDanglingImage(image)
    const isInUse = isImageInUse(image)
    const usingContainers = getContainersUsingImage(image)

    // 创建响应式的强制删除选项，如果镜像正在被使用则默认选中
    const forceDeleteRef = ref(isInUse)

    let title: string
    let contentText: string

    if (isDangling) {
      title = '确认删除悬空镜像'
      contentText = `确定要删除悬空镜像 "${displayTag}" 吗？`
    } else {
      title = '确认删除镜像'
      if (isInUse) {
        const containerList = usingContainers.map((c) => `• ${c.name}`).join('\n')
        contentText = `镜像 "${displayTag}" 正在被以下容器使用：\n\n${containerList}\n\n删除此镜像可能会影响这些容器的运行。`
      } else {
        contentText = `确定要删除镜像 "${displayTag}" 吗？`
      }
    }

    // 创建自定义对话框内容
    const content = () =>
      h(NSpace, { vertical: true }, () =>
        [
          h(NText, {}, () => contentText),
          // 只有在非悬空镜像时才显示强制删除选项
          !isDangling &&
            h(
              NCheckbox,
              {
                checked: forceDeleteRef.value,
                'onUpdate:checked': (checked: boolean) => {
                  forceDeleteRef.value = checked
                },
              },
              () => '强制删除',
            ),
        ].filter(Boolean),
      )

    const d = dialog.warning({
      title,
      content,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          d.loading = true
          const shouldForce = isDangling ? false : forceDeleteRef.value
          await store.deleteImage(image.id, shouldForce)
          message.success(`镜像 ${displayTag} 删除成功`)
          d.loading = false
        } catch (error: any) {
          d.loading = false
          message.error(`删除镜像失败: ${error.message}`)
        }
      },
    })
  }

  // 批量删除悬空镜像
  const handleDeleteDangling = () => {
    const danglingImages = store.danglingImages

    if (danglingImages.length === 0) {
      message.info('当前没有悬空镜像')
      return
    }

    dialog.warning({
      title: '批量删除悬空镜像',
      content: `发现 ${danglingImages.length} 个悬空镜像，确定要全部删除吗？`,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        let successCount = 0
        let failCount = 0

        for (const image of danglingImages) {
          try {
            await store.deleteImage(image.id, false)
            successCount++
          } catch (error) {
            failCount++
            console.error(`删除悬空镜像 ${image.id} 失败:`, error)
          }
        }

        if (successCount > 0) {
          message.success(`成功删除 ${successCount} 个悬空镜像`)
        }

        if (failCount > 0) {
          message.warning(`${failCount} 个悬空镜像删除失败`)
        }
      },
    })
  }

  // 刷新镜像列表
  const handleRefresh = async () => {
    try {
      await store.fetchImages()
      message.success('镜像列表刷新成功')
    } catch (error: any) {
      message.error(`刷新失败: ${error.message}`)
    }
  }

  // 下载镜像
  const handleDownload = async (image: ImageInfo) => {
    const displayName = getImageNameOnly(image)

    try {
      await store.downloadImage(image.id)
      message.success(`镜像 ${displayName} 已开始下载`)
    } catch (error: any) {
      message.error(`下载镜像失败: ${error.message}`)
    }
  }

  // 导入镜像
  const handleImport = async (file: File) => {
    try {
      await store.importImage(file)
      message.success(`镜像 ${file.name} 导入成功`)
    } catch (error: any) {
      message.error(`导入镜像失败: ${error.message}`)
    }
  }

  // 格式化创建时间
  const formatCreateTime = (timestamp: number): string => {
    const date = new Date(timestamp * 1000)
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  // 获取镜像年龄（多久前创建）
  const getImageAge = (timestamp: number): string => {
    const now = Date.now()
    const createTime = timestamp * 1000
    const diff = now - createTime

    const seconds = Math.floor(diff / 1000)
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)
    const days = Math.floor(hours / 24)
    const months = Math.floor(days / 30)
    const years = Math.floor(days / 365)

    if (years > 0) {
      return `${years}年前`
    }
    if (months > 0) {
      return `${months}个月前`
    }
    if (days > 0) {
      return `${days}天前`
    }
    if (hours > 0) {
      return `${hours}小时前`
    }
    if (minutes > 0) {
      return `${minutes}分钟前`
    }
    return '刚刚'
  }

  // 获取镜像版本的显示文本（兼容 registry:port/repo:tag）
  const getVersionDisplayText = (image: ImageInfo): string => {
    if (!image.repoTags || image.repoTags.length === 0) {
      return '<none>'
    }

    const validTags = image.repoTags.filter((tag) => tag !== '<none>:<none>')
    if (validTags.length === 0) {
      return '<none>'
    }

    const parsed = parseImageRef(validTags[0])
    const version = !parsed.tag || parsed.tag === 'latest' ? 'latest' : parsed.tag

    if (validTags.length > 1) {
      return `${version} +${validTags.length - 1}`
    }

    return version
  }

  // 获取镜像名称（不包含版本，正确保留 host:port）
  const getImageNameOnly = (image: ImageInfo): string => {
    if (!image.repoTags || image.repoTags.length === 0) {
      // 如果没有标签，返回短ID
      return image.id.startsWith('sha256:') ? image.id.slice(7, 19) : image.id.slice(0, 12)
    }

    const validTags = image.repoTags.filter((tag) => tag !== '<none>:<none>')
    if (validTags.length === 0) {
      // 如果没有有效标签，返回短ID
      return image.id.startsWith('sha256:') ? image.id.slice(7, 19) : image.id.slice(0, 12)
    }

    return parseImageRef(validTags[0]).repository || validTags[0]
  }

  // 获取镜像摘要的显示文本
  const getDigestDisplayText = (image: ImageInfo): string => {
    if (!image.repoDigests || image.repoDigests.length === 0) {
      return image.id.startsWith('sha256:') ? image.id.slice(7, 19) : image.id.slice(0, 12)
    }

    const digest = image.repoDigests[0]
    const parts = digest.split('@')
    if (parts.length === 2 && parts[1].startsWith('sha256:')) {
      return parts[1].slice(7, 19)
    }

    return image.id.slice(0, 12)
  }

  // 检查镜像是否被容器使用（从全局预计算 Map O(1) 读取）
  const isImageInUse = (image: ImageInfo): boolean => {
    return getContainersUsingImage(image).length > 0
  }

  // 获取使用指定镜像的容器列表（从全局预计算 Map O(1) 读取）
  const getContainersUsingImage = (image: ImageInfo): ContainerStatus[] => {
    return store.imageUsageMap.get(image.id) || []
  }

  // 获取镜像使用情况的显示文本
  const getImageUsageText = (image: ImageInfo): string => {
    const usingContainers = getContainersUsingImage(image)

    if (usingContainers.length === 0) {
      return '未使用'
    }

    if (usingContainers.length === 1) {
      return `被 ${usingContainers[0].name} 使用`
    }

    return `被 ${usingContainers.length} 个容器使用`
  }

  // 获取使用镜像的容器名称列表
  const getImageUsageContainers = (image: ImageInfo): string[] => {
    const usingContainers = getContainersUsingImage(image)
    return usingContainers.map((container) => container.name)
  }

  return {
    // 操作方法
    handleDelete,
    handleDeleteDangling,
    handleRefresh,
    handleDownload,
    handleImport,

    // 工具方法
    formatCreateTime,
    getImageAge,
    getVersionDisplayText,
    getImageNameOnly,
    getDigestDisplayText,

    // 镜像使用情况相关方法
    isImageInUse,
    getContainersUsingImage,
    getImageUsageText,
    getImageUsageContainers,
  }
}
