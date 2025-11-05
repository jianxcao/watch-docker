<template>
  <div class="volume-detail-page">
    <n-spin :show="loading">
      <div v-if="volumeDetail" class="detail-container">
        <!-- 基本信息 -->
        <n-card title="基本信息" class="info-card">
          <div class="info-grid">
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16">
                  <volumeIcon class="name-icon" />
                </n-icon>
                Volume 名称
              </div>
              <div class="info-value">{{ volumeDetail.volume.name }}</div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16"> <diskIcon class="disk-icon" /> </n-icon>驱动类型
              </div>
              <div class="info-value">
                <n-tag :bordered="false" type="default" round>{{
                  volumeDetail.volume.driver
                }}</n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="16"> <GlobeOutline class="scope-icon" /> </n-icon>作用域
              </div>
              <div class="info-value">
                <n-tag
                  :bordered="false"
                  :type="volumeDetail.volume.scope === 'local' ? 'success' : 'info'"
                  round
                >
                  {{ volumeDetail.volume.scope === 'local' ? '本地' : '全局' }}
                </n-tag>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">
                <n-icon size="14"> <CalendarIcon class="calendar-icon" /> </n-icon>创建时间
              </div>
              <div class="info-value">{{ formatCreatedTime(volumeDetail.volume.createdAt) }}</div>
            </div>
            <div class="info-item info-item-full">
              <div class="info-label">
                <n-icon size="16"> <LayersOutline class="layers-icon" /> </n-icon>挂载点
              </div>
              <div class="info-value">
                <n-text code>{{ volumeDetail.volume.mountpoint }}</n-text>
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">大小</div>
              <div class="info-value">
                {{ formatBytes(volumeDetail.volume.usageData?.size || 0) }}
              </div>
            </div>
            <div class="info-item">
              <div class="info-label">引用次数</div>
              <div class="info-value">
                <n-tag :bordered="true" type="warning" round>
                  {{ volumeDetail.volume.usageData?.refCount || 0 }} 个容器
                </n-tag>
              </div>
            </div>
          </div>
        </n-card>

        <!-- 标签信息 -->
        <n-card v-if="hasLabels" title="标签" class="info-card">
          <n-space>
            <n-tag
              v-for="(value, key) in volumeDetail.volume.labels"
              :key="key"
              :bordered="false"
              type="info"
            >
              {{ key }}: {{ value }}
            </n-tag>
          </n-space>
        </n-card>

        <!-- 已连接的容器 -->
        <n-card title="已连接的容器" class="info-card">
          <div
            v-if="!volumeDetail.containers || volumeDetail.containers.length === 0"
            class="empty-container"
          >
            <n-empty description="没有容器使用此 Volume" />
          </div>
          <div v-else class="container-list">
            <div
              v-for="container in volumeDetail.containers"
              :key="container.id"
              class="container-item"
              @click="handleContainerClick(container)"
            >
              <div class="container-info">
                <div class="container-name">
                  <n-icon size="20" class="mr-2">
                    <CubeOutline />
                  </n-icon>
                  {{ container.name }}
                  <n-tag
                    :bordered="false"
                    size="small"
                    :type="container.running ? 'success' : 'default'"
                    class="ml-2"
                  >
                    {{ container.running ? '运行中' : '已停止' }}
                  </n-tag>
                </div>
                <div class="container-image">
                  <n-text depth="3">{{ container.image }}</n-text>
                </div>
                <div class="container-mount">
                  <n-icon size="16" class="mr-1">
                    <FolderOpenOutline />
                  </n-icon>
                  <n-text depth="3" code class="text-xs">
                    {{ container.destination }}
                  </n-text>
                  <n-tag :bordered="false" size="small" type="info" class="ml-2">
                    {{ container.mode }}
                  </n-tag>
                </div>
              </div>
              <div class="container-action">
                <n-icon size="20">
                  <ChevronForwardOutline />
                </n-icon>
              </div>
            </div>
          </div>
        </n-card>
      </div>
    </n-spin>

    <!-- Teleport 到页面头部 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div class="flex items-center gap-3">
          <!-- 返回按钮 -->
          <n-button @click="handleBack" text circle>
            <template #icon>
              <n-icon size="20">
                <ArrowBackOutline />
              </n-icon>
            </template>
          </n-button>
          <div>
            <n-h2 class="m-0 text-lg">Volume 详情</n-h2>
            <n-text depth="3" class="text-xs max-md:hidden">
              {{ volumeName }}
            </n-text>
          </div>
        </div>
        <div class="flex gap-2">
          <!-- 刷新按钮 -->
          <n-button @click="handleRefresh" :loading="loading" circle size="tiny">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
          <!-- 删除按钮 -->
          <n-button @click="handleDelete" circle size="tiny" tertiary type="error">
            <template #icon>
              <n-icon>
                <TrashOutline />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { volumeApi } from '@/common/api'
