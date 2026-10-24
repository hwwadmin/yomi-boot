// Package tx 提供本地事务管理：Executor 契约、Manager 实现与 Context 事务传递。
//
// usecase 通过 Executor.Execute 控制事务边界；Repository 通过 Exec(ctx, root) 获取当前事务连接。
// 事务按数据源根句柄隔离，不同数据源的事务互不串用。
//
// @author zwei
package tx
