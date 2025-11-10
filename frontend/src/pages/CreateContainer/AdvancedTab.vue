<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <n-space vertical size="large">
      <!-- 能力配置 -->
      <div>
        <n-h3 prefix="bar">能力配置</n-h3>
        <n-alert type="info" style="margin-bottom: 16px" title="默认行为说明">
          <n-space vertical size="small">
            <n-text depth="3" style="font-size: 12px">
              如果不设置任何能力，容器将使用 Docker 默认能力集（共 14 个基础能力）
            </n-text>
            <n-collapse>
              <n-collapse-item title="查看 Docker 默认能力列表（共 14 个）" name="default-caps">
                <n-space vertical size="small" style="margin-top: 8px">
                  <n-descriptions :column="2" size="small" bordered>
                    <n-descriptions-item
                      v-for="cap in defaultCapabilities"
                      :key="cap.value"
                      :label="cap.value"
                    >
                      {{ cap.description }}
                    </n-descriptions-item>
                  </n-descriptions>
                </n-space>
              </n-collapse-item>
              <n-collapse-item
                title="查看 Docker 默认移除的能力列表（共 24 个）"
                name="default-dropped-caps"
              >
                <n-space vertical size="small" style="margin-top: 8px">
                  <n-text depth="3" style="font-size: 12px; margin-bottom: 8px">
                    以下能力默认被移除，以增强容器安全性。如需使用，请在"添加能力
                    (CapAdd)"中显式添加。
                  </n-text>
                  <n-descriptions :column="2" size="small" bordered>
                    <n-descriptions-item
                      v-for="cap in defaultDroppedCapabilities"
                      :key="cap.value"
                      :label="cap.value"
                    >
                      {{ cap.description }}
                    </n-descriptions-item>
                  </n-descriptions>
                </n-space>
              </n-collapse-item>
            </n-collapse>
          </n-space>
        </n-alert>
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="添加能力 (CapAdd)">
              <n-select
                v-model:value="formValue.capAdd"
                :options="capabilityOptions"
                multiple
                filterable
                tag
                placeholder="选择或输入能力（留空则使用 Docker 默认值）"
              />
              <template #feedback>
                <n-space vertical size="small">
                  <n-text depth="3" style="font-size: 12px">
                    添加 Linux 能力（Capabilities），如
                    SYS_ADMIN（系统管理）、NET_ADMIN（网络管理）。留空则使用 Docker 默认能力集。
                  </n-text>
                  <n-space size="small">
                    <n-button
                      size="small"
                      type="primary"
                      ghost
                      @click="addDefaultCapabilities"
                      title="一键添加所有 Docker 默认权限（14个），保留已选择的权限"
                    >
                      ⚡ 一键添加默认权限
                    </n-button>
                    <n-button
                      size="small"
                      @click="clearCapAdd"
                      title="清空所有权限，使用 Docker 默认值"
                    >
                      清空
                    </n-button>
                  </n-space>
                </n-space>
              </template>
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="移除能力 (CapDrop)">
              <n-select
                v-model:value="formValue.capDrop"
                :options="capabilityOptions"
                multiple
                filterable
                tag
                placeholder="选择或输入能力（留空则使用 Docker 默认值）"
              />
              <template #feedback>
                <n-space vertical size="small">
                  <n-text depth="3" style="font-size: 12px">
                    移除 Linux 能力以增强安全性，例如移除 NET_RAW 防止容器创建原始套接字。留空则使用
                    Docker 默认能力集（默认已移除 24 个高风险能力，如
                    SYS_ADMIN、SYS_MODULE、NET_ADMIN 等）。
                  </n-text>
                  <n-space size="small">
                    <n-button
                      size="small"
                      type="warning"
                      ghost
                      @click="addDefaultDroppedCapabilities"
                      title="一键添加所有 Docker 默认移除的权限（24个），保留已选择的权限"
                    >
                      ⚡ 一键添加默认移除权限
                    </n-button>
                    <n-button
                      size="small"
                      @click="clearCapDrop"
                      title="清空所有移除权限，使用 Docker 默认值"
                    >
                      清空
                    </n-button>
                  </n-space>
                </n-space>
              </template>
            </n-form-item>
          </n-gi>
        </n-grid>
      </div>

      <n-divider />

      <!-- 其他高级选项 -->
      <div>
        <n-h3 prefix="bar">其他高级选项</n-h3>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="PID 模式">
              <n-input v-model:value="formValue.pidMode" placeholder="例如: host" />
              <template #feedback>
                <n-text depth="3" style="font-size: 12px">
                  进程命名空间模式。host = 使用主机的 PID 命名空间，container:容器名 =
                  共享另一个容器的 PID
                </n-text>
              </template>
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="IPC 模式">
              <n-input v-model:value="formValue.ipcMode" placeholder="例如: host" />
              <template #feedback>
                <n-text depth="3" style="font-size: 12px">
                  IPC 命名空间模式。host = 使用主机的 IPC 命名空间，shareable = 允许其他容器共享
                </n-text>
              </template>
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="UTS 模式">
              <n-input v-model:value="formValue.utsMode" placeholder="例如: host" />
              <template #feedback>
                <n-text depth="3" style="font-size: 12px">
                  UTS 命名空间模式。host = 使用主机的主机名和域名
                </n-text>
              </template>
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="Cgroup">
              <n-input v-model:value="formValue.cgroup" placeholder="Cgroup 路径" />
              <template #feedback>
                <n-text depth="3" style="font-size: 12px">
                  指定容器的 Cgroup 父路径，用于自定义资源控制组
                </n-text>
              </template>
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="Runtime">
          <n-input v-model:value="formValue.runtime" placeholder="例如: nvidia" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              指定容器运行时。例如：nvidia（使用 NVIDIA GPU）、runc（默认运行时）
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="安全选项">
          <n-dynamic-tags v-model:value="formValue.securityOpt" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              设置安全选项。例如：seccomp=unconfined（禁用 seccomp）、apparmor=unconfined（禁用
              AppArmor）
            </n-text>
          </template>
        </n-form-item>
      </div>
    </n-space>
  </n-form>
