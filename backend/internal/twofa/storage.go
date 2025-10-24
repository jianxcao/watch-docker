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

	userCfg, exists := cfg.TwoFAConfig.Users[username]
	if !exists {
		return &UserTwoFAConfig{
			IsSetup: false,
		}, nil
	}

	// 转换配置格式
	result := &UserTwoFAConfig{
		Method:    TwoFAMethod(userCfg.Method),
		OTPSecret: userCfg.OTPSecret,
	}

	// 反序列化 WebAuthn 凭据（从 base64 字符串）
	if len(userCfg.WebAuthnCredentials) > 0 {
		result.WebAuthnCredentials = make([]WebAuthnCredentialWithRPID, 0, len(userCfg.WebAuthnCredentials))
		for _, credStr := range userCfg.WebAuthnCredentials {
			// Base64 解码
			credData, err := base64.StdEncoding.DecodeString(credStr)
			if err != nil {
				return nil, fmt.Errorf("decode base64 credential: %w", err)
			}

			// JSON 反序列化（新格式：包含 RPID）
			var credWithRPID WebAuthnCredentialWithRPID
			if err := json.Unmarshal(credData, &credWithRPID); err != nil {
				continue
			}
			result.WebAuthnCredentials = append(result.WebAuthnCredentials, credWithRPID)
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
		for _, credWithRPID := range userConfig.WebAuthnCredentials {
			// JSON 序列化（包含 RPID）
			data, err := json.Marshal(credWithRPID)
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
	}

	// 初始化 Users map（如果需要）
	if cfg.TwoFAConfig.Users == nil {
		cfg.TwoFAConfig.Users = make(map[string]config.TwoFAUserConfig)
	}

	// 保存到配置
	cfg.TwoFAConfig.Users[username] = configUserCfg

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

// GetUserCredentialsForRPID 获取用户在特定域名下的 WebAuthn 凭据
func GetUserCredentialsForRPID(username, rpid string) ([]webauthn.Credential, error) {
	userConfig, err := GetUserConfig(username)
	if err != nil {
		return nil, err
	}

	var credentials []webauthn.Credential
	for _, credWithRPID := range userConfig.WebAuthnCredentials {
		if credWithRPID.RPID == rpid {
			credentials = append(credentials, credWithRPID.Credential)
		}
	}

	return credentials, nil
}

// IsUserSetupForMethod 检查用户是否为特定方法和域名设置了二次验证
// 对于 OTP：rpid 参数被忽略，检查 OTPSecret 是否非空
// 对于 WebAuthn：检查指定 rpid 是否有凭据
func IsUserSetupForMethod(username string, method TwoFAMethod, rpid string) (bool, error) {
	userConfig, err := GetUserConfig(username)
	if err != nil {
		return false, err
	}

	// 如果用户配置的方法与查询的方法不匹配，返回 false
	if userConfig.Method != method {
		return false, nil
	}

	switch method {
	case MethodOTP:
		// OTP 检查密钥是否非空
		return userConfig.OTPSecret != "", nil
	case MethodWebAuthn:
		// WebAuthn 检查当前域名是否有凭据
		for _, credWithRPID := range userConfig.WebAuthnCredentials {
			if credWithRPID.RPID == rpid {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}
