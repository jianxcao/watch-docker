package twofa

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jianxcao/watch-docker/backend/internal/config"
)

// GetUserConfig 获取用户二次验证配置
func GetUserConfig(username string) (*UserTwoFAConfig, error) {
	cfg := config.Get()

	userCfg, exists := cfg.TwoFA.Users[username]
	if !exists {
		return &UserTwoFAConfig{
			IsSetup: false,
		}, nil
	}

	// 转换配置格式
	result := &UserTwoFAConfig{
		Method:    TwoFAMethod(userCfg.Method),
		OTPSecret: userCfg.OTPSecret,
		IsSetup:   userCfg.IsSetup,
	}

	// 反序列化 WebAuthn 凭据（从 base64 字符串）
	if len(userCfg.WebAuthnCredentials) > 0 {
		result.WebAuthnCredentials = make([]webauthn.Credential, 0, len(userCfg.WebAuthnCredentials))
		for _, credStr := range userCfg.WebAuthnCredentials {
			// Base64 解码
			credData, err := base64.StdEncoding.DecodeString(credStr)
			if err != nil {
				return nil, fmt.Errorf("decode base64 credential: %w", err)
			}

			// JSON 反序列化
			var cred webauthn.Credential
			if err := json.Unmarshal(credData, &cred); err != nil {
				return nil, fmt.Errorf("unmarshal webauthn credential: %w", err)
			}
			result.WebAuthnCredentials = append(result.WebAuthnCredentials, cred)
		}
	}

	return result, nil
}

// SaveUserConfig 保存用户二次验证配置
func SaveUserConfig(username string, userConfig *UserTwoFAConfig) error {
	cfg := config.Get()

	// 序列化 WebAuthn 凭据（转为 base64 字符串）
	var credStrings []string
	if len(userConfig.WebAuthnCredentials) > 0 {
		credStrings = make([]string, 0, len(userConfig.WebAuthnCredentials))
		for _, cred := range userConfig.WebAuthnCredentials {
			// JSON 序列化
			data, err := json.Marshal(cred)
			if err != nil {
				return fmt.Errorf("marshal webauthn credential: %w", err)
			}
			// Base64 编码
			credStr := base64.StdEncoding.EncodeToString(data)
			credStrings = append(credStrings, credStr)
		}
	}

	// 构建配置
	configUserCfg := config.TwoFAUserConfig{
		Method:              string(userConfig.Method),
		OTPSecret:           userConfig.OTPSecret,
		WebAuthnCredentials: credStrings,
		IsSetup:             userConfig.IsSetup,
	}

	// 初始化 Users map（如果需要）
	if cfg.TwoFA.Users == nil {
		cfg.TwoFA.Users = make(map[string]config.TwoFAUserConfig)
	}

	// 保存到配置
	cfg.TwoFA.Users[username] = configUserCfg

	// 保存配置文件
	config.SetGlobal(cfg)

	return nil
}

// IsUserSetup 检查用户是否已设置二次验证
func IsUserSetup(username string) (bool, error) {
	userConfig, err := GetUserConfig(username)
	if err != nil {
		return false, err
	}

	return userConfig.IsSetup, nil
}
