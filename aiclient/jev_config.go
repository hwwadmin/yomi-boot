// Package aiclient Jev 构造配置与协议常量。
//
// @author zwei
package aiclient

import (
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// JevProtocol Jev 上游协议标识；与对话 Protocol 相互独立
type JevProtocol string

const (
	// JevProtocolSystemOne TypeSafe System One 兼容 JSON API（含 OpenCode Zen /v1/systemone）
	JevProtocolSystemOne JevProtocol = "systemone"
)

// JevConfig 创建 JevClient 的构造配置；本包不读 YAML
type JevConfig struct {
	Protocol     JevProtocol       // Protocol 是协议标识，首版仅 systemone
	APIURL       string            // APIURL 为完整 POST 地址（如 OpenCode https://opencode.ai/zen/v1/systemone）
	APIKey       string            // APIKey 可空（如 jev-1.13-free 免鉴权路由）；非空时带 Bearer
	Model        string            // Model 是本实例锁定的决策模型名；构造期 trim 后不得为空
	Timeout      time.Duration     // Timeout 是整段墙钟超时；0 表示 60s；小于 0 非法
	HTTPClient   *http.Client      // HTTPClient 可选，仅传输层/代理；NewJev 之后再改视为未定义行为
	ExtraHeaders map[string]string // ExtraHeaders 网关扩展头；NewJev 时拷贝；不得含保留键
}

// validate 校验协议、修剪后的地址与模型名、超时与扩展头保留键；密钥允许为空
func (c JevConfig) validate() error {
	switch c.Protocol {
	case JevProtocolSystemOne:
	default:
		return errors.Sentinel(ErrInvalidArgument)
	}
	if strings.TrimSpace(c.APIURL) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if c.Timeout < 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for k := range c.ExtraHeaders {
		if isReservedHeader(k) {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}
