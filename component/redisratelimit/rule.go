// Package redisratelimit 提供调用式分布式限流能力
//
// @author zwei
package redisratelimit

import (
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// Rule 描述一次限流判定的窗口规则（由调用方按场景传入）
type Rule struct {
	Limit  int64         // Limit 是窗口内允许的最大次数
	Window time.Duration // Window 是滑动窗口长度
}

// Validate 校验规则参数
func (r Rule) Validate() error {
	if r.Limit <= 0 {
		return errors.New("redisratelimit: Rule.Limit 必须大于 0")
	}
	if r.Window <= 0 {
		return errors.New("redisratelimit: Rule.Window 必须大于 0")
	}
	if r.Window.Milliseconds() <= 0 {
		return errors.New("redisratelimit: Rule.Window 转换为毫秒后必须大于 0")
	}
	return nil
}
