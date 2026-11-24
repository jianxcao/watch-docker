package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const MaskedSecretPlaceholder = "******"

// DurationMinutes 表示以分钟为单位的时间间隔，JSON序列化时以分钟数表示
type DurationMinutes time.Duration

func (d DurationMinutes) MarshalJSON() ([]byte, error) {
	minutes := time.Duration(d).Minutes()
	return json.Marshal(minutes)
}

func (d *DurationMinutes) UnmarshalJSON(data []byte) error {
	var minutes float64
	if err := json.Unmarshal(data, &minutes); err != nil {
		return err
	}
	*d = DurationMinutes(time.Duration(minutes * float64(time.Minute)))
	return nil
}

func (d DurationMinutes) Duration() time.Duration {
	return time.Duration(d)
}

func (d DurationMinutes) String() string {
	return time.Duration(d).String()
}

type ServerConfig struct {
	Addr string `mapstructure:"addr" json:"addr"`
}

type DockerConfig struct {
	Host           string `mapstructure:"host" json:"host"`
	IncludeStopped bool   `mapstructure:"includeStopped" json:"includeStopped"`
}

type ScanConfig struct {
	Cron               string          `mapstructure:"cron" json:"cron"`
	Concurrency        int             `mapstructure:"concurrency" json:"concurrency"`
	CacheTTL           DurationMinutes `mapstructure:"cacheTTL" json:"cacheTTL"`
	IsUpdate           bool            `mapstructure:"isUpdate" json:"isUpdate"`
	AllowComposeUpdate bool            `mapstructure:"allowComposeUpdate" json:"allowComposeUpdate"`
}

type PolicyConfig struct {
	SkipLabels       []string `mapstructure:"skipLabels" json:"skipLabels"`
	OnlyLabels       []string `mapstructure:"onlyLabels" json:"onlyLabels"`
	SkipLocalBuild   bool     `mapstructure:"skipLocalBuild" json:"skipLocalBuild"`
	SkipPinnedDigest bool     `mapstructure:"skipPinnedDigest" json:"skipPinnedDigest"`
	SkipSemverPinned bool     `mapstructure:"skipSemverPinned" json:"skipSemverPinned"`
	FloatingTags     []string `mapstructure:"floatingTags" json:"floatingTags"`
}

type RegistryAuth struct {
	Host     string `mapstructure:"host" json:"host"`
	Username string `mapstructure:"username" json:"username"`
	Token    string `mapstructure:"token" json:"token"`
}

type RegistryMirror struct {
	Name    string `mapstructure:"name" json:"name"`
	URL     string `mapstructure:"url" json:"url"`
	Enabled bool   `mapstructure:"enabled" json:"enabled"`
}

type RegistryConfig struct {
	Auth    []RegistryAuth   `mapstructure:"auth" json:"auth"`
	Mirrors []RegistryMirror `mapstructure:"mirrors" json:"mirrors"`
}

type ProxyConfig struct {
	URL string `mapstructure:"url" json:"url"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level" json:"level"`
}

type NotificationConfig struct {
	URL      string `mapstructure:"url" json:"url"`
	Method   string `mapstructure:"method" json:"method"`
	IsEnable bool   `mapstructure:"isEnable" json:"isEnable"`
}

type ComposeConfig struct {
	Enabled      bool `mapstructure:"enabled" json:"enabled"`
	ScanInterval int  `mapstructure:"scanInterval" json:"scanInterval"`
	LogLines     int  `mapstructure:"logLines" json:"logLines"`
}

type TwoFAUserConfig struct {
	Method              string   `mapstructure:"method" json:"method"`
	OTPSecret           string   `mapstructure:"otpSecret" json:"otpSecret,omitempty"`
	WebAuthnCredentials []string `mapstructure:"webauthnCredentials" json:"webauthnCredentials,omitempty"`
}

type TwoFAConfig struct {
	Users map[string]TwoFAUserConfig `mapstructure:"users" json:"users"`
}

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

