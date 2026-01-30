# 容器网络创建功能重构 - 实现文档

## 功能概述

本次重构实现了容器创建时的网络配置功能，支持三种网络创建模式：

1. **默认 Bridge 网络** - 使用 Docker 默认的 bridge 网络
2. **连接已有网络** - 连接到已存在的网络，可配置静态 IP、网关等
3. **创建新网络** - 创建一个或多个新网络，支持 IPv4/IPv6 双栈配置

## 实现的功能特性

### 网络创建模式

#### 1. 默认 Bridge 网络（简单模式）
- 使用 Docker 默认 bridge 网络
- 无需额外配置
- 适合简单场景

#### 2. 连接已有网络
- 连接到一个或多个已存在的网络
- 支持配置静态 IPv4/IPv6 地址
- 支持配置网关地址
- 支持配置 MAC 地址
- 支持网络别名

#### 3. 创建新网络
- 创建一个或多个新网络
- 支持多种驱动类型：bridge、overlay、macvlan
- 支持 IPv4 配置：子网、网关
- 支持 IPv6 配置：子网、网关（可选启用）
- 支持其他选项：内部网络、可附加

### 通用功能

- DNS 服务器配置
- DNS 搜索域配置
- DNS 选项配置
- 自定义 Hosts 记录
- 发布所有已曝光的端口

## 后端实现

### 数据结构

#### NetworkToCreate
```go
type NetworkToCreate struct {
    Name       string                    `json:"name" binding:"required"`
    Driver     string                    `json:"driver"`
    EnableIPv6 bool                      `json:"enableIPv6"`
    IPAM       *NetworkIPAMCreateRequest `json:"ipam,omitempty"`
    Internal   bool                      `json:"internal"`
    Attachable bool                      `json:"attachable"`
    Labels     map[string]string         `json:"labels,omitempty"`
    Options    map[string]string         `json:"options,omitempty"`
}
```

### 网络创建流程

1. 接收容器创建请求
2. 验证并拉取镜像（如需要）
3. 处理 `networksToCreate` 列表：
   - 验证网络名称
   - 设置默认驱动为 bridge
   - 检查网络是否已存在
   - 如果不存在，创建网络
   - 记录详细日志
4. 创建并启动容器

### 错误处理

- 网络名称为空：返回 400 错误
- 网络创建失败：返回详细错误信息
- 网络已存在：跳过创建，记录日志

## 前端实现

### 界面结构

#### 第一部分：网络配置模式选择
- 使用单选按钮组选择三种模式
- 每个选项附带说明文字

#### 第二部分：模式特定配置

**默认 Bridge 模式**
- 显示提示信息
- 发布所有端口开关

**连接已有网络模式**
- 网络模式选择器
- 网络端点配置列表
- 每个端点可配置：
  - 网络名称
  - IPv4/IPv6 地址和网关
  - MAC 地址
  - 网络别名

**创建新网络模式**
- 动态网络列表
- 每个网络可配置：
  - 网络名称（必填）
  - 驱动类型（bridge/overlay/macvlan）
  - IPv6 开关
  - IPv4 配置（子网、网关）
  - IPv6 配置（子网、网关）
  - 其他选项（内部、可附加）

#### 第三部分：通用配置
- DNS 服务器
- DNS 搜索域
- DNS 选项
- 额外的 Hosts

### 数据转换逻辑

`transformNetworkForm` 函数根据 `configMode` 执行不同的转换：

- **default**: 设置 `networkMode` 为 `'bridge'`
- **existing**: 构建 `networkConfig.endpointsConfig`
- **create**: 构建 `networksToCreate` 和 `networkConfig.endpointsConfig`

## 文件清单

### 后端
- `backend/internal/api/container_router.go` - 主要实现

### 前端
- `frontend/src/pages/CreateContainer/types.ts` - 类型定义
- `frontend/src/pages/CreateContainer/NetworkTab.vue` - 界面组件
- `frontend/src/pages/CreateContainer/transformer.ts` - 数据转换
- `frontend/src/pages/CreateContainer/ContainerCreateView.vue` - 容器创建视图
- `frontend/src/common/types.ts` - 公共类型定义

## 测试场景

### 正常场景
1. 使用默认 bridge 网络创建容器
2. 创建 IPv4 网络并连接容器
3. 创建 IPv4+IPv6 双栈网络并连接容器
4. 连接到已有网络并配置静态 IP
5. 创建多个网络并连接容器
6. 尝试创建已存在的网络（应跳过）

### 错误场景
1. 网络名称为空
2. 无效的子网格式
3. 子网冲突

## 技术亮点

1. **向后兼容** - 保留原有功能，新增功能不影响现有用户
2. **错误处理** - 完善的验证和错误提示
3. **用户体验** - 清晰的模式区分，动态显示配置项
4. **类型安全** - 完整的 TypeScript 类型定义
5. **扩展性** - 易于添加新的网络驱动和配置选项

## 验证结果

- ✅ 后端 Go 代码编译通过
- ✅ 前端 TypeScript 类型检查通过
- ✅ 无 linter 错误
- ✅ 所有待办事项已完成

## 使用示例

### 创建 IPv4 网络示例

1. 选择"创建新网络"模式
2. 点击"添加新网络"
3. 配置网络：
   - 网络名称：`my-network`
   - 驱动类型：`Bridge (默认)`
   - IPv4 子网：`172.20.0.0/16`
   - IPv4 网关：`172.20.0.1`
4. 填写容器基础信息
5. 点击创建

### 创建双栈网络示例

1. 选择"创建新网络"模式
2. 点击"添加新网络"
3. 配置网络：
   - 网络名称：`dualstack-network`
   - 驱动类型：`Bridge (默认)`
   - 启用 IPv6：是
   - IPv4 子网：`172.20.0.0/16`
   - IPv4 网关：`172.20.0.1`
   - IPv6 子网：`2001:db8::/64`
   - IPv6 网关：`2001:db8::1`
4. 填写容器基础信息
5. 点击创建

## 后续改进方向

1. **表单验证增强**
   - IP 地址格式验证
   - CIDR 格式验证
   - 子网冲突检测

2. **网络管理增强**
   - 显示现有网络列表供选择
   - 网络拓扑可视化
   - 网络连通性测试

3. **高级选项**
   - MTU 配置
   - 网络插件选项
   - 更多驱动类型支持

