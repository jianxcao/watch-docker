<template>
  <div class="file-browser">
    <!-- 工具栏 -->
    <div class="toolbar" :class="{ 'toolbar-mobile': isMobile }">
      <!-- 桌面端：左侧导航+面包屑，右侧功能按钮 -->
      <template v-if="!isMobile">
        <div class="toolbar-left">
          <n-space :wrap="false">
            <n-button @click="goUp" :disabled="currentPath === '/'">
              <template #icon>
                <n-icon><ArrowUp /></n-icon>
              </template>
            </n-button>
            <n-button @click="goHome">
              <template #icon>
                <n-icon><Home /></n-icon>
              </template>
            </n-button>
          </n-space>

          <!-- 面包屑 -->
          <div class="toolbar-breadcrumb">
            <n-breadcrumb separator="/">
              <n-breadcrumb-item @click="navigateTo('/')">
                <a>root</a>
              </n-breadcrumb-item>
              <n-breadcrumb-item
                v-for="(segment, i) in pathSegments"
                :key="i"
                @click="navigateTo('/' + pathSegments.slice(0, i + 1).join('/'))"
              >
                <a>{{ segment }}</a>
              </n-breadcrumb-item>
            </n-breadcrumb>
          </div>
        </div>

        <div class="toolbar-right">
          <n-space :wrap="false">
            <!-- 上传按钮 -->
            <n-button @click="triggerFileUpload" :loading="uploading" v-if="canEdit">
              <template #icon>
                <n-icon><CloudUpload /></n-icon>
              </template>
              上传
            </n-button>

            <n-button @click="toggleHiddenFiles" :type="showHiddenFiles ? 'primary' : 'default'">
              <template #icon>
                <n-icon>
                  <Eye v-if="showHiddenFiles" />
                  <EyeOff v-else />
                </n-icon>
              </template>
            </n-button>
            <n-button @click="() => loadDirectory(currentPath)" :loading="loading">
              <template #icon>
                <n-icon><Refresh /></n-icon>
              </template>
            </n-button>
          </n-space>
        </div>
      </template>

      <!-- 移动端：第一行按钮，第二行面包屑 -->
      <template v-else>
        <div class="toolbar-buttons-row">
          <n-space :wrap="false">
            <n-button @click="goUp" :disabled="currentPath === '/'">
              <template #icon>
                <n-icon><ArrowUp /></n-icon>
              </template>
            </n-button>
            <n-button @click="goHome">
              <template #icon>
                <n-icon><Home /></n-icon>
              </template>
            </n-button>
          </n-space>

          <n-space :wrap="false">
            <!-- 上传按钮（移动端） -->
            <n-button @click="triggerFileUpload" :loading="uploading" v-if="canEdit">
              <template #icon>
                <n-icon><CloudUpload /></n-icon>
              </template>
            </n-button>

            <n-button @click="toggleHiddenFiles" :type="showHiddenFiles ? 'primary' : 'default'">
              <template #icon>
                <n-icon>
                  <Eye v-if="showHiddenFiles" />
                  <EyeOff v-else />
                </n-icon>
              </template>
            </n-button>
            <n-button @click="() => loadDirectory(currentPath)" :loading="loading">
              <template #icon>
                <n-icon><Refresh /></n-icon>
              </template>
            </n-button>
          </n-space>
        </div>

        <div class="toolbar-breadcrumb">
          <n-breadcrumb separator="/">
            <n-breadcrumb-item @click="navigateTo('/')">
              <a>root</a>
            </n-breadcrumb-item>
            <n-breadcrumb-item
              v-for="(segment, i) in pathSegments"
              :key="i"
              @click="navigateTo('/' + pathSegments.slice(0, i + 1).join('/'))"
            >
              <a>{{ segment }}</a>
            </n-breadcrumb-item>
          </n-breadcrumb>
        </div>
      </template>
    </div>

    <!-- 文件列表 -->
    <div
      class="file-list"
      :class="{ 'file-list-dragover': isDragOver }"
      @click="hideContextMenu"
      @contextmenu.prevent="(e) => showContextMenu(e, null)"
      @drop="handleFileDrop"
      @dragover.prevent="handleDragOver"
      @dragleave="handleDragLeave"
      @dragenter.prevent
    >
      <n-spin :show="loading">
        <n-alert v-if="error" :title="error" type="error" closable @close="error = null" />

        <n-empty v-else-if="!loading && displayEntries.length === 0" description="目录为空" />

        <div v-else class="file-manager">
          <!-- 表头 -->
          <div class="file-header" :class="{ 'file-header-mobile': isMobile }">
            <div class="file-header-cell file-col-name sortable" @click="handleSort('name')">
              <span>名称</span>
              <n-icon :size="14" class="sort-icon" :class="{ 'is-active': sortField === 'name' }">
                <template v-if="sortField === 'name'">
                  <CaretUp v-if="sortDirection === 'asc'" />
                  <CaretDown v-else />
                </template>
                <SwapVertical v-else class="sort-icon-default" />
              </n-icon>
            </div>
            <div class="file-header-cell file-col-size sortable" @click="handleSort('size')">
              <span>大小</span>
              <n-icon :size="14" class="sort-icon" :class="{ 'is-active': sortField === 'size' }">
                <template v-if="sortField === 'size'">
                  <CaretUp v-if="sortDirection === 'asc'" />
                  <CaretDown v-else />
                </template>
                <SwapVertical v-else class="sort-icon-default" />
              </n-icon>
            </div>
            <div class="file-header-cell file-col-permissions">权限</div>
            <div
              class="file-header-cell file-col-modified sortable"
              @click="handleSort('modified')"
            >
              <span>修改时间</span>
              <n-icon
                :size="14"
                class="sort-icon"
                :class="{ 'is-active': sortField === 'modified' }"
              >
                <template v-if="sortField === 'modified'">
                  <CaretUp v-if="sortDirection === 'asc'" />
                  <CaretDown v-else />
                </template>
                <SwapVertical v-else class="sort-icon-default" />
              </n-icon>
            </div>
            <div v-if="isMobile" class="file-header-cell file-col-actions">操作</div>
          </div>

          <!-- 虚拟列表 -->
          <VList
            :data="displayEntries"
            style="height: calc(100vh - 250px); overflow: auto"
            #default="{ item: entry }"
          >
            <div
              :key="entry.name"
              class="file-row"
              :class="{
                'file-row-selected': isEntrySelected(entry),
                'file-row-directory': entry.type === 'directory',
                'file-row-mobile': isMobile,
              }"
              @click.stop="onEntryClick(entry)"
              @dblclick.stop="onEntryDoubleClick(entry)"
              @contextmenu.prevent.stop="(e) => showContextMenu(e, entry)"
            >
              <!-- 名称列 -->
              <div class="file-cell file-col-name">
                <n-icon
                  :size="20"
                  class="file-icon"
                  :class="{
                    'icon-directory': entry.type === 'directory',
                    'icon-symlink': entry.type === 'symlink',
                    'icon-file': entry.type === 'file',
                  }"
                >
                  <component :is="getIconComponent(entry)" />
                </n-icon>
                <span class="file-name">{{ entry.name }}</span>
                <span v-if="entry.linkTarget" class="file-link-target">
                  → {{ entry.linkTarget }}
                </span>
              </div>

              <!-- 大小列 -->
              <div class="file-cell file-col-size">
                {{ entry.type === 'directory' ? '-' : formatSize(entry.size) }}
              </div>

              <!-- 权限列 -->
              <div class="file-cell file-col-permissions">
                {{ entry.permissions || '-' }}
              </div>

              <!-- 修改时间列 -->
              <div class="file-cell file-col-modified">
                {{ formatDate(entry.modified) }}
              </div>

              <!-- 移动端操作按钮 -->
              <div v-if="isMobile" class="file-cell file-col-actions">
                <n-button
                  text
                  @click.stop="(e) => showContextMenu(e, entry)"
                  class="mobile-action-btn"
                >
                  <template #icon>
                    <n-icon :size="20">
                      <EllipsisVertical />
                    </n-icon>
                  </template>
                </n-button>
              </div>
            </div>
          </VList>
        </div>
      </n-spin>
    </div>

    <!-- 右键菜单 -->
    <n-dropdown
      placement="bottom-start"
      trigger="manual"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :options="
        contextMenu.entry
          ? getContextMenuOptions(contextMenu.entry)
          : getEmptyAreaContextMenuOptions()
      "
      :show="contextMenu.show"
      :on-clickoutside="hideContextMenu"
      @select="(key) => handleContextMenuSelect(key, contextMenu.entry)"
    />

    <!-- 文件编辑器 -->
    <FileEditor
      v-model:show="fileEditor.show"
      :container-id="containerId"
      :file-path="fileEditor.filePath"
      :file-name="fileEditor.fileName"
      :read-only="fileEditor.readOnly"
      @saved="() => loadDirectory(currentPath)"
    />

    <!-- 隐藏的文件上传 input -->
    <input
      ref="fileInput"
      type="file"
      multiple
      style="display: none"
      @change="(e) => handleFileSelect(e, currentPath, () => loadDirectory(currentPath))"
    />
  </div>
