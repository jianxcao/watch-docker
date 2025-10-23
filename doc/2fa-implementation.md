# 二次验证功能实现总结

## 实现概述

成功为 Watch Docker 添加了完整的二次验证（2FA）功能，支持 OTP 和 WebAuthn 两种验证方式。

## 主要变更

### 后端变更

#### 1. 依赖库添加

- `github.com/pquerna/otp` - OTP（TOTP）生成和验证
- `github.com/go-webauthn/webauthn` - WebAuthn 生物验证

#### 2. 新增模块 `/backend/internal/twofa/`

- `types.go` - 定义数据结构和 WebAuthn 用户接口
- `otp.go` - OTP 密钥生成、二维码生成、验证功能
- `webauthn.go` - WebAuthn 注册和验证流程
- `storage.go` - 配置持久化（保存到 `twofa.yaml`）

#### 3. 修改 `/backend/internal/auth/auth.go`

- 在 `Claims` 结构添加 `TwoFAVerified` 和 `IsTempToken` 字段
- 新增 `GenerateTempToken()` - 生成临时 token
- 新增 `UpgradeTempToken()` - 升级临时 token 为完整 token
- 新增 `TempTokenMiddleware()` - 允许临时 token 的中间件

#### 4. 新增 `/backend/internal/api/twofa_handler.go`

实现以下 API 处理函数：

- `handleTwoFAStatus()` - 获取二次验证状态
- `handleOTPSetupInit()` - 初始化 OTP 设置
- `handleOTPSetupVerify()` - 验证并启用 OTP
- `handleVerifyOTP()` - 验证 OTP 登录
- `handleWebAuthnRegisterBegin/Finish()` - WebAuthn 注册流程
- `handleWebAuthnLoginBegin/Finish()` - WebAuthn 登录验证
- `handleDisableTwoFA()` - 禁用二次验证

#### 5. 修改 `/backend/internal/api/router.go`

- 添加 `/api/v1/2fa/*` 路由组（使用 `TempTokenMiddleware`）
- 修改 `handleLogin()` - 集成二次验证流程
- 在系统信息中添加 `isSecondaryVerificationEnabled` 字段

#### 6. 修改 `/backend/internal/api/res.go`

- 添加 `CodeUnauthorized` 和 `CodeInternalError` 错误代码

### 前端变更

#### 1. 依赖库添加

- `qrcode` - 二维码生成
- `@simplewebauthn/browser` - WebAuthn 客户端
- `@types/qrcode` - TypeScript 类型定义

#### 2. 修改 `/frontend/src/common/api.ts`

- 更新 `authApi.login` 返回类型支持二次验证字段
- 新增 `twoFAApi` - 所有二次验证相关 API

#### 3. 修改 `/frontend/src/common/types.ts`

- 在 `SystemInfo` 接口添加 `isSecondaryVerificationEnabled` 字段

#### 4. 修改 `/frontend/src/store/auth.ts`

- 添加二次验证状态：`twoFARequired`, `twoFASetupRequired`, `tempToken`, `twoFAMethod`
- 修改 `login()` - 处理二次验证流程
- 新增 `completeTwoFA()` - 完成二次验证后设置完整 token

#### 5. 新增组件

- `/frontend/src/components/TwoFASetup.vue` - 二次验证设置组件
  - 支持选择 OTP 或 WebAuthn
  - OTP：显示二维码、验证码输入
  - WebAuthn：触发浏览器生物识别
- `/frontend/src/components/TwoFAVerify.vue` - 二次验证验证组件
  - OTP：验证码输入
  - WebAuthn：生物识别按钮

#### 6. 修改 `/frontend/src/pages/LoginView.vue`

- 条件渲染：登录表单 vs 二次验证
- 集成 `TwoFASetup` 和 `TwoFAVerify` 组件
- 处理二次验证成功回调

#### 7. 修改 `/frontend/src/pages/SettingsView.vue`

- 添加"二次验证"设置卡片
- 显示状态和验证方式
- 支持设置和禁用功能

## 数据流程

### 登录流程（启用二次验证时）

