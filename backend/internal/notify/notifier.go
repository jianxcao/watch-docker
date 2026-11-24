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
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
)

// Notifier 根据配置发送通知
type Notifier struct {
	cfgLoader  func() *config.Config
	mu         sync.Mutex
	proxyStr   string
	httpClient *http.Client
	transport  *http.Transport
}

// New 创建一个新的 Notifier，使用给定的配置加载器
func New(cfgLoader func() *config.Config) *Notifier {
	if cfgLoader == nil {
		cfgLoader = config.Get
	}
	return &Notifier{
		cfgLoader: cfgLoader,
	}
}

func (n *Notifier) getClient(proxyURL string) *http.Client {
	proxyURL = strings.TrimSpace(proxyURL)

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.httpClient != nil && n.proxyStr == proxyURL {
		return n.httpClient
	}

	if n.transport != nil {
		n.transport.CloseIdleConnections()
	}

	tr := &http.Transport{
		MaxIdleConns:    20,
		IdleConnTimeout: 60 * time.Second,
		Proxy:           http.ProxyFromEnvironment,
	}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			tr.Proxy = http.ProxyURL(parsed)
		}
	}

	n.transport = tr
	n.proxyStr = proxyURL
	n.httpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	return n.httpClient
}

// Send 发送通知
func (n *Notifier) Send(ctx context.Context, title, content, link, image string) error {
	cfg := n.cfgLoader()
	if cfg == nil {
		cfg = config.Get()
	}
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

	client := n.getClient(cfg.Proxy.URL)

	switch method {
	case http.MethodGet:
		return n.sendGet(ctx, client, rawURL, title, content, link, image)
	case http.MethodPost:
		return n.sendPost(ctx, client, rawURL, title, content, link, image)
	default:
		return fmt.Errorf("unsupported notify method: %s", method)
	}
}

func (n *Notifier) sendGet(ctx context.Context, client *http.Client, rawURL, title, content, link, image string) error {
	formattedURL := replacePlaceholders(rawURL, title, content, link, image)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, formattedURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		logger.Logger.Warn("notify get request failed", logger.ZapField("status", resp.StatusCode))
		return fmt.Errorf("notify get request failed: %s", resp.Status)
	}

	return nil
}

func (n *Notifier) sendPost(ctx context.Context, client *http.Client, rawURL, title, content, link, image string) error {
	payload := map[string]string{
		"title":   title,
		"content": content,
		"text":    content,
		"url":     link,
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

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		logger.Logger.Warn("notify post request failed", logger.ZapField("status", resp.StatusCode))
		return fmt.Errorf("notify post request failed: %s", resp.Status)
	}

	return nil
}

func replacePlaceholders(rawURL, title, content, link, image string) string {
	replacements := map[string]string{
		"{title}":   url.QueryEscape(title),
		"{content}": url.QueryEscape(content),
		"{text}":    url.QueryEscape(content),
		"{url}":     url.QueryEscape(link),
		"{image}":   url.QueryEscape(image),
	}

	formatted := rawURL
	for placeholder, value := range replacements {
		formatted = strings.ReplaceAll(formatted, placeholder, value)
	}

	return formatted
}
