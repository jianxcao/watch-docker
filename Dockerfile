# 前端构建阶段
FROM node:22-alpine AS frontend-builder

# 设置工作目录
WORKDIR /app

# 安装pnpm
RUN npm install -g pnpm

# 复制前端代码
COPY frontend/package.json frontend/pnpm-lock.yaml ./

# 安装依赖
RUN pnpm install --frozen-lockfile

# 复制前端源代码
COPY frontend/ .

# 构建前端应用
RUN pnpm build

# 后端构建阶段
FROM golang:1.25.1-alpine AS backend-builder

# 添加平台参数，由 Docker buildx 自动提供
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# 设置工作目录
WORKDIR /app

# 安装git、ca-certificates 以及构建所需工具链（gcc、musl-dev 等）
RUN apk add --no-cache git ca-certificates build-base pkgconfig binutils musl-dev lld binutils-gold

# 复制后端代码
COPY backend/ ./

# 下载依赖
RUN go mod download

# 编译应用 - 使用动态平台参数
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o watch-docker cmd/watch-docker/main.go

# 运行阶段
FROM alpine:latest

# 安装ca-certificates、timezone数据、wget (用于健康检查)、gosu和shadow-utils
RUN apk --no-cache add ca-certificates tzdata wget gosu shadow
RUN apk add --no-cache dumb-init

# 安装 Docker CLI 和 Docker Compose
RUN apk add --no-cache docker-cli docker-cli-compose

# 设置时区
ENV TZ=Asia/Shanghai

RUN addgroup -g 1000 watch-docker && \
    adduser -D -u 1000 -G watch-docker watch-docker

RUN mkdir -p /app
RUN mkdir -p /config
RUN mkdir -p /app/static
# 复制配置文件模板
RUN touch config/config.yaml
# 设置工作目录
WORKDIR /app
VOLUME /config

# 从后端构建阶段复制二进制文件
COPY --from=backend-builder /app/watch-docker .

# 复制精简后的插件目录到镜像种子目录（仅 setting.json 和 .so）
# COPY --from=backend-builder /app/plugins-dist /opt/plugins-dist

# 从前端构建阶段复制静态文件
COPY --from=frontend-builder /app/dist /app/static
COPY ./entrypoint /app/entrypoint
RUN chmod +x /app/entrypoint
RUN chmod +x /app/watch-docker

# 设置目录权限
RUN chown -R watch-docker:watch-docker /app /config
RUN chmod -R 755 /app

ENV PGID=1000
ENV PUID=1000
ENV UMASK=022
ENV CONFIG_PATH=/config
ENV CONFIG_FILE=config.yaml
ENV PORT=8088
ENV LOG_LEVEL=info
ENV LOG_FORMAT=text
ENV STATIC_DIR=/app/static
ENV USER_NAME=admin
ENV USER_PASSWORD=admin
# 启动应用
ENTRYPOINT ["/app/entrypoint" ]
EXPOSE 8088