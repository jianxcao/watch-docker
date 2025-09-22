/// <reference types="vite/client" />
/// <reference types="vite-svg-loader" />
import type { AxiosStatic } from 'axios'

declare global {
  interface Window {
    axios: AxiosStatic
    lockUrl: Record<string, any>
    $message: any
  }
}
