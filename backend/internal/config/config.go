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
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-viper/mapstructure/v2"
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

// MarshalYAML 将 DurationMinutes 序列化为分钟数，与 YAML/JSON 配置格式保持一致。
func (d DurationMinutes) MarshalYAML() (any, error) {
	dur := time.Duration(d)
	if dur%time.Minute == 0 {
		return int64(dur / time.Minute), nil
	}
	return dur.Minutes(), nil
}

func durationMinutesDecodeHook(_, t reflect.Type, data any) (any, error) {
	if t != reflect.TypeOf(DurationMinutes(0)) {
		return data, nil
	}
	parseNumeric := func(val float64) DurationMinutes {
		// 兼容历史已落盘的纳秒整数（>= 1s 即 >= 1e9 纳秒）
		if val >= float64(time.Second) {
			return DurationMinutes(time.Duration(int64(val)))
		}
		return DurationMinutes(time.Duration(val * float64(time.Minute)))
	}
	switch v := data.(type) {
	case DurationMinutes:
		return v, nil
	case time.Duration:
		return DurationMinutes(v), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return DurationMinutes(0), nil
		}
		if dur, err := time.ParseDuration(s); err == nil {
			return DurationMinutes(dur), nil
		}
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid duration minutes %q: %w", v, err)
		}
		return parseNumeric(val), nil
	default:
		rv := reflect.ValueOf(data)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return parseNumeric(float64(rv.Int())), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return parseNumeric(float64(rv.Uint())), nil
		case reflect.Float32, reflect.Float64:
			return parseNumeric(rv.Float()), nil
		}
		return data, nil
	}
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

// NormalizeNotifyURL 兼容历史配置中的单个前导 @（仅当后续为 http:// 或 https://，忽略协议大小写），保留其余字符原样。
func NormalizeNotifyURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 1 && trimmed[0] == '@' {
		rest := trimmed[1:]
		lower := strings.ToLower(rest)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return rest
		}
	}
	return trimmed
}

func replaceURLUserPassword(rawURL, username, password string) string {
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		return rawURL
	}
	rest := rawURL[schemeEnd+3:]
	authorityEnd := strings.IndexAny(rest, "/?#")
	var authority, suffix string
	if authorityEnd < 0 {
		authority = rest
	} else {
		authority = rest[:authorityEnd]
		suffix = rest[authorityEnd:]
	}
	atIdx := strings.LastIndex(authority, "@")
	if atIdx < 0 {
		return rawURL
	}
	hostPart := authority[atIdx+1:]
	return rawURL[:schemeEnd+3] + url.UserPassword(username, password).String() + "@" + hostPart + suffix
}

func maskURLPassword(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.User == nil {
		return trimmed
	}
	pass, hasPass := u.User.Password()
	if !hasPass || pass == "" {
		return trimmed
	}
	return replaceURLUserPassword(trimmed, u.User.Username(), MaskedSecretPlaceholder)
}

func restoreMaskedURLPassword(nextURL, prevURL string) string {
	nextTrimmed := strings.TrimSpace(nextURL)
	if nextTrimmed == "" {
		return ""
	}
	u, err := url.Parse(nextTrimmed)
	if err != nil || u.User == nil {
		return nextTrimmed
	}
	pass, hasPass := u.User.Password()
	if !hasPass || pass != MaskedSecretPlaceholder {
		return nextTrimmed
	}
	prevTrimmed := strings.TrimSpace(prevURL)
	if prevTrimmed == "" {
		return nextTrimmed
	}
	prevU, err := url.Parse(prevTrimmed)
	if err != nil || prevU.User == nil {
		return nextTrimmed
	}
	// 仅在 scheme、host 与 username 均未变化时才还原旧密码，防止修改目标主机或协议降级外泄凭据
	if !strings.EqualFold(u.Scheme, prevU.Scheme) || !strings.EqualFold(u.Host, prevU.Host) || u.User.Username() != prevU.User.Username() {
		return nextTrimmed
	}
	if oldPass, ok := prevU.User.Password(); ok && oldPass != "" {
		return replaceURLUserPassword(nextTrimmed, u.User.Username(), oldPass)
	}
	return nextTrimmed
}

func hasMaskedURLPassword(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return false
	}
	u, err := url.Parse(trimmed)
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
	cp.Notify.URL = maskURLPassword(NormalizeNotifyURL(cp.Notify.URL))
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
	next.Notify.URL = NormalizeNotifyURL(next.Notify.URL)
	next.Notify.URL = restoreMaskedURLPassword(next.Notify.URL, NormalizeNotifyURL(prev.Notify.URL))
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

