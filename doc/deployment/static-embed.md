# 前端资源嵌入方案

Watch Docker 需要在两种不同的部署场景下工作：Docker 部署和原生安装包。本文档说明如何通过 Go 构建标签实现条件编译。

## 问题背景

Watch Docker 项目需要在两种不同的部署场景下工作：

1. **Docker 部署**：前端资源位于 `/app/static` 目录
2. **原生安装包**：前端资源嵌入到 Go 二进制文件中

这两种场景对 Go embed 的要求不同：
- Docker 构建时，后端编译阶段前端还未构建，无法嵌入
- 原生构建时，需要嵌入完整的前端资源到二进制中

## 解决方案

使用 **Go 构建标签（Build Tags）** 实现条件编译，根据不同的构建场景选择不同的 embed 实现。

### 文件结构

```
backend/internal/api/
├── router.go                      # 主路由文件
├── static_embed.go                # 原生构建：嵌入静态资源
├── static_embed_docker.go         # Docker 构建：空 embed
└── static/                        # 静态资源目录
    ├── .gitkeep                   # Git 占位符
    ├── README.md                  # 说明文档
    ├── index.placeholder.html     # 占位 HTML
    └── [构建时生成的前端资源]
```

### 实现细节

#### 1. static_embed.go（原生构建）

```go
// +build !docker

package api

import "embed"

//go:embed static
var staticFS embed.FS
```

- **构建标签**：`!docker`（非 Docker 构建）
- **行为**：嵌入 `static` 目录的所有文件
- **使用场景**：GoReleaser 构建原生安装包

#### 2. static_embed_docker.go（Docker 构建）

```go
// +build docker

package api

import "embed"

var staticFS embed.FS
```

- **构建标签**：`docker`
- **行为**：创建空的 `embed.FS`，不嵌入任何文件
- **使用场景**：Docker 镜像构建

#### 3. router.go

```go
func (s *Server) setupStaticRoutes(r *gin.Engine) {
    staticDir := conf.EnvCfg.STATIC_DIR
    
    if staticDir != "" {
        // 使用外部静态目录
        s.setupExternalStaticRoutes(r, staticDir)
    } else {
        // 使用嵌入式静态资源
        s.setupEmbeddedStaticRoutes(r)
    }
}
```

## 构建命令

### 原生构建（默认）

```bash
# 不指定标签，使用 static_embed.go
go build -o watch-docker cmd/watch-docker/main.go

# GoReleaser 使用
goreleaser release --snapshot --clean
```

**结果**：
- 二进制大小：~28MB（包含 ~10MB 前端资源）
- 运行时：如果 `STATIC_DIR` 为空，使用嵌入资源

### Docker 构建

```bash
# 使用 docker 标签，使用 static_embed_docker.go
go build -tags docker -o watch-docker cmd/watch-docker/main.go

# Dockerfile 中
RUN go build -tags docker -o watch-docker cmd/watch-docker/main.go
```

**结果**：
- 二进制大小：~18MB（不包含前端资源）
- 运行时：必须设置 `STATIC_DIR=/app/static`

## 部署场景

### 场景 1：Docker 部署

```yaml
# docker-compose.yaml
environment:
  STATIC_DIR: /app/static  # 使用容器内的静态目录
```

```dockerfile
# Dockerfile
ENV STATIC_DIR=/app/static
COPY --from=frontend-builder /app/dist /app/static
```

**流程**：
1. 前端构建阶段：`pnpm build` → `frontend/dist`
2. 后端构建阶段：`go build -tags docker`（不嵌入资源）
3. 运行阶段：将前端产物复制到 `/app/static`
4. 应用启动：读取 `STATIC_DIR=/app/static`

### 场景 2：原生安装包

```bash
# GoReleaser 构建流程
goreleaser release --snapshot

# before hooks:
#   1. cd frontend && pnpm build
#   2. cp frontend/dist/* backend/internal/api/static/
# builds:
#   - go build (默认，不带 docker 标签)
```

**流程**：
1. GoReleaser before hooks：构建前端 → 复制到 `static/`
2. Go 构建：`go build`（嵌入 `static/` 内容）
3. 打包：生成 DEB/RPM/tar.gz 等安装包
4. 用户安装：单个二进制文件，无需额外静态文件

### 场景 3：开发环境

```bash
# 前端开发服务器
cd frontend && pnpm dev

# 后端
cd backend
STATIC_DIR=../frontend/dist go run cmd/watch-docker/main.go
```

或者使用嵌入模式：

```bash
# 先构建前端
cd frontend && pnpm build

# 复制到 static 目录
cp -r dist/* ../backend/internal/api/static/

# 启动后端（不设置 STATIC_DIR，使用嵌入资源）
cd ../backend && go run cmd/watch-docker/main.go
```

## Git 配置

### .gitignore

```gitignore
# 忽略 static 目录的构建产物，保留占位文件
backend/internal/api/static/*
!backend/internal/api/static/.gitkeep
!backend/internal/api/static/README.md
!backend/internal/api/static/index.placeholder.html
```

**保留的占位文件作用**：
- `.gitkeep`：确保空仓库克隆后目录存在
- `README.md`：说明文档
- `index.placeholder.html`：最小化 HTML，防止 embed 完全为空

## 优势

1. **Docker 构建无依赖**：
   - 后端构建时不依赖前端资源
   - 构建顺序灵活，前后端可并行构建

2. **原生包完整独立**：
   - 单个二进制文件包含所有资源
   - 无需额外安装步骤

3. **代码简洁**：
   - 核心逻辑统一在 `router.go`
   - 通过构建标签自动选择实现

4. **向后兼容**：
   - 保留 `STATIC_DIR` 环境变量支持
   - 现有 Docker 部署无需修改

## 测试验证

### 测试原生构建

```bash
cd backend
go build -o watch-docker-native cmd/watch-docker/main.go

# 查看大小
ls -lh watch-docker-native

# 测试运行（使用嵌入资源）
./watch-docker-native
```

### 测试 Docker 构建

```bash
cd backend
go build -tags docker -o watch-docker-docker cmd/watch-docker/main.go

# 查看大小（应该比原生版本小）
ls -lh watch-docker-docker

# 测试运行（必须指定 STATIC_DIR）
STATIC_DIR=./static ./watch-docker-docker
```

### 完整 Docker 构建测试

```bash
docker build -t watch-docker:test .
docker run -p 8080:8088 watch-docker:test
```

## 故障排查

### 问题 1：Go build 找不到 static 目录

**原因**：仓库刚克隆，`static` 目录为空

**解决**：确保占位文件已提交到 Git

```bash
ls backend/internal/api/static/
# 应该看到：.gitkeep README.md index.placeholder.html
```

### 问题 2：嵌入的静态资源是空的

**原因**：构建时 `static` 目录中只有占位文件

**解决**：GoReleaser 构建前确保执行 before hooks

```yaml
before:
  hooks:
    - sh -c "cd frontend && pnpm install && pnpm build"
    - sh -c "cp -r frontend/dist/* backend/internal/api/static/"
```

### 问题 3：Docker 构建失败 - embed 错误

**原因**：Dockerfile 中未使用 `-tags docker`

**解决**：检查 Dockerfile

```dockerfile
RUN go build -tags docker -o watch-docker cmd/watch-docker/main.go
```

## 总结

这个方案通过 Go 的构建标签功能，优雅地解决了 Docker 和原生部署的不同需求：

- ✅ Docker 构建无需前端资源
- ✅ 原生包自包含所有资源
- ✅ 代码简洁，易于维护
- ✅ 向后兼容现有部署
- ✅ 支持多种开发模式
