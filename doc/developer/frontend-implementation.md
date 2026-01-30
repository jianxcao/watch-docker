# Watch Docker 前端技术实现

## 1. 技术栈实现

### 1.1 构建配置

#### Vite 配置（vite.config.ts）

```typescript
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import AutoImport from "unplugin-auto-import/vite";
import Components from "unplugin-vue-components/vite";
import { NaiveUiResolver } from "unplugin-vue-components/resolvers";
import UnoCSS from "unocss/vite";
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({
      imports: ["vue", "vue-router", "pinia"],
      dts: "src/auto-imports.d.ts",
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: "src/components.d.ts",
    }),
    UnoCSS(),
    VitePWA({
      registerType: "autoUpdate",
      manifest: {
        name: "Watch Docker",
        short_name: "Watch Docker",
        description: "Docker container management tool",
      },
    }),
  ],
  resolve: {
    alias: {
      "@": "/src",
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
```

#### TypeScript 配置（tsconfig.json）

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "jsx": "preserve",
    "moduleResolution": "bundler",
    "paths": {
      "@/*": ["./src/*"]
    },
    "types": ["vite/client", "node"],
    "strict": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*.ts", "src/**/*.d.ts", "src/**/*.tsx", "src/**/*.vue"]
}
```

### 1.2 包管理

使用 pnpm 作为包管理器，主要依赖：

```json
{
  "dependencies": {
    "vue": "^3.4.0",
    "vue-router": "^4.2.5",
    "pinia": "^2.1.7",
    "naive-ui": "^2.38.0",
    "axios": "^1.6.2",
    "dayjs": "^1.11.10",
    "qrcode": "^1.5.3",
    "@simplewebauthn/browser": "^9.0.1",
    "xterm": "^5.3.0",
    "xterm-addon-fit": "^0.8.0",
    "@monaco-editor/loader": "^1.4.0",
    "yaml": "^2.3.4"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.3.3",
    "vite": "^5.0.0",
    "unocss": "^0.58.0",
    "unplugin-auto-import": "^0.17.0",
    "unplugin-vue-components": "^0.26.0",
    "vite-plugin-pwa": "^0.17.0"
  }
}
```

## 2. 核心模块实现

### 2.1 HTTP 请求配置（common/axiosConfig.ts）

```typescript
import axios from "axios";
import type { AxiosInstance, AxiosRequestConfig, AxiosResponse } from "axios";
import { useAuthStore } from "@/store/auth";
import { useMessage } from "naive-ui";

// 创建 axios 实例
const instance: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || "",
  timeout: 30000,
  headers: {
    "Content-Type": "application/json",
  },
});

