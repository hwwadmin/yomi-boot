package kafka

import (
	"context"
	"sort"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	kafkago "github.com/segmentio/kafka-go"
)

// Broker 表示 Kafka broker 节点
type Broker struct {
	ID   int    // ID 是 broker 节点 ID
	Host string // Host 是 broker 主机名
	Port int    // Port 是 broker 端口
	Rack string // Rack 是机架标识
}

// ClusterMetadata 表示 Kafka 集群元数据
type ClusterMetadata struct {
	ClusterID  string   // ClusterID 是集群标识
	Controller Broker   // Controller 是控制器节点
	Brokers    []Broker // Brokers 是集群 broker 列表
}

// PartitionMetadata 表示 topic 分区元数据
type PartitionMetadata struct {
	ID       int      // ID 是分区编号
	Leader   Broker   // Leader 是分区 leader 节点
	Replicas []Broker // Replicas 是副本节点列表
	ISR      []Broker // ISR 是同步副本节点列表
}

// TopicMetadata 表示 topic 元数据
type TopicMetadata struct {
	Name       string              // Name 是 topic 名称
	Internal   bool                // Internal 表示是否为 Kafka 内部 topic
	Partitions []PartitionMetadata // Partitions 是分区列表
}

// TopicConfig 表示创建 topic 的参数
type TopicConfig struct {
	Topic             string            // Topic 是 topic 名称
	NumPartitions     int               // NumPartitions 是分区数，-1 表示使用 broker 默认值
	ReplicationFactor int               // ReplicationFactor 是副本数，-1 表示使用 broker 默认值
	ConfigEntries     map[string]string // ConfigEntries 是 topic 级配置
}

// AdminService 表示 Kafka 元数据与 topic 管理能力接口
type AdminService interface {
	// Metadata 查询集群实时元数据（集群标识、控制器与 broker 列表）
	Metadata(ctx context.Context) (*ClusterMetadata, error)
	// ListTopics 查询全部 topic 名称（实时，含 Kafka 内部 topic），结果按名称升序
	ListTopics(ctx context.Context) ([]string, error)
	// DescribeTopic 查询指定 topic 实时元数据；topic 不存在返回 ErrTopicNotFound
	DescribeTopic(ctx context.Context, topic string) (*TopicMetadata, error)
	// CreateTopic 创建 topic；topic 已存在返回 ErrTopicAlreadyExists
	CreateTopic(ctx context.Context, cfg TopicConfig) error
	// DeleteTopic 删除 topic；topic 不存在返回 ErrTopicNotFound
	DeleteTopic(ctx context.Context, topic string) error
	// Close 释放空闲连接，幂等安全
	Close() error
}

// admin 封装 Kafka 管理端
type admin struct {
	client    *kafkago.Client    // client 是 Kafka 管理协议客户端
	transport *kafkago.Transport // transport 是管理端独占传输层
	closeMu   sync.Mutex         // closeMu 保护 closed 并发调用
	closed    bool               // closed 标记是否已关闭
}

var _ AdminService = (*admin)(nil)

// NewAdmin 根据连接配置创建 Kafka 管理端；ctx 约束连接验证
func NewAdmin(ctx context.Context, cfg ConnectionConfig) (AdminService, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	conn, err := newConnection(cfg)
	if err != nil {
		return nil, err
	}
	if err := validateConnection(ctx, conn, cfg.DialTimeout()); err != nil {
		return nil, errors.Wrap(err, "kafka连接验证失败")
	}

	transport, err := buildAdminTransport(cfg)
	if err != nil {
		return nil, err
	}

	logger.InfoWithoutCtx(
		"初始化 Kafka 管理端成功",
		"brokers", strings.Join(conn.brokers, ","),
		"security_protocol", NormalizeSecurityProtocol(cfg.SecurityProtocol),
		"sasl_mechanism", NormalizeSASLMechanism(cfg.SASLMechanism),
	)

	return &admin{
		client: &kafkago.Client{
			Addr:      kafkago.TCP(conn.brokers...),
			Timeout:   cfg.DialTimeout(),
			Transport: transport,
		},
		transport: transport,
	}, nil
}

