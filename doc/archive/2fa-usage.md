# 二次验证功能使用说明

## 功能概述

Watch Docker 支持二次验证（Two-Factor Authentication, 2FA）功能，为您的管理界面提供额外的安全保护。在用户名密码验证的基础上，要求完成第二重验证才能访问系统，有效防止未授权访问。

### 支持的验证方式

1. **OTP（一次性密码）**

   - 基于 TOTP（时间型一次性密码）协议
   - 支持 Google Authenticator、Authy、微软身份验证器等标准应用
   - 30 秒刷新的 6 位验证码
   - 多设备支持（可在多个设备上添加相同密钥）

2. **WebAuthn（生物验证）**
   - 基于 FIDO2/WebAuthn 标准
   - 支持指纹识别、Face ID、Windows Hello
   - 支持硬件安全密钥（如 YubiKey）
   - 多域名凭据支持，适配不同访问场景
   - 更高的安全性，防钓鱼攻击

## 启用二次验证

### 1. 系统管理员启用功能

在启动 Watch Docker 时，设置环境变量：

```bash
IS_SECONDARY_VERIFICATION=true
```

或在 Docker Compose 配置中：

```yaml
environment:
  - IS_SECONDARY_VERIFICATION=true
```

### 2. 用户首次设置

当二次验证功能启用后，用户首次登录时需要设置验证方式：

#### OTP 设置步骤

1. 登录时输入用户名和密码
2. 选择"OTP (一次性密码)"
3. 点击"生成二维码"
4. 使用身份验证器应用扫描二维码
5. 输入验证器应用显示的 6 位验证码
6. 完成设置

#### WebAuthn 设置步骤

1. 登录时输入用户名和密码
2. 选择"WebAuthn (生物验证)"
3. 点击"开始设置"
4. 按照浏览器提示完成生物识别注册
5. 完成设置

## 日常使用

设置完成后，每次登录都需要进行二次验证：

### OTP 验证

1. 输入用户名和密码
2. 打开身份验证器应用
3. 输入当前显示的 6 位验证码
4. 完成登录

### WebAuthn 验证

1. 输入用户名和密码
2. 点击"使用生物验证"按钮
3. 按照浏览器提示完成生物识别
4. 完成登录

## 管理二次验证

### 查看状态

进入"系统设置"页面，在"二次验证"部分可以查看：

- 当前状态（已启用/未启用）
- 验证方式（OTP/WebAuthn）

### 禁用二次验证

在"系统设置"页面点击"禁用二次验证"按钮即可关闭。

**注意**：禁用后需要重新设置才能再次启用。

## 安全建议

1. **妥善保管密钥**：OTP 密钥一旦丢失，将无法登录。建议在设置时备份密钥或截图保存二维码
2. **多设备备份**：可以在多个设备上添加同一个 OTP 密钥
3. **定期检查**：定期确认二次验证功能正常工作
4. **浏览器兼容性**：WebAuthn 需要浏览器支持，推荐使用最新版本的 Chrome、Firefox、Safari 或 Edge

## 故障排除

### 无法扫描二维码

- 尝试使用"手动输入密钥"功能
- 复制密钥到身份验证器应用中手动添加

### 验证码错误

- 确保设备时间准确（OTP 基于时间生成）
- 等待下一个验证码再试
- 检查是否输入了正确的 6 位数字

### WebAuthn 不可用

- 确认浏览器支持 WebAuthn
- 检查是否启用了生物识别功能
- 尝试使用 HTTPS 连接（某些浏览器要求）

## 技术细节

### OTP（TOTP）

- 基于时间的一次性密码算法（RFC 6238）
- 30 秒刷新一次
- 6 位数字验证码
- 密钥使用 Base32 编码存储

### WebAuthn

- 基于 FIDO2 标准
- 公钥加密，私钥保存在设备中
- 支持多种认证器（平台认证器、安全密钥等）
- 更高的安全性，防止钓鱼攻击

## 配置存储

二次验证配置存储在主配置文件中：

