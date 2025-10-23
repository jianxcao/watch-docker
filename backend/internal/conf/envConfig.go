package conf

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type EnvConfig struct {
	CONFIG_PATH               string `default:"/config"`
	CONFIG_FILE               string `default:"config.yaml"`
	VERSION_WATCH_DOCKER      string `default:"v0.0.6"`
	USER_NAME                 string `default:"admin"`
	USER_PASSWORD             string `default:"admin"`
	STATIC_DIR                string `default:"/app/static"`
	IS_OPEN_DOCKER_SHELL      bool   `default:"false"`
	APP_PATH                  string `default:""`
	IS_SECONDARY_VERIFICATION bool   `default:"false"`
}

func NewEnvConfig() *EnvConfig {
	cfg := EnvConfig{}
	err := envconfig.Process("", &cfg)
	if err != nil {
		fmt.Println("配置加载错误")
	}
	return &cfg
}

var EnvCfg = NewEnvConfig()
