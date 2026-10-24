// 本地事务管理器
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

// Manager 表示绑定单一数据源的事务管理器
type Manager struct {
	root *gorm.DB
}

var _ Executor = (*Manager)(nil)

// NewManager 创建本地事务管理器；root 必须是数据源根句柄
func NewManager(root *gorm.DB) *Manager {
	if root == nil {
		panic("tx: NewManager root 不能为 nil")
	}
	return &Manager{root: root}
}

// Run 在本地事务中执行回调，回调返回 error 时自动回滚，否则自动提交。
// ctx 已在同一数据源的事务中时为嵌套调用：复用该事务，不再新开连接，提交或回滚由最外层决定。
func (m *Manager) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}

	if _, ok := FromContext(ctx, m.root); ok {
		return fn(ctx)
	}

	return m.root.WithContext(ctx).Transaction(func(gtx *gorm.DB) error {
		return fn(WithContext(ctx, m.root, gtx))
	})
}

// RunAutonomous 在独立连接上执行自治事务：回调返回 error 时回滚，否则提交。
// 与 ctx 中已有的事务互不影响：外层回滚不会撤销这里的写入，这里的失败也不会回滚外层。
// 回调 ctx 不随调用方取消（保留 ctx 值），超时由 timeout 控制，timeout <= 0 时取默认值。
// 每次调用额外占用一条连接，须为 max_open_conns 预留余量；回调内不得使用外层事务对象。
func (m *Manager) RunAutonomous(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}
	if m.root.Statement == nil {
		return errors.New("自治事务不可用：Manager 未绑定连接池")
	}
	if committer, ok := m.root.Statement.ConnPool.(gorm.TxCommitter); ok && committer != nil {
		return errors.New("自治事务须使用连接池，Manager 不能由事务对象构造")
	}
	if timeout <= 0 {
		timeout = DefaultAutonomousTimeout
	}

	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	return m.root.WithContext(actx).Transaction(func(gtx *gorm.DB) error {
		return fn(WithContext(actx, m.root, gtx))
	})
}