```
<CONFIG_PATH>/config.yaml
```

在配置文件的 `twofa` 部分：

```yaml
twofa:
  users:
    admin:
      method: "otp" # 验证方式：otp 或 webauthn
      otpSecret: "BASE32_ENCODED_SECRET" # OTP 密钥（Base32 编码）
      webauthnCredentials: # WebAuthn 凭据列表
        - credential: { ... } # 凭据详情
          rpid: "example.com" # 注册的域名
      isSetup: true # 是否已完成设置
```

**说明**：

- 二次验证的启用与否由 `IS_SECONDARY_VERIFICATION` 环境变量统一控制
- 用户配置包含验证方法、凭据和设置状态
- WebAuthn 凭据与域名绑定，支持多域名场景
- 配置文件包含敏感信息，应妥善保管

**重要提示**：

⚠️ **不要手动编辑二次验证配置**，以免导致配置损坏或安全问题。所有配置应通过 Web 界面管理。

⚠️ **定期备份配置文件**，特别是在设置二次验证后。配置丢失可能导致无法登录。

⚠️ **妥善保管 OTP 密钥**，建议在设置时保存二维码截图或手动记录密钥。

## 高级配置

### 域名白名单（WebAuthn）

在生产环境中，建议配置 WebAuthn 域名白名单以提高安全性：

```yaml
environment:
  - TWOFA_ALLOWED_DOMAINS=example.com,app.example.com,192.168.1.100
```

**说明**：

- 多个域名用逗号分隔
- 留空或不设置表示允许所有域名
- 白名单限制只适用于 WebAuthn，不影响 OTP
- 支持 IP 地址和域名

### 反向代理场景

如果通过反向代理（如 Nginx、Caddy）访问 Watch Docker，WebAuthn 需要正确的域名配置：

**Nginx 配置示例**：

```nginx
location / {
    proxy_pass http://watch-docker:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Real-IP $remote_addr;
}
```

**Caddy 配置示例**：

```
app.example.com {
    reverse_proxy watch-docker:8080
}
```

## 常见问题

### 1. 如何在多个设备上使用 OTP？

在首次设置时，可以在多个设备的身份验证器应用中扫描同一个二维码，或手动输入相同的密钥。所有设备将生成相同的验证码。

### 2. 如何在不同域名下使用 WebAuthn？

WebAuthn 凭据与域名绑定。如果通过不同域名访问（如 `localhost` 和 `app.example.com`），需要在每个域名下分别注册 WebAuthn 凭据。系统会自动管理多个域名的凭据。

### 3. 忘记 OTP 密钥或丢失设备怎么办？

如果完全丢失访问权限：

1. 停止 Watch Docker 容器
2. 编辑配置文件，将用户的 `isSetup` 设为 `false`
3. 重启容器后重新设置二次验证

**预防措施**：

- 在多个设备上添加 OTP 密钥
- 保存二维码截图或手动记录密钥
- 定期备份配置文件

### 4. 可以同时使用多种验证方式吗？

当前版本每个用户只能选择一种验证方式（OTP 或 WebAuthn）。如需切换验证方式，需要先禁用当前方式，然后重新设置新方式。

### 5. 二次验证会影响性能吗？

二次验证对系统性能影响极小：

- 仅在登录时触发，不影响日常操作
- OTP 验证是本地计算，响应时间 < 10ms
- WebAuthn 验证由浏览器和设备处理，服务器端开销很小

## 相关文档

- **实现总结**: `/doc/2fa-implementation.md` - 技术实现和开发文档
- **架构设计**: `/doc/design.md` - 系统架构和二次验证设计
- **技术细节**: `/doc/tech-implementation.md` - 代码结构和技术实现
- **主文档**: `/README.md` - 项目概述和快速开始指南

## 反馈与支持

如有问题或建议，欢迎：

- 提交 [Issue](https://github.com/jianxcao/watch-docker/issues)
- 查看项目 [Wiki](https://github.com/jianxcao/watch-docker/wiki)
- 参与项目讨论
