// Package snowflaketest 的测试辅助生成器自测
//
// @author zwei
package snowflaketest

import "testing"

// TestNewTestGenerator 验证测试生成器产出唯一且递增的 ID
func TestNewTestGenerator(t *testing.T) {
	gen := NewTestGenerator()
	if gen == nil {
		t.Fatal("应返回生成器实例")
	}

	prev, err := gen.NextID()
	if err != nil || prev <= 0 {
		t.Fatalf("首次生成应返回有效 ID: id=%d err=%v", prev, err)
	}
	seen := map[int64]struct{}{prev: {}}
	for i := 0; i < 100; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("生成 ID 失败: %v", err)
		}
		if id <= prev {
			t.Fatalf("ID 应单调递增: prev=%d cur=%d", prev, id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("ID 不应重复: %d", id)
		}
		seen[id] = struct{}{}
		prev = id
	}
}

// TestTestGeneratorNilSafe 验证未初始化生成器的错误兜底
func TestTestGeneratorNilSafe(t *testing.T) {
	var gen *TestGenerator
	if _, err := gen.NextID(); err == nil {
		t.Fatal("nil 生成器应返回错误")
	}
	empty := TestGenerator{}
	if _, err := empty.NextID(); err == nil {
		t.Fatal("未初始化节点应返回错误")
	}
}
