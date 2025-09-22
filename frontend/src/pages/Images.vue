<template>
    <div class="images-page">
        <!-- 页面头部 -->
        <n-card class="page-header">
            <n-space align="center" justify="space-between">
                <div>
                    <n-h2 style="margin: 0;">镜像管理</n-h2>
                    <n-text depth="3">
                        共 {{ imageStore.stats.total }} 个镜像，
                        总大小 {{ imageStore.stats.formattedTotalSize }}，
                        {{ imageStore.danglingImages.length }} 个悬空镜像
                    </n-text>
                </div>

                <n-space>
                    <!-- 搜索 -->
                    <n-input v-model:value="searchKeyword" placeholder="搜索镜像标签或ID" style="width: 200px;" clearable>
                        <template #prefix>
                            <n-icon>
                                <SearchOutline />
                            </n-icon>
                        </template>
                    </n-input>

                    <!-- 批量删除悬空镜像 -->
                    <n-button v-if="imageStore.danglingImages.length > 0" @click="handleDeleteDangling" type="warning"
                        ghost>
                        <template #icon>
                            <n-icon>
                                <TrashOutline />
                            </n-icon>
                        </template>
                        清理悬空镜像
                    </n-button>

                    <!-- 刷新按钮 -->
                    <n-button @click="handleRefresh" :loading="imageStore.loading" circle>
                        <template #icon>
                            <n-icon>
                                <RefreshOutline />
                            </n-icon>
                        </template>
                    </n-button>
                </n-space>
            </n-space>
        </n-card>

        <!-- 镜像列表 -->
        <div class="images-content">
            <n-spin :show="imageStore.loading && filteredImages.length === 0">
                <div v-if="filteredImages.length === 0 && !imageStore.loading" class="empty-state">
                    <n-empty description="没有找到镜像">
                        <template #extra>
                            <n-button @click="handleRefresh">刷新数据</n-button>
                        </template>
                    </n-empty>
                </div>

                <div v-else class="images-grid" :class="{
                    'grid-cols-1': isMobile,
                    'grid-cols-2': isTablet,
                    'grid-cols-3': isLaptop,
                    'grid-cols-4': isDesktop,
                }">
                    <n-card v-for="image in filteredImages" :key="image.id" hoverable class="image-card">
                        <template #header>
                            <n-space align="center" justify="space-between">
                                <div class="image-title">
                                    <n-text strong>{{ imageStore.getImageDisplayTag(image) }}</n-text>
                                    <n-tag v-if="imageStore.isDanglingImage(image)" type="warning" size="small"
                                        style="margin-left: 8px;">
                                        悬空
                                    </n-tag>
                                </div>
                            </n-space>
                        </template>

                        <n-space vertical>
                            <!-- 镜像信息 -->
                            <n-descriptions :column="1" size="small">
                                <n-descriptions-item label="ID">
                                    <n-text code class="image-id">{{ imageStore.getDigestDisplayText(image) }}</n-text>
                                </n-descriptions-item>

                                <n-descriptions-item label="标签">
                                    <n-text class="image-tags">{{ imageHooks.getTagsDisplayText(image) }}</n-text>
                                </n-descriptions-item>

                                <n-descriptions-item label="大小">
                                    <n-text>{{ imageStore.formatSize(image.size) }}</n-text>
                                </n-descriptions-item>

                                <n-descriptions-item label="创建时间">
                                    <n-text :depth="3">{{ imageHooks.formatCreateTime(image.created) }}</n-text>
                                </n-descriptions-item>

                                <n-descriptions-item label="年龄">
                                    <n-text :depth="3">{{ imageHooks.getImageAge(image.created) }}</n-text>
                                </n-descriptions-item>
                            </n-descriptions>

                            <!-- 完整标签列表 -->
                            <div v-if="image.repoTags && image.repoTags.length > 1" class="full-tags">
                                <n-text strong style="font-size: 12px;">所有标签:</n-text>
                                <n-space style="margin-top: 4px;" vertical size="small">
                                    <n-tag v-for="tag in validTags(image.repoTags)" :key="tag" size="tiny" type="info">
                                        {{ tag }}
                                    </n-tag>
                                </n-space>
                            </div>
                        </n-space>

                        <template #action>
                            <n-space justify="end">
                                <n-button @click="() => handleDelete(image, false)" type="error" size="small" ghost
                                    :loading="imageStore.isImageDeleting(image.id)">
                                    <template #icon>
                                        <n-icon>
                                            <TrashOutline />
                                        </n-icon>
                                    </template>
                                    删除
                                </n-button>
                            </n-space>
                        </template>
                    </n-card>
                </div>
            </n-spin>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useImageStore } from '@/store/image'
