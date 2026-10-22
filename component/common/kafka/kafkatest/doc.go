// Package kafkatest 提供 gitee.com/hq_project/yomi-boot/component/common/kafka 的单元测试辅助，模式对齐 gitee.com/hq_project/yomi-boot/component/common/redis/redistest。
//
// # 依赖边界（重要）
//
// 生产代码（gitee.com/hq_project/yomi-boot/component/common/kafka 及业务服务）仅使用 github.com/segmentio/kafka-go。
// 本包是唯一的 franz-go 引用点：底层通过 github.com/twmb/franz-go/pkg/kfake
// 启动内存级 wire-compatible broker，供 kafka-go 客户端连接。
//
// franz-go 不得用于生产路径；禁止在 kafka/ 主包或 boot/ 中 import franz-go。
// 业务测试应 import gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest，而非直接使用 franz-go。
//
// # 使用方式
//
//	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "topic"))
//	publisher := kafkatest.NewPublisher(t, broker)
//	consumer := kafkatest.NewConsumerManager(t, broker)
//
// @author zwei
package kafkatest
