package twofa

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jianxcao/watch-docker/backend/internal/config"
)

// GetUserConfig 获取用户二次验证配置（读取线程安全的深拷贝配置）
func GetUserConfig(username string) (*UserTwoFAConfig, error) {
	cfg := config.Get()

	userCfg, exists := cfg.TwoFAConfig.Users[username]
	if !exists {
		return &UserTwoFAConfig{
			IsSetup: false,
		}, nil
	}

	result := &UserTwoFAConfig{
		Method:    TwoFAMethod(userCfg.Method),
		OTPSecret: userCfg.OTPSecret,
	}

	if len(userCfg.WebAuthnCredentials) > 0 {
		result.WebAuthnCredentials = make([]WebAuthnCredentialWithRPID, 0, len(userCfg.WebAuthnCredentials))
		for _, credStr := range userCfg.WebAuthnCredentials {
			credData, err := base64.StdEncoding.DecodeString(credStr)
			if err != nil {
				return nil, fmt.Errorf("decode base64 credential: %w", err)
			}

			var credWithRPID WebAuthnCredentialWithRPID
			if err := json.Unmarshal(credData, &credWithRPID); err != nil {
				continue
			}
			result.WebAuthnCredentials = append(result.WebAuthnCredentials, credWithRPID)
		}
	}

	switch result.Method {
	case MethodOTP:
		result.IsSetup = result.OTPSecret != ""
	case MethodWebAuthn:
		result.IsSetup = len(result.WebAuthnCredentials) > 0
	default:
		result.IsSetup = false
	}

	return result, nil
}

// SaveUserConfig 线程安全地更新并持久化用户二次验证配置
func SaveUserConfig(username string, userConfig *UserTwoFAConfig) error {
	var credStrings []string
	if len(userConfig.WebAuthnCredentials) > 0 {
		credStrings = make([]string, 0, len(userConfig.WebAuthnCredentials))
		for _, credWithRPID := range userConfig.WebAuthnCredentials {
			data, err := json.Marshal(credWithRPID)
			if err != nil {
				return fmt.Errorf("marshal webauthn credential: %w", err)
			}
			credStrings = append(credStrings, base64.StdEncoding.EncodeToString(data))
		}
	}

	configUserCfg := config.TwoFAUserConfig{
		Method:              string(userConfig.Method),
		OTPSecret:           userConfig.OTPSecret,
		WebAuthnCredentials: credStrings,
	}

	return config.Update(func(cfg *config.Config) error {
		if cfg.TwoFAConfig.Users == nil {
			cfg.TwoFAConfig.Users = make(map[string]config.TwoFAUserConfig)
		}
		if !userConfig.IsSetup && userConfig.Method == "" && userConfig.OTPSecret == "" && len(credStrings) == 0 {
			delete(cfg.TwoFAConfig.Users, username)
		} else {
			cfg.TwoFAConfig.Users[username] = configUserCfg
		}
		return nil
	})
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
func IsUserSetupForMethod(username string, method TwoFAMethod, rpid string) (bool, error) {
	userConfig, err := GetUserConfig(username)
	if err != nil {
		return false, err
	}

	if userConfig.Method != method {
		return false, nil
	}

	switch method {
	case MethodOTP:
		return userConfig.OTPSecret != "", nil
	case MethodWebAuthn:
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
