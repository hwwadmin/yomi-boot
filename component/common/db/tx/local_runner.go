// 本地事务执行器
//
// @author zwei

package tx

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"gorm.io/gorm"
)

// DefaultAutonomousTimeout 是自治事务未指定超时（timeout <= 0）时使用的默认超时
const DefaultAutonomousTimeout = 3 * time.Second

// LocalRunner 表示绑定单一 GORM 数据源的本地事务执行器，实现 Runner 与 Executor
type LocalRunner struct {
	root *gorm.DB
}

var _ Executor = (*LocalRunner)(nil)

// NewLocalRunner 创建本地事务执行器；root 必须是数据源根句柄
func NewLocalRunner(root *gorm.DB) *LocalRunner {
	if root == nil {
		panic("tx: NewLocalRunner root 不能为 nil")
	}
	return &LocalRunner{root: root}
}

// Run 在本地事务中执行回调，回调返回 error 时自动回滚，否则自动提交。
// ctx 已在同一数据源的事务中时为嵌套调用：复用该事务，不再新开连接，提交或回滚由最外层决定。
func (r *LocalRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}

	if _, ok := FromContext(ctx, r.root); ok {
		return fn(ctx)
	}

	return r.root.WithContext(ctx).Transaction(func(gtx *gorm.DB) error {
		return fn(WithContext(ctx, r.root, gtx))
	})
}

// RunAutonomous 在独立连接上执行自治事务：回调返回 error 时回滚，否则提交。
// 与 ctx 中已有的事务互不影响：外层回滚不会撤销这里的写入，这里的失败也不会回滚外层。
// 回调 ctx 不随调用方取消（保留 ctx 值），超时由 timeout 控制，timeout <= 0 时取默认值。
// 每次调用额外占用一条连接，须为 max_open_conns 预留余量；回调内不得使用外层事务对象。
func (r *LocalRunner) RunAutonomous(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}
	if r.root.Statement == nil {
		return errors.New("自治事务不可用：LocalRunner 未绑定连接池")
	}
	if committer, ok := r.root.Statement.ConnPool.(gorm.TxCommitter); ok && committer != nil {
		return errors.New("自治事务须使用连接池，LocalRunner 不能由事务对象构造")
	}
	if timeout <= 0 {
		timeout = DefaultAutonomousTimeout
	}

	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	return r.root.WithContext(actx).Transaction(func(gtx *gorm.DB) error {
		return fn(WithContext(actx, r.root, gtx))
	})
}