</template>

<script setup lang="ts">
import type { FormInst } from 'naive-ui'
import type { AdvancedFormValue } from './types'

const formValue = defineModel<AdvancedFormValue>({
  default: () => ({
    capAdd: [],
    capDrop: [],
    pidMode: '',
    ipcMode: '',
    utsMode: '',
    cgroup: '',
    runtime: '',
    securityOpt: [],
  }),
})

const formRef = ref<FormInst | null>(null)

// Docker 默认能力集（共 14 个）
// 参考: https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities
const defaultCapabilities = [
  { value: 'CHOWN', description: '改变文件所有者' },
  { value: 'DAC_OVERRIDE', description: '绕过文件读写执行权限检查' },
  { value: 'FOWNER', description: '绕过文件所有者权限检查' },
  { value: 'FSETID', description: '文件修改时不清除 setuid/setgid 位' },
  { value: 'KILL', description: '绕过发送信号的权限检查' },
  { value: 'SETGID', description: '操作进程 GID 和补充 GID 列表' },
  { value: 'SETUID', description: '操作进程 UID' },
  { value: 'NET_BIND_SERVICE', description: '绑定特权端口 (<1024)' },
  { value: 'NET_RAW', description: '使用 RAW 和 PACKET 套接字' },
  { value: 'SYS_CHROOT', description: '使用 chroot' },
  { value: 'MKNOD', description: '使用 mknod 创建特殊文件' },
  { value: 'AUDIT_WRITE', description: '写入内核审计日志' },
  { value: 'SETFCAP', description: '设置文件 capabilities' },
  { value: 'SETPCAP', description: '修改进程 capabilities' },
]