func inspectConfigLoadPath(cfgPath string) (fileExists bool, fileNonEmpty bool, err error) {
	if strings.TrimSpace(cfgPath) == "" || cfgPath == "." {
		return false, false, nil
	}
	linfo, lerr := os.Lstat(cfgPath)
	if lerr != nil {
		if errors.Is(lerr, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("stat config path %s: %w", cfgPath, lerr)
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		resolved, rerr := filepath.EvalSymlinks(cfgPath)
		if rerr != nil {
			return false, false, fmt.Errorf("resolve config symlink %s: %w", cfgPath, rerr)
		}
		tinfo, terr := os.Stat(resolved)
		if terr != nil {
			return false, false, fmt.Errorf("stat resolved config target %s (from %s): %w", resolved, cfgPath, terr)
		}
		if tinfo.IsDir() {
			return false, false, fmt.Errorf("config symlink %s points to directory %s", cfgPath, resolved)
		}
		return true, tinfo.Size() > 0, nil
	}
	if linfo.IsDir() {
		return false, false, fmt.Errorf("config path %s is a directory", cfgPath)
	}
	return true, linfo.Size() > 0, nil
}

// Load 读取配置文件（YAML）并应用 ENV 覆盖，校验后设置为全局配置。
func Load(cfgPath string) (*Config, error) {
	fileExists, fileNonEmpty, statErr := inspectConfigLoadPath(cfgPath)
	if statErr != nil {
		return nil, statErr
	}

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

	if fileNonEmpty {
		v.SetConfigFile(cfgPath)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("read config yaml: %w", err)
		}
	}

	if err := v.Unmarshal(&cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		durationMinutesDecodeHook,
	))); err != nil {
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
	if cfg.Scan.CacheTTL == 0 {
		cfg.Scan.CacheTTL = DurationMinutes(10 * time.Minute)
	} else if cfg.Scan.CacheTTL < 0 {
		return fmt.Errorf("scan.cacheTTL must be > 0")
	}
	lvl := strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	if lvl == "" {
		lvl = "info"
	}
	switch lvl {
	case "debug", "info", "warn", "warning", "error":
		cfg.Logging.Level = lvl
	default:
		return fmt.Errorf("invalid logging.level %q: must be debug, info, warn, or error", cfg.Logging.Level)
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
	notifyURL := NormalizeNotifyURL(cfg.Notify.URL)
	cfg.Notify.URL = notifyURL
	if notifyURL != "" {
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
	proxyURL := strings.TrimSpace(cfg.Proxy.URL)
	cfg.Proxy.URL = proxyURL
	if proxyURL != "" {
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

func resolveConfigWritePath(configPath string) (string, error) {
	info, err := os.Lstat(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configPath, nil
		}
		return "", fmt.Errorf("stat config path %s: %w", configPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if info.IsDir() {
			return "", fmt.Errorf("config path %s is a directory", configPath)
		}
		return configPath, nil
	}
	resolved, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return "", fmt.Errorf("resolve config symlink %s: %w", configPath, err)
	}
	targetInfo, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat resolved config target %s (from %s): %w", resolved, configPath, err)
	}
	if targetInfo.IsDir() {
		return "", fmt.Errorf("resolved config target %s (from %s) is a directory", resolved, configPath)
	}
	return resolved, nil
}

func isReadOnlyOrPermErr(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EROFS)
}

func writeConfigAtomic(configPath string, cfg *Config) error {
	if strings.TrimSpace(configPath) == "" || configPath == "." {
		return nil
	}
	writePath, err := resolveConfigWritePath(configPath)
	if err != nil {
		return err
	}
	_, targetStatErr := os.Stat(writePath)
	targetExists := targetStatErr == nil
	dir := filepath.Dir(writePath)
	if err := os.MkdirAll(dir, 0700); err != nil && !targetExists {
		return fmt.Errorf("create config dir: %w", err)
	}

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config to yaml: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		if targetExists && isReadOnlyOrPermErr(err) {
			if werr := os.WriteFile(writePath, yamlData, 0600); werr != nil {
				return fmt.Errorf("write config file in place: %w", werr)
			}
			_ = os.Chmod(writePath, 0600)
			return nil
		}
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
	if err := os.Rename(tmpPath, writePath); err != nil {
		// 单文件 bind mount 时 os.Rename 会返回 EBUSY/EXDEV，尝试原地覆写并保持 0600
		if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EXDEV) {
			cleanup()
			if werr := os.WriteFile(writePath, yamlData, 0600); werr != nil {
				return fmt.Errorf("write config file in place: %w", werr)
			}
			_ = os.Chmod(writePath, 0600)
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
