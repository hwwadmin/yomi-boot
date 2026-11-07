package kafkatest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image 是测试容器使用的 Kafka 镜像，与 README 推荐的线上版本一致。
	Image = "apache/kafka:4.2.2"
	// Host 是测试代码连接容器时使用的地址。容器端口发布在本机。
	Host = "127.0.0.1"

	brokerPort           = "9092/tcp"
	internalBrokerPort   = 19092
	controllerPort       = 29093
	startupTimeout       = 3 * time.Minute
	adminTimeout         = 30 * time.Second
	topicReadyTimeout    = 30 * time.Second
	groupReleaseAttempts = 20
	// clusterID 是单节点 KRaft 的集群标识，容器每次新建日志目录，固定值即可。
	clusterID = "4L6g3nShT-eMCtK--X86sw"
)

// starterScript 在映射端口确定后写入容器，再交给官方入口启动 broker。
// 对外监听广告本机映射端口；节点内部走 127.0.0.1 上的独立监听，避免 broker 连回宿主机端口。
const starterScript = `#!/bin/bash
export KAFKA_ADVERTISED_LISTENERS='PLAINTEXT_HOST://127.0.0.1:%s,PLAINTEXT://127.0.0.1:%d'
exec /etc/kafka/docker/run
`

// brokerServer 是当前测试进程共用的 Kafka 容器。
type brokerServer struct {
	addr   string
	client *kafkago.Client
	mu     sync.Mutex
}

var (
	startOnce sync.Once
	server    *brokerServer
	startErr  error
)

// ensureServer 懒启动当前进程唯一的 Kafka 容器。容器在进程退出时由 Testcontainers 回收。
func ensureServer(t testing.TB) *brokerServer {
	t.Helper()

	startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		ctr, err := testcontainers.Run(ctx, Image,
			testcontainers.WithExposedPorts(brokerPort),
			testcontainers.WithEnv(brokerEnv()),
			testcontainers.WithEntrypoint("sh"),
			testcontainers.WithCmd("-c", "while [ ! -f /tmp/start-kafka.sh ]; do sleep 0.1; done; exec bash /tmp/start-kafka.sh"),
			testcontainers.WithWaitStrategyAndDeadline(startupTimeout, wait.ForLog("Kafka Server started").WithStartupTimeout(startupTimeout)),
			testcontainers.WithLifecycleHooks(testcontainers.ContainerLifecycleHooks{
				PostStarts: []testcontainers.ContainerHook{writeStarterScript},
			}),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 Kafka 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, brokerPort)
		if err != nil {
			startErr = fmt.Errorf("读取 Kafka 映射端口: %w", err)
			return
		}
		addr := fmt.Sprintf("%s:%d", Host, int(mapped.Num()))
		client := &kafkago.Client{Timeout: adminTimeout}
		if err := pingReady(ctx, client, addr); err != nil {
			startErr = fmt.Errorf("等待 Kafka 可连接: %w", err)
			return
		}
		server = &brokerServer{addr: addr, client: client}
	})
	if startErr != nil {
		t.Fatalf("kafkatest: 启动 Kafka 容器失败: %v", startErr)
	}
	return server
}

// brokerEnv 给出单节点 KRaft 的固定配置。广告地址由启动脚本按映射端口补上。
func brokerEnv() map[string]string {
	return map[string]string{
		"CLUSTER_ID":                                             clusterID,
		"KAFKA_NODE_ID":                                          "1",
		"KAFKA_PROCESS_ROLES":                                    "broker,controller",
		"KAFKA_LISTENERS":                                        fmt.Sprintf("CONTROLLER://:%d,PLAINTEXT_HOST://:9092,PLAINTEXT://:%d", controllerPort, internalBrokerPort),
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":                   "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT",
		"KAFKA_INTER_BROKER_LISTENER_NAME":                       "PLAINTEXT",
		"KAFKA_CONTROLLER_LISTENER_NAMES":                        "CONTROLLER",
		"KAFKA_CONTROLLER_QUORUM_VOTERS":                         fmt.Sprintf("1@127.0.0.1:%d", controllerPort),
		"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":                 "1",
		"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR":         "1",
		"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":                    "1",
		"KAFKA_SHARE_COORDINATOR_STATE_TOPIC_REPLICATION_FACTOR": "1",
		"KAFKA_SHARE_COORDINATOR_STATE_TOPIC_MIN_ISR":            "1",
		"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":                 "0",
		"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                        "false",
		"KAFKA_LOG_DIRS":                                         "/tmp/kraft-combined-logs",
	}
}

// writeStarterScript 在端口映射完成后把启动脚本写入容器。
func writeStarterScript(ctx context.Context, c testcontainers.Container) error {
	if err := wait.ForMappedPort(brokerPort).WaitUntilReady(ctx, c); err != nil {
		return err
	}
	mapped, err := c.MappedPort(ctx, brokerPort)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(starterScript, mapped.Port(), internalBrokerPort)
	return c.CopyToContainer(ctx, []byte(script), "/tmp/start-kafka.sh", 0o755)
}

// pingReady 在日志报就绪后继续探测元数据，入口重启或选举未完成时会短暂拒绝连接。
func pingReady(ctx context.Context, client *kafkago.Client, addr string) error {
	var last error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, last = client.Metadata(pingCtx, &kafkago.MetadataRequest{Addr: kafkago.TCP(addr)})
		cancel()
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
}

