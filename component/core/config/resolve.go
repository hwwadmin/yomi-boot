package config

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultConfigDir 是默认的配置文件目录名
const defaultConfigDir = "config"

// ResolvePath 返回默认配置目录下的配置文件路径，环境变量 YOMI_CONFIG 决定配置文件后缀
func ResolvePath() string {
	return ResolvePathFromDir(defaultConfigDir)
}

// ResolvePathFromDir 在指定目录中根据 YOMI_CONFIG 环境变量（local/dev/prod）解析配置文件路径
func ResolvePathFromDir(dir string) string {
	env := strings.TrimSpace(strings.ToLower(os.Getenv("YOMI_CONFIG")))
	switch env {
	case "local", "dev", "prod":
	default:
		env = "dev"
	}
	return filepath.Join(dir, env+".yml")
}
