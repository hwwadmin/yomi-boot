// Package db 提供数据库数据源能力：配置校验、DSN 构建、连接池参数、最小空闲预热与保活，
// 并以 Conn 收口连接池生命周期。
//
// 子包一览：
//   - github.com/hwwadmin/yomi-boot/component/common/db/tx：本地事务执行器与 Context 事务传递
//   - github.com/hwwadmin/yomi-boot/component/common/db/dbtest：基于 go-mysql-mocker 的测试辅助
//
// 业务代码按需 import 子包：通过 tx.Exec 感知事务，调用方（通常为 usecase）通过 tx.Runner 控制事务边界
// （需要自治事务时用 tx.Executor）；只有启动装配层使用本包的 Config/Open/MustOpen。
//
// @author zwei
package db
