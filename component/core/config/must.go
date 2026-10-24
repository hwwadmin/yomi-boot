package config

import (
	"fmt"
	"os"
)

// MustLoadConfig 按注册表加载并校验配置，失败时输出 stderr 并 panic
func MustLoadConfig(configPath, envPrefix string, reg *Registry) *Config {
	cfg, err := LoadConfig(configPath, envPrefix, reg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "配置加载失败: %v\n", err)
		panic(err)
	}
	return cfg
}
