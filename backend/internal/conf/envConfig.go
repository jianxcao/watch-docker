package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/viper"
)

// Version 应用版本号，构建时通过 ldflags 注入（从 frontend/package.json）
var Version = "dev"

// EnvConfig 应用环境配置（与 Docker 业务配置分离）
// 这些是应用运行所需的环境变量配置，不是 Docker 业务逻辑配置
type EnvConfig struct {
	CONFIG_PATH               string `default:"~/.watch-docker" mapstructure:"config_path"`
	CONFIG_FILE               string `default:"config.yaml" mapstructure:"config_file"` // Docker 业务配置文件
	ENV_FILE                  string `default:"app.yaml" mapstructure:"env_file"`       // 应用环境配置文件（新）
	VERSION_WATCH_DOCKER      string `default:"v0.1.6" mapstructure:"version"`
	USER_NAME                 string `default:"admin" mapstructure:"username"`
	USER_PASSWORD             string `default:"admin" mapstructure:"password"`
	STATIC_DIR                string `default:"" mapstructure:"static_dir"` // 空字符串表示使用嵌入式资源
	IS_OPEN_DOCKER_SHELL      bool   `default:"false" mapstructure:"enable_docker_shell"`
	APP_PATH                  string `default:"" mapstructure:"app_path"`
	IS_SECONDARY_VERIFICATION bool   `default:"false" mapstructure:"enable_2fa"`
	TWOFA_ALLOWED_DOMAINS     string `default:"" mapstructure:"twofa_allowed_domains"` // 逗号分隔的域名白名单，空值表示允许所有域名
}

// expandPath 扩展路径中的 ~ 为用户主目录
func expandPath(path string) string {
	if path == "" {
		return path
	}

	// 如果路径以 ~ 开头，扩展为用户主目录
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("警告: 无法获取用户主目录: %v\n", err)
			return path
		}
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~"))
	}

	return path
}

func NewEnvConfig() *EnvConfig {
	cfg := EnvConfig{}

	// 1. 首先从环境变量加载基础配置（CONFIG_PATH, CONFIG_FILE, ENV_FILE）
	// 这样才能知道配置文件在哪里
	if err := envconfig.Process("", &cfg); err != nil {
		fmt.Printf("警告: 环境变量加载错误: %v\n", err)
	}

	// 2. 设置版本号（构建时注入，不从配置文件读取）
	cfg.VERSION_WATCH_DOCKER = Version

	// 3. 扩展 CONFIG_PATH 中的 ~
	cfg.CONFIG_PATH = expandPath(cfg.CONFIG_PATH)

	// 4. 尝试从应用环境配置文件加载（app.yaml）
	// 这个文件专门用于应用环境配置，与 config.yaml（Docker 业务配置）分离
	// 注意：version 不从配置文件读取，始终使用构建时注入的版本号
	envFile := filepath.Join(cfg.CONFIG_PATH, cfg.ENV_FILE)
	if _, err := os.Stat(envFile); err == nil {
		// 应用配置文件存在，尝试读取
		v := viper.New()
		v.SetConfigFile(envFile)
		v.SetConfigType("yaml")

		if err := v.ReadInConfig(); err == nil {
			// 从配置文件中读取值：app.yaml 已配置的项优先于环境变量
			if v.IsSet("username") {
				cfg.USER_NAME = v.GetString("username")
			}
			if v.IsSet("password") {
				cfg.USER_PASSWORD = v.GetString("password")
			}
			if v.IsSet("enable_2fa") {
				cfg.IS_SECONDARY_VERIFICATION = v.GetBool("enable_2fa")
			}
			if v.IsSet("twofa_allowed_domains") {
				cfg.TWOFA_ALLOWED_DOMAINS = v.GetString("twofa_allowed_domains")
			}
			if v.IsSet("static_dir") {
				cfg.STATIC_DIR = v.GetString("static_dir")
			}
			if v.IsSet("enable_docker_shell") {
				cfg.IS_OPEN_DOCKER_SHELL = v.GetBool("enable_docker_shell")
			}
			if v.IsSet("app_path") {
				cfg.APP_PATH = v.GetString("app_path")
			}
			// version 不从配置文件读取

			fmt.Printf("✅ 已从应用配置文件加载: %s\n", envFile)
		} else {
			fmt.Printf("警告: 读取应用配置文件失败: %v\n", err)
		}
	} else {
		fmt.Printf("提示: 应用配置文件不存在: %s\n", envFile)
		fmt.Printf("     使用默认值和环境变量\n")
		fmt.Printf("     可以创建 app.yaml 以持久化应用配置（用户名、密码等）\n")
	}

	// 5. 再次扩展路径中的 ~（配置文件中可能也有）
	cfg.STATIC_DIR = expandPath(cfg.STATIC_DIR)
	cfg.APP_PATH = expandPath(cfg.APP_PATH)

	// 6. 自动创建配置目录（如果不存在）
	if cfg.CONFIG_PATH != "" {
		if err := os.MkdirAll(cfg.CONFIG_PATH, 0755); err != nil {
			fmt.Printf("警告: 无法创建配置目录 %s: %v\n", cfg.CONFIG_PATH, err)
		} else {
			// 如果应用配置文件不存在，创建示例配置文件
			exampleFile := filepath.Join(cfg.CONFIG_PATH, "app.yaml.example")
			if _, err := os.Stat(envFile); os.IsNotExist(err) {
				if _, err := os.Stat(exampleFile); os.IsNotExist(err) {
					createExampleAppConfig(exampleFile)
				}
			}
		}
	}

	return &cfg
}

