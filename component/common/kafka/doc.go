// Package kafka 提供 Kafka 客户端连接、消息收发与 topic 管理封装（生产实现基于 segmentio/kafka-go）。
//
// 发布者、消费者与管理端独立构造、独立校验、独立关闭；管理端提供集群元数据、topic 列表、
// topic 详情以及 topic 创建/删除能力。单元测试请使用
// gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest，franz-go 仅允许出现在 kafkatest 包内。
//
// @author zwei
package kafka
