// Package logger Fail Fast 初始化
//
// @author zwei
package logger

import (
	"fmt"
	"os"
)

// MustInit 初始化日志，失败时输出 stderr 并 panic；返回无参清理函数
func MustInit(serviceName, env string, logCfg Config) func() {
	closeFn, err := InitLogger(serviceName, env, logCfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "logger 初始化失败: %v\n", err)
		panic(err)
	}
	return func() {
		if closeFn != nil {
			_ = closeFn()
		}
	}
}
