// Package db 提供 SQL 标识符校验与结构化排序子句
//
// @author zwei
package db

import (
	"regexp"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxSQLIdentifierLen = 64

var sqlIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

var sqlReservedWords = map[string]struct{}{
	"select": {}, "insert": {}, "update": {}, "delete": {}, "drop": {},
	"union": {}, "sleep": {}, "benchmark": {}, "extractvalue": {},
	"updatexml": {}, "load_file": {}, "into": {}, "outfile": {},
}

// OrderClause 表示一条结构化排序子句，避免将原始 ORDER BY 字符串拼入 SQL
type OrderClause struct {
	Column string
	Desc   bool
}

// OrderAsc 构造升序排序子句
func OrderAsc(column string) OrderClause {
	return OrderClause{Column: column, Desc: false}
}

// OrderDesc 构造降序排序子句
func OrderDesc(column string) OrderClause {
	return OrderClause{Column: column, Desc: true}
}

// validate 校验排序字段是否为安全的 SQL 标识符
func (o OrderClause) validate() error {
	return ValidateSQLIdentifier(o.Column)
}

// ValidateSQLIdentifier 校验列名是否为安全的 SQL 标识符
func ValidateSQLIdentifier(name string) error {
	if name == "" {
		return errors.New("SQL 标识符不能为空")
	}
	if len(name) > maxSQLIdentifierLen {
		return errors.Newf("SQL 标识符过长: %s", name)
	}
	if !sqlIdentifierPattern.MatchString(name) {
		return errors.Newf("非法 SQL 标识符: %s", name)
	}
	if _, reserved := sqlReservedWords[strings.ToLower(name)]; reserved {
		return errors.Newf("SQL 保留字不可作为标识符: %s", name)
	}
	return nil
}

// applyOrders 将一组 OrderClause 安全地应用到 GORM 查询中
func applyOrders(query *gorm.DB, orders ...OrderClause) (*gorm.DB, error) {
	for _, order := range orders {
		if err := order.validate(); err != nil {
			return nil, errors.Wrap(err, "非法排序字段")
		}
		query = query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: order.Column},
			Desc:   order.Desc,
		})
	}
	return query, nil
}