// Clone 返回配置的深拷贝，保证切片与 map 互相隔离，消除并发读写竞态。
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Policy.SkipLabels = append([]string(nil), c.Policy.SkipLabels...)
	cp.Policy.OnlyLabels = append([]string(nil), c.Policy.OnlyLabels...)
	cp.Policy.FloatingTags = append([]string(nil), c.Policy.FloatingTags...)
	cp.Registry.Auth = append([]RegistryAuth(nil), c.Registry.Auth...)
	cp.Registry.Mirrors = append([]RegistryMirror(nil), c.Registry.Mirrors...)
	if c.TwoFAConfig.Users != nil {
		cp.TwoFAConfig.Users = make(map[string]TwoFAUserConfig, len(c.TwoFAConfig.Users))
		for k, u := range c.TwoFAConfig.Users {
			uCopy := u
			uCopy.WebAuthnCredentials = append([]string(nil), u.WebAuthnCredentials...)
			cp.TwoFAConfig.Users[k] = uCopy
		}
	} else {
		cp.TwoFAConfig.Users = make(map[string]TwoFAUserConfig)
	}
	return &cp
}

// MaskedCopy 返回用于对外展示的安全脱敏配置副本（隐藏密钥与 2FA 内部凭据）。
func (c *Config) MaskedCopy() *Config {
	cp := c.Clone()
	if cp == nil {
		return nil
	}
	for i := range cp.Registry.Auth {
		if cp.Registry.Auth[i].Token != "" {
			cp.Registry.Auth[i].Token = MaskedSecretPlaceholder
		}
	}
	if strings.TrimSpace(cp.Proxy.URL) != "" {
		if u, err := url.Parse(cp.Proxy.URL); err == nil && u.User != nil {
			if _, hasPass := u.User.Password(); hasPass {
				u.User = url.UserPassword(u.User.Username(), MaskedSecretPlaceholder)
				cp.Proxy.URL = u.String()
			}
		}
	}
	// 不向前端暴露任何用户的 OTP Secret 和 WebAuthn 凭据
	cp.TwoFAConfig = TwoFAConfig{Users: map[string]TwoFAUserConfig{}}
	return cp
}

// MergeMaskedSecrets 在保存来自 API 的配置前，恢复脱敏占位符、保留服务端只读字段并规范化空切片。
func MergeMaskedSecrets(next, prev *Config) {
	if next == nil || prev == nil {
		return
	}
	next.Docker.Host = prev.Docker.Host
	next.TwoFAConfig = prev.Clone().TwoFAConfig
	if next.TwoFAConfig.Users == nil {
		next.TwoFAConfig.Users = make(map[string]TwoFAUserConfig)
	}
	if next.Policy.SkipLabels == nil {
		next.Policy.SkipLabels = []string{}
	}
	if next.Policy.OnlyLabels == nil {
		next.Policy.OnlyLabels = []string{}
	}
	if next.Policy.FloatingTags == nil {
		next.Policy.FloatingTags = []string{}
	}
	if next.Registry.Auth == nil {
		next.Registry.Auth = []RegistryAuth{}
	}
	if next.Registry.Mirrors == nil {
		next.Registry.Mirrors = []RegistryMirror{}
	}

	for i := range next.Registry.Auth {
		if next.Registry.Auth[i].Token != MaskedSecretPlaceholder {
			continue
		}
		restored := ""
		for _, oldAuth := range prev.Registry.Auth {
			if oldAuth.Host == next.Registry.Auth[i].Host && oldAuth.Username == next.Registry.Auth[i].Username {
				restored = oldAuth.Token
				break
			}
		}
		if restored == "" && i < len(prev.Registry.Auth) && prev.Registry.Auth[i].Host == next.Registry.Auth[i].Host {
			restored = prev.Registry.Auth[i].Token
		}
		next.Registry.Auth[i].Token = restored
	}

	if strings.TrimSpace(next.Proxy.URL) != "" {
		if u, err := url.Parse(next.Proxy.URL); err == nil && u.User != nil {
			if pass, hasPass := u.User.Password(); hasPass && pass == MaskedSecretPlaceholder {
				restoredPass := ""
				if strings.TrimSpace(prev.Proxy.URL) != "" {
					if prevU, err := url.Parse(prev.Proxy.URL); err == nil && prevU.User != nil {
						if oldPass, ok := prevU.User.Password(); ok {
							restoredPass = oldPass
						}
					}
				}
				if restoredPass != "" {
					u.User = url.UserPassword(u.User.Username(), restoredPass)
				} else {
					u.User = url.User(u.User.Username())
				}
				next.Proxy.URL = u.String()
			}
		}
	}
}