</template>

<script setup lang="ts">
import { useResponsive } from '@/hooks/useResponsive'
import {
  ArrowUp,
  Eye,
  EyeOff,
  Home,
  Refresh,
  EllipsisVertical,
  CloudUpload,
  CaretUp,
  CaretDown,
  SwapVertical,
} from '@vicons/ionicons5'
import {
  NAlert,
  NBreadcrumb,
  NBreadcrumbItem,
  NButton,
  NDropdown,
  NEmpty,
  NIcon,
  NSpace,
  NSpin,
} from 'naive-ui'
import { VList } from 'virtua/vue'
import { onMounted, onUnmounted } from 'vue'
import {
  useContextMenu,
  useFileActions,
  useFileEditor,
  useFileInteraction,
  useFileList,
  useFileOperations,
  useFileUpload,
} from './composables'
import type { FileBrowserProps } from './types'
import { getIconComponent } from './utils'
import FileEditor from './FileEditor.vue'
import { formatDate, formatSize } from '@/utils'

// Props
const props = withDefaults(defineProps<FileBrowserProps>(), {
  initialPath: '/',
  canEdit: true,
})

const { isMobile } = useResponsive()

// 文件列表管理
const {
  currentPath,
  loading,
  error,
  showHiddenFiles,
  sortField,
  sortDirection,
  displayEntries,
  pathSegments,
  loadDirectory,
  navigateTo,
  goUp,
  goHome,
  toggleHiddenFiles,
  restoreSettings,
} = useFileList(props.containerId)

