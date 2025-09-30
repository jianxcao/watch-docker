<template>
  <div class="compose-card" :data-theme="settingStore.setting.theme"
    :class="['card-status-' + project.status, { 'card-updating': loading }]">

    <div class="card-content">
      <!-- 项目头部信息 -->
      <div class="project-header">
        <!-- 渐变图标容器 -->
        <div class="project-logo" :class="'logo-' + project.status">
          <n-icon size="20" class="logo-icon">
            <ComposeIcon />
          </n-icon>
        </div>

        <!-- 项目信息区域 -->
        <div class="project-info">
          <!-- 标题和菜单 -->
          <div class="project-title-row">
            <div class="project-name">{{ project.name }}</div>
            <n-dropdown :options="dropdownOptions" @select="handleMenuSelect" trigger="click">
              <n-button quaternary circle size="small" class="menu-btn">
                <template #icon>
                  <n-icon>
                    <MenuIcon />
                  </n-icon>
                </template>
              </n-button>
            </n-dropdown>
          </div>

          <!-- 状态和容器信息 -->
          <div class="project-meta">
            <div class="status-badge" :class="'status-' + project.status">
              <span class="status-dot"></span>
              <span class="status-text">{{ getStatusText(project.status) }}</span>
            </div>

            <div class="container-counts">
              <div class="count-badge total-count">
                <n-icon size="12" class="count-icon">
                  <LayersOutline />
                </n-icon>
                <span>{{ project.createdCount + project.runningCount + project.exitedCount }}</span>
              </div>
              <div class="count-badge running-count" :class="{ 'has-running': project.runningCount > 0 }">
                <span class="running-dot"></span>
                <span>{{ project.runningCount }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 操作按钮区域 -->
      <div class="action-buttons">
        <n-button v-if="project.status === 'exited' || project.status === 'partial'" text class="action-btn"
          @click="handleMenuSelect('start')">
          <template #icon>
            <n-icon>
              <PlayOutline />
            </n-icon>
          </template>
          启动
        </n-button>
        <n-button v-if="project.status === 'running' || project.status === 'partial'" text class="action-btn"
          @click="handleMenuSelect('stop')">
          <template #icon>
            <n-icon>
              <StopOutline />
            </n-icon>
          </template>
          停止
        </n-button>
        <n-button v-if="project.status === 'running'" text class="action-btn" @click="handleMenuSelect('restart')">
          <template #icon>
            <n-icon>
              <RefreshOutline />
            </n-icon>
          </template>
          重启
        </n-button>
        <n-button v-if="project.status === 'unknown' || project.status === 'exited' || project.status === 'draft'" text
          class="action-btn" @click="handleMenuSelect('create')">
          <template #icon>
            <n-icon>
              <RefreshOutline />
            </n-icon>
          </template>
          创建
        </n-button>
        <n-button text class="action-btn" @click="handleMenuSelect('view-logs')">
          <template #icon>
            <n-icon>
              <LogIcon />
            </n-icon>
          </template>
          日志
        </n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  NIcon,
  type DropdownOption
} from 'naive-ui'
import { useSettingStore } from '@/store/setting'
import type { ComposeProject } from '@/common/types'

import ComposeIcon from '@/assets/svg/compose.svg?component'
import {
  LayersOutline,
  PlayOutline,
  StopOutline,
  RefreshOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import LogIcon from '@/assets/svg/log.svg?component'
import MenuIcon from '@/assets/svg/overflowMenuVertical.svg?component'
import { useCompose } from '@/hooks/useCompose'
import { renderIcon } from '@/common/utils'

const {
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleViewLogs,
  handleCreate
} = useCompose()

interface Props {
  project: ComposeProject
  loading?: boolean
}

const props = defineProps<Props>()
const settingStore = useSettingStore()

// 获取状态文本
const getStatusText = (status: string) => {
  const statusMap: Record<string, string> = {
    running: '运行中',
    partial: '部分运行',
    exited: '已停止',
    paused: '暂停',
    draft: '草稿',
    created_stack: '创建中',
    unknown: '未知',
  }
  return statusMap[status] || '未知'
}

// 下拉菜单选项
const dropdownOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = []
  const project = props.project
  // 根据项目状态显示不同的操作选项
  if (project.status === 'exited' || project.status === 'partial') {
    options.push({
      label: '启动',
      key: 'start',
      icon: renderIcon(PlayOutline)
    })
  }

  if (project.status === 'running' || project.status === 'partial') {
    options.push({
      label: '停止',
      key: 'stop',
      icon: renderIcon(StopOutline)
    })
  }

  options.push(
    {
      label: '创建',
      key: 'create',
      icon: renderIcon(RefreshOutline)
    },
    {
      type: 'divider',
      key: 'divider1'
    },
    {
      label: '日志',
      key: 'logs',
      icon: renderIcon(LogIcon)
    }
  )
  if (project.status !== 'draft' && project.status !== 'created_stack' && project.status !== 'unknown') {
    options.push({
      type: 'divider',
      key: 'divider2'
    },
      {
        label: '删除',
        key: 'delete',
        icon: renderIcon(TrashOutline),
        props: {
          style: 'color: #ff4d4f'
        }
      })
  }

  return options
})

