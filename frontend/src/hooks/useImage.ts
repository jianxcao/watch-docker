import { useImageStore } from '@/store/image'
import { useMessage, useDialog } from 'naive-ui'
import type { ImageInfo } from '@/common/types'

export function useImage() {
  const store = useImageStore()
  const message = useMessage()
  const dialog = useDialog()

  // 删除镜像（带确认）
  const handleDelete = (image: ImageInfo, force: boolean = false) => {
    const displayTag = store.getImageDisplayTag(image)
    const isDangling = store.isDanglingImage(image)

    const title = isDangling ? '确认删除悬空镜像' : '确认删除镜像'
    const content = isDangling
      ? `确定要删除悬空镜像 "${displayTag}" 吗？`
      : `确定要删除镜像 "${displayTag}" 吗？${
          force ? '这将强制删除镜像，即使有容器正在使用。' : ''
        }`

    dialog.warning({
      title,
      content,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          await store.deleteImage(image.id, force)
          message.success(`镜像 ${displayTag} 删除成功`)
        } catch (error: any) {
          // 如果删除失败且不是强制删除，询问是否强制删除
          if (!force && error.message.includes('conflict')) {
            dialog.warning({
              title: '删除失败',
              content: `镜像 "${displayTag}" 正在被容器使用，是否强制删除？`,
              positiveText: '强制删除',
              negativeText: '取消',
              onPositiveClick: () => handleDelete(image, true),
            })
          } else {
            message.error(`删除镜像失败: ${error.message}`)
          }
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

    if (years > 0) return `${years}年前`
    if (months > 0) return `${months}个月前`
    if (days > 0) return `${days}天前`
    if (hours > 0) return `${hours}小时前`
    if (minutes > 0) return `${minutes}分钟前`
    return '刚刚'
  }

  // 获取镜像标签列表的显示文本
  const getTagsDisplayText = (image: ImageInfo): string => {
    if (!image.repoTags || image.repoTags.length === 0) {
      return '<none>'
    }

    const validTags = image.repoTags.filter((tag) => tag !== '<none>:<none>')
    if (validTags.length === 0) {
      return '<none>'
    }

    if (validTags.length === 1) {
      return validTags[0]
    }

    return `${validTags[0]} +${validTags.length - 1}`
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

  return {
    // 操作方法
    handleDelete,
    handleDeleteDangling,
    handleRefresh,

    // 工具方法
    formatCreateTime,
    getImageAge,
    getTagsDisplayText,
    getDigestDisplayText,
  }
}
