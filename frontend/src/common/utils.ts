import dayjs from 'dayjs'
import { h } from 'vue'
import { NIcon, type IconProps } from 'naive-ui'

export const sleep = (ms: number) => {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// 渲染菜单图标
export const renderIcon = (icon: any, props?: IconProps) => {
  return () => h(NIcon, props, { default: () => h(icon) })
}

// 格式化百分比
export const formatPercent = (value: number): string => {
  return `${value.toFixed(2)}%`
}

// 格式化字节数
export const formatBytes = (bytes: number): string => {
  if (bytes === 0) {
    return '0 B'
  }
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`
}

// 格式化网速（字节每秒）
export const formatBytesPerSecond = (bytesPerSecond: number): string => {
  if (bytesPerSecond === 0) {
    return '0 B/s'
  }
  const k = 1024
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s']
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k))
  return `${(bytesPerSecond / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`
}

export const formatTime = (startedAt: string): string => {
  if (!startedAt) {
    return '-'
  }
  const start = dayjs(startedAt)
  const now = dayjs()
  const diffMs = now.diff(start)

  const days = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  const hours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m`
  } else if (hours > 0) {
    return `${hours}h ${minutes}m`
  } else {
    return `${minutes}m`
  }
}

// 获取CPU使用率颜色
export const getCpuColor = (percent: number): string => {
  if (percent < 50) {
    return '#52c41a'
  } // 绿色
  if (percent < 80) {
    return '#faad14'
  } // 黄色
  return '#ff4d4f' // 红色
}

// 获取内存使用率颜色
export const getMemoryColor = (percent: number): string => {
  if (percent < 60) {
    return '#52c41a'
  } // 绿色
  if (percent < 85) {
    return '#faad14'
  } // 黄色
  return '#ff4d4f' // 红色
}

// 检测是否为移动设备
export const isMobile = (): boolean => {
  const userAgent = navigator.userAgent.toLowerCase()
  const mobileKeywords = [
    'android',
    'webos',
    'iphone',
    'ipad',
    'ipod',
    'blackberry',
    'windows phone',
    'mobile',
  ]
  return mobileKeywords.some((keyword) => userAgent.includes(keyword))
}

// 检测是否为平板设备
export const isTablet = (): boolean => {
  const userAgent = navigator.userAgent.toLowerCase()
  const isIPad = userAgent.includes('ipad')
  const isAndroidTablet = userAgent.includes('android') && !userAgent.includes('mobile')
  const isTabletUA = userAgent.includes('tablet')

  // 检测 iPad 桌面模式：
  // 当 iPad 开启"请求桌面网站"时，UA 会伪装成 Mac (MacIntel)
  // 但 iPad 有触摸支持，真正的 Mac（不带触摸屏）没有
  const isMacWithTouch =
    navigator.platform === 'MacIntel' &&
    navigator.maxTouchPoints > 0 &&
    // 确保屏幕尺寸符合平板范围
    window.innerWidth >= 768

  // 结合屏幕尺寸判断（可选）
  const hasTabletScreen = window.innerWidth >= 768 && window.innerWidth <= 1024

  return (
    isIPad || isMacWithTouch || isAndroidTablet || isTabletUA || (hasTabletScreen && isMobile())
  )
}

// 检测是否为手机（排除平板）
export const isPhone = (): boolean => {
  return isMobile() && !isTablet()
}

// 获取设备类型
export const getDeviceType = (): 'desktop' | 'tablet' | 'mobile' => {
  if (isTablet()) {
    return 'tablet'
  }
  if (isPhone()) {
    return 'mobile'
  }
  return 'desktop'
}
