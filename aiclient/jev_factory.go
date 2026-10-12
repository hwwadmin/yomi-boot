// Package aiclient NewJev 按协议分发实现。
//
// @author zwei
package aiclient

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// NewJev 校验配置并按协议构造 JevClient；失败不发起网络
func NewJev(cfg JevConfig) (JevClient, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	copied := cfg
	copied.APIURL = strings.TrimSpace(cfg.APIURL)
	copied.APIKey = strings.TrimSpace(cfg.APIKey)
	copied.Model = strings.TrimSpace(cfg.Model)
	copied.ExtraHeaders = copyExtraHeaders(cfg.ExtraHeaders)
	switch copied.Protocol {
	case JevProtocolSystemOne:
		return newSystemOneJevClient(copied)
	default:
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
