import { h, type Component, type VNode } from 'vue'
import { NTooltip, type DropdownOption, type DropdownGroupOption } from 'naive-ui'
import {
  PlayOutline,
  StopOutline,
  RefreshOutline,
  ReloadOutline,
  TrashOutline,
  CloudDownloadOutline,
} from '@vicons/ionicons5'
import type { ComposeProject } from '@/common/types'
import { renderIcon } from '@/common/utils'

export const UNMANAGED_COMPOSE_TOOLTIP = '配置文件位于受管目录之外'

export type ComposeDropdownOption = DropdownOption & {
  tooltip?: string
}

export function isManagedComposeProject(project?: ComposeProject | null): boolean {
  return Boolean(project && project.managedConfig !== false)
}

export function renderComposeDropdownOption({
  node,
  option,
}: {
  node: VNode
  option: DropdownOption & DropdownGroupOption
}) {
  const tooltip = (option as ComposeDropdownOption).tooltip
  if (!tooltip) {
    return node
  }
  return h(
    NTooltip,
    { placement: 'left' },
    {
      trigger: () => node,
      default: () => tooltip,
    },
  )
}

export interface ComposeCardActionState {
  managed: boolean
  canCreate: boolean
  createTooltip?: string
  dropdownOptions: ComposeDropdownOption[]
}

export function buildComposeCardActionState(
  project: ComposeProject,
  isOperating: boolean,
  logIcon?: Component,
): ComposeCardActionState {
  const managed = isManagedComposeProject(project)
  const tooltip = managed ? undefined : UNMANAGED_COMPOSE_TOOLTIP
  const options: ComposeDropdownOption[] = []

  if (project.status === 'exited' || project.status === 'partial') {
    options.push({
      label: '启动',
      key: 'start',
      icon: renderIcon(PlayOutline),
      disabled: isOperating,
    })
  }

  if (project.status === 'running' || project.status === 'partial') {
    options.push(
      {
        label: '停止',
        key: 'stop',
        icon: renderIcon(StopOutline),
        disabled: isOperating,
      },
      {
        label: '重启',
        key: 'restart',
        icon: renderIcon(ReloadOutline),
        disabled: isOperating,
      },
    )
  }

  options.push(
    {
      label: '创建',
      key: 'create',
      icon: renderIcon(RefreshOutline),
      disabled: isOperating || !managed,
      tooltip,
      props: tooltip ? { title: tooltip } : undefined,
    },
    {
      type: 'divider',
      key: 'divider1',
    },
    {
      label: '拉取镜像',
      key: 'pull',
      icon: renderIcon(CloudDownloadOutline),
      disabled: isOperating || !managed,
      tooltip,
      props: tooltip ? { title: tooltip } : undefined,
    },
    {
      label: '日志',
      key: 'logs',
      icon: logIcon ? renderIcon(logIcon) : undefined,
    },
  )

  const deleteLabel =
    project.status === 'draft' || project.status === 'created_stack' ? '删除配置' : '删除应用'
  options.push(
    {
      type: 'divider',
      key: 'divider2',
    },
    {
      label: deleteLabel,
      key: 'delete',
      icon: renderIcon(TrashOutline),
      props: {
        style: 'color: #ff4d4f',
      },
    },
  )

  return {
    managed,
    canCreate: managed && !isOperating,
    createTooltip: tooltip,
    dropdownOptions: options,
  }
}

export interface ComposeCardActionHandlers {
  onStart: (project: ComposeProject) => void
  onStop: (project: ComposeProject) => void
  onRestart: (project: ComposeProject) => void
  onDelete: (project: ComposeProject) => void
  onLogs: (project: ComposeProject) => void
  onCreate: (project: ComposeProject) => void
  onPull: (project: ComposeProject) => void
  onUnsupported?: (message: string) => void
}

export function dispatchComposeCardAction(
  key: string,
  project: ComposeProject,
  handlers: ComposeCardActionHandlers,
): boolean {
  if ((key === 'create' || key === 'pull') && !isManagedComposeProject(project)) {
    handlers.onUnsupported?.(UNMANAGED_COMPOSE_TOOLTIP)
    return false
  }
  switch (key) {
    case 'start':
      handlers.onStart(project)
      return true
    case 'stop':
      handlers.onStop(project)
      return true
    case 'restart':
      handlers.onRestart(project)
      return true
    case 'delete':
      handlers.onDelete(project)
      return true
    case 'view-logs':
    case 'logs':
      handlers.onLogs(project)
      return true
    case 'create':
      handlers.onCreate(project)
      return true
    case 'pull':
      handlers.onPull(project)
      return true
    default:
      return false
  }
}