// createTopics 按指定分区数创建主题，并等到每个分区选出 leader。
func (s *brokerServer) createTopics(ctx context.Context, configs []kafkago.TopicConfig) error {
	if len(configs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := kafkago.TCP(s.addr)
	res, err := s.client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{Addr: addr, Topics: configs})
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		if topicErr := res.Errors[cfg.Topic]; topicErr != nil {
			return fmt.Errorf("创建 topic %s: %w", cfg.Topic, topicErr)
		}
		if err := s.waitTopicReady(ctx, addr, cfg.Topic, cfg.NumPartitions); err != nil {
			return err
		}
	}
	return nil
}

// release 删除本次用例创建的主题，并清掉消费组，避免下个用例沿用已提交偏移量。
func (s *brokerServer) release(topics []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	addr := kafkago.TCP(s.addr)
	var errs []error
	if err := s.deleteGroups(ctx, addr); err != nil {
		errs = append(errs, err)
	}
	if err := s.deleteTopics(ctx, addr, topics); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *brokerServer) deleteTopics(ctx context.Context, addr net.Addr, topics []string) error {
	if len(topics) == 0 {
		return nil
	}
	res, err := s.client.DeleteTopics(ctx, &kafkago.DeleteTopicsRequest{Addr: addr, Topics: topics})
	if err != nil {
		return fmt.Errorf("删除 topic: %w", err)
	}
	for _, name := range topics {
		if topicErr := res.Errors[name]; topicErr != nil && !errors.Is(topicErr, kafkago.UnknownTopicOrPartition) {
			return fmt.Errorf("删除 topic %s: %w", name, topicErr)
		}
	}
	for _, name := range topics {
		if err := s.waitTopicGone(ctx, addr, name); err != nil {
			return err
		}
	}
	return nil
}

// deleteGroups 删除当前 broker 上的消费组。单节点时所有组的协调者相同，可以一次删除。
func (s *brokerServer) deleteGroups(ctx context.Context, addr net.Addr) error {
	var last error
	for attempt := 0; attempt < groupReleaseAttempts; attempt++ {
		groups, err := s.client.ListGroups(ctx, &kafkago.ListGroupsRequest{Addr: addr})
		if err != nil {
			last = err
		} else if groups.Error != nil {
			last = groups.Error
		} else {
			ids := make([]string, 0, len(groups.Groups))
			for _, group := range groups.Groups {
				if group.GroupID == "" || strings.HasPrefix(group.GroupID, "_") {
					continue
				}
				ids = append(ids, group.GroupID)
			}
			if len(ids) == 0 {
				return nil
			}
			res, err := s.client.DeleteGroups(ctx, &kafkago.DeleteGroupsRequest{Addr: addr, GroupIDs: ids})
			if err != nil {
				last = err
			} else {
				last = nil
				for _, id := range ids {
					groupErr := res.Errors[id]
					if groupErr == nil || errors.Is(groupErr, kafkago.GroupIdNotFound) {
						continue
					}
					last = fmt.Errorf("删除消费组 %s: %w", id, groupErr)
				}
				if last == nil {
					return nil
				}
			}
		}
		if ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
	return last
}

func (s *brokerServer) waitTopicReady(ctx context.Context, addr net.Addr, topic string, partitions int) error {
	deadline, cancel := context.WithTimeout(ctx, topicReadyTimeout)
	defer cancel()

	var last error
	for {
		meta, err := s.client.Metadata(deadline, &kafkago.MetadataRequest{Addr: addr, Topics: []string{topic}})
		if err != nil {
			last = err
		} else {
			ready, topicErr := topicHasLeader(meta, topic, partitions)
			if topicErr != nil && !errors.Is(topicErr, kafkago.UnknownTopicOrPartition) {
				return fmt.Errorf("等待 topic %s 就绪: %w", topic, topicErr)
			}
			if ready {
				return nil
			}
			last = fmt.Errorf("topic %s 尚无 leader", topic)
		}
		if deadline.Err() != nil {
			return last
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-deadline.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
}

func (s *brokerServer) waitTopicGone(ctx context.Context, addr net.Addr, topic string) error {
	var last error
	for {
		meta, err := s.client.Metadata(ctx, &kafkago.MetadataRequest{Addr: addr, Topics: []string{topic}})
		if err != nil {
			last = err
		} else if _, topicErr := topicHasLeader(meta, topic, 0); errors.Is(topicErr, kafkago.UnknownTopicOrPartition) || topicMissing(meta, topic) {
			return nil
		} else {
			last = fmt.Errorf("topic %s 仍存在", topic)
		}
		if ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
}

func topicHasLeader(meta *kafkago.MetadataResponse, topic string, partitions int) (bool, error) {
	for _, item := range meta.Topics {
		if item.Name != topic {
			continue
		}
		if item.Error != nil {
			return false, item.Error
		}
		if partitions > 0 && len(item.Partitions) != partitions {
			return false, nil
		}
		for _, partition := range item.Partitions {
			if partition.Error != nil || partition.Leader.Host == "" {
				return false, partition.Error
			}
		}
		return true, nil
	}
	return false, nil
}

func topicMissing(meta *kafkago.MetadataResponse, topic string) bool {
	for _, item := range meta.Topics {
		if item.Name == topic {
			return false
		}
	}
	return true
}
