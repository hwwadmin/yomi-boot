package config

import (
	"os"
	"regexp"
	"strings"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// envDefaultPattern 匹配 ${变量名:默认值} 格式的环境变量占位符
var envDefaultPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::([^}]*))?\}`)

// ExpandEnvDefaults 将 ${VAR:default} 占位符替换为环境变量值；未设置时使用 default。
// 环境变量已设置但为空字符串时，用空字符串，不用 default。
// 替换值含换行时返回错误，不把这段文字拼进待解析的 YAML。
func ExpandEnvDefaults(content string) (string, error) {
	var expandErr error
	expanded := envDefaultPattern.ReplaceAllStringFunc(content, func(match string) string {
		if expandErr != nil {
			return match
		}
		parts := envDefaultPattern.FindStringSubmatch(match)
		name := parts[1]
		value := ""
		if found, ok := os.LookupEnv(name); ok {
			value = found
		} else if len(parts) >= 3 {
			value = parts[2]
		}
		if strings.ContainsAny(value, "\r\n") {
			expandErr = errors.Newf("配置占位符 ${%s} 的替换值含有换行，拒绝展开", name)
			return match
		}
		return value
	})
	if expandErr != nil {
		return "", expandErr
	}
	return expanded, nil
}
