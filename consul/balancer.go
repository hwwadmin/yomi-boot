// Package consul 提供负载均衡策略：定义服务实例选择接口，实现轮询与加权随机两种均衡器
package consul

import (
	"math/rand"
	"sync"
	"time"
	"yt-boot/errors"
)

// ServiceInstance 表示一个可路由的服务实例
type ServiceInstance struct {
	Address string
	Port    int
	Weight  int // 实例权重，默认 1，值越大分配到的请求越多
}

// LoadBalancer 从健康实例列表中选择一个实例，策略可插拔替换
type LoadBalancer interface {
	Pick(serviceName string, instances []ServiceInstance) (ServiceInstance, error)
}

// RoundRobinBalancer 默认轮询负载均衡策略
type RoundRobinBalancer struct {
	mu   sync.Mutex
	next map[string]uint64
}

// NewRoundRobinBalancer 创建轮询负载均衡器
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{next: make(map[string]uint64)}
}

// Pick 按服务名轮询选择实例
func (b *RoundRobinBalancer) Pick(serviceName string, instances []ServiceInstance) (ServiceInstance, error) {
	// 无可用实例直接报错
	if len(instances) == 0 {
		return ServiceInstance{}, errors.Newf("无可用实例: %s", serviceName)
	}
	// 加锁递增轮询计数器并取模选择
	b.mu.Lock()
	idx := b.next[serviceName]
	b.next[serviceName] = idx + 1
	b.mu.Unlock()
	return instances[idx%uint64(len(instances))], nil
}

// WeightedRandomBalancer 加权随机负载均衡策略，按实例权重比例分配请求
type WeightedRandomBalancer struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewWeightedRandomBalancer 创建加权随机负载均衡器
func NewWeightedRandomBalancer() *WeightedRandomBalancer {
	return &WeightedRandomBalancer{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Pick 按权重随机选择实例，权重越大的实例被选中的概率越高
func (b *WeightedRandomBalancer) Pick(serviceName string, instances []ServiceInstance) (ServiceInstance, error) {
	// 无可用实例直接报错
	if len(instances) == 0 {
		return ServiceInstance{}, errors.Newf("无可用实例: %s", serviceName)
	}

	// 计算总权重，权重 <= 0 视为 1
	totalWeight := 0
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	// 加权随机选择：在 [0, totalWeight) 范围内取随机数，按累计权重定位
	b.mu.Lock()
	r := b.rng.Intn(totalWeight)
	b.mu.Unlock()

	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		r -= w
		if r < 0 {
			return inst, nil
		}
	}
	// 兜底返回最后一个实例（浮点累积误差等极端情况）
	return instances[len(instances)-1], nil
}