// Docker 默认移除的能力集（共 24 个）
// 这些能力默认被移除以增强安全性，如需使用需通过 capAdd 显式添加
const defaultDroppedCapabilities = [
  { value: 'AUDIT_CONTROL', description: '启用和禁用内核审计' },
  { value: 'BLOCK_SUSPEND', description: '阻止系统挂起' },
  { value: 'DAC_READ_SEARCH', description: '绕过文件读和目录读执行权限检查' },
  { value: 'IPC_LOCK', description: '锁定内存 (mlock, mlockall)' },
  { value: 'IPC_OWNER', description: '绕过 IPC 对象权限检查' },
  { value: 'LEASE', description: '建立文件租约' },
  { value: 'LINUX_IMMUTABLE', description: '设置 FS_APPEND_FL 和 FS_IMMUTABLE_FL' },
  { value: 'MAC_ADMIN', description: '覆盖强制访问控制 (MAC)' },
  { value: 'MAC_OVERRIDE', description: '允许 MAC 配置或状态更改' },
  { value: 'NET_ADMIN', description: '网络管理操作' },
  { value: 'NET_BROADCAST', description: '允许网络广播和多播' },
  { value: 'SETCAP', description: '设置文件 capabilities' },
  { value: 'SYS_ADMIN', description: '系统管理操作（高风险）' },
  { value: 'SYS_BOOT', description: '重启系统' },
  { value: 'SYS_MODULE', description: '加载和卸载内核模块（高风险）' },
  { value: 'SYS_NICE', description: '提升进程优先级和设置其他进程优先级' },
  { value: 'SYS_PACCT', description: '使用 acct' },
  { value: 'SYS_PTRACE', description: '使用 ptrace 跟踪任意进程（高风险）' },
  { value: 'SYS_RAWIO', description: '执行 I/O 端口操作' },
  { value: 'SYS_RESOURCE', description: '覆盖资源限制' },
  { value: 'SYS_TIME', description: '设置系统时钟' },
  { value: 'SYS_TTY_CONFIG', description: '配置 TTY 设备' },
  { value: 'SYSLOG', description: '执行特权 syslog 操作' },
  { value: 'WAKE_ALARM', description: '触发系统唤醒' },
]

