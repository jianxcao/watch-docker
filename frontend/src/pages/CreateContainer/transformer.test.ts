import { describe, expect, it } from 'vitest'
import {
  parseShellArgs,
  transformNetworkForm,
  transformPortForm,
  transformRuntimeResourceForm,
} from './transformer'

describe('parseShellArgs', () => {
  it('preserves single and double quoted arguments', () => {
    expect(parseShellArgs(`sh -c "echo 'hello world' && sleep 10"`)).toEqual([
      'sh',
      '-c',
      "echo 'hello world' && sleep 10",
    ])
    expect(parseShellArgs(`--flag="foo bar" 'single arg' plain\\ space`)).toEqual([
      '--flag=foo bar',
      'single arg',
      'plain space',
    ])
  })
})

describe('transformPortForm', () => {
  it('populates exposedPorts alongside portBindings', () => {
    const res = transformPortForm({
      portList: [],
      portBindings: {
        '80/tcp': [{ hostIP: '', hostPort: '8080' }],
      },
      publishAllPorts: false,
      exposedPorts: {},
    })
    expect(res.portBindings).toEqual({
      '80/tcp': [{ hostIP: '', hostPort: '8080' }],
    })
    expect(res.exposedPorts).toEqual({
      '80/tcp': {},
    })
  })
})

describe('transformNetworkForm', () => {
  it('uses bridge mode in default mode without creating container_default network', () => {
    const res = transformNetworkForm(
      {
        configMode: 'default',
        publishAllPorts: false,
        dns: [],
        dnsSearch: [],
        dnsOptions: [],
        extraHosts: [],
        customNetworks: [],
      },
      false,
      'my-app',
    )
    expect(res.networkMode).toBe('bridge')
    expect(res.networksToCreate).toBeUndefined()
    expect(res.networkConfig).toBeUndefined()
  })

  it('sets networkMode to primary sorted custom network in custom mode', () => {
    const res = transformNetworkForm(
      {
        configMode: 'custom',
        publishAllPorts: false,
        dns: [],
        dnsSearch: [],
        dnsOptions: [],
        extraHosts: [],
        customNetworks: [
          {
            name: 'z-custom-net',
            exists: false,
            driver: 'bridge',
            aliases: ['app-z'],
          },
          {
            name: 'a-custom-net',
            exists: true,
            aliases: ['app-a'],
            containerIPv4Address: '172.31.241.10',
          },
        ],
      },
      false,
      'my-app',
    )
    expect(res.networkMode).toBe('a-custom-net')
    expect(res.networksToCreate).toHaveLength(1)
    expect(res.networksToCreate?.[0]?.name).toBe('z-custom-net')
    expect(res.networkConfig?.endpointsConfig).toHaveProperty('a-custom-net')
    expect(res.networkConfig?.endpointsConfig).toHaveProperty('z-custom-net')
  })
})

describe('transformRuntimeResourceForm', () => {
  it('clears restartPolicy when autoRemove is true', () => {
    const res = transformRuntimeResourceForm({
      privileged: false,
      readonlyRootfs: false,
      autoRemove: true,
      restartPolicyName: 'unless-stopped',
      restartPolicyMaxRetry: 3,
      memoryMB: 0,
      memoryReservationMB: 0,
      cpusetCpus: '',
      shmSizeMB: 0,
    })
    expect(res.autoRemove).toBe(true)
    expect(res.restartPolicy).toEqual({ name: '', maximumRetryCount: 0 })
  })
})
