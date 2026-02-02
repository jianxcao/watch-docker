import { listContainerFiles } from '@/common/api'
import type { FileEntry } from '@/common/types'
import { computed, ref } from 'vue'
import type { SortDirection, SortField } from '../types'

/**
 * 文件列表数据管理
 */
export function useFileList(containerId: string) {
  // 状态
  const currentPath = ref('/')
  const entries = ref<FileEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const showHiddenFiles = ref(true)
  const sortField = ref<SortField>('name')
  const sortDirection = ref<SortDirection>('asc')

  // 计算属性：过滤和排序后的文件列表
  const displayEntries = computed(() => {
    let filtered = entries.value

    // 过滤隐藏文件
    if (!showHiddenFiles.value) {
      filtered = filtered.filter((e) => !e.name.startsWith('.'))
    }

    // 排序
    const sorted = [...filtered].sort((a, b) => {
      // 目录优先
      if (a.type === 'directory' && b.type !== 'directory') {
        return -1
      }
      if (a.type !== 'directory' && b.type === 'directory') {
        return 1
      }

      let cmp = 0
      switch (sortField.value) {
        case 'name':
          cmp = a.name.localeCompare(b.name)
          break
        case 'size':
          cmp = a.size - b.size
          break
        case 'modified':
          cmp = new Date(a.modified).getTime() - new Date(b.modified).getTime()
          break
        case 'type':
          cmp = a.type.localeCompare(b.type)
          break
      }

      return sortDirection.value === 'asc' ? cmp : -cmp
    })

    return sorted
  })

  // 路径段
  const pathSegments = computed(() => {
    if (currentPath.value === '/') {
      return []
    }
    return currentPath.value.split('/').filter(Boolean)
  })

  // 加载目录
  const loadDirectory = async (path: string) => {
    loading.value = true
    error.value = null

    try {
      const res = await listContainerFiles(containerId, path)
      if (res.code === 0 && res.data) {
        currentPath.value = res.data.path
        entries.value = res.data.entries || []
      } else {
        throw new Error(res.msg || '加载目录失败')
      }
    } catch (err: any) {
      error.value = err.message || '加载目录失败'
      entries.value = []
    } finally {
      loading.value = false
    }
  }

  // 导航到指定路径
  const navigateTo = (path: string) => {
    loadDirectory(path)
  }

  // 上一级
  const goUp = () => {
    if (currentPath.value === '/') {
      return
    }
    const parts = currentPath.value.split('/').filter(Boolean)
    parts.pop()
    navigateTo('/' + parts.join('/') || '/')
  }

  // 回到根目录
  const goHome = () => {
    navigateTo('/')
  }

  // 切换显示隐藏文件
  const toggleHiddenFiles = () => {
    showHiddenFiles.value = !showHiddenFiles.value
    localStorage.setItem('file-browser-show-hidden', String(showHiddenFiles.value))
  }

  // 从 localStorage 恢复设置
  const restoreSettings = () => {
    const savedShowHidden = localStorage.getItem('file-browser-show-hidden')
    if (savedShowHidden !== null) {
      showHiddenFiles.value = savedShowHidden === 'true'
    }
  }

  return {
    // 状态
    currentPath,
    entries,
    loading,
    error,
    showHiddenFiles,
    sortField,
    sortDirection,

    // 计算属性
    displayEntries,
    pathSegments,

    // 方法
    loadDirectory,
    navigateTo,
    goUp,
    goHome,
    toggleHiddenFiles,
    restoreSettings,
  }
}
