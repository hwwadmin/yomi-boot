// Package db 提供本地事务管理能力
//
// @author zwei
package db

import (
	"context"

	"gorm.io/gorm"
)

// txKey 是事务对象在上下文中的键
type txKey struct{}

// TxExecutor 表示本地事务执行器，供 usecase 注入；生产环境由 TxManager 实现，单元测试可注入 stub
type TxExecutor interface {
	// Execute 在事务边界内执行回调；回调返回 error 时回滚，否则提交
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// TxManager 表示本地事务管理器
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

// Exec 根据上下文返回当前可用的 GORM 实例（事务内返回 tx，否则返回 db）
func Exec(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := GetTx(ctx); ok {
		return tx
	}

	return db.WithContext(ctx)
}

// Execute 在本地事务中执行回调，回调返回 error 时自动回滚，否则自动提交
func (m *TxManager) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := WithTx(ctx, tx)
		return fn(txCtx)
	})
}
