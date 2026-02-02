import dayjs from 'dayjs'

/**
 * 格式化文件大小
 */
export function formatSize(bytes: number): string {
  if (bytes === 0) {
    return '-'
  }
  const units = ['B', 'KB', 'MB', 'GB']
  let size = bytes
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }
  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

/**
 * 格式化日期时间
 */
export function formatDate(isoDate: string): string {
  try {
    return dayjs(isoDate).format('YYYY-MM-DD HH:mm:ss')
  } catch {
    return isoDate
  }
}
