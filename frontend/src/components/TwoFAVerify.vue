<template>
  <n-card title="二次验证" embedded>
    <n-space vertical size="large">
      <n-alert type="info"> 请完成二次验证以继续登录 </n-alert>

      <!-- OTP 验证 -->
      <div v-if="method === 'otp'">
        <n-form-item label="验证码">
          <n-input
            v-model:value="otpCode"
            placeholder="请输入6位验证码"
            :maxlength="6"
            size="large"
            @keydown.enter="handleOTPVerify"
          />
        </n-form-item>

        <n-button
          type="primary"
          block
          size="large"
          @click="handleOTPVerify"
          :loading="verifying"
          :disabled="otpCode.length !== 6"
        >
          验证
        </n-button>
      </div>

      <!-- WebAuthn 验证 -->
      <div v-else-if="method === 'webauthn'">
        <n-alert v-if="!isWebAuthnSupported" type="warning"> 您的浏览器不支持 WebAuthn </n-alert>

        <n-button
          v-else
          type="primary"
          block
          size="large"
          @click="handleWebAuthnVerify"
          :loading="verifying"
        >
          使用生物验证
        </n-button>
      </div>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { startAuthentication } from '@simplewebauthn/browser'
import { twoFAApi } from '@/common/api'

const message = useMessage()

interface Props {
  method: string
}

defineProps<Props>()

// 状态
const verifying = ref(false)
const otpCode = ref('')
const isWebAuthnSupported = ref(false)

const emit = defineEmits<{
  success: [token: string]
}>()

// 验证 OTP
const handleOTPVerify = async () => {
  if (otpCode.value.length !== 6) {
    message.warning('请输入6位验证码')
    return
  }

  verifying.value = true
  try {
    const res = await twoFAApi.verifyOTP(otpCode.value)
    if (res.code === 0) {
      message.success('验证成功')
      emit('success', res.data.token)
    } else {
      message.error(res.msg || '验证失败')
      otpCode.value = '' // 清空输入
    }
  } catch (error: any) {
    message.error(error.message || '验证失败')
    otpCode.value = '' // 清空输入
  } finally {
    verifying.value = false
  }
}

// WebAuthn 验证
const handleWebAuthnVerify = async () => {
  verifying.value = true
  try {
    // 开始验证
    const beginRes = await twoFAApi.webauthnLoginBegin()
    if (beginRes.code !== 0) {
      message.error(beginRes.msg || '开始验证失败')
      return
    }

    const options = beginRes.data.options
    const sessionData = beginRes.data.sessionData

    // 调用浏览器 WebAuthn API
    const asseResp = await startAuthentication({ optionsJSON: options.publicKey })

    // 完成验证
    const finishRes = await twoFAApi.webauthnLoginFinish(sessionData, asseResp)
    if (finishRes.code === 0) {
      message.success('验证成功')
      emit('success', finishRes.data.token)
    } else {
      message.error(finishRes.msg || '验证失败')
    }
  } catch (error: any) {
    console.error('WebAuthn verify error:', error)
    message.error(error.message || 'WebAuthn 验证失败')
  } finally {
    verifying.value = false
  }
}

// 检查 WebAuthn 支持
onMounted(() => {
  isWebAuthnSupported.value = window.PublicKeyCredential !== undefined
})
</script>
