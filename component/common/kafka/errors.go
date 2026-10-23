package kafka

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrTopicNotFound topic 不存在（DescribeTopic / DeleteTopic）
	ErrTopicNotFound = errors.NewNoStack("kafka: topic 不存在")

	// ErrTopicAlreadyExists topic 已存在（CreateTopic）
	ErrTopicAlreadyExists = errors.NewNoStack("kafka: topic 已存在")
)
