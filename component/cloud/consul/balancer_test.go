// Package consul 的负载均衡器单测

package consul

import (
	"strings"
	"testing"
)

// TestRoundRobinBalancerPick 验证轮询均衡器的空实例报错与顺序轮询
func TestRoundRobinBalancerPick(t *testing.T) {
	t.Parallel()

	b := NewRoundRobinBalancer()
	instances := []ServiceInstance{
		{Address: "10.0.0.1", Port: 8080},
		{Address: "10.0.0.2", Port: 8081},
	}

	// 轮询顺序 0,1,0,1
	for i, want := range []ServiceInstance{instances[0], instances[1], instances[0], instances[1]} {
		got, err := b.Pick("svc", instances)
		if err != nil {
			t.Fatalf("第 %d 次 Pick 失败: %v", i+1, err)
		}
		if got != want {
			t.Fatalf("第 %d 次 Pick 期望 %v，实际 %v", i+1, want, got)
		}
	}

	// 不同服务名各自独立轮询
	if got, _ := b.Pick("other", instances); got != instances[0] {
		t.Fatalf("新服务名应从 0 开始轮询，实际 %v", got)
	}
	if got, _ := b.Pick("svc", instances); got != instances[0] {
		t.Fatalf("svc 的第 5 次轮询应回到 0，实际 %v", got)
	}
}

// TestRoundRobinBalancerPickEmpty 验证无可用实例时返回错误
func TestRoundRobinBalancerPickEmpty(t *testing.T) {
	t.Parallel()

	_, err := NewRoundRobinBalancer().Pick("svc", nil)
	if err == nil || !strings.Contains(err.Error(), "无可用实例") {
		t.Fatalf("空实例列表应返回可读错误，实际: %v", err)
	}
}

// TestWeightedRandomBalancerPickEmpty 验证加权随机均衡器空实例报错
func TestWeightedRandomBalancerPickEmpty(t *testing.T) {
	t.Parallel()

	_, err := NewWeightedRandomBalancer().Pick("svc", nil)
	if err == nil || !strings.Contains(err.Error(), "无可用实例") {
		t.Fatalf("空实例列表应返回可读错误，实际: %v", err)
	}
}

// TestWeightedRandomBalancerSingleInstance 验证单实例（含零权重）总被选中
func TestWeightedRandomBalancerSingleInstance(t *testing.T) {
	t.Parallel()

	want := ServiceInstance{Address: "10.0.0.1", Port: 8080, Weight: 0}
	b := NewWeightedRandomBalancer()
	for i := 0; i < 50; i++ {
		got, err := b.Pick("svc", []ServiceInstance{want})
		if err != nil {
			t.Fatalf("第 %d 次 Pick 失败: %v", i+1, err)
		}
		if got != want {
			t.Fatalf("单实例应总被选中，实际 %v", got)
		}
	}
}

// TestWeightedRandomBalancerZeroWeights 验证权重 <=0 视为 1，不会 panic 且总返回合法实例
func TestWeightedRandomBalancerZeroWeights(t *testing.T) {
	t.Parallel()

	instances := []ServiceInstance{
		{Address: "10.0.0.1", Port: 8080, Weight: -3},
		{Address: "10.0.0.2", Port: 8081, Weight: 0},
		{Address: "10.0.0.3", Port: 8082, Weight: 0},
	}
	b := NewWeightedRandomBalancer()
	for i := 0; i < 200; i++ {
		got, err := b.Pick("svc", instances)
		if err != nil {
			t.Fatalf("第 %d 次 Pick 失败: %v", i+1, err)
		}
		if got.Address == "" || got.Port <= 0 {
			t.Fatalf("应返回合法实例，实际 %v", got)
		}
	}
}

// TestWeightedRandomBalancerDistribution 验证权重比例决定选中概率（9:1 权重约 9:1 分布）
func TestWeightedRandomBalancerDistribution(t *testing.T) {
	t.Parallel()

	heavy := ServiceInstance{Address: "10.0.0.1", Port: 8080, Weight: 9}
	light := ServiceInstance{Address: "10.0.0.2", Port: 8081, Weight: 1}
	b := NewWeightedRandomBalancer()

	const n = 2000
	heavyCount := 0
	for i := 0; i < n; i++ {
		got, err := b.Pick("svc", []ServiceInstance{heavy, light})
		if err != nil {
			t.Fatalf("第 %d 次 Pick 失败: %v", i+1, err)
		}
		if got == heavy {
			heavyCount++
		}
	}
	// p=0.9, n=2000 时标准差约 13.4，边界 [0.8, 0.95] 远在 ±6σ 之外
	ratio := float64(heavyCount) / n
	if ratio < 0.8 || ratio > 0.95 {
		t.Fatalf("权重 9:1 的选中比例期望约 0.9，实际 %d/%d=%.3f", heavyCount, n, ratio)
	}
}
