// Package db 提供本地事务管理能力
//
// @author zwei
package db

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"

	"gorm.io/gorm"
)

// DefaultAutonomousTimeout 是自治事务未指定超时（timeout <= 0）时使用的默认超时
const DefaultAutonomousTimeout = 3 * time.Second

// txKey 是事务对象在上下文中的键
type txKey struct{}

// TxExecutor 表示事务执行器，供 usecase 注入；生产环境由 TxManager 实现，单元测试可注入 stub
type TxExecutor interface {
	// Execute 在事务边界内执行回调；回调返回 error 时回滚，否则提交
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
	// ExecuteAutonomous 在独立连接上执行自治事务；timeout <= 0 时使用 DefaultAutonomousTimeout
	ExecuteAutonomous(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error
}

// TxManager 表示事务管理器
type TxManager struct {
	db *gorm.DB
}

var _ TxExecutor = (*TxManager)(nil)

// NewTxManager 创建本地事务管理器
func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

// WithTx 将 GORM 事务实例写入上下文
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// GetTx 从上下文读取 GORM 事务实例
func GetTx(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	return tx, ok
}

// Exec 根据上下文返回当前可用的 GORM 实例（事务内返回 tx，否则返回 db）。
// 事务内同样把调用方 ctx 应用到本次查询，使超时、取消与 ctx 值生效。
func Exec(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := GetTx(ctx); ok {
		return tx.WithContext(ctx)
	}

	return db.WithContext(ctx)
}

// Execute 在本地事务中执行回调，回调返回 error 时自动回滚，否则自动提交。
// ctx 已在事务中时为嵌套调用：复用该事务，不再新开连接，提交或回滚由最外层决定。
func (m *TxManager) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}

	if _, ok := GetTx(ctx); ok {
		return fn(ctx)
	}

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := WithTx(ctx, tx)
		return fn(txCtx)
	})
}

// ExecuteAutonomous 在独立连接上执行自治事务：回调返回 error 时回滚，否则提交。
// 与 ctx 中已有的事务互不影响：外层回滚不会撤销这里的写入，这里的失败也不会回滚外层。
// 回调 ctx 不随调用方取消（保留 ctx 值），超时由 timeout 控制，timeout <= 0 时取默认值。
// 每次调用额外占用一条连接，须为 max_open_conns 预留余量；回调内不得使用外层事务对象。
func (m *TxManager) ExecuteAutonomous(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}
	if m.db == nil || m.db.Statement == nil {
		return errors.New("自治事务不可用：TxManager 未绑定连接池")
	}
	if committer, ok := m.db.Statement.ConnPool.(gorm.TxCommitter); ok && committer != nil {
		return errors.New("自治事务须使用连接池，TxManager 不能由事务对象构造")
	}
	if timeout <= 0 {
		timeout = DefaultAutonomousTimeout
	}

	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	return m.db.WithContext(actx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(actx, tx))
	})
}
