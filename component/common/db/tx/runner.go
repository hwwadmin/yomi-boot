// 事务执行契约
//
// @author zwei

package tx

import (
	"context"
	"time"
)

// Runner 在事务边界内执行闭包；本地 Manager 与 dtm.TxRunner 均满足
type Runner interface {
	// Run 在事务中执行 fn；已处于同一数据源事务中时复用外层事务
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// Executor 在 Runner 之上提供自治事务能力
type Executor interface {
	Runner
	// RunAutonomous 在独立连接上执行自治事务；timeout <= 0 时使用 DefaultAutonomousTimeout
	RunAutonomous(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error
}
