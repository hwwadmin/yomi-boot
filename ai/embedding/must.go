// Fail Fast 构造。
//
// @author zwei

package embedding

import "gitee.com/hq_project/yomi-boot/logger"

// MustNew 创建 Client，失败时记录错误日志并立即崩溃；日志不得包含完整密钥
func MustNew(cfg Config) Client {
	cli, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("ai/embedding 初始化失败", "error", err, "protocol", string(cfg.Protocol))
		panic(err)
	}
	return cli
}
