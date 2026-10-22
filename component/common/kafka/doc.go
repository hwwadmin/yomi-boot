// Package kafka 提供 Kafka 客户端连接与消息收发封装（生产实现基于 segmentio/kafka-go）。
//
// 发布者与消费者独立构造、独立校验、独立关闭；单元测试请使用
// gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest，franz-go 仅允许出现在 kafkatest 包内。
//
// @author zwei
package kafka
