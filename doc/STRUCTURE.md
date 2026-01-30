# 文档结构说明

## 📁 文档目录结构

```
doc/
├── README.md                    # 文档索引（主入口）
│
├── user-guide/                  # 👤 用户指南
│   ├── installation.md         # 安装指南（合并了 Windows 安装）
│   ├── configuration.md         # 配置指南
│   └── 2fa.md                  # 二次验证使用指南
│
├── developer/                   # 👨‍💻 开发者文档
│   ├── architecture.md          # 架构设计（合并后端和前端设计）
│   ├── backend-implementation.md  # 后端技术实现
│   └── frontend-implementation.md  # 前端技术实现
│
├── deployment/                  # 🚀 部署文档
│   ├── build.md                 # 构建产物说明
│   ├── static-embed.md          # 前端资源嵌入方案
│   └── systemd.md               # Systemd 服务配置（合并了所有 systemd 相关）
│
├── features/                    # ✨ 功能文档
│   ├── network.md               # 网络功能
│   └── volume.md                # Volume 管理功能
│
└── archive/                     # 📦 归档目录（旧文档）
    └── README.md                # 归档说明
```

## 📝 文档分类说明

### 用户指南（user-guide/）
面向最终用户的使用文档，包括安装、配置和使用说明。

### 开发者文档（developer/）
面向开发者的技术文档，包括架构设计和技术实现细节。

### 部署文档（deployment/）
部署和构建相关文档，包括构建说明、静态资源嵌入和系统服务配置。

### 功能文档（features/）
特定功能的详细文档，包括网络功能和 Volume 管理功能。

## 🔄 文档整理说明

### 合并的文档
- `installation-guide.md` + `windows-install.md` → `user-guide/installation.md`
- `backend-design.md` + `frontend-design.md` → `developer/architecture.md`
- `systemd-service-fix.md` + `systemd-chdir-fix.md` → `deployment/systemd.md`
- `static-embed-solution.md` + `static-embed-optimization-summary.md` → `deployment/static-embed.md`

### 移动的文档
- `2fa-usage.md` → `user-guide/2fa.md`
- `configuration-guide.md` → `user-guide/configuration.md`
- `backend-implementation.md` → `developer/backend-implementation.md`
- `frontend-implementation.md` → `developer/frontend-implementation.md`
- `build-artifacts.md` → `deployment/build.md`
- `network-feature-update.md` → `features/network.md`
- `volume-implementation-summary.md` → `features/volume.md`

### 归档的文档
所有旧文档已移动到 `archive/` 目录，保留作为历史参考。

