// Package aiclient 向量化构造配置与协议常量。
//
// @author zwei
package aiclient

import (
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// EmbeddingProtocol 向量化协议标识；与对话 Protocol 相互独立
type EmbeddingProtocol string

const (
	// EmbeddingProtocolOpenAI OpenAI Embeddings 协议（含兼容网关）
	EmbeddingProtocolOpenAI EmbeddingProtocol = "openai"
)

// EmbeddingConfig 创建 EmbeddingClient 的构造配置；本包不读 YAML
type EmbeddingConfig struct {
	Protocol     EmbeddingProtocol // Protocol 是协议标识，首版仅 openai
	APIURL       string            // APIURL 原样注入官方客户端 BaseURL，本包不拼接路径
	APIKey       string            // APIKey 是鉴权密钥；日志与错误正文不得回显完整值
	Model        string            // Model 是本实例锁定的向量模型名；构造期 trim 后不得为空
	Dimensions   *int              // Dimensions 是默认输出维度；空则不传；若设置必须 >0
	Timeout      time.Duration     // Timeout 是整段墙钟超时；0 表示 60s；小于 0 非法
	HTTPClient   *http.Client      // HTTPClient 可选，仅传输层/代理；New 之后再改视为未定义行为
	ExtraHeaders map[string]string // ExtraHeaders 网关扩展头；New 时拷贝；不得含保留键
}

// timeoutOrDefault 返回请求超时；Timeout 为 0 时回退 60s
func (c EmbeddingConfig) timeoutOrDefault() time.Duration {
	if c.Timeout == 0 {
		return defaultTimeout
	}
	return c.Timeout
}

// validate 校验协议、修剪后的地址与密钥、模型名、默认维度、超时与扩展头保留键
//
// 返回值：
//   - error: 非法时为 ErrInvalidArgument；通过时为 nil
func (c EmbeddingConfig) validate() error {
	// 协议必须在白名单内
	switch c.Protocol {
	case EmbeddingProtocolOpenAI:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 地址、密钥与模型名修剪后不得为空
	if strings.TrimSpace(c.APIURL) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.Sentinel(ErrInvalidArgument)
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
	for k := range c.ExtraHeaders {
		// 与鉴权或版本头冲突的键拒绝构造
		if isReservedHeader(k) {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}
