// Package orm 提供 GORM 持久化基类：BaseModel、BaseRepository、分页与 LIKE 辅助、SQL 标识符校验。
//
// 业务 Model 内嵌 orm.BaseModel；Repository 内嵌 orm.BaseRepository，并通过 tx.Exec 感知事务。
// 本包不负责连接创建与事务边界。
//
// @author zwei
package orm
