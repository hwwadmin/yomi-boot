// Fail Fast 构造。
//
// @author zwei

package captcha

import "gitee.com/hq_project/yomi-boot/component/core/logger"

// MustNew 创建验证码服务，失败时记录错误日志并立即崩溃；日志不得包含验证码答案
func MustNew(store Store, cfg Config) Service {
	svc, err := New(store, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("captcha 初始化失败", "error", err, "driver", string(cfg.Driver))
		panic(err)
	}
	return svc
}
