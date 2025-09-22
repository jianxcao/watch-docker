package conf

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type EnvConfig struct {
	CONFIG_PATH       string `default:"/config"`
	CONFIG_FILE       string `default:"config.yaml"`
	VERSION_MEDIA_302 string `default:"v2.1.6"`
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
