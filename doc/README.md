# Watch Docker 文档中心

欢迎来到 Watch Docker 文档中心！这里包含了项目的完整文档，帮助你快速上手和深入了解项目。

## 📚 文档导航

### 👤 用户指南

面向最终用户的使用文档：

- **[安装指南](./user-guide/installation.md)** - 在不同平台上安装 Watch Docker
- **[配置指南](./user-guide/configuration.md)** - 配置系统使用说明
- **[二次验证](./user-guide/2fa.md)** - 双因素认证功能使用指南

### 👨‍💻 开发者文档

面向开发者的技术文档：

- **[架构设计](./developer/architecture.md)** - 系统整体架构和设计思路
- **[后端实现](./developer/backend-implementation.md)** - 后端技术实现细节
- **[前端实现](./developer/frontend-implementation.md)** - 前端技术实现细节

### 🚀 部署文档

部署和构建相关文档：

- **[构建说明](./deployment/build.md)** - 构建产物和发布说明
- **[静态资源嵌入](./deployment/static-embed.md)** - 前端资源嵌入方案
- **[Systemd 服务](./deployment/systemd.md)** - Linux 系统服务配置

### ✨ 功能文档

特定功能的详细文档：

- **[网络功能](./features/network.md)** - 容器网络配置功能
- **[Volume 管理](./features/volume.md)** - Docker Volume 管理功能

## 🗂️ 文档结构

```
doc/
├── README.md                    # 文档索引（本文件）
├── user-guide/                 # 用户指南
│   ├── installation.md         # 安装指南
│   ├── configuration.md        # 配置指南
│   └── 2fa.md                 # 二次验证
├── developer/                  # 开发者文档
│   ├── architecture.md         # 架构设计
│   ├── backend-implementation.md
│   └── frontend-implementation.md
├── deployment/                  # 部署文档
│   ├── build.md                # 构建说明
│   ├── static-embed.md         # 静态资源嵌入
│   └── systemd.md              # Systemd 服务
└── features/                   # 功能文档
    ├── network.md              # 网络功能
    └── volume.md               # Volume 管理
```

## 🔍 快速查找

### 我想...

- **安装 Watch Docker** → [安装指南](./user-guide/installation.md)
- **配置应用** → [配置指南](./user-guide/configuration.md)
- **启用二次验证** → [二次验证指南](./user-guide/2fa.md)
- **了解架构设计** → [架构设计](./developer/architecture.md)
- **参与开发** → [后端实现](./developer/backend-implementation.md) / [前端实现](./developer/frontend-implementation.md)
- **构建项目** → [构建说明](./deployment/build.md)
- **配置系统服务** → [Systemd 服务](./deployment/systemd.md)

## 📝 文档贡献

如果你发现文档有错误或需要改进，欢迎：

1. 提交 [Issue](https://github.com/jianxcao/watch-docker/issues)
2. 创建 [Pull Request](https://github.com/jianxcao/watch-docker/pulls)

## 🔗 相关链接

- [项目主页](../../README.md)
- [GitHub 仓库](https://github.com/jianxcao/watch-docker)
- [问题反馈](https://github.com/jianxcao/watch-docker/issues)
- [讨论区](https://github.com/jianxcao/watch-docker/discussions)
