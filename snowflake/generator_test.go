package snowflake

import (
	"testing"

	"gitee.com/hq_project/yomi-boot/snowflake/snowflaketest"
)

var _ Generator = (*snowflaketest.TestGenerator)(nil)

// TestNewGenerator_ValidWorkerIDs 验证合法 worker ID 能正常创建生成器
func TestNewGenerator_ValidWorkerIDs(t *testing.T) {
	validIDs := []int64{0, 1, 512, 1023}
	for _, id := range validIDs {
		gen, err := NewGenerator(id)
		if err != nil {
			t.Fatalf("NewGenerator(%d) 期望成功，实际错误: %v", id, err)
		}
		if gen == nil {
			t.Fatalf("NewGenerator(%d) 返回 nil 生成器", id)
		}
	}
}

// TestNewGenerator_InvalidWorkerIDs 验证非法 worker ID 返回错误
func TestNewGenerator_InvalidWorkerIDs(t *testing.T) {
	invalidIDs := []int64{-1, 1024, 9999}
	for _, id := range invalidIDs {
		gen, err := NewGenerator(id)
		if err == nil {
			t.Fatalf("NewGenerator(%d) 期望错误，实际成功返回: %v", id, gen)
		}
	}
}

// TestGenerator_NextID_Uniqueness 验证连续生成的 ID 不重复
func TestGenerator_NextID_Uniqueness(t *testing.T) {
	gen, err := NewGenerator(0)
	if err != nil {
		t.Fatalf("NewGenerator(0) 失败: %v", err)
	}

	seen := make(map[int64]bool)
	for i := 0; i < 1000; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i, err)
		}
		if id == 0 {
			t.Fatalf("第 %d 次调用返回 0，可能生成器异常", i)
		}
		if seen[id] {
			t.Fatalf("第 %d 次调用返回重复 ID: %d", i, id)
		}
		seen[id] = true
	}
}

// TestGenerator_NextID_Monotonic 验证生成的 ID 单调递增
func TestGenerator_NextID_Monotonic(t *testing.T) {
	gen, err := NewGenerator(1)
	if err != nil {
		t.Fatalf("NewGenerator(1) 失败: %v", err)
	}

	prev := int64(0)
	for i := 0; i < 100; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i, err)
		}
		if id <= prev {
			t.Fatalf("第 %d 次调用 ID=%d 不大于前值 %d，期望单调递增", i, id, prev)
		}
		prev = id
	}
}

// TestTestGenerator_NextID 验证测试用生成器生成的 ID 唯一且单调递增
func TestTestGenerator_NextID(t *testing.T) {
	g := snowflaketest.NewTestGenerator()

	seen := make(map[int64]bool)
	prev := int64(0)
	for i := 0; i < 100; i++ {
		id, err := g.NextID()
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i, err)
		}
		if id == 0 {
			t.Fatalf("第 %d 次调用返回 0，可能生成器异常", i)
		}
		if seen[id] {
			t.Fatalf("第 %d 次调用返回重复 ID: %d", i, id)
		}
		if id <= prev {
			t.Fatalf("第 %d 次调用 ID=%d 不大于前值 %d，期望单调递增", i, id, prev)
		}
		seen[id] = true
		prev = id
	}
}

// TestTestGenerator_ImplementsGenerator 编译期校验 TestGenerator 实现 Generator 接口
func TestTestGenerator_ImplementsGenerator(t *testing.T) {
	var g Generator = snowflaketest.NewTestGenerator()
	id, err := g.NextID()
	if err != nil {
		t.Fatalf("TestGenerator.NextID 失败: %v", err)
	}
	if id == 0 {
		t.Fatal("TestGenerator.NextID 不应返回 0")
	}
}
