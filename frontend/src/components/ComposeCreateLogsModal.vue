<template>
  <n-modal
    v-model:show="show"
    :icon="getIcon()"
    display-directive="if"
    preset="dialog"
    :title="title"
    class="compose-create-logs-modal"
    contentClass="compose-create-logs-modal-content"
    :style="{
      padding: '8px',
      width: '90vw',
      maxWidth: '1200px',
      height: '90vh',
    }"
    :mask-closable="false"
    :closable="true"
    @after-leave="handleClose"
  >
    <logs-stream-view ref="logsStreamViewRef" :socket-url="socketUrl" />
    <template #action>
      <n-space justify="end">
        <n-button @click="handleClearLogs">
          <template #icon>
            <n-icon>
              <TrashOutline />
            </n-icon>
          </template>
          清空日志
        </n-button>
        <n-button @click="show = false">关闭</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import ComposeIcon from '@/assets/svg/compose.svg?component'
import type { ComposeProject } from '@/common/types'
import { renderIcon } from '@/common/utils'
import LogsStreamView from '@/components/LogsStreamView.vue'
import { useSettingStore } from '@/store/setting'
import { useComposeStore } from '@/store/compose'
import { TrashOutline } from '@vicons/ionicons5'
import { useThemeVars } from 'naive-ui'
import { computed, ref, watch } from 'vue'
import { buildComposeManagedActionSocketUrl } from './composeActionMenus'

interface Props {
  project: ComposeProject | null
}

const props = defineProps<Props>()
const show = defineModel<boolean>('show')
const settingStore = useSettingStore()
const composeStore = useComposeStore()
const theme = useThemeVars()
const logsStreamViewRef = ref<InstanceType<typeof LogsStreamView>>()

const title = computed(() => {
  if (!props.project) {
    return '创建/重建项目'
  }
  return `创建/重建 - ${props.project.name}`
})

const getIcon = () => {
  return renderIcon(ComposeIcon, {
    color: theme.value.primaryColor,
    size: 20,
  })
}

const socketUrl = computed(() => {
  return buildComposeManagedActionSocketUrl('up', props.project, settingStore.getToken())
})

const handleClose = () => {
  logsStreamViewRef.value?.close()
  composeStore.clearOperationState(props.project?.name ?? '')
  composeStore.fetchProjects(true)
}

const handleClearLogs = () => {
  logsStreamViewRef.value?.handleClearLogs()
}

watch(show, (val) => {
  if (val && props.project) {
    composeStore.setOperationState(props.project.name, { type: 'creating' })
  }
})
</script>

<style lang="less">
.compose-create-logs-modal {
  .n-dialog__content {
    margin-bottom: 0;
    margin-top: 0;
  }
  .n-dialog__close {
    top: -8px;
  }
}
</style>
