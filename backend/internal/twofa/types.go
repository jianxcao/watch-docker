package twofa

import "github.com/go-webauthn/webauthn/webauthn"

// TwoFAMethod 二次验证方式
type TwoFAMethod string

const (
	MethodOTP      TwoFAMethod = "otp"
	MethodWebAuthn TwoFAMethod = "webauthn"
)

// WebAuthnCredentialWithRPID WebAuthn 凭据及其绑定的域名
type WebAuthnCredentialWithRPID struct {
	Credential webauthn.Credential `json:"credential" yaml:"credential"`
	RPID       string              `json:"rpid" yaml:"rpid"` // 注册时的域名
}

// UserTwoFAConfig 用户二次验证配置
type UserTwoFAConfig struct {
	Method              TwoFAMethod                  `json:"method" yaml:"method"`
	OTPSecret           string                       `json:"otpSecret,omitempty" yaml:"otpSecret,omitempty"` // 所有域名共用
	WebAuthnCredentials []WebAuthnCredentialWithRPID `json:"webauthnCredentials,omitempty" yaml:"webauthnCredentials,omitempty"`
	IsSetup             bool                         `json:"isSetup" yaml:"isSetup"`
}

// TwoFAConfig 二次验证总配置
type TwoFAConfig struct {
	Users map[string]*UserTwoFAConfig `json:"users" yaml:"users"`
}

// WebAuthnUser 实现 webauthn.User 接口
type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID 返回用户ID
func (u *WebAuthnUser) WebAuthnID() []byte {
	return u.ID
}

// WebAuthnName 返回用户名
func (u *WebAuthnUser) WebAuthnName() string {
	return u.Name
}

// WebAuthnDisplayName 返回显示名称
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.DisplayName
}

// WebAuthnCredentials 返回用户的凭据
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// WebAuthnIcon 返回用户图标
func (u *WebAuthnUser) WebAuthnIcon() string {
	return ""
}