// 请求拦截器
instance.interceptors.request.use(
  (config) => {
    const authStore = useAuthStore();
    const token = authStore.token || authStore.tempToken;

    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }

    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器
instance.interceptors.response.use(
  (response: AxiosResponse) => {
    const { code, msg, data } = response.data;

    if (code === 0) {
      return response;
    } else if (code === 401) {
      // 未授权，跳转登录
      const authStore = useAuthStore();
      authStore.logout();
      window.location.href = "/login";
      return Promise.reject(new Error(msg || "未授权"));
    } else {
      const message = useMessage();
      message.error(msg || "请求失败");
      return Promise.reject(new Error(msg));
    }
  },
  (error) => {
    const message = useMessage();
    message.error(error.message || "网络请求失败");
    return Promise.reject(error);
  }
);

export default instance;
```

### 2.2 状态管理实现

#### 认证状态（store/auth.ts）

```typescript
import { defineStore } from "pinia";
import { authApi, twoFAApi } from "@/common/api";

export const useAuthStore = defineStore("auth", () => {
  // 状态
  const token = ref(localStorage.getItem("token") || "");
  const username = ref(localStorage.getItem("username") || "");
  const isLoggedIn = ref(!!token.value);

  // 二次验证相关
  const twoFARequired = ref(false);
  const twoFASetupRequired = ref(false);
  const tempToken = ref("");
  const twoFAMethod = ref("");

  // 登录
  const login = async (username: string, password: string) => {
    try {
      const { data } = await authApi.login(username, password);

      if (data.needTwoFA) {
        // 需要二次验证
        twoFARequired.value = true;
        twoFASetupRequired.value = !data.isSetup;
        twoFAMethod.value = data.method || "";
        tempToken.value = data.tempToken || "";
        username.value = data.username || "";
      } else {
        // 直接登录成功
        setToken(data.token!);
        username.value = username;
        isLoggedIn.value = true;
      }

      return data;
    } catch (error) {
      throw error;
    }
  };

  // 完成二次验证
  const completeTwoFA = (fullToken: string) => {
    setToken(fullToken);
    isLoggedIn.value = true;
    twoFARequired.value = false;
    twoFASetupRequired.value = false;
    tempToken.value = "";
  };

  // 设置 Token
  const setToken = (newToken: string) => {
    token.value = newToken;
    localStorage.setItem("token", newToken);
  };

  // 登出
  const logout = () => {
    token.value = "";
    username.value = "";
    isLoggedIn.value = false;
    tempToken.value = "";
    localStorage.removeItem("token");
    localStorage.removeItem("username");
  };

  return {
    token,
    username,
    isLoggedIn,
    twoFARequired,
    twoFASetupRequired,
    tempToken,
    twoFAMethod,
    login,
    completeTwoFA,
    logout,
  };
});
```

#### 容器状态（store/container.ts）

```typescript
import { defineStore } from "pinia";
import { containerApi } from "@/common/api";
import type { ContainerStatus } from "@/common/types";

export const useContainerStore = defineStore("container", () => {
  const containers = ref<ContainerStatus[]>([]);
  const loading = ref(false);
  const updating = ref(new Set<string>());

  // 获取容器列表
  const fetchContainers = async () => {
    loading.value = true;
    try {
      const { data } = await containerApi.getContainers();
      containers.value = data.data.containers;
    } catch (error) {
      console.error("获取容器列表失败:", error);
    } finally {
      loading.value = false;
    }
  };

  // 更新单个容器
  const updateContainer = async (id: string, image?: string) => {
    updating.value.add(id);
    try {
      await containerApi.updateContainer(id, image);
      await fetchContainers();
    } finally {
      updating.value.delete(id);
    }
  };

  // 批量更新
  const batchUpdate = async () => {
    loading.value = true;
    try {
      const { data } = await containerApi.batchUpdate();
      await fetchContainers();
      return data.data;
    } finally {
      loading.value = false;
    }
  };

  // 启动容器
  const startContainer = async (id: string) => {
    await containerApi.startContainer(id);
    await fetchContainers();
  };

  // 停止容器
  const stopContainer = async (id: string) => {
    await containerApi.stopContainer(id);
    await fetchContainers();
  };

  // 删除容器
  const deleteContainer = async (id: string) => {
    await containerApi.deleteContainer(id);
    await fetchContainers();
  };

  // 计算属性
  const updateableContainers = computed(() =>
    containers.value.filter((c) => c.status === "UpdateAvailable" && !c.skipped)
  );

  const runningContainers = computed(() =>
    containers.value.filter((c) => c.running)
  );

  const stoppedContainers = computed(() =>
    containers.value.filter((c) => !c.running)
  );

  return {
    containers,
    loading,
    updating,
    fetchContainers,
    updateContainer,
    batchUpdate,
    startContainer,
    stopContainer,
    deleteContainer,
    updateableContainers,
    runningContainers,
    stoppedContainers,
  };
});
```

### 2.3 Hooks 实现

#### 容器操作 Hook（hooks/useContainer.ts）

```typescript
import { useContainerStore } from "@/store/container";
import { useMessage, useDialog } from "naive-ui";

export function useContainer() {
  const store = useContainerStore();
  const message = useMessage();
  const dialog = useDialog();

  const handleStart = async (id: string, name: string) => {
    try {
      await store.startContainer(id);
      message.success(`容器 ${name} 启动成功`);
    } catch (error: any) {
      message.error(`启动容器失败: ${error.message}`);
    }
  };

  const handleStop = async (id: string, name: string) => {
    try {
      await store.stopContainer(id);
      message.success(`容器 ${name} 停止成功`);
    } catch (error: any) {
      message.error(`停止容器失败: ${error.message}`);
    }
  };

  const handleUpdate = async (id: string, name: string, image?: string) => {
    try {
      await store.updateContainer(id, image);
      message.success(`容器 ${name} 更新成功`);
    } catch (error: any) {
      message.error(`更新容器失败: ${error.message}`);
    }
  };

  const handleDelete = async (id: string, name: string) => {
    return new Promise((resolve, reject) => {
      dialog.warning({
        title: "确认删除",
        content: `确定要删除容器 ${name} 吗？此操作不可恢复。`,
        positiveText: "删除",
        negativeText: "取消",
        onPositiveClick: async () => {
          try {
            await store.deleteContainer(id);
            message.success(`容器 ${name} 删除成功`);
            resolve(true);
          } catch (error: any) {
            message.error(`删除容器失败: ${error.message}`);
            reject(error);
          }
        },
      });
    });
  };

  return {
    handleStart,
    handleStop,
    handleUpdate,
    handleDelete,
  };
}
```

#### 响应式设计 Hook（hooks/useResponsive.ts）

```typescript
import { useBreakpoints, useWindowSize } from "@vueuse/core";

export function useResponsive() {
  const { width } = useWindowSize();

  const breakpoints = useBreakpoints({
    mobile: 640,
    tablet: 768,
    laptop: 1024,
    desktop: 1280,
  });

  const isMobile = computed(() => width.value < 768);
  const isTablet = computed(() => width.value >= 768 && width.value < 1024);
  const isLaptop = computed(() => width.value >= 1024 && width.value < 1280);
  const isDesktop = computed(() => width.value >= 1280);

  const gridCols = computed(() => {
    if (isMobile.value) return 1;
    if (isTablet.value) return 2;
    if (isLaptop.value) return 3;
    return 4;
  });

  return {
    width,
    isMobile,
    isTablet,
    isLaptop,
    isDesktop,
    gridCols,
    breakpoints,
  };
}
```

#### WebSocket Hook（hooks/useStatsWebSocket.ts）

```typescript
import { ref, onMounted, onUnmounted } from "vue";
import type { ContainerStatus } from "@/common/types";

export function useStatsWebSocket() {
  const stats = ref<Map<string, ContainerStatus>>(new Map());
  const connected = ref(false);
  let ws: WebSocket | null = null;
  let reconnectTimer: NodeJS.Timeout | null = null;

  const connect = () => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/api/containers/stats/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      connected.value = true;
      console.log("WebSocket 连接成功");
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === "stats") {
          stats.value.set(data.containerId, data.stats);
        }
      } catch (error) {
        console.error("解析 WebSocket 消息失败:", error);
      }
    };

    ws.onerror = (error) => {
      console.error("WebSocket 错误:", error);
      connected.value = false;
    };

    ws.onclose = () => {
      connected.value = false;
      console.log("WebSocket 连接关闭，3秒后重连...");
      reconnectTimer = setTimeout(() => {
        connect();
      }, 3000);
    };
  };

  const disconnect = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
    }
    if (ws) {
      ws.close();
      ws = null;
    }
    connected.value = false;
  };

  onMounted(() => {
    connect();
  });

  onUnmounted(() => {
    disconnect();
  });

  return {
    stats,
    connected,
    connect,
    disconnect,
  };
}
```

### 2.4 组件实现

#### 容器卡片组件（components/ContainerCard.vue）

```vue
<template>
  <n-card :title="container.name" hoverable>
    <template #header-extra>
      <RunningStatusBadge :running="container.running" />
      <UpdateStatusBadge :status="container.status" />
    </template>

    <n-space vertical>
      <div>
        <n-text depth="3">镜像: </n-text>
        <n-text>{{ container.image }}</n-text>
      </div>

      <div v-if="container.currentDigest">
        <n-text depth="3">摘要: </n-text>
        <n-text code>{{ container.currentDigest.slice(0, 12) }}...</n-text>
      </div>

      <div v-if="stats">
        <n-text depth="3">CPU: </n-text>
        <n-text>{{ stats.cpuPercent?.toFixed(2) }}%</n-text>
        <n-text depth="3" class="ml-4">内存: </n-text>
        <n-text>{{ formatMemory(stats.memoryUsage) }}</n-text>
      </div>

      <div>
        <n-text depth="3">最后检查: </n-text>
        <n-text>{{ formatTime(container.lastCheckedAt) }}</n-text>
      </div>
    </n-space>

    <template #footer>
      <n-space>
        <n-button
          v-if="!container.running"
          @click="emit('start')"
          type="primary"
          size="small"
        >
          启动
        </n-button>
        <n-button v-else @click="emit('stop')" type="warning" size="small">
          停止
        </n-button>
        <n-button
          v-if="container.status === 'UpdateAvailable'"
          @click="emit('update')"
          type="info"
          size="small"
          :loading="updating"
        >
          更新
        </n-button>
        <n-button @click="emit('delete')" type="error" size="small" ghost>
          删除
        </n-button>
      </n-space>
    </template>
  </n-card>