// 完整的 Linux Capabilities 列表
// 参考: https://man7.org/linux/man-pages/man7/capabilities.7.html
const capabilityOptions = [
  { label: 'AUDIT_CONTROL - 启用和禁用内核审计', value: 'AUDIT_CONTROL' },
  { label: 'AUDIT_WRITE - 写入内核审计日志', value: 'AUDIT_WRITE' },
  { label: 'BLOCK_SUSPEND - 阻止系统挂起', value: 'BLOCK_SUSPEND' },
  { label: 'CHOWN - 改变文件所有者', value: 'CHOWN' },
  { label: 'DAC_OVERRIDE - 绕过文件读写执行权限检查', value: 'DAC_OVERRIDE' },
  { label: 'DAC_READ_SEARCH - 绕过文件读和目录读执行权限检查', value: 'DAC_READ_SEARCH' },
  { label: 'FOWNER - 绕过文件所有者权限检查', value: 'FOWNER' },
  { label: 'FSETID - 文件修改时不清除 setuid/setgid 位', value: 'FSETID' },
  { label: 'IPC_LOCK - 锁定内存 (mlock, mlockall)', value: 'IPC_LOCK' },
  { label: 'IPC_OWNER - 绕过 IPC 对象权限检查', value: 'IPC_OWNER' },
  { label: 'KILL - 绕过发送信号的权限检查', value: 'KILL' },
  { label: 'LEASE - 建立文件租约', value: 'LEASE' },
  { label: 'LINUX_IMMUTABLE - 设置 FS_APPEND_FL 和 FS_IMMUTABLE_FL', value: 'LINUX_IMMUTABLE' },
  { label: 'MAC_ADMIN - 覆盖强制访问控制 (MAC)', value: 'MAC_ADMIN' },
  { label: 'MAC_OVERRIDE - 允许 MAC 配置或状态更改', value: 'MAC_OVERRIDE' },
  { label: 'MKNOD - 使用 mknod 创建特殊文件', value: 'MKNOD' },
  { label: 'NET_ADMIN - 网络管理操作', value: 'NET_ADMIN' },
  { label: 'NET_BIND_SERVICE - 绑定特权端口 (<1024)', value: 'NET_BIND_SERVICE' },
  { label: 'NET_BROADCAST - 允许网络广播和多播', value: 'NET_BROADCAST' },
  { label: 'NET_RAW - 使用 RAW 和 PACKET 套接字', value: 'NET_RAW' },
  { label: 'SETCAP - 设置文件 capabilities', value: 'SETCAP' },
  { label: 'SETFCAP - 设置文件 capabilities', value: 'SETFCAP' },
  { label: 'SETGID - 操作进程 GID 和补充 GID 列表', value: 'SETGID' },
  { label: 'SETPCAP - 修改进程 capabilities', value: 'SETPCAP' },
  { label: 'SETUID - 操作进程 UID', value: 'SETUID' },
  { label: 'SYS_ADMIN - 系统管理操作', value: 'SYS_ADMIN' },
  { label: 'SYS_BOOT - 重启系统', value: 'SYS_BOOT' },
  { label: 'SYS_CHROOT - 使用 chroot', value: 'SYS_CHROOT' },
  { label: 'SYS_MODULE - 加载和卸载内核模块', value: 'SYS_MODULE' },
  { label: 'SYS_NICE - 提升进程优先级和设置其他进程优先级', value: 'SYS_NICE' },
  { label: 'SYS_PACCT - 使用 acct', value: 'SYS_PACCT' },
  { label: 'SYS_PTRACE - 使用 ptrace 跟踪任意进程', value: 'SYS_PTRACE' },
  { label: 'SYS_RAWIO - 执行 I/O 端口操作', value: 'SYS_RAWIO' },
  { label: 'SYS_RESOURCE - 覆盖资源限制', value: 'SYS_RESOURCE' },
  { label: 'SYS_TIME - 设置系统时钟', value: 'SYS_TIME' },
  { label: 'SYS_TTY_CONFIG - 配置 TTY 设备', value: 'SYS_TTY_CONFIG' },
  { label: 'SYSLOG - 执行特权 syslog 操作', value: 'SYSLOG' },
  { label: 'WAKE_ALARM - 触发系统唤醒', value: 'WAKE_ALARM' },
]

// 一键添加默认权限（保留已选择的权限，避免重复）
const addDefaultCapabilities = () => {
  const defaultCapValues = defaultCapabilities.map((cap) => cap.value)
  const currentCaps = formValue.value.capAdd || []
  // 合并默认权限和已选择的权限，去重
  const mergedCaps = Array.from(new Set([...currentCaps, ...defaultCapValues]))
  formValue.value.capAdd = mergedCaps
}

// 清空 CapAdd（重置为使用 Docker 默认值）
const clearCapAdd = () => {
  formValue.value.capAdd = []
}

// 一键添加默认移除的权限（保留已选择的权限，避免重复）
const addDefaultDroppedCapabilities = () => {
  const defaultDroppedCapValues = defaultDroppedCapabilities.map((cap) => cap.value)
  const currentCaps = formValue.value.capDrop || []
  // 合并默认移除权限和已选择的权限，去重
  const mergedCaps = Array.from(new Set([...currentCaps, ...defaultDroppedCapValues]))
  formValue.value.capDrop = mergedCaps
}

// 清空 CapDrop（重置为使用 Docker 默认值）
const clearCapDrop = () => {
  formValue.value.capDrop = []
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>
