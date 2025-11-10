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
  containerName: string,
): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {
    publishAllPorts: form.publishAllPorts || portPublishAllPorts || false,
  }

  // 根据配置模式处理网络配置
  if (form.configMode === 'default') {
    // 默认模式：创建一个 容器名_default 的网络
    data.networkMode = 'bridge'

    // 如果提供了容器名，创建一个默认网络
    if (containerName && containerName.trim()) {
      const networkName = `${containerName}_default`

      // 创建网络配置
      data.networksToCreate = [
        {
          name: networkName,
          driver: 'bridge',
          enableIPv6: false,
          internal: false,
          attachable: true,
        },
      ]

      // 配置容器连接到该网络
      const endpointsConfig: Record<string, any> = {}
      endpointsConfig[networkName] = {
        aliases: [],
      }

      data.networkConfig = {
        endpointsConfig,
      }
    }
  } else if (form.configMode === 'custom') {
    // 自定义网络配置模式
    data.networkMode = 'bridge'

    if (form.customNetworks && form.customNetworks.length > 0) {
      const networksToCreate: any[] = []
      const endpointsConfig: Record<string, any> = {}

      form.customNetworks.forEach((network) => {
        if (!network.name) {
          return
        }

        // 如果网络不存在，构建网络创建配置
        if (!network.exists) {
          const networkToCreate: any = {
            name: network.name,
            driver: network.driver || 'bridge',
            enableIPv6: network.enableIPv6 || false,
            internal: network.internal || false,
            attachable: network.attachable || false,
          }

          // 如果是 macvlan 驱动，添加 parent 参数到 options
          if (network.driver === 'macvlan' && network.parentInterface) {
            networkToCreate.options = {
              parent: network.parentInterface,
            }
          }

          // 构建 IPAM 配置
          const ipamConfig: any[] = []

          // IPv4 配置
          if (network.ipv4Subnet || network.ipv4Gateway) {
            const ipv4Config: any = {}
            if (network.ipv4Subnet) {
              ipv4Config.subnet = network.ipv4Subnet
            }
            if (network.ipv4Gateway) {
              ipv4Config.gateway = network.ipv4Gateway
            }
            ipamConfig.push(ipv4Config)
          }

          // IPv6 配置
          if (network.enableIPv6 && (network.ipv6Subnet || network.ipv6Gateway)) {
            const ipv6Config: any = {}
            if (network.ipv6Subnet) {
              ipv6Config.subnet = network.ipv6Subnet
            }
            if (network.ipv6Gateway) {
              ipv6Config.gateway = network.ipv6Gateway
            }
            ipamConfig.push(ipv6Config)
          }

          // 如果有 IPAM 配置，添加到网络创建请求中
          if (ipamConfig.length > 0) {
            networkToCreate.ipam = {
              config: ipamConfig,
            }
          }

          networksToCreate.push(networkToCreate)
        }

        // 配置容器连接到网络的参数
        const endpointSettings: any = {
          aliases: network.aliases || [],
        }

        // 配置容器的静态 IP
        if (network.containerIPv4Address || network.containerIPv6Address) {
          endpointSettings.ipamConfig = {}
          if (network.containerIPv4Address) {
            endpointSettings.ipamConfig.ipv4Address = network.containerIPv4Address
          }
          if (network.containerIPv6Address) {
            endpointSettings.ipamConfig.ipv6Address = network.containerIPv6Address
          }
        }

        // 配置 MAC 地址
        if (network.macAddress) {
          endpointSettings.macAddress = network.macAddress
        }

        endpointsConfig[network.name] = endpointSettings
      })

      // 如果有需要创建的网络，添加到请求中
      if (networksToCreate.length > 0) {
        data.networksToCreate = networksToCreate
      }

      // 添加网络连接配置
      if (Object.keys(endpointsConfig).length > 0) {
        data.networkConfig = {
          endpointsConfig,
        }
      }
    }
  }

  // 通用 DNS 配置（所有模式共享）
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
    ...transformNetworkForm(networkForm, portForm.publishAllPorts, basicForm.name),
    ...transformRuntimeResourceForm(runtimeResourceForm),
    ...transformLabelForm(labelForm),
    ...transformAdvancedForm(advancedForm),
  } as ContainerCreateRequest
}
