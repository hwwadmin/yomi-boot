// Package db 提供数据库辅助方法（LIKE 查询、GORM 分页）
//
// @author zwei
package db

import (
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/shared"
	"gitee.com/hq_project/yomi-boot/shared/enums"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// likeEscape 对 LIKE 模式中的特殊字符进行转义
// 数据库 LIKE 默认使用 \ 作为转义字符，需转义 \、%、_ 三个字符
func likeEscape(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString("\\\\")
		case '%':
			builder.WriteString("\\%")
		case '_':
			builder.WriteString("\\_")
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

// LikeLeft 拼接左模糊 LIKE 模式：%value
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeLeft(value string) string {
	return "%" + likeEscape(value)
}

// LikeRight 拼接右模糊 LIKE 模式：value%
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeRight(value string) string {
	return likeEscape(value) + "%"
}

// LikeFull 拼接全模糊 LIKE 模式：%value%
// 返回的字符串已对 LIKE 通配符做转义，可直接作为参数化查询的值
func LikeFull(value string) string {
	return "%" + likeEscape(value) + "%"
}

// ApplyPage 应用分页与结构化排序
// page 从 1 开始；若 page <= 0 则默认为 1
// pageSize 若 <= 0 则默认为 10，若超过 shared.MaxPageSize 则截断
func ApplyPage(query *gorm.DB, page, pageSize int, orders ...OrderClause) (*gorm.DB, error) {
	if page <= 0 {
		page = shared.DefaultPage
	}
	if pageSize <= 0 {
		pageSize = shared.DefaultPageSize
	}
	if pageSize > shared.MaxPageSize {
		pageSize = shared.MaxPageSize
	}
	offset := (page - 1) * pageSize

	paged, err := applyOrders(query, orders...)
	if err != nil {
		return nil, err
	}
	return paged.Limit(pageSize).Offset(offset), nil
}

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
