// 事务上下文传递
//
// @author zwei

package tx

import (
	"context"

	"gorm.io/gorm"
)

// ctxKey 以数据源根句柄为键，隔离不同数据源的事务上下文
type ctxKey struct {
	root *gorm.DB
}

// WithContext 将数据源 root 的事务实例 gtx 写入上下文
func WithContext(ctx context.Context, root, gtx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxKey{root: root}, gtx)
}

// FromContext 读取数据源 root 当前的事务实例
func FromContext(ctx context.Context, root *gorm.DB) (*gorm.DB, bool) {
	gtx, ok := ctx.Value(ctxKey{root: root}).(*gorm.DB)
	return gtx, ok
}

// Exec 返回数据源 root 当前可用的 GORM 实例：事务内返回事务实例，否则返回根句柄；
// 两种情况都应用调用方 ctx，使超时、取消与 ctx 值生效
func Exec(ctx context.Context, root *gorm.DB) *gorm.DB {
	if gtx, ok := FromContext(ctx, root); ok {
		return gtx.WithContext(ctx)
	}
	return root.WithContext(ctx)
}
