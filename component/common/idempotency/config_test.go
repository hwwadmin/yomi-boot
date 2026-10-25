// 业务幂等配置、键与快照序列化的单元测试
//
// @author zwei
package idempotency

import (
	"strings"
	"testing"
	"time"
)

// TestConfigDefaults 验证未配置字段的默认值
func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	if got := cfg.ScanIntervalOrDefault(); got != 10*time.Minute {
		t.Fatalf("ScanInterval 默认期望 10m，实际 %v", got)
	}
	if got := cfg.StaleTimeoutOrDefault(); got != 60*time.Minute {
		t.Fatalf("StaleTimeout 默认期望 60m，实际 %v", got)
	}
	if got := cfg.BatchSizeOrDefault(); got != 100 {
		t.Fatalf("BatchSize 默认期望 100，实际 %d", got)
	}
}

// TestConfigOverrides 验证已配置字段原样生效
func TestConfigOverrides(t *testing.T) {
	cfg := Config{ScanIntervalMinutes: 3, StaleTimeoutMinutes: 30, BatchSize: 20}
	if got := cfg.ScanIntervalOrDefault(); got != 3*time.Minute {
		t.Fatalf("ScanInterval 期望 3m，实际 %v", got)
	}
	if got := cfg.StaleTimeoutOrDefault(); got != 30*time.Minute {
		t.Fatalf("StaleTimeout 期望 30m，实际 %v", got)
	}
	if got := cfg.BatchSizeOrDefault(); got != 20 {
		t.Fatalf("BatchSize 期望 20，实际 %d", got)
	}
}

// TestConfigValidate 验证配置校验的各错误分支
func TestConfigValidate(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("空配置应取默认值并通过校验: %v", err)
	}
	if err := (Config{ScanIntervalMinutes: 2, StaleTimeoutMinutes: 10, BatchSize: 5}).Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "scan_interval 为负", cfg: Config{ScanIntervalMinutes: -1}},
		{name: "stale_timeout 为负", cfg: Config{StaleTimeoutMinutes: -1}},
		{name: "batch_size 为负", cfg: Config{BatchSize: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}

// TestStaleCleanerConfigFrom 验证运行时配置解析与服务名裁剪
func TestStaleCleanerConfigFrom(t *testing.T) {
	got := StaleCleanerConfigFrom(Config{ScanIntervalMinutes: 2, StaleTimeoutMinutes: 15, BatchSize: 30}, " app ")
	if got.ScanInterval != 2*time.Minute || got.StaleTimeout != 15*time.Minute || got.BatchSize != 30 {
		t.Fatalf("配置映射不符: %+v", got)
	}
	if got.ServiceName != "app" {
		t.Fatalf("服务名应裁剪空白，实际 %q", got.ServiceName)
	}

	def := StaleCleanerConfigFrom(Config{}, "")
	if def.ScanInterval != 10*time.Minute || def.StaleTimeout != 60*time.Minute || def.BatchSize != 100 {
		t.Fatalf("空配置应取默认值: %+v", def)
	}
}

// TestStaleCleanerRoundRobinName 验证轮询名称构建与空服务名兜底
func TestStaleCleanerRoundRobinName(t *testing.T) {
	if got := staleCleanerRoundRobinName(" svc "); got != "idempotencyStaleProcessing_svc" {
		t.Fatalf("轮询名称不符: %q", got)
	}
	if got := staleCleanerRoundRobinName(""); got != "idempotencyStaleProcessing_unknown" {
		t.Fatalf("空服务名应兜底 unknown: %q", got)
	}
}

// TestBizID 验证幂等 ID 生成非空且互不重复
func TestBizID(t *testing.T) {
	first := BizID()
	second := BizID()
	if first == "" || second == "" {
		t.Fatal("幂等 ID 不应为空")
	}
	if first == second {
		t.Fatal("幂等 ID 不应重复")
	}
}

// TestMarshalAndUnmarshalResult 验证结果快照序列化的边界行为
func TestMarshalAndUnmarshalResult(t *testing.T) {
	data, err := marshalResult(map[string]int{"n": 1})
	if err != nil || string(data) != `{"n":1}` {
		t.Fatalf("序列化结果不符: %s, %v", data, err)
	}

	// null 结果返回 nil 字节
	var nullValue *string
	data, err = marshalResult[*string](nullValue)
	if err != nil {
		t.Fatalf("null 结果序列化失败: %v", err)
	}
	if data != nil {
		t.Fatalf("null 结果应返回 nil 字节，实际 %s", data)
	}

	// 不可序列化对象报错
	if _, err := marshalResult(map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("不可序列化对象应返回错误")
	}

	// 空数据反序列化为零值
	got, err := unmarshalResult[string](nil)
	if err != nil || got != "" {
		t.Fatalf("空数据应返回零值: %q, %v", got, err)
	}

	// 合法数据往返
	got, err = unmarshalResult[string]([]byte(`"round-trip"`))
	if err != nil || got != "round-trip" {
		t.Fatalf("反序列化结果不符: %q, %v", got, err)
	}

	// 非法 JSON 报错
	if _, err := unmarshalResult[string]([]byte("not-json")); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// TestRedisKeys 验证结果缓存与分布式锁 Key 拼接
func TestRedisKeys(t *testing.T) {
	key := Key{BizID: "b1", BizSubID: "s1"}
	if got := resultRedisKey(key); got != "sysbiz:result:b1:s1" {
		t.Fatalf("结果缓存 Key 不符: %q", got)
	}
	if got := lockRedisKey(key); !strings.HasPrefix(got, "idempotency:lock:b1:s1") {
		t.Fatalf("锁 Key 不符: %q", got)
	}
}
