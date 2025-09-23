package auth

import (
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
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func GenerateToken(username string) (string, error) {
	expirationTime := time.Now().Add(24 * 365 * time.Hour)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
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

		// 验证 token
		claims, err := ValidateToken(tokenParts[1])
		if err != nil {
			if errors.Is(err, ErrTokenExpired) {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已过期"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			}
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