// 处理菜单选择
const handleMenuSelect = (key: string) => {
  switch (key) {
    case 'start':
      handleStart(props.project)
      break
    case 'stop':
      handleStop(props.project)
      break
    case 'restart':
      handleRestart(props.project)
      break
    case 'delete':
      handleDelete(props.project)
      break
    case 'logs':
      handleViewLogs(props.project)
      break
    case 'create':
      handleCreate(props.project)
      break
  }
}
</script>

<style scoped lang="less">
.compose-card {
  position: relative;
  border-radius: 14px;
  transition: all 0.3s ease;
  overflow: hidden;
  color: var(--text-color-1);
  border: 1px solid rgba(49, 65, 88, 0.5);
  background: rgba(29, 41, 61, 0.5);
  min-width: 320px;

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--box-shadow-1);
  }

  .card-content {
    display: flex;
    flex-direction: column;
  }

  .project-header {
    display: flex;
    gap: 16px;
    padding: 21px;
    align-items: flex-start;

    // 渐变图标容器
    .project-logo {
      width: 48px;
      height: 48px;
      border-radius: 14px;
      display: flex;
      align-items: center;
      justify-content: center;
      border: 1px solid rgba(69, 85, 108, 0.3);
      flex-shrink: 0;
      box-shadow: 0px 4px 6px -4px rgba(0, 188, 125, 0.2), 0px 10px 15px -3px rgba(0, 188, 125, 0.2);

      .logo-icon {
        color: #FFFFFF;
      }

      &.logo-running,
      &.logo-partial {
        background: linear-gradient(135deg, rgba(0, 153, 102, 1) 0%, rgba(0, 122, 85, 1) 100%);
        box-shadow: 0px 4px 6px -4px rgba(0, 188, 125, 0.2), 0px 10px 15px -3px rgba(0, 188, 125, 0.2);
      }

      &.logo-exited,
      &.logo-unknown {
        background: linear-gradient(135deg, rgba(49, 65, 88, 1) 0%, rgba(49, 65, 88, 0.5) 100%);
        box-shadow: 0px 4px 6px -4px rgba(98, 116, 142, 0.1), 0px 10px 15px -3px rgba(98, 116, 142, 0.1);
      }

      &.logo-draft,
      &.logo-created_stack {
        background: linear-gradient(135deg, rgba(225, 113, 0, 1) 0%, rgba(187, 77, 0, 1) 100%);
        box-shadow: 0px 4px 6px -4px rgba(254, 154, 0, 0.2), 0px 10px 15px -3px rgba(254, 154, 0, 0.2);
      }
    }

    // 项目信息区域
    .project-info {
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 12px;
      min-width: 0;

      .project-title-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 8px;

        .project-name {
          font-size: 17px;
          font-weight: 400;
          line-height: 1.5;
          letter-spacing: -0.05em;
          overflow: hidden;
          text-overflow: ellipsis;
          white-space: nowrap;
        }

        .menu-btn {
          flex-shrink: 0;
        }
      }

      .project-meta {
        display: flex;
        align-items: center;
        gap: 8px;

        .status-badge {
          display: flex;
          align-items: center;
          gap: 6px;
          padding: 0 10px;
          height: 34px;
          border-radius: 10px;
          font-size: 14px;
          line-height: 1.428;
          letter-spacing: -0.01em;
          box-shadow: 0px 1px 2px -1px rgba(0, 0, 0, 0.1), 0px 1px 3px 0px rgba(0, 0, 0, 0.1);

          .status-dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
          }

          &.status-running,
          &.status-partial {
            background: rgba(0, 188, 125, 0.1);
            border: 1px solid rgba(0, 188, 125, 0.2);

            .status-dot {
              background: #00BC7D;
              opacity: 0.5;
            }

            .status-text {
              color: #00D492;
            }
          }

          &.status-exited,
          &.status-draft,
          &.status-created_stack,
          &.status-unknown {
            background: rgba(98, 116, 142, 0.1);
            border: 1px solid rgba(98, 116, 142, 0.2);

            .status-dot {
              background: #62748E;
            }

            .status-text {
              color: #90A1B9;
            }
          }

          &.status-paused {
            background: rgba(254, 154, 0, 0.1);
            border: 1px solid rgba(254, 154, 0, 0.2);

            .status-dot {
              background: #FE9A00;
            }

            .status-text {
              color: #FFB900;
            }
          }
        }

        .container-counts {
          display: flex;
          align-items: center;
          gap: 6px;

          .count-badge {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 0 8px;
            height: 30px;
            border-radius: 8px;
            font-size: 14px;
            line-height: 1.428;

            &.total-count {
              background: rgba(49, 65, 88, 0.4);
              border: 1px solid rgba(69, 85, 108, 0.3);

              .count-icon {
                color: #90A1B9;
              }

              span {
                color: #CAD5E2;
              }
            }

            &.running-count {
              background: rgba(0, 188, 125, 0.1);
              border: 1px solid rgba(0, 188, 125, 0.2);

              .running-dot {
                width: 6px;
                height: 6px;
                border-radius: 50%;
                background: #00BC7D;
              }

              span {
                color: #00D492;
              }

              &.has-running {
                box-shadow: 0px 1px 2px -1px rgba(0, 188, 125, 0.5), 0px 1px 3px 0px rgba(0, 188, 125, 0.5);
              }
            }
          }
        }
      }
    }
  }

  // 操作按钮区域
  .action-buttons {
    display: flex;
    border-top: 1px solid rgba(49, 65, 88, 0.5);
    background: rgba(29, 41, 61, 0.8);
    padding: 0 20px;
    gap: 4px;

    .action-btn {
      flex: 1;
      height: 56px;
      border-radius: 8px;
      color: #90A1B9;
      font-size: 14px;
      font-weight: 500;
      line-height: 1.428;
      letter-spacing: -0.01em;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 4px;

      &:hover {
        background: rgba(49, 65, 88, 0.3);
        color: #CAD5E2;
      }

      :deep(.n-button__icon) {
        margin: 0;
      }
    }
  }
}

