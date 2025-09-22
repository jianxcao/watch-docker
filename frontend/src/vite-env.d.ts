/// <reference types="vite/client" />
import type { AxiosStatic } from 'axios'

declare global {
  interface Window {
    axios: AxiosStatic
    lockUrl: Record<string, any>
  }
}
