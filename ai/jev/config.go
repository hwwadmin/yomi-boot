// 构造配置与协议常量。
//
// @author zwei

package jev

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Protocol Jev 上游协议标识
type Protocol string

const (
	// ProtocolSystemOne TypeSafe System One 兼容 JSON API（含 OpenCode Zen /v1/systemone）
	ProtocolSystemOne Protocol = "systemone"

	// defaultTimeout 未配置 Timeout 时的默认墙钟
	defaultTimeout = 60 * time.Second
)

// Config 创建 Client 的构造配置；本包不读 YAML
type Config struct {
	Protocol     Protocol          // Protocol 是协议标识，首版仅 systemone
	APIURL       string            // APIURL 为完整 POST 地址（如 OpenCode https://opencode.ai/zen/v1/systemone）
	APIKey       string            // APIKey 可空（如 jev-1.13-free 免鉴权路由）；非空时带 Bearer
	Model        string            // Model 是本实例锁定的决策模型名；构造期 trim 后不得为空
	Timeout      time.Duration     // Timeout 是整段墙钟超时；0 表示 60s；小于 0 非法
	HTTPClient   *http.Client      // HTTPClient 可选，仅传输层/代理；New 之后再改视为未定义行为
	ExtraHeaders map[string]string // ExtraHeaders 网关扩展头；New 时拷贝；不得含保留键
}

// timeoutOrDefault 将超时时长回退为默认墙钟；0 表示 60s
func timeoutOrDefault(d time.Duration) time.Duration {
	if d == 0 {
		return defaultTimeout
	}
	return d
}

// validate 校验协议、修剪后的地址与模型名、超时与扩展头保留键；密钥允许为空
func (c Config) validate() error {
	switch c.Protocol {
	case ProtocolSystemOne:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	if err := validateAPIURL(c.APIURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Timeout < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	seenHeaders := make(map[string]struct{}, len(c.ExtraHeaders))
	for k := range c.ExtraHeaders {
		key := strings.TrimSpace(k)
		// 空键、与鉴权或版本头冲突、修剪后重名的键拒绝构造
		if key == "" || isReservedHeader(key) {
			return errors.Sentinel(ErrInvalidArgument)
		}
		if _, ok := seenHeaders[key]; ok {
			return errors.Sentinel(ErrInvalidArgument)
		}
		seenHeaders[key] = struct{}{}
	}
	return nil
}

// validateAPIURL 校验地址为 http/https 绝对地址
func validateAPIURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// isReservedHeader 判断扩展头是否与鉴权或版本头冲突（大小写不敏感）
func isReservedHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "x-api-key", "anthropic-version":
		return true
	default:
		return false
	}
}

// copyExtraHeaders 拷贝扩展头，避免调用方与客户端共享可变映射；空则返回空
func copyExtraHeaders(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[strings.TrimSpace(k)] = v
	}
	return dst
}
