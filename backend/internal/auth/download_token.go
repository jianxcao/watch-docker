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
	Token       string
	ContainerID string
	FilePath    string
	Username    string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// DownloadTokenManager 下载令牌管理器
type DownloadTokenManager struct {
	tokens map[string]*DownloadToken
	mu     sync.Mutex
}

var downloadTokenManager = &DownloadTokenManager{
	tokens: make(map[string]*DownloadToken),
}

// GetDownloadTokenManager 获取下载令牌管理器实例
func GetDownloadTokenManager() *DownloadTokenManager {
	return downloadTokenManager
}

func generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateDownloadToken 生成下载令牌（惰性清理过期条目，避免每次请求泄漏睡眠 goroutine）
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
	}

	m.mu.Lock()
	for k, existing := range m.tokens {
		if now.After(existing.ExpiresAt) {
			delete(m.tokens, k)
		}
	}
	m.tokens[token] = downloadToken
	m.mu.Unlock()

	return token, nil
}

// ConsumeDownloadToken 在单锁下原子校验并立即销毁下载令牌（用后即焚，消除并发重放 TOCTOU）
func (m *DownloadTokenManager) ConsumeDownloadToken(token, containerID, filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	downloadToken, exists := m.tokens[token]
	if !exists {
		return fmt.Errorf("invalid or already used token")
	}

	if time.Now().After(downloadToken.ExpiresAt) {
		delete(m.tokens, token)
		return fmt.Errorf("token expired")
	}

	if downloadToken.ContainerID != containerID || downloadToken.FilePath != filePath {
		return fmt.Errorf("token mismatch")
	}

	delete(m.tokens, token)
	return nil
}

// CleanupAll 清理所有过期令牌
func (m *DownloadTokenManager) CleanupAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, downloadToken := range m.tokens {
		if now.After(downloadToken.ExpiresAt) {
			delete(m.tokens, token)
		}
	}
}
