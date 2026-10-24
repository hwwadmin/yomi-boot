// 数据库方言错误映射
//
// @author zwei

package db

import (
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// IsDuplicateKey 根据配置的数据库类型判断错误是否为唯一键冲突
func IsDuplicateKey(err error, dbType enums.DbType) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	switch dbType {
	case enums.MySQL:
		if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok {
			return mysqlErr.Number == 1062
		}
	case enums.Postgres:
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			return pgErr.Code == "23505"
		}
	}
	return false
}
