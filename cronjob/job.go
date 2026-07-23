// Package cronjob Job 定义与参数校验
//
// @author zwei
package cronjob

import (
	"context"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/robfig/cron/v3"
)

// Handler 单次触发时的业务执行体
type Handler func(ctx context.Context) error

// Job 业务定时任务定义
type Job struct {
	Name string // Name 集群内唯一，用作锁 Key 后缀；建议带服务前缀
	Spec string // Spec 6 段 cron：秒 分 时 日 月 周（robfig WithSeconds）
}

// validate 校验 Job、Handler 与锁工厂；并解析 Spec
func (j Job) validate(h Handler, factory distlock.DistLockFactory) error {
	if j.Name == "" {
		return errors.New("cronjob.name 不能为空")
	}
	if j.Spec == "" {
		return errors.New("cronjob.spec 不能为空")
	}
	if h == nil {
		return errors.New("cronjob.handler 不能为空")
	}
	if factory == nil {
		return errors.New("cronjob.distlock_factory 不能为空")
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(j.Spec); err != nil {
		return errors.Wrap(err, "cronjob.spec 无法解析")
	}
	return nil
}
