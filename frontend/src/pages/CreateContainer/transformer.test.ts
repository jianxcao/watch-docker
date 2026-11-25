import { describe, expect, it } from 'vitest'
import {
  parseShellArgs,
  transformBasicForm,
  transformEnvForm,
  transformLabelForm,
  transformNetworkForm,
  transformPortForm,
  transformRuntimeResourceForm,
  transformVolumeForm,
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

  it('preserves ordinary backslashes inside double quotes while handling special escapes', () => {
    expect(parseShellArgs(String.raw`printf "\n"`)).toEqual(['printf', String.raw`\n`])
    expect(parseShellArgs(String.raw`grep "\d+"`)).toEqual(['grep', String.raw`\d+`])
    expect(parseShellArgs(String.raw`echo "a\"b" "a\\b" "\$HOME" "\`id\`"`)).toEqual([
      'echo',
      'a"b',
      String.raw`a\b`,
      '$HOME',
      '`id`',
    ])
    expect(parseShellArgs('echo "a\\\nb"')).toEqual(['echo', 'ab'])
    expect(parseShellArgs('echo "a\\\r\nb"')).toEqual(['echo', 'ab'])
    expect(parseShellArgs('--flag1 \\\n  --flag2')).toEqual(['--flag1', '--flag2'])
    expect(parseShellArgs('--flag1 \\\r\n  --flag2')).toEqual(['--flag1', '--flag2'])
    expect(parseShellArgs('cmd \\')).toEqual(['cmd'])
  })
})

describe('transformBasicForm', () => {
  it('parses both entrypointString and cmdString with double-quoted backslashes', () => {
    const res = transformBasicForm({
      name: 'demo',
      image: 'alpine:latest',
      entrypointString: String.raw`/bin/sh -c "grep \"\d+\""`,
      cmdString: String.raw`printf "\n" 'raw\narg' plain\ space`,
      workingDir: '',
      user: '',
      hostname: '',
      domainname: '',
      tty: false,
      openStdin: false,
      stdinOnce: false,
    })

    expect(res.entrypoint).toEqual(['/bin/sh', '-c', String.raw`grep "\d+"`])
    expect(res.cmd).toEqual(['printf', String.raw`\n`, String.raw`raw\narg`, 'plain space'])

    const trailingEscaped = transformBasicForm({
      name: 'demo',
      image: 'alpine:latest',
      entrypointString: 'cmd \\',
      cmdString: 'echo hello\\ ',
      workingDir: '',
      user: '',
      hostname: '',
      domainname: '',
      tty: false,
      openStdin: false,
      stdinOnce: false,
    })
    expect(trailingEscaped.entrypoint).toEqual(['cmd'])
    expect(trailingEscaped.cmd).toEqual(['echo', 'hello '])
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

describe('transformEnvForm / transformVolumeForm / transformLabelForm', () => {
  it('derives env from envList, skips empty keys, and preserves value whitespace', () => {
    const res = transformEnvForm({
      env: ['STALE=ignored'],
      envList: [
        { key: '  GREETING ', value: '  hello world  ' },
        { key: '   ', value: 'skip' },
      ],
      envText: '',
    })
    expect(res.env).toEqual(['GREETING=  hello world  '])
  })

  it('derives binds from volumeList without waiting for blur', () => {
    const res = transformVolumeForm({
      binds: [],
      volumeList: [
        { source: '/data/app ', target: ' /app', readonly: true },
        { source: '', target: '/skip', readonly: false },
      ],
      volumeText: '',
    })
    expect(res.binds).toEqual(['/data/app:/app:ro'])
  })

  it('derives labels from labelList without waiting for blur', () => {
    const res = transformLabelForm({
      labelList: [
        { key: ' env ', value: 'prod' },
        { key: '', value: 'ignored' },
      ],
      labels: {},
    })
    expect(res.labels).toEqual({ env: 'prod' })
  })
})

describe('transformNetworkForm host/none/bridge handling', () => {
  it('sets networkMode and clears publishAllPorts and networkConfig for host mode', () => {
    const res = transformNetworkForm(
      {
        configMode: 'custom',
        publishAllPorts: true,
        dns: [],
        dnsSearch: [],
        dnsOptions: [],
        extraHosts: [],
        customNetworks: [
          {
            name: 'host',
            exists: true,
          },
        ],
      },
      true,
      'host-app',
    )
    expect(res.networkMode).toBe('host')
    expect(res.publishAllPorts).toBe(false)
    expect(res.networksToCreate).toBeUndefined()
    expect(res.networkConfig).toBeUndefined()
  })

  it('omits aliases when connecting directly to default bridge network in custom mode', () => {
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
            name: 'bridge',
            exists: true,
            aliases: ['ignored-alias'],
          },
        ],
      },
      false,
      'bridge-app',
    )
    expect(res.networkMode).toBe('bridge')
    expect(res.networkConfig?.endpointsConfig?.bridge?.aliases).toBeUndefined()
  })
})
