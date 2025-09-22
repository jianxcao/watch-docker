package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/viper"
)

// ServerConfig HTTP 服务端口等配置
// addr: 监听地址，例如 ":8080"
type ServerConfig struct {
	Addr string `mapstructure:"addr" envconfig:"SERVER_ADDR"`
}

// DockerConfig Docker 连接与容器发现相关配置
// host: Docker API 访问地址（空则走环境变量 DOCKER_HOST / 默认本地）
// includeStopped: 是否包含已停止容器
type DockerConfig struct {
	Host           string `mapstructure:"host" envconfig:"DOCKER_HOST"`
	IncludeStopped bool   `mapstructure:"includeStopped" envconfig:"DOCKER_INCLUDE_STOPPED"`
}

// ScanConfig 扫描相关配置
// interval: 周期扫描间隔（与 cron 二选一）
// cron: 使用 cron 表达式触发扫描
// initialScanOnStart: 进程启动后立即进行一次扫描
// concurrency: 并发获取远端 digest 的 worker 数
// cacheTTL: registry 远端 digest 的缓存 TTL
type ScanConfig struct {
	Interval           time.Duration `mapstructure:"interval" envconfig:"SCAN_INTERVAL"`
	Cron               string        `mapstructure:"cron" envconfig:"SCAN_CRON"`
	InitialScanOnStart bool          `mapstructure:"initialScanOnStart" envconfig:"SCAN_INITIAL_ON_START"`
	Concurrency        int           `mapstructure:"concurrency" envconfig:"SCAN_CONCURRENCY"`
	CacheTTL           time.Duration `mapstructure:"cacheTTL" envconfig:"SCAN_CACHE_TTL"`
}

// UpdateConfig 自动更新相关配置
// enabled: 是否允许自动更新
// autoUpdateCron: 自动更新任务的 cron 表达式
// allowComposeUpdate: 是否允许更新由 Compose 管理的容器
// recreateStrategy: 更新策略（recreate/rolling）
// removeOldContainer: 更新成功后是否删除旧容器
type UpdateConfig struct {
	Enabled            bool   `mapstructure:"enabled" envconfig:"UPDATE_ENABLED"`
	AutoUpdateCron     string `mapstructure:"autoUpdateCron" envconfig:"UPDATE_CRON"`
	AllowComposeUpdate bool   `mapstructure:"allowComposeUpdate" envconfig:"UPDATE_ALLOW_COMPOSE"`
	RecreateStrategy   string `mapstructure:"recreateStrategy" envconfig:"UPDATE_STRATEGY"`
	RemoveOldContainer bool   `mapstructure:"removeOldContainer" envconfig:"UPDATE_REMOVE_OLD"`
}

// PolicyConfig 策略配置
// skip* 相关开关用于定义默认跳过规则
// onlyLabels/excludeLabels 为包含/排除的 label 过滤
// floatingTags 指定哪些 tag 被视为“浮动”，仅这些会被检查更新
type PolicyConfig struct {
	SkipLabels       []string `mapstructure:"skipLabels"`
	OnlyLabels       []string `mapstructure:"onlyLabels"`
	ExcludeLabels    []string `mapstructure:"excludeLabels"`
	SkipLocalBuild   bool     `mapstructure:"skipLocalBuild"`
	SkipPinnedDigest bool     `mapstructure:"skipPinnedDigest"`
	SkipSemverPinned bool     `mapstructure:"skipSemverPinned"`
	FloatingTags     []string `mapstructure:"floatingTags"`
}

// RegistryAuth per-registry 凭据配置
// host/username/password: 访问私有仓库需要的账号密码（如 ghcr.io）
type RegistryAuth struct {
	Host     string `mapstructure:"host" envconfig:"HOST"`
	Username string `mapstructure:"username" envconfig:"USERNAME"`
	Password string `mapstructure:"password" envconfig:"PASSWORD"`
}

// RegistryConfig registry 相关配置容器
// auth: 支持配置多个 registry 的凭据
type RegistryConfig struct {
	Auth []RegistryAuth `mapstructure:"auth"`
}

// LoggingConfig 日志相关配置
// level: 日志级别（debug/info/warn/error）
type LoggingConfig struct {
	Level string `mapstructure:"level" envconfig:"LOG_LEVEL"`
}

// Config 顶层配置聚合
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Docker   DockerConfig   `mapstructure:"docker"`
	Scan     ScanConfig     `mapstructure:"scan"`
	Update   UpdateConfig   `mapstructure:"update"`
	Policy   PolicyConfig   `mapstructure:"policy"`
	Registry RegistryConfig `mapstructure:"registry"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

var (
	globalCfg *Config
	globalMu  sync.RWMutex
)

// SetGlobal 设置进程内的全局配置（供动态读取）。
func SetGlobal(c *Config) {
	globalMu.Lock()
	globalCfg = c
	globalMu.Unlock()
}

// Get 返回当前全局配置的快照；若未设置则返回默认值，避免空指针。
func Get() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalCfg == nil {
		// fall back to defaults to avoid nil deref
		return defaults()
	}
	return globalCfg
}

// Loader 返回一个闭包，用于按需获取最新全局配置。
func Loader() func() *Config { return Get }

// defaults 返回默认配置（当未提供配置文件/环境变量时使用）。
func defaults() *Config {
	return &Config{
		Server: ServerConfig{Addr: ":8080"},
		Scan: ScanConfig{
			Interval:           10 * time.Minute,
			Cron:               "",
			InitialScanOnStart: true,
			Concurrency:        3,
			CacheTTL:           5 * time.Minute,
		},
		Update: UpdateConfig{
			Enabled:            true,
			AutoUpdateCron:     "",
			AllowComposeUpdate: false,
			RecreateStrategy:   "recreate",
			RemoveOldContainer: true,
		},
		Policy: PolicyConfig{
			SkipLabels:       []string{"watchdocker.skip=true"},
			OnlyLabels:       []string{},
			ExcludeLabels:    []string{},
			SkipLocalBuild:   true,
			SkipPinnedDigest: true,
			SkipSemverPinned: true,
			FloatingTags:     []string{"latest", "main", "stable"},
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

// Load 读取配置文件（YAML）并应用 ENV 覆盖，校验后设置为全局配置。
// 覆盖顺序：defaults < YAML config < ENV（WATCH_*）。
func Load(path string) (*Config, error) {
	cfg := defaults()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("WATCH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err == nil {
		if err := v.Unmarshal(&cfg); err != nil {
			return nil, fmt.Errorf("unmarshal yaml: %w", err)
		}
	}

	// ENV 覆盖（结构体嵌套使用前缀 WATCH_）
	if err := envconfig.Process("WATCH", cfg); err != nil {
		return nil, fmt.Errorf("envconfig: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	SetGlobal(cfg)
	return cfg, nil
}

// validate 校验关键字段，提前发现配置错误。
func validate(cfg *Config) error {
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if cfg.Scan.Concurrency <= 0 {
		return fmt.Errorf("scan.concurrency must be > 0")
	}
	switch cfg.Update.RecreateStrategy {
	case "recreate", "rolling":
	default:
		return fmt.Errorf("update.recreateStrategy must be one of: recreate, rolling")
	}
	return nil
}