</template>

<script setup lang="ts">
import type { ContainerStatus } from "@/common/types";
import { formatTime, formatMemory } from "@/common/utils";

interface Props {
  container: ContainerStatus;
  stats?: ContainerStatus;
  updating?: boolean;
}

interface Emits {
  (e: "start"): void;
  (e: "stop"): void;
  (e: "update"): void;
  (e: "delete"): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();
</script>
```

#### 二次验证设置组件（components/TwoFASetup.vue）

```vue
<template>
  <n-card title="设置二次验证">
    <n-space vertical size="large">
      <!-- 选择验证方式 -->
      <n-radio-group v-model:value="method">
        <n-space>
          <n-radio value="otp">OTP (一次性密码)</n-radio>
          <n-radio value="webauthn">WebAuthn (生物验证)</n-radio>
        </n-space>
      </n-radio-group>

      <!-- OTP 设置 -->
      <div v-if="method === 'otp'">
        <n-space vertical>
          <n-button @click="initOTP" :loading="loading"> 生成二维码 </n-button>

          <div v-if="qrCodeURL">
            <img :src="qrCodeURL" alt="QR Code" />
            <n-text depth="3">或手动输入密钥：</n-text>
            <n-text code>{{ secret }}</n-text>
          </div>

          <n-input
            v-model:value="otpCode"
            placeholder="输入6位验证码"
            maxlength="6"
          />

