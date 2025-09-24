export const sleep = (ms: number) => {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// 格式化大小的工具函数
export const formatSize = (bytes: number): string => {
  if (bytes === 0) return '0 Bytes'
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// 格式化百分比
export const formatPercent = (value: number): string => {
  return `${value.toFixed(1)}%`
}

// 格式化字节数
export const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

// 获取CPU使用率颜色
export const getCpuColor = (percent: number): string => {
  if (percent < 50) return '#52c41a' // 绿色
  if (percent < 80) return '#faad14' // 黄色
  return '#ff4d4f' // 红色
}

// 获取内存使用率颜色
export const getMemoryColor = (percent: number): string => {
  if (percent < 60) return '#52c41a' // 绿色
  if (percent < 85) return '#faad14' // 黄色
  return '#ff4d4f' // 红色
}
