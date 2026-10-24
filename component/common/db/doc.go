// Package db 提供数据库数据源能力：配置校验、DSN 构建、连接池参数、最小空闲预热与保活，
// 并以 Conn 收口连接池生命周期。
//
// 子包一览：
//   - gitee.com/hq_project/yomi-boot/component/common/db/orm：GORM 持久化基类（BaseModel、BaseRepository、分页、LIKE 与 SQL 标识符校验）
//   - gitee.com/hq_project/yomi-boot/component/common/db/tx：本地事务管理器与 Context 事务传递
//   - gitee.com/hq_project/yomi-boot/component/common/db/dbtest：基于 go-mysql-mocker 的测试辅助
//
// 业务代码按需 import 子包：Model 内嵌 orm.BaseModel，Repository 内嵌 orm.BaseRepository 并通过 tx.Exec
// 感知事务，usecase 通过 tx.Runner 控制事务边界（需要自治事务时用 tx.Executor）；只有启动装配层使用
// 本包的 Config/Open/MustOpen。
//
// @author zwei
package db
