package appctx

import (
	"strings"
	"time"
	_ "time/tzdata" // 内嵌 IANA 时区库，避免精简镜像缺少 tzdata 导致启动失败

	"gitee.com/hq_project/yomi-boot/errors"
)

// LockTimezone 将进程时区锁定为 name 指定的 IANA 时区。
//
// 必须在启动阶段、任何依赖 time.Local 的 goroutine 启动前调用；
// name 为空时使用 DefaultTimezone（UTC），为 Local 时拒绝，避免时区随部署环境漂移。
func LockTimezone(name string) error {
	tz := strings.TrimSpace(name)
	if tz == "" || strings.EqualFold(tz, DefaultTimezone) {
		tz = DefaultTimezone
	}
	if strings.EqualFold(tz, "Local") {
		return errors.New("时区不支持 Local，请使用 IANA 时区名（如 UTC、Asia/Shanghai）")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return errors.Wrap(err, "加载时区失败: %s", tz)
	}
	time.Local = loc
	return nil
}
