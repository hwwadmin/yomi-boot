// Host 定义插件初始化阶段的共享上下文。

package plugin

import (
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"

	redisv9 "github.com/redis/go-redis/v9"
)

// Host 插件 Init 阶段共享上下文：Services 供业务装配读取，原始句柄仅在初始化期使用。
// MustInit 成功后只把 Host.Services 交给业务；原始连接与组件不得越过此处。
type Host struct {
	Services

	RedisClient           *redisv9.Client         // 仅内置 redis 插件写入、redisstream 插件读取
	KafkaComponents       *yomikafka.Components   // 仅内置 kafka 插件写入与关闭
	RedisStreamComponents *redisstream.Components // 仅内置 redisstream 插件写入与关闭
}
