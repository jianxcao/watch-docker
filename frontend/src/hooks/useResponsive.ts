import { useBreakpoints } from '@vueuse/core'

export function useResponsive() {
  const breakpoints = useBreakpoints({
    mobile: 640,
    tablet: 768,
    laptop: 1024,
    desktop: 1280,
    desktopLarge: 1600,
  })

  const isMobile = breakpoints.smaller('tablet')
  const isTablet = breakpoints.between('tablet', 'laptop')
  const isLaptop = breakpoints.between('laptop', 'desktop')
  const isDesktop = breakpoints.between('desktop', 'desktopLarge')
  const isDesktopLarge = breakpoints.greaterOrEqual('desktopLarge')
  // 是否为小屏设备（需要抽屉菜单）
  const isSmallScreen = breakpoints.smaller('laptop')

  // 是否为大屏设备（可以显示侧边菜单）
  const isLargeScreen = breakpoints.greaterOrEqual('laptop')

  return {
    isMobile,
    isTablet,
    isLaptop,
    isDesktop,
    isDesktopLarge,
    isSmallScreen,
    isLargeScreen,
  }
}