          <n-button
            @click="verifyOTP"
            type="primary"
            :loading="verifying"
            :disabled="otpCode.length !== 6"
          >
            验证并完成设置
          </n-button>
        </n-space>
      </div>

      <!-- WebAuthn 设置 -->
      <div v-if="method === 'webauthn'">
        <n-space vertical>
          <n-alert type="info">
            请确保您的设备支持生物识别（指纹、Face ID 等）或安全密钥
          </n-alert>

          <n-button @click="setupWebAuthn" type="primary" :loading="loading">
            开始设置
          </n-button>
        </n-space>
      </div>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useMessage } from "naive-ui";
import { twoFAApi } from "@/common/api";
import { startRegistration } from "@simplewebauthn/browser";
import QRCode from "qrcode";

const message = useMessage();
const method = ref<"otp" | "webauthn">("otp");
const loading = ref(false);
const verifying = ref(false);

// OTP 相关
const qrCodeURL = ref("");
const secret = ref("");
const otpCode = ref("");

// 初始化 OTP
const initOTP = async () => {
  loading.value = true;
  try {
    const { data } = await twoFAApi.setupOTPInit();
    secret.value = data.data.secret;

    // 生成二维码
    qrCodeURL.value = await QRCode.toDataURL(data.data.qrCodeURL);
  } catch (error: any) {
    message.error(error.message);
  } finally {
    loading.value = false;
  }
};

