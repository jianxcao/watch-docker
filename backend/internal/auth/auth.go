package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

var (
	jwtSecret       = []byte("watch-docker-secret-key") // 在生产环境中应该使用更安全的密钥
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

type Claims struct {
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash"` // 密码的哈希值，用于验证密码是否被修改
	TwoFAVerified bool   `json:"twoFAVerified"`
	IsTempToken   bool   `json:"isTempToken"`
	jwt.RegisteredClaims
}

// hashPassword 计算密码的 SHA256 哈希
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// GenerateToken 生成JWT token
func GenerateToken(username string) (string, error) {
	envCfg := conf.EnvCfg
	expirationTime := time.Now().Add(24 * 365 * time.Hour)
	claims := &Claims{
		Username:      username,
		PasswordHash:  hashPassword(envCfg.USER_PASSWORD), // 存储密码哈希
		TwoFAVerified: true,
		IsTempToken:   false,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// GenerateTempToken 生成临时 token（需要二次验证）
func GenerateTempToken(username string) (string, error) {
	envCfg := conf.EnvCfg
	// 临时 token 有效期较短，15分钟
	expirationTime := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		Username:      username,
		PasswordHash:  hashPassword(envCfg.USER_PASSWORD), // 存储密码哈希
		TwoFAVerified: false,
		IsTempToken:   true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// UpgradeTempToken 将临时 token 升级为完整 token
func UpgradeTempToken(tempToken string) (string, error) {
	claims, err := ValidateToken(tempToken)
	if err != nil {
		return "", err
	}

	if !claims.IsTempToken {
		return "", errors.New("not a temp token")
	}

	// 生成完整 token
	return GenerateToken(claims.Username)
}

// ValidateTempToken 验证临时 token
func ValidateTempToken(tokenString string) (*Claims, error) {
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if !claims.IsTempToken {
		return nil, errors.New("not a temp token")
	}

	return claims, nil
}

// ValidateToken 验证JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// AuthMiddleware 授权中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否配置了用户名和密码，如果没有配置则跳过验证
		envCfg := conf.EnvCfg
		if envCfg.USER_NAME == "" || envCfg.USER_PASSWORD == "" {
			c.Next()
			return
		}
		token := c.Query("token")
		if token == "" {
			// 获取 Authorization header
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "需要登录"})
				c.Abort()
				return
			}

			// 检查 Bearer token 格式
			tokenParts := strings.SplitN(authHeader, " ", 2)
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token格式"})
				c.Abort()
				return
			}
			token = tokenParts[1]
		}
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "需要登录"})
			c.Abort()
			return
		}
		// 验证 token
		claims, err := ValidateToken(token)
		if err != nil {
			if errors.Is(err, ErrTokenExpired) {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已过期"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			}
			c.Abort()
			return
		}

		// 检查是否为临时 token - 临时 token 不能访问受保护的资源
		if claims.IsTempToken {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "需要完成二次验证"})
			c.Abort()
			return
		}

		// 验证 token 中的用户名和密码是否与配置的用户名密码匹配
		if claims.Username != envCfg.USER_NAME {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			c.Abort()
			return
		}

		// 验证密码哈希，确保密码未被修改
		currentPasswordHash := hashPassword(envCfg.USER_PASSWORD)
		if claims.PasswordHash != currentPasswordHash {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已失效，请重新登录"})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("username", claims.Username)
		c.Next()
	}
}

// ValidateCredentials 验证用户凭据
func ValidateCredentials(username, password string) bool {
	envCfg := conf.EnvCfg
	return username == envCfg.USER_NAME && password == envCfg.USER_PASSWORD
}

// IsAuthEnabled 检查是否启用了身份验证
func IsAuthEnabled() bool {
	envCfg := conf.EnvCfg
	return envCfg.USER_NAME != "" && envCfg.USER_PASSWORD != ""
}

// TempTokenMiddleware 临时 token 中间件（允许临时 token）
func TempTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否配置了用户名和密码，如果没有配置则跳过验证
		envCfg := conf.EnvCfg
		if envCfg.USER_NAME == "" || envCfg.USER_PASSWORD == "" {
			c.Next()
			return
		}

		token := c.Query("token")
		if token == "" {
			// 获取 Authorization header
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "需要登录"})
				c.Abort()
				return
			}

			// 检查 Bearer token 格式
			tokenParts := strings.SplitN(authHeader, " ", 2)
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token格式"})
				c.Abort()
				return
			}
			token = tokenParts[1]
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "需要登录"})
			c.Abort()
			return
		}

		// 验证 token（允许临时 token）
		claims, err := ValidateToken(token)
		if err != nil {
			if errors.Is(err, ErrTokenExpired) {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已过期"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			}
			c.Abort()
			return
		}

		// 验证 token 中的用户名和密码是否与配置的用户名密码匹配
		if claims.Username != envCfg.USER_NAME {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			c.Abort()
			return
		}

		// 验证密码哈希，确保密码未被修改
		currentPasswordHash := hashPassword(envCfg.USER_PASSWORD)
		if claims.PasswordHash != currentPasswordHash {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已失效，请重新登录"})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("username", claims.Username)
		c.Set("isTempToken", claims.IsTempToken)
		c.Next()
	}
}
