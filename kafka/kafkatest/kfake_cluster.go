// Package kafkatest 提供 Kafka 内存测试集群封装，本文件使用 franz-go 启动测试用内存 broker
//
// franz-go 仅允许在本文件（kafkatest 包）中使用。生产路径请使用 yomi-boot/kafka（基于 segmentio/kafka-go）。
//
// @author zwei
package kafkatest

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kfake"
)

// kfakeOpt 是 kfake 集群选项类型别名
type kfakeOpt = kfake.Opt

// kfakeCluster 封装 kfake 内存集群实例
type kfakeCluster struct {
	cluster *kfake.Cluster
}

// kfakeOptNumBrokers 指定 kfake 集群 broker 节点数量
func kfakeOptNumBrokers(n int) kfakeOpt {
	return kfake.NumBrokers(n)
}

// kfakeOptSeedTopics 预创建主题并指定分区数
func kfakeOptSeedTopics(partitions int32, topics ...string) kfakeOpt {
	return kfake.SeedTopics(partitions, topics...)
}

// newKfakeCluster 创建 kfake 内存集群并在测试结束时自动关闭
func newKfakeCluster(t testing.TB, opts []kfakeOpt) *kfakeCluster {
	t.Helper()

	cluster, err := kfake.NewCluster(opts...)
	if err != nil {
		t.Fatalf("启动 kfake 集群失败: %v", err)
	}

	c := &kfakeCluster{cluster: cluster}
	t.Cleanup(c.close)
	return c
}

// addrs 返回集群监听地址列表
func (c *kfakeCluster) addrs() []string {
	if c == nil || c.cluster == nil {
		return nil
	}
	return append([]string(nil), c.cluster.ListenAddrs()...)
}

// close 关闭 kfake 集群并释放资源
func (c *kfakeCluster) close() {
	if c == nil || c.cluster == nil {
		return
	}
	c.cluster.Close()
	c.cluster = nil
}