// 验证 OTP
const verifyOTP = async () => {
  verifying.value = true;
  try {
    const { data } = await twoFAApi.setupOTPVerify(otpCode.value, secret.value);
    message.success("设置成功");
    emit("success", data.data.token);
  } catch (error: any) {
    message.error(error.message);
  } finally {
    verifying.value = false;
  }
};

// 设置 WebAuthn
const setupWebAuthn = async () => {
  loading.value = true;
  try {
    // 开始注册
    const { data: beginData } = await twoFAApi.setupWebAuthnBegin();

    // 调用浏览器 WebAuthn API
    const attResp = await startRegistration(beginData.data.options.publicKey);

    // 完成注册
    const { data: finishData } = await twoFAApi.setupWebAuthnFinish(
      beginData.data.sessionData,
      attResp
    );

    message.success("设置成功");
    emit("success", finishData.data.token);
  } catch (error: any) {
    message.error(error.message);
  } finally {
    loading.value = false;
  }
};

interface Emits {
  (e: "success", token: string): void;
}

const emit = defineEmits<Emits>();
</script>
```

### 2.5 路由实现

```typescript
import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "@/store/auth";

const routes = [
  {
    path: "/login",
    name: "Login",
    component: () => import("@/pages/LoginView.vue"),
  },
  {
    path: "/",
    name: "Layout",
    component: () => import("@/components/LayoutView.vue"),
    children: [
      {
        path: "",
        name: "Home",
        component: () => import("@/pages/HomeView.vue"),
      },
      {
        path: "containers",
        name: "Containers",
        component: () => import("@/pages/ContainersView.vue"),
      },
      {
        path: "images",
        name: "Images",
        component: () => import("@/pages/ImagesView.vue"),
      },
      {
        path: "compose",
        name: "Compose",
        component: () => import("@/pages/ComposeView.vue"),
      },
      {
        path: "compose/create",
        name: "ComposeCreate",
        component: () => import("@/pages/ComposeCreateView.vue"),
      },
      {
        path: "terminal",
        name: "Terminal",
        component: () => import("@/pages/TerminalView.vue"),
        meta: { requiresShell: true },
      },
      {
        path: "settings",
        name: "Settings",
        component: () => import("@/pages/SettingsView.vue"),
      },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

// 全局前置守卫
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore();

  // 检查认证
  if (to.path !== "/login" && !authStore.isLoggedIn) {
    next("/login");
    return;
  }

  // 检查 Shell 权限
  if (to.meta.requiresShell) {
    // 从系统信息检查是否启用 Shell
    // 这里需要在 app store 中保存系统信息
    const appStore = useAppStore();
    if (!appStore.systemInfo?.isShellEnabled) {
      message.warning("Shell 功能未启用");
      next("/");
      return;
    }
  }

  next();
});

export default router;
```

## 3. 特殊功能实现

### 3.1 终端实现（xterm.js）

```typescript
import { Terminal } from "xterm";
import { FitAddon } from "xterm-addon-fit";
import "xterm/css/xterm.css";

export function useTerminal(container: Ref<HTMLElement | null>) {
  const terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: "#1e1e1e",
      foreground: "#d4d4d4",
    },
  });

  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  let ws: WebSocket | null = null;

  const connect = () => {
    if (container.value) {
      terminal.open(container.value);
      fitAddon.fit();

      // 连接 WebSocket
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/api/shell/ws`);

      ws.onopen = () => {
        terminal.write("连接成功\r\n");
      };

      ws.onmessage = (event) => {
        terminal.write(event.data);
      };

      ws.onerror = () => {
        terminal.write("\r\n连接错误\r\n");
      };

      ws.onclose = () => {
        terminal.write("\r\n连接已关闭\r\n");
      };

      // 监听终端输入
      terminal.onData((data) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(data);
        }
      });
    }
  };

  const disconnect = () => {
    if (ws) {
      ws.close();
    }
    terminal.dispose();
  };

  return {
    connect,
    disconnect,
  };
}
```

### 3.2 YAML 编辑器实现（Monaco Editor）

```typescript
import * as monaco from "monaco-editor";
import loader from "@monaco-editor/loader";

export function useYamlEditor(
  container: Ref<HTMLElement | null>,
  initialValue: string
) {
  let editor: monaco.editor.IStandaloneCodeEditor | null = null;

  const init = async () => {
    if (!container.value) return;

    await loader.init();

    editor = monaco.editor.create(container.value, {
      value: initialValue,
      language: "yaml",
      theme: "vs-dark",
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
    });
  };

  const getValue = () => {
    return editor?.getValue() || "";
  };

  const setValue = (value: string) => {
    editor?.setValue(value);
  };

  const dispose = () => {
    editor?.dispose();
  };

  return {
    init,
    getValue,
    setValue,
    dispose,
  };
}
```

## 4. 性能优化

### 4.1 虚拟滚动

对于大列表使用虚拟滚动：

```vue
<template>
  <n-virtual-list
    :items="containers"
    :item-size="120"
    :item-key="(item) => item.id"
  >
    <template #default="{ item }">
      <ContainerCard :container="item" />
    </template>
  </n-virtual-list>
</template>
```

### 4.2 懒加载

路由懒加载：

```typescript
const routes = [
  {
    path: "/containers",
    component: () => import("@/pages/ContainersView.vue"),
  },
];
```

### 4.3 防抖节流

```typescript
import { debounce, throttle } from "lodash-es";

// 搜索防抖
const search = debounce((keyword: string) => {
  // 执行搜索
}, 300);

// 滚动节流
const handleScroll = throttle(() => {
  // 处理滚动
}, 100);
```

## 5. 构建与部署

### 5.1 开发环境

```bash
# 安装依赖
pnpm install

# 启动开发服务器
pnpm dev

# 代码检查
pnpm lint

# 类型检查
pnpm type-check
```

### 5.2 生产构建

```bash
# 构建
pnpm build

# 预览构建结果
pnpm preview
```

### 5.3 Docker 部署

前端静态文件由后端 Gin 服务提供，无需单独部署。

## 6. 测试

### 6.1 单元测试（可选）

```bash
# 使用 Vitest
pnpm test

# 测试覆盖率
pnpm test:coverage
```

### 6.2 端到端测试（可选）

```bash
# 使用 Playwright
pnpm test:e2e
```

## 7. 开发建议

1. **组件化**: 尽可能将复杂组件拆分为小组件
2. **类型安全**: 充分利用 TypeScript 的类型系统
3. **状态管理**: 合理使用 Pinia，避免 props drilling
4. **性能优化**: 关注首屏加载时间和运行时性能
5. **用户体验**: 提供清晰的加载状态和错误提示
6. **代码规范**: 遵循 ESLint 和 Prettier 配置
7. **文档完善**: 为复杂组件和函数添加注释

## 8. 已实现功能清单

- ✅ 基础架构搭建
- ✅ 容器管理功能
- ✅ 镜像管理功能
- ✅ Compose 项目管理
- ✅ 系统设置功能
- ✅ 二次验证（OTP + WebAuthn）
- ✅ 终端访问功能
- ✅ 响应式设计
- ✅ WebSocket 实时通信
- ✅ YAML 编辑器
- ✅ 日志查看功能
- ✅ 实时资源监控

## 9. 待优化项

- [ ] 国际化支持
- [ ] 单元测试覆盖
- [ ] 端到端测试
- [ ] PWA 离线支持
- [ ] 性能监控和分析
- [ ] 无障碍性改进
- [ ] 主题定制功能
