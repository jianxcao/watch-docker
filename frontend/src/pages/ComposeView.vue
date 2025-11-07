<template>
  <div class="compose-page">
    <!-- 页面头部操作 -->
    <n-space>
      <!-- 状态筛选菜单 -->
      <n-dropdown :options="statusFilterMenuOptions" @select="handleFilterSelect">
        <n-button circle size="small" :type="statusFilter ? 'primary' : 'default'">
          <template #icon>
            <n-icon>
              <FunnelOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- 排序菜单 -->
      <n-dropdown :options="sortMenuOptions" @select="handleSortSelect">
        <n-button circle size="small" :type="isSortActive ? 'primary' : 'default'">
          <template #icon>
            <n-icon>
              <SwapVerticalOutline />
            </n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- 搜索框 -->
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索项目名称或路径"
        style="width: 200px"
        clearable
      >
        <template #prefix>
          <n-icon>
            <SearchOutline />
          </n-icon>
        </template>
      </n-input>
    </n-space>

    <!-- 项目列表 -->
    <div class="compose-content">
      <n-spin :show="composeStore.loading && filteredProjects.length === 0">
        <!-- 空状态 -->
        <div v-if="filteredProjects.length === 0 && !composeStore.loading" class="empty-state">
          <n-empty description="没有找到 Compose 项目">
            <template #extra>
              <n-button @click="handleRefresh">刷新数据</n-button>
            </template>
          </n-empty>
        </div>

        <!-- 项目网格 -->
        <div
          v-else
          class="compose-grid"
          :class="{
            'grid-cols-1': isMobile,
            'grid-cols-2': isTablet || isLaptop,
            'grid-cols-3': isDesktop,
            'grid-cols-4': isDesktopLarge,
          }"
        >
          <ComposeCard
            v-for="project in filteredProjects"
            :key="project.name"
            :project="project"
            :loading="composeStore.isProjectOperating(project.name).value"
            @log="() => handleViewLogs(project)"
          />
        </div>
      </n-spin>
    </div>

    <!-- 页面标题信息 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div>
          <n-h2 class="m-0 text-lg"> Compose 项目管理 </n-h2>
          <n-text depth="3" class="text-xs max-md:hidden">
            共 {{ composeStore.stats.total }} 个项目， {{ composeStore.stats.running }} 个运行中
          </n-text>
        </div>
        <div class="flex gap-2">
          <n-button circle size="tiny" type="primary" @click="handleAddProject">
            <template #icon>
              <n-icon>
                <AddCircleOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="composeStore.loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>

    <!-- 日志弹窗 -->
    <ComposeLogsModal v-model:show="showLogsModal" :project="currentProject" />
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useCompose } from '@/hooks/useCompose'
import { useResponsive } from '@/hooks/useResponsive'
import { useComposeStore } from '@/store/compose'
import ComposeCard from '@/components/ComposeCard.vue'
import ComposeLogsModal from '@/components/ComposeLogsModal.vue'
import {
  AppsOutline,
  FunnelOutline,
  PlayOutline,
  RadioButtonOnOutline,
  RefreshOutline,
  SearchOutline,
  StopOutline,
  SwapVerticalOutline,
  TextOutline,
  WarningOutline,
  AddCircleOutline,
} from '@vicons/ionicons5'
import { computed, onMounted, ref, watchEffect } from 'vue'
import { NIcon, type DropdownOption } from 'naive-ui'
import { renderIcon, sleep } from '@/common/utils'
import type { ComposeProject } from '@/common/types'

const router = useRouter()
const showLogsModal = ref(false)
const currentProject = ref<ComposeProject | null>(null)

const composeStore = useComposeStore()
const { handleRefresh } = useCompose()
const { isMobile, isTablet, isLaptop, isDesktop, isDesktopLarge } = useResponsive()

// 响应式状态
const searchKeyword = ref('')
const statusFilter = ref<string>('')
const sortBy = ref<string>('name')
const sortOrder = ref<'asc' | 'desc'>('asc')

const handleViewLogs = (project: ComposeProject) => {
  currentProject.value = project
  showLogsModal.value = true
}

const handleAddProject = () => {
  router.push({ name: 'compose-create' })
}

