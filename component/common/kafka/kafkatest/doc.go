// Package kafkatest 提供 github.com/hwwadmin/yomi-boot/component/common/kafka 的单元测试辅助，模式对齐 github.com/hwwadmin/yomi-boot/component/common/redis/redistest。
//
// 每个测试进程共用一个 apache/kafka:4.2.2 容器。NewMockBroker 创建本用例的主题，
// 用例结束时删除这些主题和消费组，不停止容器。容器在测试进程退出时由 Testcontainers 回收。
//
// 生产代码只使用 github.com/segmentio/kafka-go。业务测试应 import 本包。
//
// # 使用方式
//
//	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "topic"))
//	publisher := kafkatest.NewPublisher(t, broker)
//	consumer := kafkatest.NewConsumerManager(t, broker)
//
// @author zwei
package kafkatest
