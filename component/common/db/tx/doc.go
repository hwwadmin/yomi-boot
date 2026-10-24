// Package tx 提供本地事务管理：Runner/Executor 契约、Manager 实现与 Context 事务传递。
//
// usecase 通过 Runner.Run 控制事务边界，需要自治事务时使用 Executor.RunAutonomous；
// Repository 通过 Exec(ctx, root) 获取当前事务连接。事务按数据源根句柄隔离，不同数据源互不串用。
//
// @author zwei
package tx
