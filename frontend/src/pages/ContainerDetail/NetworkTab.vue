<template>
  <div class="tab-content network-tab">
    <div class="detail-container">
      <!-- 网络列表 -->
      <n-card title="网络连接" class="info-card" size="small">
        <div
          v-if="Object.keys(containerDetail.NetworkSettings.Networks || {}).length === 0"
          class="empty-container"
        >
          <n-empty description="没有网络连接" />
        </div>
        <div v-else class="network-list">
          <div
            v-for="(network, networkName) in containerDetail.NetworkSettings.Networks"
            :key="networkName"
            class="network-item"
          >
            <div class="network-header">
              <n-text strong>{{ networkName }}</n-text>
            </div>
            <div class="network-details">
              <div class="network-detail-item" v-if="network.IPAddress">
                <span class="label">IPv4 地址:</span>
                <n-text code>{{ network.IPAddress }}/{{ network.IPPrefixLen }}</n-text>
              </div>
              <div class="network-detail-item" v-if="network.GlobalIPv6Address">
                <span class="label">IPv6 地址:</span>
                <n-text code
                  >{{ network.GlobalIPv6Address }}/{{ network.GlobalIPv6PrefixLen }}</n-text
                >
              </div>
              <div class="network-detail-item" v-if="network.Gateway">
                <span class="label">网关:</span>
                <n-text code>{{ network.Gateway }}</n-text>
              </div>
              <div class="network-detail-item" v-if="network.MacAddress">
                <span class="label">MAC 地址:</span>
                <n-text code>{{ network.MacAddress }}</n-text>
              </div>
            </div>
          </div>
        </div>
      </n-card>

      <!-- 端口映射 -->
      <n-card title="端口映射" class="info-card" v-if="portMappings.length > 0" size="small">
        <div class="port-mapping-list">
          <div v-for="(port, index) in portMappings" :key="index" class="port-mapping-item">
            <n-tag type="info" size="large">
              {{ port.hostIp }}:{{ port.hostPort }} → {{ port.containerPort }}/{{ port.protocol }}
            </n-tag>
          </div>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  containerDetail: any
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
</script>

<style scoped lang="less">
@import './styles.less';
</style>
