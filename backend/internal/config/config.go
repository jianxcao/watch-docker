package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// DurationMinutes 表示以分钟为单位的时间间隔，JSON序列化时以分钟数表示
type DurationMinutes time.Duration

// MarshalJSON 将时间间隔序列化为分钟数
func (d DurationMinutes) MarshalJSON() ([]byte, error) {
	minutes := time.Duration(d).Minutes()
	return json.Marshal(minutes)
}

// UnmarshalJSON 从分钟数反序列化时间间隔
func (d *DurationMinutes) UnmarshalJSON(data []byte) error {
	var minutes float64
	if err := json.Unmarshal(data, &minutes); err != nil {
		return err
	}
	*d = DurationMinutes(time.Duration(minutes * float64(time.Minute)))
	return nil
}

// Duration 返回标准 time.Duration
func (d DurationMinutes) Duration() time.Duration {
	return time.Duration(d)
}

// String 返回可读的字符串表示
func (d DurationMinutes) String() string {
	return time.Duration(d).String()
}

// ServerConfig HTTP 服务端口等配置
// addr: 监听地址，例如 ":8080"
type ServerConfig struct {
	Addr string `mapstructure:"addr" json:"addr"`
}

// DockerConfig Docker 连接与容器发现相关配置
// host: Docker API 访问地址（空则走环境变量 DOCKER_HOST / 默认本地）
// includeStopped: 是否包含已停止容器
type DockerConfig struct {
	Host           string `mapstructure:"host" json:"host"`
	IncludeStopped bool   `mapstructure:"includeStopped" json:"includeStopped"`
}

// ScanConfig 扫描相关配置
// interval: 周期扫描间隔（与 cron 二选一）
// cron: 使用 cron 表达式触发扫描
// initialScanOnStart: 进程启动后立即进行一次扫描
// concurrency: 并发获取远端 digest 的 worker 数
// cacheTTL: registry 远端 digest 的缓存 TTL
type ScanConfig struct {
	Cron               string          `mapstructure:"cron" json:"cron"`
	Concurrency        int             `mapstructure:"concurrency" json:"concurrency"`
	CacheTTL           DurationMinutes `mapstructure:"cacheTTL" json:"cacheTTL"`
	IsUpdate           bool            `mapstructure:"isUpdate" json:"isUpdate"`
	AllowComposeUpdate bool            `mapstructure:"allowComposeUpdate" json:"allowComposeUpdate"`
}

// PolicyConfig 策略配置
// skip* 相关开关用于定义默认跳过规则
// onlyLabels 为包含/排除的 label 过滤
// floatingTags 指定哪些 tag 被视为"浮动"，仅这些会被检查更新
type PolicyConfig struct {
	SkipLabels       []string `mapstructure:"skipLabels" json:"skipLabels"`
	OnlyLabels       []string `mapstructure:"onlyLabels" json:"onlyLabels"`
	SkipLocalBuild   bool     `mapstructure:"skipLocalBuild" json:"skipLocalBuild"`
	SkipPinnedDigest bool     `mapstructure:"skipPinnedDigest" json:"skipPinnedDigest"`
	SkipSemverPinned bool     `mapstructure:"skipSemverPinned" json:"skipSemverPinned"`
	FloatingTags     []string `mapstructure:"floatingTags" json:"floatingTags"`
}

// RegistryAuth per-registry 凭据配置
// host: registry 主机地址，支持 "dockerhub"/"docker.io"、"ghcr.io" 或自定义私有仓库
// username: 用户名
// token: 访问令牌或密码
type RegistryAuth struct {
	Host     string `mapstructure:"host" json:"host"`
	Username string `mapstructure:"username" json:"username"`
	Token    string `mapstructure:"token" json:"token"`
}

// RegistryConfig registry 相关配置容器
// auth: 支持配置多个 registry 的凭据
type RegistryConfig struct {
	Auth []RegistryAuth `mapstructure:"auth" json:"auth"`
}

// ProxyConfig 代理相关配置
// url: 代理服务器完整地址，支持以下格式：
//   - HTTP 代理: http://proxy.example.com:8080
//   - 带认证的 HTTP 代理: http://user:pass@proxy.example.com:8080
//   - SOCKS5 代理: socks5://127.0.0.1:1080
//   - 带认证的 SOCKS5 代理: socks5://user:pass@127.0.0.1:1080
type ProxyConfig struct {
	URL string `mapstructure:"url" json:"url"`
}