watchEffect(() => {
  if (!showLogsModal.value) {
    currentProject.value = null
  }
})

// 状态筛选菜单选项
const statusFilterMenuOptions = computed<DropdownOption[]>(() => [
  {
    label: '全部状态',
    key: '',
    icon: renderIcon(AppsOutline),
  },
  {
    label: '运行中',
    key: 'running',
    icon: renderIcon(PlayOutline),
  },
  {
    label: '已停止',
    key: 'exited',
    icon: renderIcon(StopOutline),
  },
  {
    label: '其他',
    key: 'other',
    icon: renderIcon(WarningOutline),
  },
])

const sortMenuOptions = computed(() => [
  {
    label: `名称 ${sortBy.value === 'name' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'name',
    icon: renderIcon(TextOutline),
  },
  {
    label: `状态 ${sortBy.value === 'status' ? (sortOrder.value === 'asc' ? '↑' : '↓') : ''}`,
    key: 'status',
    icon: renderIcon(RadioButtonOnOutline),
  },
])

// 检查排序是否激活
const isSortActive = computed(() => sortBy.value !== 'name' || sortOrder.value !== 'asc')

// 过滤后的项目列表
const filteredProjects = computed(() => {
  let result = composeStore.projects

  // 状态筛选
  if (statusFilter.value) {
    result = result.filter((project) => {
      if (statusFilter.value === 'other') {
        return project.status !== 'running' && project.status !== 'exited'
      }
      return project.status === statusFilter.value
    })
  }

  // 搜索筛选
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    result = result.filter((project) => project.name.toLowerCase().includes(keyword))
  }

  // 排序
  result = [...result].sort((a, b) => {
    let comparison = 0

    switch (sortBy.value) {
      case 'name':
        comparison = a.name.localeCompare(b.name)
        break
      case 'status':
        comparison = a.status.localeCompare(b.status)
        break
      default:
        comparison = a.name.localeCompare(b.name)
    }

    return sortOrder.value === 'desc' ? -comparison : comparison
  })

  return result
})

// 处理状态筛选选择
const handleFilterSelect = (key: string) => {
  statusFilter.value = key
}

// 处理排序菜单选择
const handleSortSelect = (key: string) => {
  if (sortBy.value === key) {
    // 如果选择的是相同字段，切换升序/降序
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    // 如果选择的是不同字段，设置新字段并默认为升序
    sortBy.value = key
    sortOrder.value = 'asc'
  }
}

// 组件挂载后加载数据
onMounted(async () => {
  try {
    await composeStore.fetchProjects()
  } catch (error) {
    console.error('初始化 Compose 项目数据失败:', error)
  }
})
const visibility = useDocumentVisibility()

watch(visibility, (newVal) => {
  if (newVal === 'visible') {
    sleep(1000).then(() => {
      composeStore.fetchProjects()
    })
  }
})
</script>

<style scoped lang="less">
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
}

.compose-page {
  width: 100%;

  .compose-content {
    position: relative;
    min-height: 400px;
    margin-block: 16px;

    .n-spin-container {
      min-height: 400px;
    }
  }

  .empty-state {
    padding: 60px 0;
    text-align: center;
  }

  .compose-grid {
    display: grid;
    gap: 16px;

    &.grid-cols-1 {
      grid-template-columns: 1fr;
    }

    &.grid-cols-2 {
      grid-template-columns: repeat(2, minmax(1fr, 50%));
    }

    &.grid-cols-3 {
      grid-template-columns: repeat(3, minmax(1fr, 33.33%));
    }

    &.grid-cols-4 {
      grid-template-columns: repeat(4, minmax(1fr, 25%));
    }
  }
}

// 响应式调整
@media (max-width: 768px) {
  .compose-page {
    .compose-grid {
      gap: 8px;
    }
  }
}

@media (max-width: 640px) {
  .compose-page {
    .page-header {
      .n-space {
        flex-direction: column;
        align-items: stretch !important;

        & > div:last-child {
          .n-space {
            flex-wrap: wrap;

            .n-input {
              width: 100% !important;
              min-width: 200px;
            }
          }
        }
      }
    }
  }
}
</style>
