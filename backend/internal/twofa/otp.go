package twofa

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"

	"github.com/pquerna/otp/totp"
)

// GenerateOTPSecret 生成 OTP 密钥
func GenerateOTPSecret() (string, error) {
	// 生成 20 字节随机密钥
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}

	// Base32 编码
	return base32.StdEncoding.EncodeToString(secret), nil
}

// GenerateQRCodeURL 生成二维码 URL
func GenerateQRCodeURL(secret, username, issuer string) (string, error) {
	// secret 是 base32 编码的字符串，需要先解码
	secretBytes, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("decode base32 secret: %w", err)
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
		Secret:      secretBytes,
	})
	if err != nil {
		return "", fmt.Errorf("generate totp key: %w", err)
	}

	return key.URL(), nil
}

// ValidateOTPCode 验证 OTP 代码
func ValidateOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}
