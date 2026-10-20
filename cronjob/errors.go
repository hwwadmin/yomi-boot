// 调度错误定义
//
// @author zwei

package cronjob

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrDuplicateJobName 同一进程内重复启动同名 Job
	ErrDuplicateJobName = errors.NewNoStack("cronjob: 同名 Job 已在运行")
)