import type { VolumeDetailResponse, ContainerRef } from '@/common/types'
import { formatBytes } from '@/common/utils'
import volumeIcon from '@/assets/svg/volume.svg?component'
import diskIcon from '@/assets/svg/disk.svg?component'
import {
  RefreshOutline,
  TrashOutline,
  ArrowBackOutline,
  CubeOutline,
  FolderOpenOutline,
  ChevronForwardOutline,
  GlobeOutline,
  LayersOutline,
  CalendarOutline as CalendarIcon,
} from '@vicons/ionicons5'
import { useDialog, useMessage } from 'naive-ui'
import dayjs from 'dayjs'

const route = useRoute()
const router = useRouter()
const dialog = useDialog()
const message = useMessage()

const loading = ref(false)
const volumeDetail = ref<VolumeDetailResponse | null>(null)

const volumeName = computed(() => route.params.name as string)

const hasLabels = computed(() => {
  return volumeDetail.value && Object.keys(volumeDetail.value.volume.labels || {}).length > 0
})

// 格式化创建时间
const formatCreatedTime = (createdAt: string): string => {
  if (!createdAt) {
    return '-'
  }
  return dayjs(createdAt).format('YYYY-MM-DD HH:mm:ss')
}

// 获取Volume详情
const fetchVolumeDetail = async () => {
  loading.value = true
  try {
    const response = await volumeApi.getVolume(volumeName.value)
    if (response.code === 0) {
      volumeDetail.value = response.data
    } else {
      message.error(`获取 Volume 详情失败：${response.msg}`)
      router.push('/volumes')
    }
  } catch (error: any) {
    message.error(`获取 Volume 详情失败：${error.message || '未知错误'}`)
    router.push('/volumes')
  } finally {
    loading.value = false
  }
}

// 处理返回
const handleBack = () => {
  router.push('/volumes')
}

// 处理刷新
const handleRefresh = async () => {
  await fetchVolumeDetail()
}

// 处理删除
const handleDelete = () => {
  if (!volumeDetail.value) {
    return
  }

  const refCount = volumeDetail.value.volume.usageData?.refCount || 0

  if (refCount > 0) {
    dialog.warning({
      title: '无法删除',
      content: `此 Volume 正在被 ${refCount} 个容器使用，无法删除。请先停止或删除使用该 Volume 的容器。`,
      positiveText: '知道了',
    })
    return
  }

  const d = dialog.warning({
    title: '确认删除',
    content: `确定要删除 Volume "${volumeName.value}" 吗？此操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        d.loading = true
        const response = await volumeApi.deleteVolume(volumeName.value, false)
        if (response.code === 0) {
          message.success('Volume 删除成功')
          router.push('/volumes')
        } else {
          message.error(`删除失败：${response.msg}`)
        }
      } catch (error: any) {
        message.error(`删除失败：${error.message || '未知错误'}`)
      } finally {
        d.loading = false
      }
    },
  })
}

// 处理容器点击
const handleContainerClick = (container: ContainerRef) => {
  router.push(`/containers/${container.id}`)
}

// 页面初始化
onMounted(async () => {
  await fetchVolumeDetail()
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

.volume-detail-page {
  width: 100%;

  .detail-container {
    display: flex;
    flex-direction: column;
    gap: 16px;

    .info-card {
      box-shadow: var(--box-shadow-1);
    }

    .info-grid {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 24px 32px;
      .info-item {
        display: flex;
        flex-direction: column;
        gap: 8px;

        &.info-item-full {
          grid-column: 1 / -1;
        }
        .info-label {
          font-size: 14px;
          line-height: 20px;
          color: var(--text-color-3);
          font-weight: 500;
          white-space: nowrap;
          display: inline-flex;
          align-items: center;
          gap: 8px;
          .name-icon,
          .scope-icon,
          .layers-icon,
          .calendar-icon {
            color: var(--primary-color);
          }
          .disk-icon {
            color: #2b7fff;
          }
        }

        .info-value {
          font-size: 14px;
          color: var(--n-text-color-1);
          word-break: break-all;
        }
      }
    }

    .empty-container {
      padding: 40px 0;
    }

    .container-list {
      display: flex;
      flex-direction: column;
      gap: 12px;

      .container-item {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 16px;
        border-radius: 8px;
        background: var(--n-color-embedded);
        cursor: pointer;
        transition: all 0.3s ease;

        &:hover {
          background: var(--n-color-embedded-popover);
          transform: translateX(4px);
        }

        .container-info {
          flex: 1;
          display: flex;
          flex-direction: column;
          gap: 8px;

          .container-name {
            font-weight: 600;
            font-size: 16px;
            display: flex;
            align-items: center;
          }

          .container-image {
            font-size: 14px;
          }

          .container-mount {
            display: flex;
            align-items: center;
            font-size: 12px;
          }
        }

        .container-action {
          display: flex;
          align-items: center;
          color: var(--text-color-3);
        }
      }
    }
  }
}

@media (max-width: 768px) {
  .volume-detail-page {
    .detail-container {
      gap: 12px;

      .info-grid {
        grid-template-columns: 1fr;
        gap: 20px;

        .info-item {
          &.info-item-full {
            grid-column: 1;
          }
        }
      }
    }
  }
}
</style>
