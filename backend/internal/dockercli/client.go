package dockercli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
)

type Client struct {
	docker       *client.Client
	statsManager *StatsManager
}

func New(ctx context.Context, host string) (*Client, error) {
	return NewWithStatsConfig(ctx, host, StatsManagerConfig{})
}

// NewWithStatsConfig 使用自定义统计配置创建Docker客户端
func NewWithStatsConfig(ctx context.Context, host string, statsConfig StatsManagerConfig) (*Client, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if strings.TrimSpace(host) != "" {
		opts = append(opts, client.WithHost(host))
	}

	// 检查是否配置了代理
	cfg := config.Get()
	if cfg.Proxy.URL != "" {
		httpClient, err := createHTTPClientWithProxy(cfg.Proxy.URL)
		if err != nil {
			logger.Logger.Error("创建代理HTTP客户端失败", zap.String("proxy", cfg.Proxy.URL), logger.ZapErr(err))
			return nil, fmt.Errorf("创建代理HTTP客户端失败: %w", err)
		}
		opts = append(opts, client.WithHTTPClient(httpClient))
		logger.Logger.Info("Docker客户端使用代理", zap.String("proxy", cfg.Proxy.URL))
	}

	dockerClient, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	// 创建统计管理器
	var statsManager *StatsManager
	if statsConfig.MaxConcurrency > 0 || statsConfig.CollectTimeout > 0 {
		statsManager = NewStatsManagerWithConfig(dockerClient, statsConfig)
	} else {
		statsManager = NewStatsManager(dockerClient)
	}

	clientInstance := &Client{
		docker:       dockerClient,
		statsManager: statsManager,
	}
	return clientInstance, nil
}

// createHTTPClientWithProxy 创建带代理的HTTP客户端
// 支持 HTTP、HTTPS 和 SOCKS5 代理
func createHTTPClientWithProxy(proxyURL string) (*http.Client, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("解析代理URL失败: %w", err)
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	switch parsedURL.Scheme {
	case "http", "https":
		// HTTP/HTTPS 代理
		transport.Proxy = http.ProxyURL(parsedURL)

	case "socks5":
		// SOCKS5 代理
		var auth *proxy.Auth
		if parsedURL.User != nil {
			password, _ := parsedURL.User.Password()
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("创建SOCKS5代理失败: %w", err)
		}

		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}

	default:
		return nil, fmt.Errorf("不支持的代理类型: %s", parsedURL.Scheme)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Minute * 5,
	}, nil
}

func (c *Client) Close() error {
	c.statsManager.StopMonitoring()
	return c.docker.Close()
}

// GetDockerClient 返回底层的 Docker 客户端，供其他模块使用
func (c *Client) GetDockerClient() *client.Client {
	return c.docker
}

// AddStatsConnection 添加统计 WebSocket 连接
func (c *Client) AddStatsConnection(ctx context.Context) {
	c.statsManager.AddConnection(ctx)
}

// RemoveStatsConnection 移除统计 WebSocket 连接
func (c *Client) RemoveStatsConnection() {
	c.statsManager.RemoveConnection()
}

// GetContainerStats 获取容器统计信息
func (c *Client) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	return c.statsManager.GetContainerStats(ctx, id), nil
}

// GetContainersStats 获取多个容器统计信息
func (c *Client) GetContainersStats(ctx context.Context, containerIDs []string) (map[string]*ContainerStats, error) {
	return c.statsManager.GetContainersStats(ctx, containerIDs)
}

// GetVersion 获取Docker版本信息
func (c *Client) GetVersion(ctx context.Context) (types.Version, error) {
	return c.docker.ServerVersion(ctx)
}
