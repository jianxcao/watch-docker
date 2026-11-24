import { systemErrCode } from '@/constants/code'
import { systemErrInfo } from '@/constants/msg'
import axios from 'axios'
import type { AxiosRequestConfig, CancelTokenSource } from 'axios'
import router from '@/router'
// format全局只有在应用初始化后才可以使用
// import toast from './toast'
export interface IResData<T = any> {
  code: number
  msg: string
  data: T
}
axios.defaults.baseURL = '/api/v1'
type IAxiosRequestConfig = AxiosRequestConfig & {
  lockUrl?: Record<string, any>
  lockKey?: string
  source?: CancelTokenSource
}
const CancelToken = axios.CancelToken
axios.defaults.timeout = 30 * 1000
window.axios = axios

const lockUrl: Record<string, any> = {}
window.lockUrl = {}
// 后到优先策略：取消前一个未完成的ajax请求，然后发送新的ajax请求
const s1 = /^@.+/

// 节约型策略：即共享类型，由同类型的第一个请求发送ajax，（在第一个ajax返回之前的）后续的同类型共享ajax返回结果
const s2 = /^!.+/

const detailLockKey = (config: IAxiosRequestConfig, promise: Promise<any>) => {
  const { lockKey } = config
  if (!lockKey) {
    return promise
  }
  lockUrl[lockKey] = lockUrl[lockKey] || []
  const cur = lockUrl[lockKey].slice(0)
  // 取消前面的请求
  if (cur.length && s1.test(lockKey)) {
    cur.forEach((one: { config: IAxiosRequestConfig; promise: Promise<any> }) => {
      one.config.source?.cancel()
    })
    lockUrl[lockKey] = []
  }
  if (cur.length && s2.test(lockKey)) {
    const p = cur[0].promise
    config.source?.cancel()
    return p
  }
  let entry: { promise: Promise<any>; config: IAxiosRequestConfig }
  const trackedPromise = promise.finally(() => {
    const entries = lockUrl[lockKey]
    if (!Array.isArray(entries)) {
      return
    }
    const idx = entries.indexOf(entry)
    if (idx >= 0) {
      entries.splice(idx, 1)
    }
    if (entries.length === 0) {
      delete lockUrl[lockKey]
    }
  })
  entry = {
    promise: trackedPromise,
    config,
  }
  lockUrl[lockKey].push(entry)
  return trackedPromise
}

// 动态获取 token 的函数，避免循环依赖
let getTokenFromStore: ((url?: string) => string) | null = null

export const initTokenStore = (getTokenFn: (url?: string) => string) => {
  getTokenFromStore = getTokenFn
}

export const getToken = (url?: string) => {
  // 优先从 store 获取，如果没有初始化则返回空字符串
  return getTokenFromStore ? getTokenFromStore(url) : ''
}

// 添加时间戳
axios.interceptors.request.use(
  async function (config) {
    const params = { ...(config.params || {}), _t: Date.now() }
    const headers = Object.assign({}, config.headers || {})
    const currentToken = getToken(config.url)
    if (currentToken) {
      headers.Authorization = `Bearer ${currentToken}`
    }
    config.params = params
    config.headers = headers
    return config
  },
  function (error) {
    console.log(error)
    return Promise.reject(error)
  },
)

let isRedirectingToLogin = false

axios.interceptors.response.use(
  async function (res) {
    const data = res.data
    const status = res.status
    if (status >= 200 && status < 300) {
      if (!data || (typeof data === 'string' && data.indexOf('<!DOCTYPE') >= 0)) {
        return {
          code: systemErrCode,
          msg: systemErrInfo,
          message: systemErrInfo,
          data: null,
          response: res,
        }
      }
      return data
    } else if (data?.code !== undefined) {
      return data
    }

    return {
      code: systemErrCode,
      msg: systemErrInfo,
      message: systemErrInfo,
      data: null,
      response: res,
    }
  },
  async function (err) {
    const res = err?.response
    const status = res?.status
    if (status === 401 || status === 403) {
      const { useAuthStore } = await import('@/store/auth')
      const authStore = useAuthStore()
      authStore.setAuthStatus(true)
      if (!isRedirectingToLogin) {
        isRedirectingToLogin = true
        try {
          if (router.currentRoute.value.path === '/login') {
            authStore.cancelTwoFA()
          } else {
            authStore.forceLogout()
          }
        } finally {
          setTimeout(() => {
            isRedirectingToLogin = false
          }, 500)
        }
      }
      const unauthorizedMsg = res?.data?.msg || res?.data?.message || '登录已过期，请重新登录'
      return Promise.reject({
        code: status,
        msg: unauthorizedMsg,
        message: unauthorizedMsg,
        response: res,
      })
    }
    const errMsg = res?.data?.msg || res?.data?.message || err?.message || systemErrInfo
    return Promise.reject({
      ...err,
      code: res?.data?.code ?? status ?? systemErrCode,
      msg: errMsg,
      message: errMsg,
      response: res,
    })
  },
)

const req = axios.Axios.prototype.request
/**
 * 覆盖全局request的方法，方便处理异常出现的情况
 */
const wrappedRequest = function <R = any>(
  this: unknown,
  configOrUrl: string | IAxiosRequestConfig,
  configArg?: IAxiosRequestConfig,
): Promise<R> {
  const config: IAxiosRequestConfig =
    typeof configOrUrl === 'string'
      ? { ...(configArg || {}), url: configOrUrl }
      : configOrUrl || {}
  if (config.lockKey) {
    const source = CancelToken.source()
    config.source = source
    config.cancelToken = source.token
  }
  const promise = req.call(this, config)
  return detailLockKey(config, promise) as Promise<R>
} as typeof req

axios.Axios.prototype.request = wrappedRequest
axios.request = wrappedRequest.bind(axios) as typeof axios.request

const http = {
  request<P = any, T = any>(config: AxiosRequestConfig<T>) {
    return axios.request<IResData<P>, IResData<P>, T>(config)
  },
  get<P = any, T = any>(url: string, config?: AxiosRequestConfig<T> | undefined) {
    return axios.get<IResData<P>, IResData<P>, T>(url, config)
  },
  post<P = any, T = any>(url: string, data?: any, config?: AxiosRequestConfig<any> | undefined) {
    return axios.post<IResData<P>, IResData<P>, T>(url, data, config)
  },
  put<P = any, T = any>(url: string, data?: any, config?: AxiosRequestConfig<any> | undefined) {
    return axios.put<IResData<P>, IResData<P>, T>(url, data, config)
  },
  delete<P = any, T = any>(url: string, config?: AxiosRequestConfig<T> | undefined) {
    return axios.delete<IResData<P>, IResData<P>, T>(url, config)
  },
}

export default http
