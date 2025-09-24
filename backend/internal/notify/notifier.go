package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
)

// Notifier 根据配置发送通知
type Notifier struct {
	client *http.Client
}

// New 创建一个新的 Notifier，使用给定的配置加载器
func New(cfgLoader func() *config.Config) *Notifier {
	return &Notifier{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send 发送通知
func (n *Notifier) Send(ctx context.Context, title, content, url, image string) error {
	cfg := config.Get()
	notifyCfg := cfg.Notify

	rawURL := strings.TrimSpace(notifyCfg.URL)
	if rawURL == "" {
		return nil
	}

	rawURL = strings.TrimPrefix(rawURL, "@")

	method := strings.ToUpper(strings.TrimSpace(notifyCfg.Method))
	if method == "" {
		method = http.MethodGet
	}

	switch method {
	case http.MethodGet:
		return n.sendGet(ctx, rawURL, title, content, url, image)
	case http.MethodPost:
		return n.sendPost(ctx, rawURL, title, content, url, image)
	default:
		return fmt.Errorf("unsupported notify method: %s", method)
	}
}

func (n *Notifier) sendGet(ctx context.Context, rawURL, title, content, url, image string) error {
	formattedURL := replacePlaceholders(rawURL, title, content, url, image)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, formattedURL, nil)
	if err != nil {
		return err
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		logger.Logger.Warn("notify get request failed", logger.ZapField("status", resp.StatusCode))
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("notify get request failed: %s", resp.Status)
	}

	io.Copy(io.Discard, resp.Body)
	return nil
}

func (n *Notifier) sendPost(ctx context.Context, rawURL, title, content, url, image string) error {
	payload := map[string]string{
		"title":   title,
		"content": content,
		"url":     url,
		"image":   image,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		logger.Logger.Warn("notify post request failed", logger.ZapField("status", resp.StatusCode))
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("notify post request failed: %s", resp.Status)
	}

	io.Copy(io.Discard, resp.Body)
	return nil
}

func replacePlaceholders(rawURL, title, content, link, image string) string {
	replacements := map[string]string{
		"{title}":   url.QueryEscape(title),
		"{content}": url.QueryEscape(content),
		"{url}":     url.QueryEscape(link),
		"{image}":   url.QueryEscape(image),
	}

	formatted := rawURL
	for placeholder, value := range replacements {
		formatted = strings.ReplaceAll(formatted, placeholder, value)
	}

	return formatted
}
