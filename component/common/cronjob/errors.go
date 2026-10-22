// 调度错误定义
//
// @author zwei

package cronjob

import "gitee.com/hq_project/yomi-boot/component/core/errors"

// ErrDuplicateJobName 同一进程内重复启动同名 Job
var ErrDuplicateJobName = errors.NewNoStack("cronjob: 同名 Job 已在运行")