// 文件操作
const { downloadFile, deleteEntry } = useFileOperations(props.containerId)

// 文件编辑器
const { fileEditor, openFileEditor } = useFileEditor()

// 右键菜单
const {
  contextMenu,
  selectedEntry,
  justClosed,
  getContextMenuOptions,
  getEmptyAreaContextMenuOptions,
  showContextMenu,
  hideContextMenu,
  isEntrySelected,
} = useContextMenu(props.canEdit)

// 文件交互
const { handleEntryClick, handleDoubleClick } = useFileInteraction()

// 文件操作（创建、重命名）
const { createPath, renamePath } = useFileActions(props.containerId)

// 文件上传
const { uploading, handleFileSelect, handleDrop } = useFileUpload(props.containerId)
const fileInput = ref<HTMLInputElement | null>(null)
const isDragOver = ref(false)

// 触发文件选择
const triggerFileUpload = () => {
  fileInput.value?.click()
}

// 处理拖拽覆盖
const handleDragOver = () => {
  if (!props.canEdit) {
    return
  }
  isDragOver.value = true
}

// 处理拖拽离开
const handleDragLeave = (e: DragEvent) => {
  // 只有当离开整个文件列表区域时才取消高亮
  if (e.target === e.currentTarget) {
    isDragOver.value = false
  }
}

// 处理文件放置
const handleFileDrop = async (e: DragEvent) => {
  if (!props.canEdit) {
    return
  }

  isDragOver.value = false
  await handleDrop(e, currentPath.value, () => loadDirectory(currentPath.value))
}

// 排序功能
const handleSort = (field: typeof sortField.value) => {
  if (sortField.value === field) {
    // 同一字段，切换排序方向
    sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
  } else {
    // 不同字段，重置为升序
    sortField.value = field
    sortDirection.value = 'asc'
  }
}

