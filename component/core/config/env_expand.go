package config

import (
	"os"
	"regexp"
)

// envDefaultPattern 匹配 ${变量名:默认值} 格式的环境变量占位符
var envDefaultPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::([^}]*))?\}`)

// ExpandEnvDefaults 将 ${VAR:default} 占位符替换为环境变量值；未设置时使用 default
func ExpandEnvDefaults(content string) string {
	return envDefaultPattern.ReplaceAllStringFunc(content, func(match string) string {
		// 提取变量名和默认值
		parts := envDefaultPattern.FindStringSubmatch(match)
		// 优先使用环境变量值
		if value, ok := os.LookupEnv(parts[1]); ok {
			return value
		}
		// 环境变量未设置时回退到默认值
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	})
}
