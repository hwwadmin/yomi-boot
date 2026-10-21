// Package db 提供 SQL 安全校验的单元测试
package db

import (
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/errors"

	"gorm.io/gorm"
)

// TestValidateSQLIdentifier 验证 SQL 标识符合法性与非法 payload 拒绝
func TestValidateSQLIdentifier(t *testing.T) {
	valid := []string{"id", "username", "request_at", "_hidden", "col2"}
	for _, name := range valid {
		if err := ValidateSQLIdentifier(name); err != nil {
			t.Errorf("期望合法标识符 %q，得到错误: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"1id",
		"id; DROP TABLE users",
		"id DESC",
		"select",
		"username OR 1=1",
	}
	for _, name := range invalid {
		if err := ValidateSQLIdentifier(name); err == nil {
			t.Errorf("期望非法标识符 %q 被拒绝", name)
		}
	}
}

// TestValidateSQLIdentifier_KeepsStack 验证错误经项目 errors 构造，保留调用栈
func TestValidateSQLIdentifier_KeepsStack(t *testing.T) {
	t.Parallel()

	err := ValidateSQLIdentifier("id; DROP TABLE users")
	if err == nil {
		t.Fatal("期望非法标识符返回错误")
	}
	if !strings.Contains(errors.Stack(err), "sql_safe.go") {
		t.Fatalf("期望错误保留调用栈，实际 %q", errors.Stack(err))
	}
}

// TestOrderClause_validate 验证排序子句标识符校验
func TestOrderClause_validate(t *testing.T) {
	for _, clause := range []OrderClause{OrderDesc("id"), OrderAsc("username")} {
		if err := clause.validate(); err != nil {
			t.Fatalf("合法排序子句 %v 不应报错: %v", clause, err)
		}
	}

	if err := OrderDesc("id; DROP TABLE users").validate(); err == nil {
		t.Fatal("期望注入 payload 被拒绝")
	}
}

// TestApplyPage_InvalidOrderReturnsError 验证非法排序字段时分页返回错误
func TestApplyPage_InvalidOrderReturnsError(t *testing.T) {
	_, err := ApplyPage(&gorm.DB{}, 1, 10, OrderDesc("id; DROP TABLE users"))
	if err == nil {
		t.Fatal("期望非法排序字段返回错误")
	}
}