// SetGlobal 设置进程内的全局配置并原子落盘（供动态读取）。
func SetGlobal(c *Config) {
	if err := Update(func(dst *Config) error {
		*dst = *c.Clone()
		return nil
	}); err != nil {
		fmt.Printf("警告: 保存全局配置失败: %v\n", err)
	}
}

// Update 在写锁保护下安全修改全局配置、校验并原子写入磁盘。
func Update(fn func(cfg *Config) error) error {
	globalMu.Lock()
	var next *Config
	if globalCfg == nil {
		next = defaults()
	} else {
		next = globalCfg.Clone()
	}
	if err := fn(next); err != nil {
		globalMu.Unlock()
		return err
	}
	if err := Validate(next); err != nil {
		globalMu.Unlock()
		return err
	}
	if err := writeConfigAtomic(next); err != nil {
		globalMu.Unlock()
		return err
	}
	globalCfg = next
	globalMu.Unlock()
	return nil
}

// Get 返回当前全局配置的只读深拷贝快照；若未设置则返回默认值，避免空指针或数据竞争。
func Get() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalCfg == nil {
		return defaults()
	}
	return globalCfg.Clone()
}

// Loader 返回一个闭包，用于按需获取最新全局配置。
func Loader() func() *Config { return Get }

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
		Proxy:   ProxyConfig{},
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
func Load(cfgPath string) (*Config, error) {
	cfg := defaults()

	v := viper.New()
	v.SetConfigFile(cfgPath)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("WATCH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err == nil {
		if err := v.Unmarshal(&cfg); err != nil {
			return nil, fmt.Errorf("unmarshal yaml: %w", err)
		}
	}

	// 兼容 PORT 环境变量（当未通过 YAML 或 WATCH_SERVER_ADDR 显式指定端口时生效）
	if portEnv := strings.TrimSpace(os.Getenv("PORT")); portEnv != "" && os.Getenv("WATCH_SERVER_ADDR") == "" && !v.IsSet("server.addr") {
		if !strings.HasPrefix(portEnv, ":") {
			portEnv = ":" + portEnv
		}
		cfg.Server.Addr = portEnv
	}

	if cfg.TwoFAConfig.Users == nil {
		cfg.TwoFAConfig.Users = make(map[string]TwoFAUserConfig)
	}
	if cfg.Policy.SkipLabels == nil {
		cfg.Policy.SkipLabels = []string{}
	}
	if cfg.Policy.OnlyLabels == nil {
		cfg.Policy.OnlyLabels = []string{}
	}
	if cfg.Policy.FloatingTags == nil {
		cfg.Policy.FloatingTags = []string{}
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	SetGlobal(cfg)
	return cfg.Clone(), nil
}

// Validate 校验关键字段，提前发现配置错误。
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(cfg.Server.Addr) == "" {
		return fmt.Errorf("server.addr is required")
	}
	if cfg.Scan.Concurrency <= 0 {
		return fmt.Errorf("scan.concurrency must be > 0")
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Notify.Method))
	switch method {
	case http.MethodGet, http.MethodPost:
		cfg.Notify.Method = method
	case "":
		cfg.Notify.Method = http.MethodGet
	default:
		return fmt.Errorf("notify.method must be GET or POST")
	}
	if strings.TrimSpace(cfg.Proxy.URL) != "" {
		u, err := url.Parse(strings.TrimSpace(cfg.Proxy.URL))
		if err != nil || u.Host == "" {
			return fmt.Errorf("invalid proxy.url")
		}
	}
	return nil
}

func writeConfigAtomic(cfg *Config) error {
	configPath := path.Join(conf.EnvCfg.CONFIG_PATH, conf.EnvCfg.CONFIG_FILE)
	if strings.TrimSpace(configPath) == "" || configPath == "." {
		return nil
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config to yaml: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmpFile.Write(yamlData); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("chmod temp config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		cleanup()
		return fmt.Errorf("rename config file: %w", err)
	}
	return nil
}

// Save 将当前全局配置保存到 YAML 文件
func Save() error {
	globalMu.RLock()
	if globalCfg == nil {
		globalMu.RUnlock()
		return fmt.Errorf("config is nil")
	}
	cp := globalCfg.Clone()
	globalMu.RUnlock()

	if err := Validate(cp); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	return writeConfigAtomic(cp)
}
