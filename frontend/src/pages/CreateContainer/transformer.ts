import type { ContainerCreateRequest } from '@/common/types'
import type {
  BasicFormValue,
  EnvFormValue,
  PortFormValue,
  VolumeFormValue,
  NetworkFormValue,
  RuntimeResourceFormValue,
  LabelFormValue,
  AdvancedFormValue,
} from './types'

/**
 * 转换基础配置表单数据
 */
export function transformBasicForm(form: BasicFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {
    name: form.name || '',
    image: form.image,
    tty: form.tty || false,
    openStdin: form.openStdin || false,
    stdinOnce: form.stdinOnce || false,
  }

  // 转换 entrypointString 为数组
  if (form.entrypointString && form.entrypointString.trim()) {
    data.entrypoint = form.entrypointString.trim().split(/\s+/)
  }

  // 转换 cmdString 为数组
  if (form.cmdString && form.cmdString.trim()) {
    data.cmd = form.cmdString.trim().split(/\s+/)
  }

  if (form.workingDir) {
    data.workingDir = form.workingDir
  }

  if (form.user) {
    data.user = form.user
  }

  if (form.hostname) {
    data.hostname = form.hostname
  }

  if (form.domainname) {
    data.domainname = form.domainname
  }

  return data
}

/**
 * 转换环境变量表单数据
 */
export function transformEnvForm(form: EnvFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  if (form.env && form.env.length > 0) {
    data.env = toRaw(form.env)
  }

  return data
}

/**
 * 转换端口配置表单数据
 */
export function transformPortForm(form: PortFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  if (form.portBindings && Object.keys(form.portBindings).length > 0) {
    data.portBindings = toRaw(form.portBindings)
  }

  return data
}

/**
 * 转换数据卷表单数据
 */
export function transformVolumeForm(form: VolumeFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  if (form.binds && form.binds.length > 0) {
    data.binds = toRaw(form.binds)
  }

  return data
}

/**
 * 转换网络配置表单数据
 */
export function transformNetworkForm(
  form: NetworkFormValue,
  portPublishAllPorts: boolean,
): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {
    networkMode: form.networkMode || 'bridge',
    publishAllPorts: form.publishAllPorts || portPublishAllPorts || false,
  }

  if (form.dns && form.dns.length > 0) {
    data.dns = form.dns
  }

  if (form.dnsSearch && form.dnsSearch.length > 0) {
    data.dnsSearch = form.dnsSearch
  }

  if (form.dnsOptions && form.dnsOptions.length > 0) {
    data.dnsOptions = form.dnsOptions
  }

  if (form.extraHosts && form.extraHosts.length > 0) {
    data.extraHosts = form.extraHosts
  }

  // 转换网络端点配置
  // 注意: host 和 none 模式下不支持网络端点配置
  const networkMode = form.networkMode || 'bridge'
  const canUseNetworkConfig = networkMode !== 'host' && networkMode !== 'none'

  if (canUseNetworkConfig && form.networkEndpoints && form.networkEndpoints.length > 0) {
    const endpointsConfig: Record<string, any> = {}

    form.networkEndpoints.forEach((endpoint) => {
      if (!endpoint.networkName) {
        return
      }

      // 为每个网络端点创建独立的配置对象
      // 注意：每个端点都有自己的 endpointSettings，不会互相覆盖
      const endpointSettings: any = {
        aliases: endpoint.aliases || [],
      }

      // 配置 IPv4 地址（在 endpointSettings 对象的顶层，而非 ipamConfig 嵌套结构中）
      if (endpoint.ipv4Address) {
        endpointSettings.ipAddress = endpoint.ipv4Address
      }

      // 配置 IPv4 网关（在 endpointSettings 对象的顶层）
      if (endpoint.ipv4Gateway) {
        endpointSettings.gateway = endpoint.ipv4Gateway
      }

      // 配置 IPv6 地址（在 endpointSettings 对象的顶层）
      // GlobalIPv6Address 用于运行时设置全局 IPv6 地址
      if (endpoint.ipv6Address) {
        endpointSettings.globalIPv6Address = endpoint.ipv6Address
      }

      // 配置 IPv6 网关（在 endpointSettings 对象的顶层）
      if (endpoint.ipv6Gateway) {
        endpointSettings.ipv6Gateway = endpoint.ipv6Gateway
      }

      // 配置 MAC 地址（在 endpointSettings 对象的顶层）
      if (endpoint.macAddress) {
        endpointSettings.macAddress = endpoint.macAddress
      }

      // 配置 IPAM（嵌套结构，用于 IPAM 系统分配 IP 地址）
      // IPAMConfig.IPv6Address 用于创建时指定 IPv6 地址给 IPAM 系统
      // 注意：虽然 GlobalIPv6Address 和 IPAMConfig.IPv6Address 通常设置为相同值，
      // 但它们的作用不同：IPAMConfig 用于地址分配，GlobalIPv6Address 用于运行时配置
      if (endpoint.ipv4Address || endpoint.ipv6Address) {
        endpointSettings.ipamConfig = {}
        if (endpoint.ipv4Address) {
          endpointSettings.ipamConfig.ipv4Address = endpoint.ipv4Address
        }
        if (endpoint.ipv6Address) {
          endpointSettings.ipamConfig.ipv6Address = endpoint.ipv6Address
        }
      }

      // 将当前端点的配置存储到 endpointsConfig 中，使用 networkName 作为 key
      // 每个网络端点都有独立的配置，不会互相覆盖
      endpointsConfig[endpoint.networkName] = endpointSettings
    })

    if (Object.keys(endpointsConfig).length > 0) {
      data.networkConfig = {
        endpointsConfig,
      }
    }
  }

  return data
}

