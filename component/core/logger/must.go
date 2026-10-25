package logger

import (
	"fmt"
	"os"

	"gitee.com/hq_project/yomi-boot/component/core/masking"
)

// MustInit 初始化日志，失败时输出 stderr 并 panic；返回无参清理函数。
// maskSvc 为注入的统一脱敏服务；是否调用由 logCfg.MaskSensitive 在组件内聚判定。
func MustInit(serviceName, env string, logCfg Config, maskSvc *masking.Service) func() {
	closeFn, err := InitLogger(serviceName, env, logCfg, maskSvc)
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
