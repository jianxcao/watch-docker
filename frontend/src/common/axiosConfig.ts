import { systemErrCode } from '@/constants/code'
import { systemErrInfo } from '@/constants/msg'
import { navigateTo } from '@/router'
import axios from 'axios'
import type { AxiosRequestConfig, CancelTokenSource } from 'axios'
// format全局只有在应用初始化后才可以使用
// import toast from './toast'
export interface IResData<T = any> {
  code: number
  msg: string
  data: T
}
axios.defaults.baseURL = '/api/v1'
type IAxiosRequestConfig = AxiosRequestConfig & {
  lockUrl: Record<string, any>
  lockKey: string
  source: CancelTokenSource
}
const CancelToken = axios.CancelToken
axios.defaults.timeout = 300000
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
      one.config.source.cancel()
    })
    lockUrl[lockKey] = []
  }
  if (cur.length && s2.test(lockKey)) {
    const p = cur[0].promise
    p.then(
      () => {
        lockUrl[lockKey] = []
      },
      () => {
        lockUrl[lockKey] = []
      }
    )
    config.source.cancel()
    return cur[0].promise
  }
  lockUrl[lockKey].push({
    promise,
    config,
  })
  return promise
}

let token = localStorage.getItem('token') || ''

export const setToken = (t: string) => {
  token = t
}
export const getToken = () => {
  return token
}

axios.interceptors.request.clear()
axios.interceptors.response.clear()
// 添加时间戳
axios.interceptors.request.use(
  async function (config) {
    const params = config.params || {}
    const headers = Object.assign(config.headers || {})
    if (token) {
      headers.Authorization = token
    }
    params._t = +new Date()
    config.headers = headers
    return config
  },
  function (error) {
    console.log(error)
    return Promise.reject(error)
  }
)

axios.interceptors.response.use(
  async function (res) {
    return res
  },
  async function (err) {
    const res = err.response
    // const config = err?.config || res?.config
    const status = res?.status
    if (status === 401 || status === 403) {
      navigateTo('/login')
    }
    return Promise.reject(err)
  }
)

axios.interceptors.response.use(async function (res) {
  const data = res.data
  const status = res.status
  if (status >= 200 && status < 300) {
    if (!data || (typeof data === 'string' && data.indexOf('<!DOCTYPE') >= 0)) {
      return {
        bizCode: systemErrCode,
        message: systemErrInfo,
        response: res,
      }
    }
    return data
  } else if (data?.code !== undefined) {
    return data
  }

  return {
    bizCode: systemErrCode,
    message: systemErrInfo,
    response: res,
  }
})

const req = axios.Axios.prototype.request
/**
 * 覆盖全局request的方法，方便处理异常出现的情况
 */
axios.Axios.prototype.request = function (config: IAxiosRequestConfig) {
  if (config.lockKey) {
    const source = CancelToken.source()
    config.source = source
    config.cancelToken = source.token
  }
  const promise = req.call(this, config)
  return detailLockKey(config, promise)
}

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
  delete<P = any, T = any>(url: string, config?: AxiosRequestConfig<T> | undefined) {
    return axios.delete<IResData<P>, IResData<P>, T>(url, config)
  },
}

export default http