// 亮色主题适配
.compose-card[data-theme='light'] {
  border-color: rgba(226, 232, 240, 1);
  background: rgba(248, 250, 252, 1);

  .project-header {
    .project-logo {

      // 在浅色主题下，保持渐变背景但图标保持白色以确保对比度
      .logo-icon {
        color: #FFFFFF;
      }

      // 针对不同状态优化边框颜色
      &.logo-running,
      &.logo-partial {
        border-color: rgba(0, 188, 125, 0.3);
      }

      &.logo-exited,
      &.logo-unknown {
        border-color: rgba(98, 116, 142, 0.3);
      }

      &.logo-draft,
      &.logo-created_stack {
        border-color: rgba(254, 154, 0, 0.3);
      }
    }

    .project-info {
      .project-title-row {
        .project-name {
          color: #1E293B;
        }
      }

      .project-meta {
        .status-badge {

          // 浅色主题下的状态标签优化
          &.status-running,
          &.status-partial {
            background: rgba(0, 188, 125, 0.15);
            border-color: rgba(0, 188, 125, 0.3);

            .status-text {
              color: #00875A;
            }
          }

          &.status-exited,
          &.status-unknown {
            background: rgba(98, 116, 142, 0.15);
            border-color: rgba(98, 116, 142, 0.3);

            .status-text {
              color: #475569;
            }
          }

          &.status-paused {
            background: rgba(254, 154, 0, 0.15);
            border-color: rgba(254, 154, 0, 0.3);

            .status-text {
              color: #B76E00;
            }
          }
        }

        .container-counts {
          .count-badge {
            &.total-count {
              background: rgba(148, 163, 184, 0.15);
              border-color: rgba(148, 163, 184, 0.3);

              .count-icon {
                color: #64748B;
              }

              span {
                color: #475569;
              }
            }

            &.running-count {
              background: rgba(0, 188, 125, 0.15);
              border-color: rgba(0, 188, 125, 0.3);

              span {
                color: #00875A;
              }
            }
          }
        }
      }
    }
  }

  .action-buttons {
    border-top-color: rgba(226, 232, 240, 1);
    background: rgba(241, 245, 249, 1);

    .action-btn {
      color: #64748B;

      &:hover {
        background: rgba(226, 232, 240, 0.5);
        color: #334155;
      }
    }
  }
}
</style>