export function buildComposeDetailDropdownOptions(params: {
  project?: ComposeProject | null
  canDeploy: boolean
  errorColor: string
}): ComposeDropdownOption[] {
  const { project, canDeploy, errorColor } = params
  const managed = isManagedComposeProject(project)
  const tooltip = managed ? undefined : UNMANAGED_COMPOSE_TOOLTIP
  const options: ComposeDropdownOption[] = []
  const status = project?.status

  if (status === 'exited' || status === 'partial') {
    options.push({
      label: '启动',
      key: 'start',
      icon: renderIcon(PlayOutline),
    })
  }

  if (status === 'running' || status === 'partial') {
    options.push(
      {
        label: '停止',
        key: 'stop',
        icon: renderIcon(StopOutline),
      },
      {
        label: '重启',
        key: 'restart',
        icon: renderIcon(ReloadOutline),
      },
    )
  }

  options.push(
    {
      label: '重新部署',
      key: 'deploy',
      disabled: !managed || !canDeploy,
      tooltip,
      props: tooltip ? { title: tooltip } : undefined,
      icon: renderIcon(RefreshOutline),
    },
    {
      type: 'divider',
      key: 'divider1',
    },
    {
      label: '拉取镜像',
      key: 'pull',
      disabled: !managed,
      tooltip,
      props: tooltip ? { title: tooltip } : undefined,
      icon: renderIcon(CloudDownloadOutline),
    },
  )

  if (status !== 'unknown') {
    const label = status === 'draft' || status === 'created_stack' ? '删除配置' : '删除应用'
    options.push(
      {
        type: 'divider',
        key: 'divider2',
      },
      {
        label,
        key: 'delete',
        icon: renderIcon(TrashOutline),
        props: {
          style: `color: ${errorColor}`,
        },
      },
    )
  }

  return options
}

export interface ComposeDetailActionHandlers {
  onDeploy: (project: ComposeProject) => Promise<void> | void
  onStart: (project: ComposeProject) => Promise<void> | void
  onStop: (project: ComposeProject) => Promise<void> | void
  onRestart: (project: ComposeProject) => Promise<void> | void
  onDelete: (project: ComposeProject) => Promise<void> | void
  onPull: (project: ComposeProject) => Promise<void> | void
  onUnsupported?: (message: string) => void
}

export async function dispatchComposeDetailAction(
  key: string,
  project: ComposeProject | undefined,
  handlers: ComposeDetailActionHandlers,
): Promise<boolean> {
  if (!project) {
    return false
  }
  if ((key === 'pull' || key === 'deploy') && !isManagedComposeProject(project)) {
    handlers.onUnsupported?.(UNMANAGED_COMPOSE_TOOLTIP)
    return false
  }
  switch (key) {
    case 'deploy':
      await handlers.onDeploy(project)
      return true
    case 'start':
      await handlers.onStart(project)
      return true
    case 'stop':
      await handlers.onStop(project)
      return true
    case 'restart':
      await handlers.onRestart(project)
      return true
    case 'delete':
      await handlers.onDelete(project)
      return true
    case 'pull':
      await handlers.onPull(project)
      return true
    default:
      return false
  }
}

export function buildComposeManagedActionSocketUrl(
  op: 'up' | 'pull',
  project: ComposeProject | null | undefined,
  token?: string | null,
  locationLike?: { protocol: string; host: string },
): string | undefined {
  if (!project || !isManagedComposeProject(project)) {
    return undefined
  }
  const loc = locationLike ?? (typeof window !== 'undefined' ? window.location : undefined)
  if (!loc) {
    return undefined
  }
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:'
  const tokenQuery = token ? `token=${encodeURIComponent(token)}&` : ''
  return `${protocol}//${loc.host}/api/v1/compose/${op}/${encodeURIComponent(project.name)}/ws?${tokenQuery}composeFile=${encodeURIComponent(project.composeFile)}&projectName=${encodeURIComponent(project.name)}`
}
