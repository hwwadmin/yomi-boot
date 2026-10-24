// Package dtm 提供 DTM Saga 分布式事务引擎集成能力
//
// @author zwei
package dtm

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"github.com/dtm-labs/client/dtmcli"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DefaultBarrierTableName 默认 DTM Barrier 表名
const DefaultBarrierTableName = "sys_barrier"

// barrierKey 是 DTM Barrier 在上下文中的键
type barrierKey struct{}

// TxRunner 运行本地事务，当上下文中包含 DTM Barrier 元数据时自动使用 Barrier 事务
type TxRunner struct {
	root      *gorm.DB
	runner    tx.Runner
	dtmDBType string
}

var _ tx.Runner = (*TxRunner)(nil)

// NewTxRunner 创建支持 Saga 事务的事务运行器。
// root 必须是数据源根句柄，普通事务与 barrier 事务绑定同一数据源；
// barrierTableName 为空时使用 DefaultBarrierTableName。
func NewTxRunner(root *gorm.DB, dbType enums.DbType, barrierTableName string) *TxRunner {
	if root == nil {
		panic("dtm: NewTxRunner root 不能为 nil")
	}
	if barrierTableName == "" {
		barrierTableName = DefaultBarrierTableName
	}
	dtmcli.SetBarrierTableName(barrierTableName)
	return &TxRunner{root: root, runner: tx.NewManager(root), dtmDBType: toDBType(dbType)}
}

// Run 在普通事务或 DTM Barrier 事务中执行 fn
func (r *TxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	if bb, ok := BarrierFromContext(ctx); ok {
		return r.runBarrier(ctx, bb, fn)
	}
	return r.runner.Run(ctx, fn)
}

// runBarrier 在 DTM Barrier 事务中执行回调函数
func (r *TxRunner) runBarrier(ctx context.Context, bb *dtmcli.BranchBarrier, fn func(ctx context.Context) error) error {
	bb.DBType = r.dtmDBType
	gtx := r.root.WithContext(ctx).Begin()
	if gtx.Error != nil {
		return errors.Wrap(gtx.Error, "开启 DTM Barrier 事务失败")
	}
	sqlTx, ok := gtx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		_ = gtx.Rollback()
		return errors.Newf("DTM Barrier 无法获取 *sql.Tx，ConnPool 实际类型: %T", gtx.Statement.ConnPool)
	}
	return bb.Call(sqlTx, func(*sql.Tx) error {
		txCtx := tx.WithContext(ctx, r.root, gtx)
		return fn(txCtx)
	})
}

// BarrierFromContext 从上下文中读取 DTM 分支 Barrier（从 HTTP 回调中解析而来）
func BarrierFromContext(ctx context.Context) (*dtmcli.BranchBarrier, bool) {
	bb, ok := ctx.Value(barrierKey{}).(*dtmcli.BranchBarrier)
	return bb, ok && bb != nil
}

// ContextWithBarrier 将查询字符串中的 DTM Barrier 元数据附加到上下文
func ContextWithBarrier(ctx context.Context, query url.Values) (context.Context, error) {
	bb, err := dtmcli.BarrierFromQuery(query)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, barrierKey{}, bb), nil
}

// ContextWithRequest 从 HTTP 请求中附加 DTM Barrier 到上下文
func ContextWithRequest(ctx context.Context, req *http.Request) (context.Context, error) {
	if req == nil {
		return ctx, nil
	}
	return ContextWithBarrier(ctx, req.URL.Query())
}

// GinContextWithBarrier 从 Gin 请求中附加 DTM Barrier 到上下文
func GinContextWithBarrier(c *gin.Context) (context.Context, error) {
	if c == nil || c.Request == nil {
		return context.Background(), nil
	}
	return ContextWithRequest(c.Request.Context(), c.Request)
}

// toDBType 将内部数据库类型转换为 DTM 驱动识别的类型字符串
func toDBType(dbType enums.DbType) string {
	switch dbType {
	case enums.MySQL:
		return dtmcli.DBTypeMysql
	case enums.Postgres:
		return dtmcli.DBTypePostgres
	default:
		panic(fmt.Sprintf("不支持的数据库类型: %s", dbType))
	}
}
