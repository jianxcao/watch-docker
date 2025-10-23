<template>
  <n-card title="设置二次验证" embedded>
    <n-space vertical size="large">
      <!-- 选择验证方式 -->
      <n-form-item label="选择验证方式">
        <n-radio-group v-model:value="selectedMethod">
          <n-space>
            <n-radio value="otp"> OTP (一次性密码) </n-radio>
            <n-radio value="webauthn"> WebAuthn (生物验证) </n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>

      <!-- OTP 设置 -->
      <div v-if="selectedMethod === 'otp'">
        <n-space vertical size="large">
          <n-alert type="info">
            请使用支持 TOTP 的应用（如 Google Authenticator、Authy、微软身份验证器等）扫描下方二维码
          </n-alert>

          <div v-if="loading" class="text-center">
            <n-spin size="large" />
          </div>

          <div v-else-if="otpSecret" class="text-center">
            <!-- 二维码 -->
            <div class="qr-code-container">
              <canvas ref="qrCanvas"></canvas>
            </div>

            <!-- 手动输入密钥 -->
            <n-collapse>
              <n-collapse-item title="手动输入密钥">
                <n-input :value="otpSecret" readonly>
                  <template #suffix>
                    <n-button text @click="copySecret"> 复制 </n-button>
                  </template>
                </n-input>
              </n-collapse-item>
            </n-collapse>

            <!-- 验证码输入 -->
            <n-form-item label="输入验证码" class="mt-4">
              <n-input
                v-model:value="otpCode"
                placeholder="请输入6位验证码"
                :maxlength="6"
                @keydown.enter="handleOTPVerify"
              />
            </n-form-item>
          </div>

          <n-button v-if="!otpSecret" type="primary" @click="handleOTPInit" :loading="loading">
            生成二维码
          </n-button>

          <n-button
            v-else
            type="primary"
            @click="handleOTPVerify"
            :loading="verifying"
            :disabled="otpCode.length !== 6"
          >
            验证并启用
          </n-button>
        </n-space>
      </div>

      <!-- WebAuthn 设置 -->
      <div v-else-if="selectedMethod === 'webauthn'">
        <n-space vertical size="large">
          <n-alert type="info">
            WebAuthn 支持使用指纹、Face ID、Windows Hello 等生物识别方式进行验证
          </n-alert>

          <n-alert v-if="!isWebAuthnSupported" type="warning"> 您的浏览器不支持 WebAuthn </n-alert>

          <n-button v-else type="primary" @click="handleWebAuthnSetup" :loading="verifying">
            开始设置
          </n-button>
        </n-space>
      </div>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useMessage } from 'naive-ui'
import QRCode from 'qrcode'
import { startRegistration } from '@simplewebauthn/browser'
import { twoFAApi } from '@/common/api'

const message = useMessage()

// 选择的验证方式
const selectedMethod = ref<'otp' | 'webauthn'>('otp')

// OTP 相关
const loading = ref(false)
const verifying = ref(false)
const otpSecret = ref('')
const otpCode = ref('')
const qrCanvas = useTemplateRef<HTMLCanvasElement>('qrCanvas')

// WebAuthn 相关
const isWebAuthnSupported = ref(false)

const emit = defineEmits<{
  success: [token: string]
}>()

// 初始化 OTP
const handleOTPInit = async () => {
  loading.value = true
  try {
    const res = await twoFAApi.setupOTPInit()
    if (res.code === 0) {
      otpSecret.value = res.data.secret
      const qrCodeURL = res.data.qrCodeURL
      nextTick(() => {
        if (qrCanvas.value) {
          QRCode.toCanvas(qrCanvas.value, qrCodeURL, {
            width: 256,
            margin: 2,
          })
        }
      })
    } else {
      message.error(res.msg || '初始化失败')
    }
  } catch (error: any) {
    message.error(error.message || '初始化失败')
  } finally {
    loading.value = false
  }
}

// 验证 OTP
const handleOTPVerify = async () => {
  if (otpCode.value.length !== 6) {
    message.warning('请输入6位验证码')
    return
  }

  verifying.value = true
  try {
    const res = await twoFAApi.setupOTPVerify(otpCode.value, otpSecret.value)
    if (res.code === 0) {
      message.success('设置成功')
      emit('success', res.data.token)
    } else {
      message.error(res.msg || '验证失败')
    }
  } catch (error: any) {
    message.error(error.message || '验证失败')
  } finally {
    verifying.value = false
  }
}

// 复制密钥
const copySecret = () => {
  navigator.clipboard.writeText(otpSecret.value)
  message.success('已复制到剪贴板')
}

// WebAuthn 设置
const handleWebAuthnSetup = async () => {
  verifying.value = true
  try {
    // 开始注册
    const beginRes = await twoFAApi.webauthnRegisterBegin()
    if (beginRes.code !== 0) {
      message.error(beginRes.msg || '开始注册失败')
      return
    }

    const options = beginRes.data.options
    const sessionData = beginRes.data.sessionData
    // 调用浏览器 WebAuthn API
    const attResp = await startRegistration({
      optionsJSON: options.publicKey,
    })

    // 完成注册
    const finishRes = await twoFAApi.webauthnRegisterFinish(sessionData, attResp)
    if (finishRes.code === 0) {
      message.success('设置成功')
      emit('success', finishRes.data.token)
    } else {
      message.error(finishRes.msg || '完成注册失败')
    }
  } catch (error: any) {
    console.error('WebAuthn setup error:', error)
    message.error(error.message || 'WebAuthn 设置失败')
  } finally {
    verifying.value = false
  }
}

// 检查 WebAuthn 支持
onMounted(() => {
  isWebAuthnSupported.value = window.PublicKeyCredential !== undefined
})

// 当方法改变时，重置状态
watch(selectedMethod, () => {
  otpSecret.value = ''
  otpCode.value = ''
  loading.value = false
  verifying.value = false
})
</script>

<style scoped>
.qr-code-container {
  display: flex;
  justify-content: center;
  margin: 20px 0;
}

.text-center {
  text-align: center;
}

.mt-4 {
  margin-top: 16px;
}
</style>
