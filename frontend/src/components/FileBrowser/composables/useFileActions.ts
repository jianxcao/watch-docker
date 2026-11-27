import { createContainerPath, renameContainerPath } from '@/common/api'
import { useDialog, useMessage } from 'naive-ui'
import { h, toValue, type MaybeRefOrGetter } from 'vue'
import { NInput } from 'naive-ui'

export const validateEntryName = (rawName: string, typeName: string): string | null => {
  if (!rawName || rawName.trim() === '') {
    return `请输入${typeName}名称`
  }
  if (rawName === '.' || rawName === '..') {
    return `${typeName}名称不能为 "." 或 ".."`
  }
  if (rawName.includes('/') || rawName.includes('\\') || rawName.includes('\0')) {
    return `${typeName}名称不能包含路径分隔符`
  }
  return null
}

/**
 * 文件/文件夹创建和重命名功能
 */
export function useFileActions(containerId: MaybeRefOrGetter<string>) {
  const message = useMessage()
  const dialog = useDialog()

  // 新建文件或文件夹
  const createPath = (currentPath: string, type: 'file' | 'directory', onSuccess?: () => void) => {
    let inputValue = ''
    const typeName = type === 'file' ? '文件' : '文件夹'

    const d = dialog.create({
      title: `新建${typeName}`,
      content: () =>
        h(NInput, {
          placeholder: `请输入${typeName}名称`,
          autofocus: true,
          onUpdateValue: (v: string) => {
            inputValue = v
          },
            onKeyup: async (e: KeyboardEvent) => {
              if (e.key === 'Enter' && !d.loading) {
                const validationErr = validateEntryName(inputValue, typeName)
                if (validationErr) {
                  message.error(validationErr)
                  return
                }
                d.loading = true
                const ok = await handleCreate()
                d.loading = false
                if (ok) {
                  d.destroy()
                }
              }
            },
        }),
      positiveText: '创建',
      negativeText: '取消',
      onPositiveClick: () => {
        const validationErr = validateEntryName(inputValue, typeName)
        if (validationErr) {
          message.error(validationErr)
          return false
        }
        d.loading = true
        return handleCreate().finally(() => {
          d.loading = false
        })
      },
    })

    const handleCreate = async (): Promise<boolean> => {
      const name = inputValue
      const fullPath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`

      try {
        const res = await createContainerPath(toValue(containerId), {
          path: fullPath,
          type,
        })

        if (res.code === 0) {
          message.success(`${typeName}创建成功`)
          onSuccess?.()
          return true
        }
        message.error(res.msg || '创建失败')
        return false
      } catch (err) {
        message.error(err instanceof Error ? err.message : '创建失败')
        return false
      }
    }
  }

  // 重命名文件或文件夹
  const renamePath = (
    entry: { name: string; type: string },
    currentPath: string,
    onSuccess?: () => void,
  ) => {
    let inputValue = entry.name
    const isDirectory = entry.type === 'directory'
    const typeName = isDirectory ? '文件夹' : '文件'

    const oldPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`

    const d = dialog.create({
      title: `重命名${typeName}`,
      content: () =>
        h(NInput, {
          defaultValue: entry.name,
          placeholder: `请输入新${typeName}名称`,
          autofocus: true,
          onUpdateValue: (v: string) => {
            inputValue = v
          },
          onKeyup: async (e: KeyboardEvent) => {
            if (e.key === 'Enter' && !d.loading) {
              const newName = inputValue
              const validationErr = validateEntryName(newName, typeName)
              if (validationErr) {
                message.error(validationErr)
                return
              }
              if (newName === entry.name) {
                message.warning('名称未改变')
                return
              }
              d.loading = true
              const ok = await handleRename()
              d.loading = false
              if (ok) {
                d.destroy()
              }
            }
          },
        }),
      positiveText: '重命名',
      negativeText: '取消',
      onPositiveClick: () => {
        const newName = inputValue
        const validationErr = validateEntryName(newName, typeName)
        if (validationErr) {
          message.error(validationErr)
          return false
        }

        if (newName === entry.name) {
          message.warning('名称未改变')
          return false
        }

        d.loading = true
        return handleRename().finally(() => {
          d.loading = false
        })
      },
    })

    const handleRename = async (): Promise<boolean> => {
      const newName = inputValue
      const newPath = currentPath === '/' ? `/${newName}` : `${currentPath}/${newName}`

      try {
        const res = await renameContainerPath(toValue(containerId), {
          oldPath,
          newPath,
        })

        if (res.code === 0) {
          message.success(`${typeName}重命名成功`)
          onSuccess?.()
          return true
        }
        message.error(res.msg || '重命名失败')
        return false
      } catch (err) {
        message.error(err instanceof Error ? err.message : '重命名失败')
        return false
      }
    }
  }

  return {
    createPath,
    renamePath,
  }
}
