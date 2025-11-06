<template>
  <div class="tab-content">
    <div class="detail-container">
      <!-- 基本信息 -->
      <n-card title="基本信息" class="info-card" size="small">
        <div class="info-grid">
          <div class="info-item">
            <div class="info-label">容器 ID</div>
            <div class="info-value">
              <n-text code>{{ containerDetail.Id.substring(0, 12) }}</n-text>
            </div>
          </div>
          <div class="info-item">
            <div class="info-label">容器名称</div>
            <div class="info-value">{{ containerName }}</div>
          </div>
          <div class="info-item">
            <div class="info-label">镜像</div>
            <div class="info-value">{{ containerDetail.Config.Image }}</div>
          </div>
          <div class="info-item">
            <div class="info-label">状态</div>
            <div class="info-value">
              <n-tag :type="containerDetail.State.Running ? 'success' : 'default'" size="small">
                {{ containerDetail.State.Status }}
              </n-tag>
            </div>
          </div>
          <div class="info-item">
            <div class="info-label">创建时间</div>
            <div class="info-value">{{ formatTime(containerDetail.Created) }}</div>
          </div>
          <div class="info-item">
            <div class="info-label">启动时间</div>
            <div class="info-value">{{ formatTime(containerDetail.State.StartedAt) }}</div>
          </div>
        </div>
      </n-card>

      <!-- 运行配置 -->
      <n-card title="运行配置" class="info-card" size="small">
        <div class="info-grid">
          <div class="info-item">
            <div class="info-label">主机名</div>
            <div class="info-value">{{ containerDetail.Config.Hostname || '-' }}</div>
          </div>
          <div class="info-item">
            <div class="info-label">工作目录</div>
            <div class="info-value">
              <n-text code>{{ containerDetail.Config.WorkingDir || '/' }}</n-text>
            </div>
          </div>
          <div class="info-item info-item-full">
            <div class="info-label">入口点</div>
            <div class="info-value">
              <n-text code v-if="containerDetail.Config.Entrypoint">{{
                containerDetail.Config.Entrypoint.join(' ')
              }}</n-text>
              <span v-else>-</span>
            </div>
          </div>
          <div class="info-item info-item-full">
            <div class="info-label">命令</div>
            <div class="info-value">
              <n-text code v-if="containerDetail.Config.Cmd">{{
                containerDetail.Config.Cmd.join(' ')
              }}</n-text>
              <span v-else>-</span>
            </div>
          </div>
          <div class="info-item">
            <div class="info-label">重启策略</div>
            <div class="info-value">
              <n-tag size="small">{{
                containerDetail.HostConfig.RestartPolicy.Name || 'no'
              }}</n-tag>
            </div>
          </div>
          <div class="info-item">
            <div class="info-label">网络模式</div>
            <div class="info-value">{{ containerDetail.HostConfig.NetworkMode || '-' }}</div>
          </div>
        </div>
      </n-card>

      <!-- 端口映射 -->
      <n-card title="端口映射" class="info-card" v-if="portMappings.length > 0">
        <div class="port-list">
          <div v-for="(port, index) in portMappings" :key="index" class="port-item">
            <n-tag type="info">
              {{ port.hostPort }} → {{ port.containerPort }}/{{ port.protocol }}
            </n-tag>
          </div>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import dayjs from 'dayjs'

interface Props {
  containerDetail: any
  containerName: string
}

const props = defineProps<Props>()

// 端口映射
const portMappings = computed(() => {
  if (!props.containerDetail?.NetworkSettings?.Ports) {
    return []
  }

  const ports: any[] = []
  const portsObj = props.containerDetail.NetworkSettings.Ports

  Object.keys(portsObj).forEach((containerPort) => {
    const bindings = portsObj[containerPort]
    if (bindings && bindings.length > 0) {
      bindings.forEach((binding: any) => {
        const [port, protocol] = containerPort.split('/')
        ports.push({
          containerPort: port,
          protocol: protocol || 'tcp',
          hostIp: binding.HostIp || '0.0.0.0',
          hostPort: binding.HostPort,
        })
      })
    }
  })

  return ports
})

// 格式化时间
const formatTime = (timeStr: string) => {
  if (!timeStr || timeStr === '0001-01-01T00:00:00Z') {
    return '-'
  }
  return dayjs(timeStr).format('YYYY-MM-DD HH:mm:ss')
}
</script>

<style scoped lang="less">
@import './styles.less';
</style>
