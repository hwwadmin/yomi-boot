// Fail Fast 初始化的单元测试
//
// @author zwei
package snowflake

import "testing"

// TestMustNewGenerator 验证 Fail Fast 构造的成功与失败路径
func TestMustNewGenerator(t *testing.T) {
	gen := MustNewGenerator(1)
	if gen == nil {
		t.Fatal("合法 workerID 应返回生成器")
	}
	id, err := gen.NextID()
	if err != nil || id <= 0 {
		t.Fatalf("生成器应产出有效 ID: id=%d err=%v", id, err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("非法 workerID 应 panic")
		}
	}()
	MustNewGenerator(1024)
}
