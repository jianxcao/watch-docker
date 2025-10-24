/// <reference types="vite/client" />
/// <reference types="vite-svg-loader" />
import type { AxiosStatic } from 'axios'

declare global {
  // 声明全局版本号变量（通过 Vite define 注入）
  const __APP_VERSION__: string

  interface Window {
    axios: AxiosStatic
    lockUrl: Record<string, any>
    $message: any
  }
}
