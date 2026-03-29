package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DownloadToken 下载令牌信息
type DownloadToken struct {
	Token       string    // 令牌
	ContainerID string    // 容器ID
	FilePath    string    // 文件路径
	Username    string    // 用户名
	CreatedAt   time.Time // 创建时间
	ExpiresAt   time.Time // 过期时间
	Used        bool      // 是否已使用
}

// DownloadTokenManager 下载令牌管理器
type DownloadTokenManager struct {
	tokens map[string]*DownloadToken
	mu     sync.RWMutex
}

var downloadTokenManager = &DownloadTokenManager{
	tokens: make(map[string]*DownloadToken),
}

// GetDownloadTokenManager 获取下载令牌管理器实例
func GetDownloadTokenManager() *DownloadTokenManager {
	return downloadTokenManager
}

// generateRandomToken 生成随机令牌
func generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateDownloadToken 生成下载令牌
func (m *DownloadTokenManager) GenerateDownloadToken(containerID, filePath, username string, ttl time.Duration) (string, error) {
	token, err := generateRandomToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	downloadToken := &DownloadToken{
		Token:       token,
		ContainerID: containerID,
		FilePath:    filePath,
		Username:    username,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
		Used:        false,
	}

	m.mu.Lock()
	m.tokens[token] = downloadToken
	m.mu.Unlock()

	// 启动定时清理
	go m.cleanupExpiredToken(token, ttl)

	return token, nil
}

// ValidateDownloadToken 验证下载令牌
func (m *DownloadTokenManager) ValidateDownloadToken(token, containerID, filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	downloadToken, exists := m.tokens[token]
	if !exists {
		return fmt.Errorf("invalid token")
	}

	// 检查是否已过期
	if time.Now().After(downloadToken.ExpiresAt) {
		delete(m.tokens, token)
		return fmt.Errorf("token expired")
	}

	// 检查是否已使用
	if downloadToken.Used {
		return fmt.Errorf("token already used")
	}

	// 验证容器ID和文件路径
	if downloadToken.ContainerID != containerID || downloadToken.FilePath != filePath {
		return fmt.Errorf("token mismatch")
	}

	return nil
}

// MarkTokenUsed 标记令牌已使用（使令牌失效）
func (m *DownloadTokenManager) MarkTokenUsed(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if downloadToken, exists := m.tokens[token]; exists {
		downloadToken.Used = true
		// 立即删除已使用的令牌
		delete(m.tokens, token)
	}
}

// cleanupExpiredToken 清理过期令牌
func (m *DownloadTokenManager) cleanupExpiredToken(token string, ttl time.Duration) {
	time.Sleep(ttl + 5*time.Second) // 多等待5秒确保过期

	m.mu.Lock()
	defer m.mu.Unlock()

	if downloadToken, exists := m.tokens[token]; exists {
		if time.Now().After(downloadToken.ExpiresAt) {
			delete(m.tokens, token)
		}
	}
}

// CleanupAll 清理所有过期令牌（定期调用）
func (m *DownloadTokenManager) CleanupAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, downloadToken := range m.tokens {
		if now.After(downloadToken.ExpiresAt) || downloadToken.Used {
			delete(m.tokens, token)
		}
	}
}
