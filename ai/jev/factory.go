// New 按协议分发实现。
//
// @author zwei

package jev

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// New 校验配置并按协议构造 Client；失败不发起网络
func New(cfg Config) (Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// 拷贝配置，避免调用方事后修改扩展头影响已构造客户端
	copied := cfg
	copied.Endpoint = strings.TrimSpace(cfg.Endpoint)
	copied.APIKey = strings.TrimSpace(cfg.APIKey)
	copied.Model = strings.TrimSpace(cfg.Model)
	copied.ExtraHeaders = copyExtraHeaders(cfg.ExtraHeaders)
	switch copied.Protocol {
	case ProtocolSystemOne:
		return newSystemOneClient(copied), nil
	default:
		// validate 已覆盖，此处为防御
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
