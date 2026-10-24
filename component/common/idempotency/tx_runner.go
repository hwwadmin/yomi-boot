package idempotency

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
)

// TxRunner 在事务边界内执行幂等业务闭包；由调用方注入实现。
type TxRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// DBTxRunner 基于 tx.Executor 的本地事务运行器。
type DBTxRunner struct {
	executor tx.Executor
}

var _ TxRunner = (*DBTxRunner)(nil)

// NewDBTxRunner 创建本地事务运行器；executor 为空时 panic。
func NewDBTxRunner(executor tx.Executor) *DBTxRunner {
	if executor == nil {
		panic("idempotency: NewDBTxRunner executor 不能为 nil")
	}
	return &DBTxRunner{executor: executor}
}

// Run 在本地事务中执行 fn。
func (r *DBTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.executor.Execute(ctx, fn)
}