// isEnvSet 检查环境变量是否已设置
func isEnvSet(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// createExampleAppConfig 创建应用配置示例文件
func createExampleAppConfig(path string) {
	exampleContent := `# Watch Docker 应用配置文件
# 此文件用于配置应用运行时环境，与 config.yaml（Docker 业务配置）分离
# 
# 配置优先级：app.yaml > 环境变量 > 默认值
#
# 复制此文件为 app.yaml 并根据需要修改

# =============================================================================
# 认证配置
# =============================================================================

# 登录用户名
username: "admin"

# 登录密码
# ⚠️ 安全警告：请立即修改默认密码！
password: "admin"

# 是否启用双因素认证（2FA）
enable_2fa: false

# 2FA 允许的域名白名单（逗号分隔）
# 空值表示允许所有域名
twofa_allowed_domains: ""

# =============================================================================
# 静态资源配置
# =============================================================================

# 静态资源目录
# 空字符串表示使用嵌入在二进制中的资源（推荐）
# 如需自定义前端，设置为前端构建产物目录
static_dir: ""

# =============================================================================
# 功能开关
# =============================================================================

# 是否开启容器 Shell 功能
# 启用后可以直接在 Web 界面进入容器终端
enable_docker_shell: false

# =============================================================================
# 应用配置
# =============================================================================

# 应用路径
app_path: ""


# =============================================================================
# 说明
# =============================================================================
#
# 1. 此文件（app.yaml）用于应用环境配置
#    config.yaml 用于 Docker 业务配置（扫描、通知、服务器等）
#
# 2. 所有配置都可以通过环境变量覆盖，例如：
#    export USER_NAME="myuser"
#    export USER_PASSWORD="mypassword"
#
# 3. 修改配置后需要重启应用：
#    systemctl restart watch-docker
#
# 4. 安全建议：
#    - 修改默认密码
#    - 设置文件权限：chmod 600 ~/.watch-docker/app.yaml
#
# 5. 更多配置说明请查看：doc/configuration-guide.md
#
# =============================================================================
`
	if err := os.WriteFile(path, []byte(exampleContent), 0644); err == nil {
		fmt.Printf("✅ 已创建应用配置示例文件: %s\n", path)
	}
}

var EnvCfg = NewEnvConfig()
