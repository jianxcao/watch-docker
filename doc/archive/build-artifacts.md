# 构建产物说明

## 📦 最终产物（会上传到 GitHub Release）

GoReleaser 构建完成后，**只会**将以下文件上传到 GitHub Release：

### 压缩包
- `watch-docker_{{version}}_{os}_{arch}.tar.gz` - Linux/macOS 压缩包
- `watch-docker_{{version}}_{os}_{arch}.zip` - Windows 压缩包

### 安装包
- `watch-docker_{{version}}_{os}_{arch}.deb` - Debian/Ubuntu 安装包
- `watch-docker_{{version}}_{os}_{arch}.rpm` - RHEL/CentOS/Fedora 安装包

### 校验文件
- `checksums.txt` - 所有文件的 SHA256 校验和

**总计：** 每个版本约 9 个文件（5 个平台 × 2 种格式 + checksums.txt）

## 🗂️ 临时目录（不会上传）

构建过程中会在 `dist/` 目录下生成以下临时目录，**这些目录不会被上传到 GitHub Release**：

### 构建临时目录
```
dist/watch-docker_darwin_amd64_v1/     # macOS Intel 构建目录
dist/watch-docker_darwin_arm64_v8.0/   # macOS Apple Silicon 构建目录
dist/watch-docker_linux_amd64_v1/      # Linux AMD64 构建目录
dist/watch-docker_linux_arm64_v8.0/    # Linux ARM64 构建目录
dist/watch-docker_windows_amd64_v1/    # Windows AMD64 构建目录
```

**用途：** 这些目录包含编译好的二进制文件，GoReleaser 会从中提取文件打包成压缩包和安装包。

### Homebrew 配置目录
```
dist/homebrew/Casks/watch-docker.rb    # Homebrew Cask 配置文件
```

**用途：** 这个文件需要推送到 `homebrew-tap` 仓库，用于 Homebrew 安装。

### 元数据文件
```
dist/artifacts.json    # 构建产物元数据
dist/metadata.json     # 构建元数据
dist/config.yaml       # GoReleaser 配置快照
```

**用途：** 用于调试和构建追踪，不会上传。

## ✅ 优化措施

### 1. `.gitignore` 配置
```gitignore
dist    # 整个 dist 目录已忽略，不会被提交到 Git
```

### 2. CI/CD 清理步骤
在 `.github/workflows/release.yml` 中添加了清理步骤：

```yaml
- name: Cleanup temporary directories
  run: |
    find dist -type d -name "watch-docker_*" -exec rm -rf {} + 2>/dev/null || true
```

### 3. Artifact 上传优化
只上传最终产物：

```yaml
path: |
  dist/*.tar.gz
  dist/*.zip
  dist/*.deb
  dist/*.rpm
  dist/checksums.txt
  dist/homebrew/
```

## 📊 文件大小对比

| 类型 | 大小 | 说明 |
|------|------|------|
| 压缩包 | ~7-8 MB | 最终产物，会上传 |
| 安装包 | ~7-8 MB | 最终产物，会上传 |
| 临时目录 | ~25 MB × 5 | 构建中间产物，不会上传 |
| Homebrew | ~1 KB | 配置文件，需要推送 |

## 🎯 总结

1. **GitHub Release 只包含最终产物**：压缩包、安装包、校验文件
2. **临时目录只是本地构建产物**：不会上传，也不会提交到 Git
3. **CI/CD 已优化**：自动清理临时目录，只上传需要的文件
4. **用户下载的只有最终产物**：压缩包或安装包，不包含任何临时文件

## 🔍 验证方法

### 检查本地构建产物
```bash
# 查看所有文件
ls -lh dist/

# 只查看最终产物
ls -lh dist/*.{tar.gz,zip,deb,rpm} dist/checksums.txt

# 查看临时目录
find dist -type d -name "watch-docker_*"
```

### 检查 GitHub Release
访问 GitHub Release 页面，确认只看到：
- ✅ 压缩包（.tar.gz, .zip）
- ✅ 安装包（.deb, .rpm）
- ✅ checksums.txt
- ❌ 没有临时目录
- ❌ 没有元数据文件

## 📝 注意事项

1. **`dist/homebrew/` 目录需要保留**：用于推送到 homebrew-tap 仓库
2. **本地开发时临时目录会存在**：这是正常的，不影响发布
3. **使用 `--clean` 标志**：`goreleaser release --clean` 会在构建前清理 dist 目录
4. **CI/CD 会自动清理**：GitHub Actions 工作流已配置清理步骤