import { useImage } from '@/hooks/useImage'
import { useResponsive } from '@/hooks/useResponsive'
import type { ImageInfo } from '@/common/types'
import {
    SearchOutline,
    RefreshOutline,
    TrashOutline,
} from '@vicons/ionicons5'

const imageStore = useImageStore()
const imageHooks = useImage()
const { isMobile, isTablet, isLaptop, isDesktop } = useResponsive()

// 搜索关键词
const searchKeyword = ref('')

// 过滤后的镜像列表
const filteredImages = computed(() => {
    let images = imageStore.images

    // 搜索过滤
    if (searchKeyword.value) {
        const keyword = searchKeyword.value.toLowerCase()
        images = images.filter(image => {
            const displayTag = imageStore.getImageDisplayTag(image).toLowerCase()
            const id = image.id.toLowerCase()
            const tags = image.repoTags?.join(' ').toLowerCase() || ''

            return displayTag.includes(keyword) ||
                id.includes(keyword) ||
                tags.includes(keyword)
        })
    }

    // 按创建时间排序（最新的在前）
    return images.sort((a, b) => b.created - a.created)
})

// 获取有效标签（过滤掉 <none>:<none>）
const validTags = (tags: string[]): string[] => {
    return tags.filter(tag => tag !== '<none>:<none>')
}

// 操作处理函数
const handleDelete = async (image: ImageInfo, force: boolean = false) => {
    await imageHooks.handleDelete(image, force)
}

const handleDeleteDangling = async () => {
    await imageHooks.handleDeleteDangling()
}

const handleRefresh = async () => {
    await imageHooks.handleRefresh()
}

// 页面初始化
onMounted(async () => {
    if (imageStore.images.length === 0) {
        await imageStore.fetchImages()
    }
})
</script>

<style scoped lang="less">
.images-page {
    .page-header {
        margin-bottom: 16px;
    }

    .images-content {
        position: relative;
        min-height: 400px;
    }

    .empty-state {
        padding: 60px 0;
        text-align: center;
    }

    .images-grid {
        display: grid;
        gap: 16px;

        &.grid-cols-1 {
            grid-template-columns: 1fr;
        }

        &.grid-cols-2 {
            grid-template-columns: repeat(2, 1fr);
        }

        &.grid-cols-3 {
            grid-template-columns: repeat(3, 1fr);
        }

        &.grid-cols-4 {
            grid-template-columns: repeat(4, 1fr);
        }
    }

    .image-card {
        .image-title {
            display: flex;
            align-items: center;
            flex-wrap: wrap;
        }

        .image-id {
            font-size: 11px;
            font-family: 'Monaco', 'Consolas', monospace;
        }

        .image-tags {
            word-break: break-all;
            font-family: 'Monaco', 'Consolas', monospace;
            font-size: 12px;
        }

        .full-tags {
            padding: 8px 0;
            border-top: 1px solid #f0f0f0;
        }
    }
}

// 响应式调整
@media (max-width: 768px) {
    .images-page {
        .images-grid {
            gap: 8px;
        }
    }
}

@media (max-width: 640px) {
    .images-page {
        .page-header {
            .n-space {
                flex-direction: column;
                align-items: stretch !important;

                &>div:last-child {
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
