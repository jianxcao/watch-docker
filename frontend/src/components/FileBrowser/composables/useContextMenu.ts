import type { FileEntry } from '@/common/types'
import { renderIcon } from '@/common/utils'
import {
  Create,
  Download,
  Eye,
  FolderOpen,
  Trash,
  Text,
  DocumentText,
  CreateOutline,
} from '@vicons/ionicons5'
import { useThemeVars, type DropdownOption } from 'naive-ui'
import { nextTick, ref } from 'vue'
import type { ContextMenuState } from '../types'

/**
 * 右键菜单管理
 */
export function useContextMenu(canEdit: boolean) {
  const contextMenu = ref<ContextMenuState>({
    show: false,
    x: 0,
    y: 0,
    entry: null,
  })
  const theme = useThemeVars()
  const selectedEntry = ref<FileEntry | null>(null)

  // 标志位：菜单是否刚刚关闭（用于防止点击时误触发行为）
  const justClosed = ref(false)

  // 获取右键菜单选项
  const getContextMenuOptions = (entry: FileEntry): DropdownOption[] => {
    const isFile = entry.type === 'file'
    const isDirectory = entry.type === 'directory'

    const options: DropdownOption[] = []

    if (isFile) {
      options.push(
        {
          label: '查看',
          key: 'view',
          icon: renderIcon(Eye),
        },
        {
          label: '编辑',
          key: 'edit',
          icon: renderIcon(Create),
          disabled: !canEdit,
        },
      )
    }

    if (isDirectory) {
      options.push({
        label: '打开',
        key: 'open',
        icon: renderIcon(FolderOpen),
      })
    }

    options.push(
      {
        label: '下载',
        key: 'download',
        icon: renderIcon(Download),
      },
      {
        label: '重命名',
        key: 'rename',
        icon: renderIcon(Text),
        disabled: !canEdit,
      },
      {
        type: 'divider',
        key: 'divider1',
      },
      {
        label: '删除',
        key: 'delete',
        icon: renderIcon(Trash, {
          color: theme.value.errorColor,
        }),
        disabled: !canEdit,
        props: {
          style: `color: ${theme.value.errorColor}`,
        },
      },
    )

    return options
  }

  // 获取空白区域右键菜单选项
  const getEmptyAreaContextMenuOptions = (): DropdownOption[] => {
    if (!canEdit) {
      return []
    }

    return [
      {
        label: '新建文件',
        key: 'create-file',
        icon: renderIcon(DocumentText),
      },
      {
        label: '新建文件夹',
        key: 'create-directory',
        icon: renderIcon(FolderOpen),
      },
      {
        label: '上传文件',
        key: 'upload',
        icon: renderIcon(CreateOutline),
      },
    ]
  }

  // 显示右键菜单
  const showContextMenu = (e: MouseEvent, entry: FileEntry | null = null) => {
    e.preventDefault()

    selectedEntry.value = entry
    contextMenu.value = {
      show: false,
      x: e.clientX,
      y: e.clientY,
      entry,
    }

    nextTick(() => {
      contextMenu.value.show = true
    })
  }

  // 隐藏右键菜单
  const hideContextMenu = () => {
    if (contextMenu.value.show) {
      contextMenu.value.show = false
      selectedEntry.value = null

      // 设置标志位，表示菜单刚刚关闭
      justClosed.value = true

      // 在下一个事件循环中清除标志
      setTimeout(() => {
        justClosed.value = false
      }, 0)
    }
  }

  // 检查条目是否被选中
  const isEntrySelected = (entry: FileEntry) => {
    return selectedEntry.value?.name === entry.name
  }

  return {
    contextMenu,
    selectedEntry,
    justClosed,
    getContextMenuOptions,
    getEmptyAreaContextMenuOptions,
    showContextMenu,
    hideContextMenu,
    isEntrySelected,
  }
}
