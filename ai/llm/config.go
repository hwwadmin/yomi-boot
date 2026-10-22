// 构造配置与协议常量。
//
// 本文件与 ai/embedding/config.go、ai/jev/config.go 共享相同的校验/拷贝工具；
// 三包保持零依赖是有意设计，修改时需同步三处。
//
// @author zwei

package llm

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"
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
	// defaultTemperature 未配置 Temperature 时的缺省采样温度
	defaultTemperature = 1.0
	// defaultMaxTokens 未配置 MaxTokens 时的缺省最大输出令牌
	defaultMaxTokens = 4096
)

// MaxTokensField 控制 OpenAI 协议最大令牌字段的落点
type MaxTokensField string

const (
	// MaxTokensFieldAuto 按模型名自动选择；默认
	MaxTokensFieldAuto MaxTokensField = ""
	// MaxTokensFieldLegacy 仅写 max_tokens，兼容旧模型与兼容网关
	MaxTokensFieldLegacy MaxTokensField = "max_tokens"
	// MaxTokensFieldCompletion 仅写 max_completion_tokens，适配 reasoning / GPT-5 系列
	MaxTokensFieldCompletion MaxTokensField = "max_completion_tokens"
)

// Config 创建 Client 的构造配置；本包不读 YAML
type Config struct {
	Protocol         Protocol          // Protocol 是协议标识，仅 openai / anthropic
	APIURL           string            // APIURL 原样注入官方客户端 BaseURL，本包不拼接路径
	APIKey           string            // APIKey 是鉴权密钥；日志与错误正文不得回显完整值
	Model            string            // Model 是本实例锁定的模型名；构造期 trim 后不得为空
	Temperature      *float64          // Temperature 是默认采样温度；空则 1.0；0 为合法值
	MaxTokens        *int              // MaxTokens 是默认最大输出令牌；空则 4096；若设置必须 >0
	MaxTokensField   MaxTokensField    // MaxTokensField 是 OpenAI 协议最大令牌字段；空则按模型名自动选择
	Timeout          time.Duration     // Timeout 是整段墙钟超时；0 表示 60s；小于 0 非法
	HTTPClient       *http.Client      // HTTPClient 可选，仅传输层/代理；New 之后再改视为未定义行为
	AnthropicVersion string            // AnthropicVersion 仅 anthropic；空则 2023-06-01
	ExtraHeaders     map[string]string // ExtraHeaders 网关扩展头；New 时拷贝；不得含保留键
}

// timeoutOrDefault 将超时时长回退为默认墙钟；0 表示 60s
func timeoutOrDefault(d time.Duration) time.Duration {
	if d == 0 {
		return defaultTimeout
	}
	return d
}

// anthropicVersionOrDefault 返回 anthropic-version 头；空白则回退默认版本
func (c Config) anthropicVersionOrDefault() string {
	v := strings.TrimSpace(c.AnthropicVersion)
	if v == "" {
		return defaultAnthropicVersion
	}
	return v
}

// validate 校验协议、修剪后的地址与密钥、模型名、默认最大令牌数、超时与扩展头保留键
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
	// 最大令牌字段只能是自动或两个白名单值
	switch c.MaxTokensField {
	case MaxTokensFieldAuto, MaxTokensFieldLegacy, MaxTokensFieldCompletion:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 地址必须是 http/https 绝对地址；密钥与模型名修剪后不得为空
	if err := validateAPIURL(c.APIURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 默认最大令牌数若设置则必须大于 0
	if c.MaxTokens != nil && *c.MaxTokens <= 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 超时不得为负
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

// resolvedMaxTokensField 返回实际落点；自动模式按模型名判断 reasoning 系列
func (c Config) resolvedMaxTokensField() MaxTokensField {
	if c.MaxTokensField != MaxTokensFieldAuto {
		return c.MaxTokensField
	}
	if isReasoningModel(c.Model) {
		return MaxTokensFieldCompletion
	}
	return MaxTokensFieldLegacy
}

// isReasoningModel 判断模型是否属于只接受 max_completion_tokens 的系列
func isReasoningModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	// 兼容 provider/model 形式，只看最后一段
	if i := strings.LastIndexByte(m, '/'); i >= 0 {
		m = m[i+1:]
	}
	for _, prefix := range []string{"o1", "o3", "o4", "gpt-5"} {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
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

// cloneFloat64 拷贝可选浮点，避免与调用方共享指针；空则返回空
func cloneFloat64(src *float64) *float64 {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// cloneInt 拷贝可选整数，避免与调用方共享指针；空则返回空
func cloneInt(src *int) *int {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// applyGenerationDefaults 为空的温度与最大令牌数填入缺省值
func applyGenerationDefaults(cfg *Config) {
	if cfg.Temperature == nil {
		v := defaultTemperature
		cfg.Temperature = &v
	}
	if cfg.MaxTokens == nil {
		v := defaultMaxTokens
		cfg.MaxTokens = &v
	}
}