/**
 * 转换运行与资源配置表单数据
 */
export function transformRuntimeResourceForm(
  form: RuntimeResourceFormValue,
): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {
    privileged: form.privileged || false,
    readonlyRootfs: form.readonlyRootfs || false,
    autoRemove: form.autoRemove || false,
    restartPolicy: {
      name: form.restartPolicyName as any,
      maximumRetryCount: form.restartPolicyMaxRetry || 0,
    },
  }

  // 资源限制
  if (form.memoryMB && form.memoryMB > 0) {
    data.memory = form.memoryMB * 1024 * 1024
  }

  if (form.memoryReservationMB && form.memoryReservationMB > 0) {
    data.memoryReservation = form.memoryReservationMB * 1024 * 1024
  }

  if (form.cpusetCpus) {
    data.cpusetCpus = form.cpusetCpus
  }

  if (form.shmSizeMB && form.shmSizeMB > 0) {
    data.shmSize = form.shmSizeMB * 1024 * 1024
  }

  return data
}

/**
 * 转换高级配置表单数据
 */
export function transformAdvancedForm(form: AdvancedFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  // 能力
  if (form.capAdd && form.capAdd.length > 0) {
    data.capAdd = form.capAdd
  }

  if (form.capDrop && form.capDrop.length > 0) {
    data.capDrop = form.capDrop
  }

  // 其他高级选项
  if (form.pidMode) {
    data.pidMode = form.pidMode
  }

  if (form.ipcMode) {
    data.ipcMode = form.ipcMode
  }

  if (form.utsMode) {
    data.utsMode = form.utsMode
  }

  if (form.cgroup) {
    data.cgroup = form.cgroup
  }

  if (form.runtime) {
    data.runtime = form.runtime
  }

  if (form.securityOpt && form.securityOpt.length > 0) {
    data.securityOpt = form.securityOpt
  }

  return data
}

/**
 * 转换标签配置表单数据
 */
export function transformLabelForm(form: LabelFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  // 标签
  if (form.labels && Object.keys(form.labels).length > 0) {
    data.labels = form.labels
  }

  return data
}

/**
 * 合并所有表单数据为 ContainerCreateRequest
 */
export function mergeFormData(
  basicForm: BasicFormValue,
  envForm: EnvFormValue,
  portForm: PortFormValue,
  volumeForm: VolumeFormValue,
  networkForm: NetworkFormValue,
  runtimeResourceForm: RuntimeResourceFormValue,
  labelForm: LabelFormValue,
  advancedForm: AdvancedFormValue,
): ContainerCreateRequest {
  return {
    ...transformBasicForm(basicForm),
    ...transformEnvForm(envForm),
    ...transformPortForm(portForm),
    ...transformVolumeForm(volumeForm),
    ...transformNetworkForm(networkForm, portForm.publishAllPorts),
    ...transformRuntimeResourceForm(runtimeResourceForm),
    ...transformLabelForm(labelForm),
    ...transformAdvancedForm(advancedForm),
  } as ContainerCreateRequest
}
