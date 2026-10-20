// Job 定义与参数校验
//
// @author zwei

package cronjob

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/robfig/cron/v3"
)

// specParser 解析 Job.Spec 的 cron 解析器：6 段（秒 分 时 日 月 周）+ 描述符
var specParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// Handler 单次触发时的业务执行体
type Handler func(ctx context.Context) error

// Job 业务定时任务定义
type Job struct {
	// Name 集群内唯一，用作锁 Key 后缀；建议带服务前缀。同一进程内重复启动同名 Job 会输出 Warn
	Name string

	// Spec 是 6 段 cron（秒 分 时 日 月 周）；也支持 @daily 等描述符与 TZ=/CRON_TZ= 时区前缀
	Spec string

	// MaxRunDuration 单次 Handler 最长执行时间；0 表示不限制。
	// 到期后 cancel Handler ctx 并停止续锁，Handler 须尊重 ctx 才能及时退出。
	MaxRunDuration time.Duration
}

// validate 校验 Job、Handler 与锁工厂；并解析 Spec
func (j Job) validate(h Handler, factory distlock.DistLockFactory) error {
	if j.Name == "" {
		return errors.New("cronjob.name 不能为空")
	}
	if j.Spec == "" {
		return errors.New("cronjob.spec 不能为空")
	}
	if j.MaxRunDuration < 0 {
		return errors.New("cronjob.max_run_duration 不能为负数")
	}
	if h == nil {
		return errors.New("cronjob.handler 不能为空")
	}
	if factory == nil {
		return errors.New("cronjob.distlock_factory 不能为空")
	}
	if err := parseSpec(j.Spec); err != nil {
		return errors.Wrap(err, "cronjob.spec 无法解析")
	}
	return nil
}

// parseSpec 解析 cron Spec；将第三方解析器对畸形输入的 panic 转为 error，避免进程崩溃
func parseSpec(spec string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("解析异常（spec=%q）: %v", spec, r)
		}
	}()
	_, err = specParser.Parse(spec)
	return err
}