```
1. 用户输入用户名密码 → 提交
2. 后端验证用户名密码 ✓
3. 后端检查 IS_SECONDARY_VERIFICATION 环境变量
4. 后端检查用户是否已设置二次验证
5. 后端生成临时 token 返回
   ↓
   {
     needTwoFA: true,
     isSetup: false/true,
     method: "otp"/"webauthn",
     tempToken: "xxx",
     username: "admin"
   }
6. 前端保存临时 token
7. 前端显示：
   - isSetup=false → TwoFASetup 组件（设置）
   - isSetup=true → TwoFAVerify 组件（验证）
8. 用户完成设置/验证
9. 后端升级临时 token 为完整 token
10. 前端保存完整 token，登录成功
```

### 配置存储

二次验证配置集成到主配置文件 `config.yaml` 中：

```yaml
# ... 其他配置 ...

twofa:
  users:
    admin:
      method: "otp" # 或 "webauthn"
      otpSecret: "BASE32_ENCODED_SECRET"
      webauthnCredentials: [] # JSON 序列化的 WebAuthn 凭据
      isSetup: true
```

**说明**：

- `enabled` 字段已移除，二次验证是否启用完全由 `IS_SECONDARY_VERIFICATION` 环境变量控制
- 每个用户只需要存储验证方法、凭据和设置状态

**优势**：

- 统一配置管理，使用现有的 `internal/config` 包
- 自动支持配置的加载、保存和验证
- 与其他系统配置保持一致
- 不需要单独的配置文件

## API 接口

### 二次验证相关

- `GET /api/v1/2fa/status` - 获取二次验证状态
- `POST /api/v1/2fa/setup/otp/init` - 初始化 OTP
- `POST /api/v1/2fa/setup/otp/verify` - 验证并启用 OTP
- `POST /api/v1/2fa/setup/webauthn/begin` - 开始 WebAuthn 注册
- `POST /api/v1/2fa/setup/webauthn/finish` - 完成 WebAuthn 注册
- `POST /api/v1/2fa/verify/otp` - 验证 OTP
- `POST /api/v1/2fa/verify/webauthn/begin` - 开始 WebAuthn 验证
- `POST /api/v1/2fa/verify/webauthn/finish` - 完成 WebAuthn 验证
- `POST /api/v1/2fa/disable` - 禁用二次验证

所有二次验证 API 都使用 `TempTokenMiddleware`，允许使用临时 token。

## 安全特性

1. **临时 token** - 首次登录后颁发临时 token（15 分钟有效期），完成二次验证后升级为完整 token
2. **密钥加密存储** - OTP 密钥使用 Base32 编码存储
3. **WebAuthn 安全** - 基于公钥加密，私钥保存在用户设备
4. **配置文件保护** - twofa.yaml 使用 0600 权限
5. **分离验证流程** - 用户名密码验证与二次验证分离

## 测试建议

### 后端测试

```bash
cd backend
go test ./internal/twofa/...
go build ./cmd/watch-docker/
```

### 前端测试

```bash
cd frontend
pnpm run build
```

### 功能测试

1. **启用二次验证环境变量**

   ```bash
   IS_SECONDARY_VERIFICATION=true
   ```

2. **测试 OTP 设置流程**

   - 登录 → 选择 OTP → 扫描二维码 → 输入验证码 → 完成

3. **测试 OTP 验证流程**

   - 登录 → 输入验证码 → 完成登录

4. **测试 WebAuthn 设置流程**（需要支持的浏览器和设备）

   - 登录 → 选择 WebAuthn → 完成生物识别注册

5. **测试 WebAuthn 验证流程**

   - 登录 → 点击验证按钮 → 完成生物识别

6. **测试禁用功能**
   - 设置页面 → 点击"禁用二次验证"

## 已知限制

1. **WebAuthn 浏览器兼容性**

   - 需要现代浏览器支持
   - HTTPS 环境更佳（某些浏览器要求）

2. **OTP 时间同步**

   - 需要设备时间准确
   - 可能需要允许时间偏移

3. **单用户系统**
   - 当前仅支持单个管理员账户
   - 多用户需要扩展用户管理系统

## 文档

- 用户使用文档：`/doc/2fa-usage.md`
- 本实现总结：`/doc/2fa-implementation.md`

## 构建状态

✅ 后端编译成功  
✅ 前端构建成功  
✅ TypeScript 类型检查通过  
✅ 无 linter 错误

## 下一步

建议添加：

1. 备份恢复码（recovery codes）
2. 多设备支持
3. 二次验证日志记录
4. 管理员强制启用二次验证
5. 邮件/短信二次验证方式
