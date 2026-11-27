import { describe, expect, it, vi } from 'vitest'
import type { ComposeProject } from '@/common/types'
import {
  UNMANAGED_COMPOSE_TOOLTIP,
  buildComposeCardActionState,
  buildComposeDetailDropdownOptions,
  buildComposeManagedActionSocketUrl,
  dispatchComposeCardAction,
  dispatchComposeDetailAction,
} from './composeActionMenus'

const managedProject: ComposeProject = {
  name: 'managed-app',
  composeFile: '/apps/managed-app/compose.yaml',
  status: 'exited',
  runningCount: 0,
  exitedCount: 1,
  createdCount: 0,
  managedConfig: true,
}

const externalProject: ComposeProject = {
  name: 'external-app',
  composeFile: '/opt/external-app/compose.yaml',
  status: 'exited',
  runningCount: 0,
  exitedCount: 1,
  createdCount: 0,
  managedConfig: false,
}

describe('composeActionMenus managedConfig capability gating', () => {
  it('enables create, pull and deploy controls with zero restriction tooltips for managed projects', () => {
    const cardState = buildComposeCardActionState(managedProject, false)
    expect(cardState.managed).toBe(true)
    expect(cardState.canCreate).toBe(true)
    expect(cardState.createTooltip).toBeUndefined()

    const cardCreate = cardState.dropdownOptions.find((o) => o.key === 'create')
    const cardPull = cardState.dropdownOptions.find((o) => o.key === 'pull')
    const cardLogs = cardState.dropdownOptions.find((o) => o.key === 'logs')
    const cardStart = cardState.dropdownOptions.find((o) => o.key === 'start')
    expect(cardCreate?.disabled).toBe(false)
    expect(cardCreate?.tooltip).toBeUndefined()
    expect(cardPull?.disabled).toBe(false)
    expect(cardPull?.tooltip).toBeUndefined()
    expect(cardLogs?.disabled).toBeUndefined()
    expect(cardStart?.disabled).toBe(false)

    const detailOpts = buildComposeDetailDropdownOptions({
      project: managedProject,
      canDeploy: true,
      errorColor: '#ff4d4f',
    })
    const detailPull = detailOpts.find((o) => o.key === 'pull')
    const detailDeploy = detailOpts.find((o) => o.key === 'deploy')
    expect(detailPull?.disabled).toBe(false)
    expect(detailPull?.tooltip).toBeUndefined()
    expect(detailDeploy?.disabled).toBe(false)
    expect(detailDeploy?.tooltip).toBeUndefined()

    const fakeLoc = { protocol: 'https:', host: 'example.com' }
    expect(buildComposeManagedActionSocketUrl('up', managedProject, 'tok', fakeLoc)).toBe(
      'wss://example.com/api/v1/compose/up/managed-app/ws?token=tok&composeFile=%2Fapps%2Fmanaged-app%2Fcompose.yaml&projectName=managed-app',
    )
    expect(buildComposeManagedActionSocketUrl('pull', managedProject, 'tok', fakeLoc)).toBe(
      'wss://example.com/api/v1/compose/pull/managed-app/ws?token=tok&composeFile=%2Fapps%2Fmanaged-app%2Fcompose.yaml&projectName=managed-app',
    )
  })

  it('disables create, pull and deploy with tooltip and initiates 0 connections for external projects', async () => {
    const cardState = buildComposeCardActionState(externalProject, false)
    expect(cardState.managed).toBe(false)
    expect(cardState.canCreate).toBe(false)
    expect(cardState.createTooltip).toBe(UNMANAGED_COMPOSE_TOOLTIP)

    const cardCreate = cardState.dropdownOptions.find((o) => o.key === 'create')
    const cardPull = cardState.dropdownOptions.find((o) => o.key === 'pull')
    const cardLogs = cardState.dropdownOptions.find((o) => o.key === 'logs')
    const cardStart = cardState.dropdownOptions.find((o) => o.key === 'start')
    const cardDelete = cardState.dropdownOptions.find((o) => o.key === 'delete')
    expect(cardCreate?.disabled).toBe(true)
    expect(cardCreate?.tooltip).toBe(UNMANAGED_COMPOSE_TOOLTIP)
    expect(cardCreate?.props).toMatchObject({ title: UNMANAGED_COMPOSE_TOOLTIP })
    expect(cardPull?.disabled).toBe(true)
    expect(cardPull?.tooltip).toBe(UNMANAGED_COMPOSE_TOOLTIP)
    expect(cardPull?.props).toMatchObject({ title: UNMANAGED_COMPOSE_TOOLTIP })
    expect(cardLogs?.disabled).toBeUndefined()
    expect(cardStart?.disabled).toBe(false)
    expect(cardDelete?.disabled).toBeUndefined()

    const openedConnections: string[] = []
    const onStart = vi.fn()
    const onLogs = vi.fn()
    const onUnsupported = vi.fn()
    const cardHandlers = {
      onStart,
      onStop: vi.fn(),
      onRestart: vi.fn(),
      onDelete: vi.fn(),
      onLogs,
      onCreate: (p: ComposeProject) => {
        const url = buildComposeManagedActionSocketUrl('up', p, 'tok', {
          protocol: 'http:',
          host: 'localhost',
        })
        if (url) openedConnections.push(url)
      },
      onPull: (p: ComposeProject) => {
        const url = buildComposeManagedActionSocketUrl('pull', p, 'tok', {
          protocol: 'http:',
          host: 'localhost',
        })
        if (url) openedConnections.push(url)
      },
      onUnsupported,
    }

    expect(dispatchComposeCardAction('create', externalProject, cardHandlers)).toBe(false)
    expect(dispatchComposeCardAction('pull', externalProject, cardHandlers)).toBe(false)
    expect(openedConnections).toEqual([])
    expect(onUnsupported).toHaveBeenCalledTimes(2)
    expect(onUnsupported).toHaveBeenNthCalledWith(1, UNMANAGED_COMPOSE_TOOLTIP)
    expect(onUnsupported).toHaveBeenNthCalledWith(2, UNMANAGED_COMPOSE_TOOLTIP)
    expect(buildComposeManagedActionSocketUrl('up', externalProject, 'tok')).toBeUndefined()
    expect(buildComposeManagedActionSocketUrl('pull', externalProject, 'tok')).toBeUndefined()

    // 日志与按运行时名称启停仍可选
    expect(dispatchComposeCardAction('start', externalProject, cardHandlers)).toBe(true)
    expect(dispatchComposeCardAction('logs', externalProject, cardHandlers)).toBe(true)
    expect(onStart).toHaveBeenCalledWith(externalProject)
    expect(onLogs).toHaveBeenCalledWith(externalProject)

    // 详情页菜单同样禁用拉取镜像和重新部署，且不触发任何连接
    const runningExternal: ComposeProject = { ...externalProject, status: 'running', runningCount: 1, exitedCount: 0 }
    const detailOpts = buildComposeDetailDropdownOptions({
      project: runningExternal,
      canDeploy: true,
      errorColor: '#ff4d4f',
    })
    const detailPull = detailOpts.find((o) => o.key === 'pull')
    const detailDeploy = detailOpts.find((o) => o.key === 'deploy')
    const detailRestart = detailOpts.find((o) => o.key === 'restart')
    expect(detailPull?.disabled).toBe(true)
    expect(detailPull?.tooltip).toBe(UNMANAGED_COMPOSE_TOOLTIP)
    expect(detailPull?.props).toMatchObject({ title: UNMANAGED_COMPOSE_TOOLTIP })
    expect(detailDeploy?.disabled).toBe(true)
    expect(detailDeploy?.tooltip).toBe(UNMANAGED_COMPOSE_TOOLTIP)
    expect(detailRestart?.disabled).toBeUndefined()

    const detailUnsupported = vi.fn()
    const detailStop = vi.fn()
    const detailHandlers = {
      onDeploy: (p: ComposeProject) => {
        const url = buildComposeManagedActionSocketUrl('up', p, 'tok')
        if (url) openedConnections.push(url)
      },
      onStart: vi.fn(),
      onStop: detailStop,
      onRestart: vi.fn(),
      onDelete: vi.fn(),
      onPull: (p: ComposeProject) => {
        const url = buildComposeManagedActionSocketUrl('pull', p, 'tok')
        if (url) openedConnections.push(url)
      },
      onUnsupported: detailUnsupported,
    }
    expect(await dispatchComposeDetailAction('pull', runningExternal, detailHandlers)).toBe(false)
    expect(await dispatchComposeDetailAction('deploy', runningExternal, detailHandlers)).toBe(false)
    expect(openedConnections).toEqual([])
    expect(detailUnsupported).toHaveBeenCalledTimes(2)
    expect(await dispatchComposeDetailAction('stop', runningExternal, detailHandlers)).toBe(true)
    expect(detailStop).toHaveBeenCalledWith(runningExternal)
  })
})
