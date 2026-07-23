// Package snowflake Fail Fast 初始化
//
// @author zwei
package snowflake

import "yomi-boot/logger"

// MustNewGenerator 创建雪花 ID 生成器，失败时 log + panic
func MustNewGenerator(workerID int64) Generator {
	gen, err := NewGenerator(workerID)
	if err != nil {
		logger.ErrorWithoutCtx("snowflake 初始化失败", "error", err)
		panic(err)
	}
	return gen
}