// 处理右键菜单选择
const handleContextMenuSelect = (key: string, entry: typeof contextMenu.value.entry) => {
  hideContextMenu()

  // 处理空白区域的菜单
  if (!entry) {
    switch (key) {
      case 'create-file':
        createPath(currentPath.value, 'file', () => loadDirectory(currentPath.value))
        break
      case 'create-directory':
        createPath(currentPath.value, 'directory', () => loadDirectory(currentPath.value))
        break
      case 'upload':
        triggerFileUpload()
        break
    }
    return
  }

  // 处理文件/文件夹的菜单
  switch (key) {
    case 'view':
      openFileEditor(entry, currentPath.value, true)
      break
    case 'edit':
      openFileEditor(entry, currentPath.value, false)
      break
    case 'open':
      handleEntryClick(
        entry,
        currentPath.value,
        {
          onNavigate: navigateTo,
          onOpenFile: (e, readOnly) => openFileEditor(e, currentPath.value, readOnly),
          onSelect: (e) => {
            selectedEntry.value = e
          },
        },
        props.canEdit,
      )
      break
    case 'download':
      downloadFile(entry, currentPath.value)
      break
    case 'rename':
      renamePath(entry, currentPath.value, () => loadDirectory(currentPath.value))
      break
    case 'delete':
      deleteEntry(entry, currentPath.value, () => loadDirectory(currentPath.value))
      break
  }
}

// 处理文件行点击
const onEntryClick = (entry: (typeof displayEntries.value)[0]) => {
  // 如果菜单刚刚关闭，跳过这次点击事件
  if (justClosed.value) {
    return
  }

  handleEntryClick(
    entry,
    currentPath.value,
    {
      onNavigate: navigateTo,
      onOpenFile: (e, readOnly) => openFileEditor(e, currentPath.value, readOnly),
      onSelect: (e) => {
        selectedEntry.value = e
      },
    },
    props.canEdit,
  )
}

// 处理文件行双击
const onEntryDoubleClick = (entry: (typeof displayEntries.value)[0]) => {
  // 如果菜单刚刚关闭，跳过这次双击事件
  if (justClosed.value) {
    return
  }

  handleDoubleClick(
    entry,
    currentPath.value,
    {
      onNavigate: navigateTo,
      onOpenFile: (e, readOnly) => openFileEditor(e, currentPath.value, readOnly),
    },
    props.canEdit,
  )
}

// 生命周期
onMounted(() => {
  restoreSettings()
  loadDirectory(props.initialPath)

  // 添加全局点击事件监听器
  document.addEventListener('click', hideContextMenu)
})

onUnmounted(() => {
  document.removeEventListener('click', hideContextMenu)
})
</script>

<style scoped lang="less">
.file-browser {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: var(--n-color);

  .toolbar {
    padding: 12px;
    border-bottom: 1px solid var(--n-border-color);
    background-color: var(--n-color);

    // 桌面端布局：单行显示
    &:not(.toolbar-mobile) {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;

      .toolbar-left {
        display: flex;
        align-items: center;
        gap: 12px;
        flex: 1;
        min-width: 0;

        .toolbar-breadcrumb {
          flex: 1;
          min-width: 0;
          overflow-x: auto;

          &::-webkit-scrollbar {
            height: 4px;
          }

          &::-webkit-scrollbar-thumb {
            background-color: rgba(0, 0, 0, 0.2);
            border-radius: 2px;
          }
        }
      }

      .toolbar-right {
        flex-shrink: 0;
      }
    }

    // 移动端布局：两行显示
    &.toolbar-mobile {
      display: flex;
      flex-direction: column;
      gap: 12px;

      .toolbar-buttons-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
      }

      .toolbar-breadcrumb {
        width: 100%;
        overflow-x: auto;
        padding-bottom: 4px;

        &::-webkit-scrollbar {
          height: 4px;
        }

        &::-webkit-scrollbar-thumb {
          background-color: rgba(0, 0, 0, 0.2);
          border-radius: 2px;
        }

        :deep(.n-breadcrumb) {
          white-space: nowrap;
        }
      }
    }
  }

  .file-list {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background-color: var(--n-color);
    position: relative;
    transition: background-color 0.3s ease;

    // 拖拽覆盖状态
    &.file-list-dragover {
      background-color: var(--n-color-modal);

      &::after {
        content: '拖放文件到此处上传';
        position: absolute;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        font-size: 20px;
        color: var(--n-primary-color);
        font-weight: 600;
        z-index: 10;
        pointer-events: none;
        padding: 20px 40px;
        border: 2px dashed var(--n-primary-color);
        border-radius: 8px;
        background-color: var(--n-color);
      }
    }
  }
}

