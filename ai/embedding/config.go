// Package embedding 构造配置与协议常量。
//
// @author zwei
package embedding

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Protocol 向量化协议标识
type Protocol string

const (
	// ProtocolOpenAI OpenAI Embeddings 协议（含兼容网关）
	ProtocolOpenAI Protocol = "openai"

	// defaultTimeout 未配置 Timeout 时的默认墙钟
	defaultTimeout = 60 * time.Second
)

// Config 创建 Client 的构造配置；本包不读 YAML
type Config struct {
	Protocol     Protocol          // Protocol 是协议标识，首版仅 openai
	APIURL       string            // APIURL 原样注入官方客户端 BaseURL，本包不拼接路径
	APIKey       string            // APIKey 可选；空表示不带鉴权，供本地或免费网关；非空时日志与错误正文不得回显完整值
	Model        string            // Model 是本实例锁定的向量模型名；构造期 trim 后不得为空
	Dimensions   *int              // Dimensions 是默认输出维度；空则不传；若设置必须 >0；仅部分模型支持，不支持的网关会 400
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

// validate 校验协议、修剪后的地址与模型名、默认维度、超时与扩展头保留键；密钥允许为空
//
// 返回值：
//   - error: 非法时为 ErrInvalidArgument；通过时为 nil
func (c Config) validate() error {
	// 协议必须在白名单内
	switch c.Protocol {
	case ProtocolOpenAI:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 地址必须是 http/https 绝对地址；模型名修剪后不得为空；密钥允许为空
	if err := validateAPIURL(c.APIURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 默认维度若设置则必须大于 0；空表示不传上游
	if c.Dimensions != nil && *c.Dimensions <= 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 超时不得为负
	if c.Timeout < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	seenHeaders := make(map[string]struct{}, len(c.ExtraHeaders))
	for k := range c.ExtraHeaders {
		key := strings.TrimSpace(k)
		// 空键、与鉴权头冲突、修剪后重名的键拒绝构造
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

// isReservedHeader 判断扩展头是否与鉴权头冲突（大小写不敏感）
func isReservedHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "x-api-key":
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

// cloneInt 拷贝可选整数，避免与调用方共享指针；空则返回空
func cloneInt(src *int) *int {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}
