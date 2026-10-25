// 异步写入层：按物理输出目标隔离的缓冲写入器。
//
// @author zwei

package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// defaultAsyncQueueSize 是每个异步 sink 的默认队列容量（条数）
const defaultAsyncQueueSize = 4096

// defaultFlushTimeout 是停机/显式刷新时等待在途日志落盘的默认预算
const defaultFlushTimeout = 3 * time.Second

// dropWarnInterval 是队列丢弃/写失败 stderr 告警的最小间隔
const dropWarnInterval = 5 * time.Second

// sinkTask 是异步队列中的一条写入任务
type sinkTask struct {
	data []byte        // data 是已编码的整行日志字节快照
	done chan struct{} // done 非 nil 表示屏障任务：排到此处即代表其之前的日志均已落盘
}

// asyncSink 单个输出目标的异步写入器：调用方非阻塞入队、队列满则丢弃并计数，
// 独立协程按序落盘。按 sink 隔离使单目标故障（stdout 管道积压、磁盘卡死）
// 只丢弃自身日志，不传染其他目标。
type asyncSink struct {
	name string    // name 是 stderr 告警中的 sink 标识
	dst  io.Writer // dst 是真正落盘的底层 writer

	queue    chan sinkTask // queue 是待落盘任务队列
	dropped  atomic.Int64  // dropped 累计入队失败（队列满）条数
	failed   atomic.Int64  // failed 累计底层写入失败条数
	lastWarn atomic.Int64  // lastWarn 是上次 stderr 告警的 unix 纳秒，用于限频
	running  atomic.Bool   // running 为 false 后不再入队，Write 直接同步写底层
	stopOnce sync.Once
	stop     chan struct{} // stop 关闭后消费协程排空队列退出
	done     chan struct{} // done 在消费协程退出后关闭
}

// newAsyncSink 创建异步写入器并启动消费协程；queueSize <= 0 时取默认容量
func newAsyncSink(name string, dst io.Writer, queueSize int) *asyncSink {
	if queueSize <= 0 {
		queueSize = defaultAsyncQueueSize
	}
	s := &asyncSink{
		name:  name,
		dst:   dst,
		queue: make(chan sinkTask, queueSize),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	s.running.Store(true)
	go s.consumeLoop()
	return s
}

// Write 非阻塞入队，队列满时丢弃并计数；绝不阻塞调用方。
// zap 传入的缓冲在 Write 返回后即被复用，必须在此拷贝字节快照。
// 停机后同步直写底层兜底（文件已关时错误忽略）。
func (s *asyncSink) Write(p []byte) (int, error) {
	if !s.running.Load() {
		_, _ = s.dst.Write(p)
		return len(p), nil
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	select {
	case s.queue <- sinkTask{data: cp}:
	default:
		s.dropped.Add(1)
	}
	return len(p), nil
}

// Flush 等待入队时刻之前的全部日志落盘；超时返回错误
func (s *asyncSink) Flush(timeout time.Duration) error {
	if !s.running.Load() {
		return nil
	}
	barrier := sinkTask{done: make(chan struct{})}
	select {
	case s.queue <- barrier:
	case <-time.After(timeout):
		return errors.Newf("logger: sink[%s] flush 入队超时（队列已满）", s.name)
	}
	select {
	case <-barrier.done:
		return nil
	case <-s.done:
		// 屏障排到前 sink 已停止：停止路径自身会排空队列
		return nil
	case <-time.After(timeout):
		return errors.Newf("logger: sink[%s] flush 超时", s.name)
	}
}

// Stop 停止接受新日志并排空残余后退出消费协程；幂等
func (s *asyncSink) Stop() {
	s.stopOnce.Do(func() {
		s.running.Store(false)
		close(s.stop)
		<-s.done
	})
}

// consumeLoop 消费主循环；单轮 panic 时记录并退避后重建，sink 不永久失效。
// 无条件先跑一轮：Stop 抢在协程首次调度前发生时，仍须进 consume 排空队列。
func (s *asyncSink) consumeLoop() {
	defer close(s.done)
	for {
		if s.consume() {
			return
		}
		// 走到这里说明上轮 panic，退避避免持续 panic 热循环
		time.Sleep(100 * time.Millisecond)
	}
}

// consume 单轮消费：处理任务直至停止（此时排空队列并返回 true）；
// panic 在此兜底转为返回 false，由外层重建
func (s *asyncSink) consume() (stopped bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "logger: sink[%s] 消费协程 panic: %v\n", s.name, r)
		}
	}()
	for {
		select {
		case <-s.stop:
			s.drain()
			return true
		case t := <-s.queue:
			s.handle(t)
			s.reportCounters()
		}
	}
}

// drain 停机路径排空队列残余；此后到达的任务随 running=false 走同步兜底
func (s *asyncSink) drain() {
	for {
		select {
		case t := <-s.queue:
			s.handle(t)
		default:
			return
		}
	}
}

// handle 处理单条任务；屏障任务只关闭通知，不写底层
func (s *asyncSink) handle(t sinkTask) {
	if t.done != nil {
		close(t.done)
		return
	}
	if _, err := s.dst.Write(t.data); err != nil {
		s.failed.Add(1)
	}
}

// reportCounters 在有丢弃/写失败时向 stderr 限频告警
func (s *asyncSink) reportCounters() {
	dropped := s.dropped.Swap(0)
	failed := s.failed.Swap(0)
	if dropped == 0 && failed == 0 {
		return
	}
	now := time.Now().UnixNano()
	last := s.lastWarn.Load()
	if now-last < int64(dropWarnInterval) || !s.lastWarn.CompareAndSwap(last, now) {
		return
	}
	fmt.Fprintf(os.Stderr, "logger: sink[%s] 队列满丢弃 %d 条、写失败 %d 条\n", s.name, dropped, failed)
}
