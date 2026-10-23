package kafka

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	kafkago "github.com/segmentio/kafka-go"
)

// connection 表示一次 Kafka 连接所需的 broker 列表与拨号器
type connection struct {
	brokers []string        // brokers 是规范化后的 broker 地址列表
	dialer  *kafkago.Dialer // dialer 是带 SASL/TLS 能力的连接拨号器
}

// newConnection 规范化 broker 列表并构建拨号器
func newConnection(cfg ConnectionConfig) (connection, error) {
	dialer, err := buildDialer(cfg)
	if err != nil {
		return connection{}, err
	}
	return connection{
		brokers: normalizeBrokers(cfg.Brokers),
		dialer:  dialer,
	}, nil
}

// normalizeBrokers 去除 broker 地址首尾空白并过滤空地址
func normalizeBrokers(brokers []string) []string {
	result := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		if trimmed := strings.TrimSpace(broker); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// validateConnection 在连接超时内逐个拨号验证 broker 可达
func validateConnection(conn connection, timeout time.Duration) error {
	if conn.dialer == nil {
		return errors.New("kafka dialer 未初始化")
	}

	ctx, cancel := appctx.BackgroundWithTimeout(timeout)
	defer cancel()

	var joined error
	for _, broker := range conn.brokers {
		rawConn, err := conn.dialer.DialContext(ctx, "tcp", broker)
		if err != nil {
			joined = errors.Join(joined, errors.Wrap(err, "连接 broker %s 失败", broker))
			continue
		}
		_ = rawConn.Close()
	}
	return joined
}