// Metadata 查询集群实时元数据
func (a *admin) Metadata(ctx context.Context) (*ClusterMetadata, error) {
	a.invalidateMetadataCache()
	res, err := a.client.Metadata(ctx, &kafkago.MetadataRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "查询 Kafka 集群元数据失败")
	}
	return &ClusterMetadata{
		ClusterID:  res.ClusterID,
		Controller: toBroker(res.Controller),
		Brokers:    toBrokers(res.Brokers),
	}, nil
}

// ListTopics 查询全部 topic 名称，结果按名称升序
func (a *admin) ListTopics(ctx context.Context) ([]string, error) {
	a.invalidateMetadataCache()
	res, err := a.client.Metadata(ctx, &kafkago.MetadataRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "查询 Kafka topic 列表失败")
	}

	names := make([]string, 0, len(res.Topics))
	for i := range res.Topics {
		if name := strings.TrimSpace(res.Topics[i].Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// DescribeTopic 查询指定 topic 实时元数据
func (a *admin) DescribeTopic(ctx context.Context, topic string) (*TopicMetadata, error) {
	name := strings.TrimSpace(topic)
	if name == "" {
		return nil, errors.New("kafka topic 不能为空")
	}

	a.invalidateMetadataCache()
	res, err := a.client.Metadata(ctx, &kafkago.MetadataRequest{Topics: []string{name}})
	if err != nil {
		return nil, errors.Wrap(err, "查询 Kafka topic %s 元数据失败", name)
	}
	for i := range res.Topics {
		if res.Topics[i].Name != name {
			continue
		}
		if res.Topics[i].Error != nil {
			return nil, mapDescribeTopicError(res.Topics[i].Error, name)
		}
		return toTopicMetadata(res.Topics[i]), nil
	}
	return nil, errors.Sentinel(ErrTopicNotFound)
}

// CreateTopic 创建 topic
func (a *admin) CreateTopic(ctx context.Context, cfg TopicConfig) error {
	cfg.Topic = strings.TrimSpace(cfg.Topic)
	if err := cfg.validate(); err != nil {
		return err
	}

	res, err := a.client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{
		Topics: []kafkago.TopicConfig{{
			Topic:             cfg.Topic,
			NumPartitions:     cfg.NumPartitions,
			ReplicationFactor: cfg.ReplicationFactor,
			ConfigEntries:     toKafkaConfigEntries(cfg.ConfigEntries),
		}},
	})
	if err != nil {
		return errors.Wrap(err, "创建 Kafka topic %s 失败", cfg.Topic)
	}
	if topicErr := res.Errors[cfg.Topic]; topicErr != nil {
		return mapCreateTopicError(topicErr, cfg.Topic)
	}
	return nil
}

// DeleteTopic 删除 topic
func (a *admin) DeleteTopic(ctx context.Context, topic string) error {
	name := strings.TrimSpace(topic)
	if name == "" {
		return errors.New("kafka topic 不能为空")
	}

	res, err := a.client.DeleteTopics(ctx, &kafkago.DeleteTopicsRequest{Topics: []string{name}})
	if err != nil {
		return errors.Wrap(err, "删除 Kafka topic %s 失败", name)
	}
	if topicErr := res.Errors[name]; topicErr != nil {
		return mapDeleteTopicError(topicErr, name)
	}
	a.invalidateMetadataCache()
	return nil
}

// invalidateMetadataCache 丢弃传输层元数据缓存。
// kafka-go 的 Transport 会缓存 metadata，且删除 topic 后不会主动失效；管理端需读取实时状态。
func (a *admin) invalidateMetadataCache() {
	if a.transport != nil {
		a.transport.CloseIdleConnections()
	}
}

// Close 释放管理端空闲连接，幂等安全
func (a *admin) Close() error {
	if a == nil {
		return nil
	}
	a.closeMu.Lock()
	defer a.closeMu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	if a.transport != nil {
		a.transport.CloseIdleConnections()
	}
	return nil
}

// validate 校验创建 topic 参数
func (c TopicConfig) validate() error {
	if c.Topic == "" {
		return errors.New("kafka topic 不能为空")
	}
	if c.NumPartitions != -1 && c.NumPartitions <= 0 {
		return errors.New("kafka topic 分区数必须为 -1 或大于 0")
	}
	if c.ReplicationFactor != -1 && c.ReplicationFactor <= 0 {
		return errors.New("kafka topic 副本数必须为 -1 或大于 0")
	}
	for key := range c.ConfigEntries {
		if strings.TrimSpace(key) == "" {
			return errors.New("kafka topic 配置项名称不能为空")
		}
	}
	return nil
}

// buildAdminTransport 构建管理端独占传输层，Close 时仅释放本传输层的空闲连接
func buildAdminTransport(cfg ConnectionConfig) (*kafkago.Transport, error) {
	security, err := buildSecuritySettings(cfg)
	if err != nil {
		return nil, err
	}
	return &kafkago.Transport{
		DialTimeout: cfg.DialTimeout(),
		ClientID:    strings.TrimSpace(cfg.ClientID),
		TLS:         security.tlsConfig,
		SASL:        security.mechanism,
	}, nil
}

// mapDescribeTopicError 将查询 topic 元数据错误映射为包内错误
func mapDescribeTopicError(err error, topic string) error {
	if errors.Is(err, kafkago.UnknownTopicOrPartition) || errors.Is(err, kafkago.UnknownTopicID) {
		return errors.Sentinel(ErrTopicNotFound)
	}
	return errors.Wrap(err, "查询 Kafka topic %s 元数据失败", topic)
}

// mapCreateTopicError 将创建 topic 错误映射为包内错误
func mapCreateTopicError(err error, topic string) error {
	if errors.Is(err, kafkago.TopicAlreadyExists) {
		return errors.Sentinel(ErrTopicAlreadyExists)
	}
	return errors.Wrap(err, "创建 Kafka topic %s 失败", topic)
}

// mapDeleteTopicError 将删除 topic 错误映射为包内错误
func mapDeleteTopicError(err error, topic string) error {
	if errors.Is(err, kafkago.UnknownTopicOrPartition) || errors.Is(err, kafkago.UnknownTopicID) {
		return errors.Sentinel(ErrTopicNotFound)
	}
	return errors.Wrap(err, "删除 Kafka topic %s 失败", topic)
}

// toBroker 将 kafka-go broker 转换为包内 Broker
func toBroker(broker kafkago.Broker) Broker {
	return Broker{
		ID:   broker.ID,
		Host: broker.Host,
		Port: broker.Port,
		Rack: broker.Rack,
	}
}

// toBrokers 将 kafka-go broker 列表转换为包内 Broker 列表
func toBrokers(brokers []kafkago.Broker) []Broker {
	if len(brokers) == 0 {
		return nil
	}
	result := make([]Broker, 0, len(brokers))
	for _, broker := range brokers {
		result = append(result, toBroker(broker))
	}
	return result
}

// toTopicMetadata 将 kafka-go topic 转换为包内 TopicMetadata
func toTopicMetadata(topic kafkago.Topic) *TopicMetadata {
	return &TopicMetadata{
		Name:       topic.Name,
		Internal:   topic.Internal,
		Partitions: toPartitionMetadataList(topic.Partitions),
	}
}

// toPartitionMetadataList 将 kafka-go 分区列表转换为包内分区元数据列表
func toPartitionMetadataList(partitions []kafkago.Partition) []PartitionMetadata {
	if len(partitions) == 0 {
		return nil
	}
	result := make([]PartitionMetadata, 0, len(partitions))
	for _, partition := range partitions {
		result = append(result, PartitionMetadata{
			ID:       partition.ID,
			Leader:   toBroker(partition.Leader),
			Replicas: toBrokers(partition.Replicas),
			ISR:      toBrokers(partition.Isr),
		})
	}
	return result
}

// toKafkaConfigEntries 将 topic 配置项转换为 kafka-go 配置项；按名称排序保证请求稳定
func toKafkaConfigEntries(entries map[string]string) []kafkago.ConfigEntry {
	if len(entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]kafkago.ConfigEntry, 0, len(keys))
	for _, key := range keys {
		result = append(result, kafkago.ConfigEntry{
			ConfigName:  key,
			ConfigValue: entries[key],
		})
	}
	return result
}
