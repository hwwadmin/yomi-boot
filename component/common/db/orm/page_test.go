// 分页辅助的单元测试
//
// @author zwei

package orm

import (
	"testing"

	"gorm.io/gorm"
)

// TestApplyPage_InvalidOrderReturnsError 验证非法排序字段时分页返回错误
func TestApplyPage_InvalidOrderReturnsError(t *testing.T) {
	_, err := ApplyPage(&gorm.DB{}, 1, 10, OrderDesc("id; DROP TABLE users"))
	if err == nil {
		t.Fatal("期望非法排序字段返回错误")
	}
}
