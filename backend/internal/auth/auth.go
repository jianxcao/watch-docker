package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

var (
	jwtSecret       []byte
	jwtSecretOnce   sync.Once
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

func initJWTSecret() {
	if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
		if len(envSecret) < 32 {
			fmt.Println("警告: JWT_SECRET 长度建议至少 32 字节以保证签名强度")
		}
		jwtSecret = []byte(envSecret)
		return
	}

	secretFile := filepath.Join(conf.EnvCfg.CONFIG_PATH, ".jwt_secret")
	if data, err := os.ReadFile(secretFile); err == nil && len(data) >= 32 {
		jwtSecret = data
		return
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(fmt.Sprintf("failed to generate JWT secret: %v", err))
	}
	jwtSecret = secret

	if err := os.MkdirAll(conf.EnvCfg.CONFIG_PATH, 0700); err != nil {
		fmt.Printf("警告: 无法创建 JWT 密钥目录 %s: %v\n", conf.EnvCfg.CONFIG_PATH, err)
		return
	}
	if err := os.WriteFile(secretFile, secret, 0600); err != nil {
		fmt.Printf("警告: 无法持久化 JWT 密钥文件 %s: %v\n", secretFile, err)
	}
}

func getJWTSecret() []byte {
	jwtSecretOnce.Do(initJWTSecret)
	return jwtSecret
}

type Claims struct {
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash"`
	TwoFAVerified bool   `json:"twoFAVerified"`
	IsTempToken   bool   `json:"isTempToken"`
	jwt.RegisteredClaims
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func GenerateToken(username string) (string, error) {
	envCfg := conf.EnvCfg
	expirationTime := time.Now().Add(30 * 24 * time.Hour)
	claims := &Claims{
		Username:      username,
		PasswordHash:  hashPassword(envCfg.USER_PASSWORD),
		TwoFAVerified: true,
		IsTempToken:   false,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

func GenerateTempToken(username string) (string, error) {
	envCfg := conf.EnvCfg
	expirationTime := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		Username:      username,
		PasswordHash:  hashPassword(envCfg.USER_PASSWORD),
		TwoFAVerified: false,
		IsTempToken:   true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

func UpgradeTempToken(tempToken string) (string, error) {
	claims, err := ValidateTempToken(tempToken)
	if err != nil {
		return "", err
	}
	return GenerateToken(claims.Username)
}

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

func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getJWTSecret(), nil
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

// ExtractTokenFromRequest 从 query 参数 (?token=) 或 Authorization Bearer 头安全提取 JWT
func ExtractTokenFromRequest(c *gin.Context) (string, error) {
	if token := strings.TrimSpace(c.Query("token")); token != "" {
		return token, nil
	}
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", errors.New("需要登录")
	}
	tokenParts := strings.SplitN(authHeader, " ", 2)
	if len(tokenParts) != 2 || !strings.EqualFold(tokenParts[0], "Bearer") || strings.TrimSpace(tokenParts[1]) == "" {
		return "", errors.New("无效的token格式")
	}
	return strings.TrimSpace(tokenParts[1]), nil
}

// ValidateFullAccessClaims 校验完整访问令牌（禁止 tempToken、校验用户名与密码哈希）
func ValidateFullAccessClaims(tokenString string) (*Claims, error) {
	envCfg := conf.EnvCfg
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.IsTempToken {
		return nil, errors.New("需要完成二次验证")
	}
	if claims.Username != envCfg.USER_NAME {
		return nil, ErrInvalidToken
	}
	if claims.PasswordHash != hashPassword(envCfg.USER_PASSWORD) {
		return nil, errors.New("token已失效，请重新登录")
	}
	return claims, nil
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		envCfg := conf.EnvCfg
		if envCfg.USER_NAME == "" || envCfg.USER_PASSWORD == "" {
			c.Next()
			return
		}
		token, err := ExtractTokenFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
			c.Abort()
			return
		}

		claims, err := ValidateFullAccessClaims(token)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, ErrTokenExpired) {
				msg = "token已过期"
			} else if errors.Is(err, ErrInvalidToken) {
				msg = "无效的token"
			}
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": msg})
			c.Abort()
			return
		}

		c.Set("username", claims.Username)
		c.Set("isTempToken", false)
		c.Next()
	}
}

func ValidateCredentials(username, password string) bool {
	envCfg := conf.EnvCfg
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(envCfg.USER_NAME)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(envCfg.USER_PASSWORD)) == 1
	return usernameMatch && passwordMatch
}

func IsAuthEnabled() bool {
	envCfg := conf.EnvCfg
	return envCfg.USER_NAME != "" && envCfg.USER_PASSWORD != ""
}

func TempTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		envCfg := conf.EnvCfg
		if envCfg.USER_NAME == "" || envCfg.USER_PASSWORD == "" {
			c.Next()
			return
		}

		token, err := ExtractTokenFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
			c.Abort()
			return
		}

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

		if claims.Username != envCfg.USER_NAME {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "无效的token"})
			c.Abort()
			return
		}

		currentPasswordHash := hashPassword(envCfg.USER_PASSWORD)
		if claims.PasswordHash != currentPasswordHash {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token已失效，请重新登录"})
			c.Abort()
			return
		}

		c.Set("username", claims.Username)
		c.Set("isTempToken", claims.IsTempToken)
		c.Set("rawToken", token)
		c.Next()
	}
}
