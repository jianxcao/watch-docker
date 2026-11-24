import { toRaw } from 'vue'
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
 * 解析带单引号/双引号和反斜杠转义的命令行字符串为参数数组
 */
export function parseShellArgs(input: string): string[] {
  const args: string[] = []
  let current = ''
  let quote: '"' | "'" | null = null
  let escaped = false
  let hasToken = false

  for (let i = 0; i < input.length; i++) {
    const ch = input[i]
    if (escaped) {
      current += ch
      escaped = false
      hasToken = true
      continue
    }
    if (quote === "'") {
      if (ch === "'") {
        quote = null
      } else {
        current += ch
      }
      hasToken = true
      continue
    }
    if (quote === '"') {
      if (ch === '\\') {
        escaped = true
      } else if (ch === '"') {
        quote = null
      } else {
        current += ch
      }
      hasToken = true
      continue
    }
    if (ch === '\\') {
      escaped = true
      hasToken = true
    } else if (ch === '"' || ch === "'") {
      quote = ch
      hasToken = true
    } else if (/\s/.test(ch)) {
      if (hasToken) {
        args.push(current)
        current = ''
        hasToken = false
      }
    } else {
      current += ch
      hasToken = true
    }
  }

  if (hasToken) {
    args.push(current)
  }
  return args
}

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

  // 转换 entrypointString 为数组（支持引号与转义）
  if (form.entrypointString && form.entrypointString.trim()) {
    data.entrypoint = parseShellArgs(form.entrypointString.trim())
  }

  // 转换 cmdString 为数组（支持引号与转义）
  if (form.cmdString && form.cmdString.trim()) {
    data.cmd = parseShellArgs(form.cmdString.trim())
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

  if (form.envList && form.envList.length > 0) {
    const env = form.envList
      .filter((item) => item.key.trim() !== '')
      .map((item) => `${item.key.trim()}=${item.value}`)
    if (env.length > 0) {
      data.env = env
    }
  } else if (form.env && form.env.length > 0) {
    const env = toRaw(form.env).filter(Boolean)
    if (env.length > 0) {
      data.env = env
    }
  }

  return data
}

/**
 * 转换端口配置表单数据（同时填充 portBindings 与 exposedPorts）
 */
export function transformPortForm(form: PortFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}
  const exposedPorts: Record<string, Record<string, never>> = { ...(form.exposedPorts || {}) }
  let portBindings: Record<string, { hostIP: string; hostPort: string }[]> = {}

  if (form.portList && form.portList.length > 0) {
    form.portList.forEach((port) => {
      if (port.containerPort && port.protocol.length > 0) {
        port.protocol.forEach((protocol) => {
          const key = `${port.containerPort}/${protocol}`
          if (!exposedPorts[key]) {
            exposedPorts[key] = {}
          }
          if (port.hostPort) {
            if (!portBindings[key]) {
              portBindings[key] = []
            }
            portBindings[key].push({
              hostIP: '',
              hostPort: port.hostPort.toString(),
            })
          }
        })
      }
    })
  } else if (form.portBindings && Object.keys(form.portBindings).length > 0) {
    portBindings = toRaw(form.portBindings)
    for (const portKey of Object.keys(portBindings)) {
      if (!exposedPorts[portKey]) {
        exposedPorts[portKey] = {}
      }
    }
  }

  if (Object.keys(portBindings).length > 0) {
    data.portBindings = portBindings
  }

  if (Object.keys(exposedPorts).length > 0) {
    data.exposedPorts = exposedPorts
  }

  return data
}

/**
 * 转换数据卷表单数据
 */
export function transformVolumeForm(form: VolumeFormValue): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {}

  if (form.volumeList && form.volumeList.length > 0) {
    const binds = form.volumeList
      .filter((item) => item.source.trim() && item.target.trim())
      .map((item) => `${item.source.trim()}:${item.target.trim()}${item.readonly ? ':ro' : ''}`)
    if (binds.length > 0) {
      data.binds = binds
    }
  } else if (form.binds && form.binds.length > 0) {
    const binds = toRaw(form.binds).filter(Boolean)
    if (binds.length > 0) {
      data.binds = binds
    }
  }

  return data
}

/**
 * 转换网络配置表单数据
 */
export function transformNetworkForm(
  form: NetworkFormValue,
  portPublishAllPorts: boolean,
  _containerName: string,
): Partial<ContainerCreateRequest> {
  const data: Partial<ContainerCreateRequest> = {
    publishAllPorts: form.publishAllPorts || portPublishAllPorts || false,
  }

  // 默认模式：直接复用 Docker 默认 bridge 网络，避免每次创建容器产生孤儿网络
  if (form.configMode === 'default') {
    data.networkMode = 'bridge'
  } else if (form.configMode === 'custom') {
    // 自定义网络配置模式
    data.networkMode = 'bridge'

    const validNetworks = (form.customNetworks || [])
      .map((network) => ({ ...network, name: network.name?.trim() || '' }))
      .filter((network) => network.name !== '')

    const specialModeNetwork = validNetworks.find(
      (network) => network.name === 'host' || network.name === 'none',
    )

    if (specialModeNetwork) {
      data.networkMode = specialModeNetwork.name
      data.publishAllPorts = false
    } else if (validNetworks.length > 0) {
      const networksToCreate: any[] = []
      const endpointsConfig: Record<string, any> = {}

      validNetworks.forEach((network) => {
        const networkName = network.name

        // 如果网络不存在，构建网络创建配置
        if (!network.exists) {
          const networkToCreate: any = {
            name: networkName,
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
        const endpointSettings: any = {}
        if (network.aliases && network.aliases.length > 0 && networkName !== 'bridge') {
          endpointSettings.aliases = network.aliases
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

        endpointsConfig[networkName] = endpointSettings
      })

      // 如果有需要创建的网络，添加到请求中
      if (networksToCreate.length > 0) {
        data.networksToCreate = networksToCreate
      }

      // 添加网络连接配置
      if (Object.keys(endpointsConfig).length > 0) {
        const sortedNetworkNames = Object.keys(endpointsConfig).sort()
        data.networkMode = sortedNetworkNames[0]
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
  const autoRemove = form.autoRemove || false
  const data: Partial<ContainerCreateRequest> = {
    privileged: form.privileged || false,
    readonlyRootfs: form.readonlyRootfs || false,
    autoRemove,
    restartPolicy: autoRemove
      ? {
          name: '' as any,
          maximumRetryCount: 0,
        }
      : {
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
  if (form.labelList && form.labelList.length > 0) {
    const labels: Record<string, string> = {}
    form.labelList.forEach((item) => {
      const key = item.key.trim()
      if (key) {
        labels[key] = item.value
      }
    })
    if (Object.keys(labels).length > 0) {
      data.labels = labels
    }
  } else if (form.labels && Object.keys(form.labels).length > 0) {
    data.labels = toRaw(form.labels)
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