// LoggingConfig 日志相关配置
// level: 日志级别（debug/info/warn/error）
type LoggingConfig struct {
	Level string `mapstructure:"level" json:"level"`
}

// NotificationConfig 通知相关配置
// url: 通知地址，仅支持配置一个；允许使用占位符 {title}/{content}/{text}
// method: 请求方法，仅允许 GET 或 POST
type NotificationConfig struct {
	URL      string `mapstructure:"url" json:"url"`
	Method   string `mapstructure:"method" json:"method"`
	IsEnable bool   `mapstructure:"isEnable" json:"isEnable"`
}

// ComposeConfig Docker Compose 相关配置
// enabled: 是否启用 Compose 功能
// scanInterval: 扫描间隔(秒)
// logLines: 默认日志行数
type ComposeConfig struct {
	Enabled      bool `mapstructure:"enabled" json:"enabled"`
	ScanInterval int  `mapstructure:"scanInterval" json:"scanInterval"`
	LogLines     int  `mapstructure:"logLines" json:"logLines"`
}

// TwoFAUserConfig 用户二次验证配置
type TwoFAUserConfig struct {
	Method              string   `mapstructure:"method" json:"method"`
	OTPSecret           string   `mapstructure:"otpSecret" json:"otpSecret,omitempty"`
	WebAuthnCredentials []string `mapstructure:"webauthnCredentials" json:"webauthnCredentials,omitempty"` // Base64 编码的凭据
}

// TwoFAConfig 二次验证配置
type TwoFAConfig struct {
	Users map[string]TwoFAUserConfig `mapstructure:"users" json:"users"`
}

// Config 顶层配置聚合
type Config struct {
	Server      ServerConfig       `mapstructure:"server" json:"server"`
	Docker      DockerConfig       `mapstructure:"docker" json:"docker"`
	Scan        ScanConfig         `mapstructure:"scan" json:"scan"`
	Policy      PolicyConfig       `mapstructure:"policy" json:"policy"`
	Registry    RegistryConfig     `mapstructure:"registry" json:"registry"`
	Proxy       ProxyConfig        `mapstructure:"proxy" json:"proxy"`
	Logging     LoggingConfig      `mapstructure:"logging" json:"logging"`
	Notify      NotificationConfig `mapstructure:"notify" json:"notify"`
	Compose     ComposeConfig      `mapstructure:"compose" json:"compose"`
	TwoFAConfig TwoFAConfig        `mapstructure:"twofaConfig" json:"twofaConfig"`
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
	Save()
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
			Cron:               "0 0 */2 * * *",
			Concurrency:        3,
			CacheTTL:           DurationMinutes(10 * time.Minute),
			IsUpdate:           true,
			AllowComposeUpdate: false,
		},
		Policy: PolicyConfig{
			SkipLabels:       []string{"watchdocker.skip=true"},
			OnlyLabels:       []string{},
			SkipLocalBuild:   true,
			SkipPinnedDigest: true,
			SkipSemverPinned: true,
			FloatingTags:     []string{"latest", "main", "stable"},
		},
		Proxy:   ProxyConfig{}, // 默认不使用代理
		Logging: LoggingConfig{Level: "info"},
		Notify:  NotificationConfig{Method: http.MethodGet, IsEnable: true},
		Compose: ComposeConfig{
			Enabled:      true,
			ScanInterval: 30,
			LogLines:     100,
		},
		TwoFAConfig: TwoFAConfig{
			Users: make(map[string]TwoFAUserConfig),
		},
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
	err := v.ReadInConfig()
	if err == nil {
		if err := v.Unmarshal(&cfg); err != nil {
			return nil, fmt.Errorf("unmarshal yaml: %w", err)
		}
	} else {
		fmt.Println("read config file failed", err)
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
	if strings.TrimSpace(cfg.Notify.URL) != "" {
		method := strings.ToUpper(strings.TrimSpace(cfg.Notify.Method))
		switch method {
		case http.MethodGet, http.MethodPost:
			cfg.Notify.Method = method
		case "":
			cfg.Notify.Method = http.MethodGet
		default:
			return fmt.Errorf("notify.method must be GET or POST")
		}
	}
	return nil
}

// Save 将指定配置保存到指定路径的 YAML 文件
func Save() error {
	configPath := path.Join(conf.EnvCfg.CONFIG_PATH, conf.EnvCfg.CONFIG_FILE)
	cfg := Get()
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if err := validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}
