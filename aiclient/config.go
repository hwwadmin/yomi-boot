// Package aiclient 构造配置与协议常量。
//
// @author zwei
package aiclient

import (
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Protocol 大模型协议标识
type Protocol string

const (
	// ProtocolOpenAI OpenAI Chat Completions 协议（含兼容网关）
	ProtocolOpenAI Protocol = "openai"
	// ProtocolAnthropic Anthropic Messages 协议
	ProtocolAnthropic Protocol = "anthropic"

	// defaultTimeout 未配置 Timeout 时的默认墙钟
	defaultTimeout = 60 * time.Second
	// defaultAnthropicVersion 未配置 AnthropicVersion 时的协议版本头
	defaultAnthropicVersion = "2023-06-01"
	// defaultAnthropicMaxTokens 请求未设置 MaxTokens 时 Anthropic 的必填回退值
	defaultAnthropicMaxTokens = 4096
)

// Config 创建 AIClient 的构造配置；本包不读 YAML
type Config struct {
	Protocol         Protocol          // Protocol 是协议标识，仅 openai / anthropic
	APIURL           string            // APIURL 原样注入官方客户端 BaseURL，本包不拼接路径
	APIKey           string            // APIKey 是鉴权密钥；日志与错误正文不得回显完整值
	DefaultModel     string            // DefaultModel 是请求未指定 model 时的回退；构造期可空
	Timeout          time.Duration     // Timeout 是整段墙钟超时；0 表示 60s；小于 0 非法
	HTTPClient       *http.Client      // HTTPClient 可选，仅传输层/代理；New 之后再改视为未定义行为
	AnthropicVersion string            // AnthropicVersion 仅 anthropic；空则 2023-06-01
	ExtraHeaders     map[string]string // ExtraHeaders 网关扩展头；New 时拷贝；不得含保留键
}

// timeoutOrDefault 返回请求超时；Timeout 为 0 时回退 60s
func (c Config) timeoutOrDefault() time.Duration {
	if c.Timeout == 0 {
		return defaultTimeout
	}
	return c.Timeout
}

// anthropicVersionOrDefault 返回 anthropic-version 头；空白则回退默认版本
func (c Config) anthropicVersionOrDefault() string {
	v := strings.TrimSpace(c.AnthropicVersion)
	if v == "" {
		return defaultAnthropicVersion
	}
	return v
}

// validate 校验协议、修剪后的地址与密钥、超时与扩展头保留键
//
// 返回值：
//   - error: 非法时为 ErrInvalidArgument；通过时为 nil
func (c Config) validate() error {
	// 协议必须在白名单内
	switch c.Protocol {
	case ProtocolOpenAI, ProtocolAnthropic:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 地址与密钥修剪后不得为空
	if strings.TrimSpace(c.APIURL) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 超时不得为负
	if c.Timeout < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for k := range c.ExtraHeaders {
		// 与鉴权或版本头冲突的键拒绝构造
		if isReservedHeader(k) {
			return errors.Sentinel(ErrInvalidArgument)
		}
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
		dst[k] = v
	}
	return dst
}