.file-manager {
  display: flex;
  flex-direction: column;
  flex: 1;
  overflow: hidden;
}

// 表头
.file-header {
  display: flex;
  align-items: center;
  height: 40px;
  padding: 0 12px;
  background-color: var(--table-header-color);
  border-bottom: 1px solid var(--divider-color);
  font-weight: var(--font-weight-strong);
  font-size: var(--font-size-small);
  color: var(--text-color-1);
  user-select: none;
}

.file-header-cell {
  padding: 0 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 4px;

  &.sortable {
    cursor: pointer;
    transition: background-color 0.2s;

    &:hover {
      background-color: var(--n-color-hover);

      .sort-icon-default {
        opacity: 0.5;
      }
    }

    &:active {
      background-color: var(--n-color-pressed);
    }
  }

  .sort-icon {
    flex-shrink: 0;
    transition: opacity 0.2s;

    &.is-active {
      opacity: 0.8;
    }
    .sort-icon-default {
      opacity: 0;
    }
  }
}

// 列宽定义
.file-col-name {
  flex: 1;
  min-width: 200px;
}

.file-col-size {
  width: 100px;
  flex-shrink: 0;
}

.file-col-permissions {
  width: 100px;
  flex-shrink: 0;
}

.file-col-modified {
  width: 180px;
  flex-shrink: 0;
}

// 文件行
.file-row {
  display: flex;
  align-items: center;
  height: 40px;
  padding: 0 4px;
  cursor: pointer;
  user-select: none;
  border-bottom: 1px solid var(--divider-color);
  transition: background-color var(--cubic-bezier-ease-in-out) 0.2s;

  &:hover {
    background-color: var(--hover-color);
  }

  &:active {
    background-color: var(--pressed-color);
  }

  &.file-row-selected {
    background-color: var(--action-color);
  }

  &.file-row-directory {
    font-weight: var(--font-weight-strong);
  }
}

.file-cell {
  padding: 0 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size);
  color: var(--text-color-2);

  &.file-col-name {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.file-icon {
  flex-shrink: 0;

  &.icon-directory {
    color: var(--success-color);
  }

  &.icon-symlink {
    color: var(--warning-color);
  }

  &.icon-file {
    color: var(--icon-color);
  }
}

.file-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-link-target {
  color: var(--text-color-3);
  font-size: var(--font-size-small);
  flex-shrink: 0;
}

// 移动端操作按钮列
.file-col-actions {
  width: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 !important;
}

.mobile-action-btn {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-color-2);
  transition: color 0.3s;

  &:hover {
    color: var(--primary-color);
  }

  &:active {
    color: var(--primary-color-pressed);
  }
}

// 移动端适配
@media (max-width: 768px) {
  .file-header {
    padding: 0 8px;
    font-size: var(--font-size-mini);
  }

  .file-header-cell {
    padding: 0 4px;
    font-size: var(--font-size-mini);
  }

  .file-row {
    padding: 0 8px;
    height: 48px;

    &.file-row-mobile {
      // 移动端增加右侧操作按钮的空间
      padding-right: 0;
    }
  }

  .file-cell {
    padding: 0 4px;
    font-size: var(--font-size-small);
  }

  // 移动端隐藏权限列
  .file-col-permissions {
    display: none;
  }

  .file-col-size {
    width: 80px;
  }

  .file-col-modified {
    width: 100px;
    font-size: var(--font-size-mini);
  }
}

// 小屏幕适配
@media (max-width: 480px) {
  .file-col-size {
    display: none;
  }

  .file-col-modified {
    width: 90px;
  }

  .file-col-name {
    min-width: 0;
    flex: 1;
  }

  .file-col-actions {
    width: 36px;
  }

  .mobile-action-btn {
    width: 36px;
    height: 36px;
  }
}
</style>
