package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/robfig/cron/v3"
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
	globalCfg        *Config
	globalCfgPath    string
	globalEnvCfgPath string
	globalMu         sync.RWMutex
	cronParser       = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
)

func resolveConfigPathLocked() string {
	envPath := ""
	if strings.TrimSpace(conf.EnvCfg.CONFIG_PATH) != "" && strings.TrimSpace(conf.EnvCfg.CONFIG_FILE) != "" {
		envPath = filepath.Join(conf.EnvCfg.CONFIG_PATH, conf.EnvCfg.CONFIG_FILE)
	}
	if globalCfgPath != "" && (envPath == "" || envPath == globalEnvCfgPath) {
		return globalCfgPath
	}
	return envPath
}

func maskURLPassword(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	if _, hasPass := u.User.Password(); !hasPass {
		return rawURL
	}
	u.User = url.UserPassword(u.User.Username(), MaskedSecretPlaceholder)
	return u.String()
}

func restoreMaskedURLPassword(nextURL, prevURL string) string {
	if strings.TrimSpace(nextURL) == "" {
		return nextURL
	}
	u, err := url.Parse(nextURL)
	if err != nil || u.User == nil {
		return nextURL
	}
	pass, hasPass := u.User.Password()
	if !hasPass || pass != MaskedSecretPlaceholder {
		return nextURL
	}
	if strings.TrimSpace(prevURL) == "" {
		return nextURL
	}
	prevU, err := url.Parse(prevURL)
	if err != nil || prevU.User == nil {
		return nextURL
	}
	// 仅在 host 与 username 未变化时才还原旧密码，防止修改主机时把旧密码外泄到新目标
	if !strings.EqualFold(u.Host, prevU.Host) || u.User.Username() != prevU.User.Username() {
		return nextURL
	}
	if oldPass, ok := prevU.User.Password(); ok && oldPass != "" {
		u.User = url.UserPassword(u.User.Username(), oldPass)
		return u.String()
	}
	return nextURL
}

func hasMaskedURLPassword(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return false
	}
	pass, ok := u.User.Password()
	return ok && pass == MaskedSecretPlaceholder
}

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
	cp.Proxy.URL = maskURLPassword(cp.Proxy.URL)
	cp.Notify.URL = maskURLPassword(cp.Notify.URL)
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
		for _, oldAuth := range prev.Registry.Auth {
			if strings.EqualFold(oldAuth.Host, next.Registry.Auth[i].Host) && oldAuth.Username == next.Registry.Auth[i].Username {
				next.Registry.Auth[i].Token = oldAuth.Token
				break
			}
		}
	}

	next.Proxy.URL = restoreMaskedURLPassword(next.Proxy.URL, prev.Proxy.URL)
	next.Notify.URL = restoreMaskedURLPassword(next.Notify.URL, prev.Notify.URL)
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
	if err := writeConfigAtomic(resolveConfigPathLocked(), next); err != nil {
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
	defYAML, _ := yaml.Marshal(cfg)

	v := viper.New()
	v.SetConfigType("yaml")
	if len(defYAML) > 0 {
		_ = v.ReadConfig(bytes.NewReader(defYAML))
	}
	v.SetEnvPrefix("WATCH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	fileExists := false
	fileNonEmpty := false
	if info, statErr := os.Stat(cfgPath); statErr == nil {
		fileExists = true
		fileNonEmpty = info.Size() > 0
	}
	if fileNonEmpty {
		v.SetConfigFile(cfgPath)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("read config yaml: %w", err)
		}
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// 兼容 PORT 环境变量（仅当显式设置 PORT 且未设置 WATCH_SERVER_ADDR 时生效）
	if portEnv := strings.TrimSpace(os.Getenv("PORT")); portEnv != "" && strings.TrimSpace(os.Getenv("WATCH_SERVER_ADDR")) == "" {
		if !strings.Contains(portEnv, ":") {
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
	if cfg.Registry.Auth == nil {
		cfg.Registry.Auth = []RegistryAuth{}
	}
	if cfg.Registry.Mirrors == nil {
		cfg.Registry.Mirrors = []RegistryMirror{}
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	globalMu.Lock()
	globalCfg = cfg.Clone()
	globalCfgPath = cfgPath
	if strings.TrimSpace(conf.EnvCfg.CONFIG_PATH) != "" && strings.TrimSpace(conf.EnvCfg.CONFIG_FILE) != "" {
		globalEnvCfgPath = filepath.Join(conf.EnvCfg.CONFIG_PATH, conf.EnvCfg.CONFIG_FILE)
	} else {
		globalEnvCfgPath = ""
	}
	globalMu.Unlock()

	// 仅在配置文件缺失或为空文件时初始化落盘；若挂载为只读(:ro)仅记录警告但不清零内存配置
	if !fileExists || !fileNonEmpty {
		if err := writeConfigAtomic(cfgPath, cfg); err != nil {
			fmt.Printf("警告: 初始化写入配置文件失败 (%s): %v\n", cfgPath, err)
		}
	}

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
	if cronExpr := strings.TrimSpace(cfg.Scan.Cron); cronExpr != "" {
		if _, err := cronParser.Parse(cronExpr); err != nil {
			return fmt.Errorf("invalid scan.cron: %w", err)
		}
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
	if notifyURL := strings.TrimSpace(cfg.Notify.URL); notifyURL != "" {
		u, err := url.Parse(notifyURL)
		if err != nil || u == nil || u.Host == "" {
			return fmt.Errorf("invalid notify.url")
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("invalid notify.url")
		}
		if hasMaskedURLPassword(notifyURL) {
			return fmt.Errorf("notify.url host or username changed: please re-enter password")
		}
	}
	if proxyURL := strings.TrimSpace(cfg.Proxy.URL); proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil || u == nil || u.Host == "" {
			return fmt.Errorf("invalid proxy.url")
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" && scheme != "socks5" && scheme != "socks5h" {
			return fmt.Errorf("invalid proxy.url")
		}
		if hasMaskedURLPassword(proxyURL) {
			return fmt.Errorf("proxy.url host or username changed: please re-enter password")
		}
	}
	for _, item := range cfg.Registry.Auth {
		if item.Token == MaskedSecretPlaceholder {
			return fmt.Errorf("registry auth %q host or username changed: please re-enter token", item.Host)
		}
	}
	return nil
}

func writeConfigAtomic(configPath string, cfg *Config) error {
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
		// 单文件 bind mount 时 os.Rename 会返回 EBUSY/EXDEV，尝试原地覆写并保持 0600
		if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EXDEV) {
			cleanup()
			if werr := os.WriteFile(configPath, yamlData, 0600); werr != nil {
				return fmt.Errorf("write config file in place: %w", werr)
			}
			_ = os.Chmod(configPath, 0600)
			return nil
		}
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
	targetPath := resolveConfigPathLocked()
	globalMu.RUnlock()

	if err := Validate(cp); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	return writeConfigAtomic(targetPath, cp)
}
