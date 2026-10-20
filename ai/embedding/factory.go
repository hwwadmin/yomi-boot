// New 按协议分发实现。
//
// @author zwei

package embedding

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
	copied.Dimensions = cloneInt(cfg.Dimensions)
	copied.ExtraHeaders = copyExtraHeaders(cfg.ExtraHeaders)
	switch copied.Protocol {
	case ProtocolOpenAI:
		return newOpenAIClient(copied)
	default:
		// validate 已覆盖，此处为防御
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
