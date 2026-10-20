// New 按协议分发实现。
//
// @author zwei

package llm

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// New 校验配置并按协议构造 Client；失败不发起网络
func New(cfg Config) (Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// 拷贝配置，避免调用方事后修改扩展头或可选指针影响已构造客户端
	copied := cfg
	copied.APIURL = strings.TrimSpace(cfg.APIURL)
	copied.APIKey = strings.TrimSpace(cfg.APIKey)
	copied.Model = strings.TrimSpace(cfg.Model)
	copied.Temperature = cloneFloat64(cfg.Temperature)
	copied.MaxTokens = cloneInt(cfg.MaxTokens)
	copied.ExtraHeaders = copyExtraHeaders(cfg.ExtraHeaders)
	// 温度为 nil 时填 1.0，最大令牌为 nil 时填 4096
	applyGenerationDefaults(&copied)
	switch copied.Protocol {
	case ProtocolOpenAI:
		return newOpenAIClient(copied), nil
	case ProtocolAnthropic:
		return newAnthropicClient(copied), nil
	default:
		// validate 已覆盖，此处为防御
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
