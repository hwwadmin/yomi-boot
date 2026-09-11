// Package ws 实现每连接发送队列：有界阻塞入队 / 无界；心跳 Ping 插队
//
// @author zwei
package ws

import (
	"context"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/coder/websocket"
)

// sendTask 是一条待写出的业务消息
type sendTask struct {
	typ  MsgType
	data []byte
}

// writeQueue 是连接级发送队列；capacity==0 表示无界
type writeQueue struct {
	conn     *connImpl
	capacity int // capacity 是业务队列容量；0 表示无界

	mu          sync.Mutex
	closed      bool
	tasks       []sendTask
	pingPending bool
	pingRes     chan error
	pingWait    time.Duration

	notEmpty *sync.Cond
	notFull  *sync.Cond
}

// newWriteQueue 创建发送队列并启动写循环
func newWriteQueue(conn *connImpl, capacity int) *writeQueue {
	q := &writeQueue{conn: conn, capacity: capacity}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

// enqueue 入队业务消息；有界满时阻塞；队列已关闭返回 ErrNotConnected
func (q *writeQueue) enqueue(typ MsgType, data []byte) error {
	if q == nil || q.conn == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	switch typ {
	case MsgText, MsgBinary:
	default:
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "不支持的 MsgType")
	}
	if len(data) > q.conn.writeMax {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "超过 WriteMaxPayloadSize")
	}
	cp := append([]byte(nil), data...)

	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.closed {
			return errors.Sentinel(ErrNotConnected)
		}
		if q.capacity <= 0 || len(q.tasks) < q.capacity {
			q.tasks = append(q.tasks, sendTask{typ: typ, data: cp})
			q.notEmpty.Signal()
			return nil
		}
		q.notFull.Wait()
	}
}

// enqueuePing 插队请求 Ping；与业务队列共享写循环
func (q *writeQueue) enqueuePing(wait time.Duration) error {
	if q == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	if wait <= 0 {
		wait = time.Second
	}
	resCh := make(chan error, 1)

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errors.Sentinel(ErrNotConnected)
	}
	// 已有待处理 Ping：合并为一次，共享结果通道不可行；等待当前写循环消化后再挂
	for q.pingPending && !q.closed {
		q.notFull.Wait()
	}
	if q.closed {
		q.mu.Unlock()
		return errors.Sentinel(ErrNotConnected)
	}
	q.pingPending = true
	q.pingRes = resCh
	q.pingWait = wait
	q.notEmpty.Signal()
	q.mu.Unlock()

	return <-resCh
}

// shutdown 关闭队列、清空任务并唤醒所有等待者；幂等
func (q *writeQueue) shutdown() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.tasks = nil
	if q.pingRes != nil {
		q.pingRes <- errors.Sentinel(ErrNotConnected)
		q.pingRes = nil
		q.pingPending = false
	}
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

// isClosed 队列是否已关闭
func (q *writeQueue) isClosed() bool {
	if q == nil {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

// purgeIfDead 若队列已关闭则确保已清空（巡检兜底）
func (q *writeQueue) purgeIfDead() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		return
	}
	q.tasks = nil
	if q.pingRes != nil {
		q.pingRes <- errors.Sentinel(ErrNotConnected)
		q.pingRes = nil
		q.pingPending = false
	}
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

// writeLoop 串行写出：优先 Ping，再业务 FIFO
func (c *connImpl) writeLoop() {
	q := c.sendQ
	if q == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = c.Close(CloseGoingAway, "write loop panic")
		}
	}()
	for {
		q.mu.Lock()
		for !q.closed && !q.pingPending && len(q.tasks) == 0 {
			q.notEmpty.Wait()
		}
		if q.closed && !q.pingPending && len(q.tasks) == 0 {
			q.mu.Unlock()
			return
		}

		if q.pingPending {
			resCh := q.pingRes
			wait := q.pingWait
			q.pingPending = false
			q.pingRes = nil
			q.notFull.Broadcast()
			q.mu.Unlock()

			if wait <= 0 {
				wait = time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), wait)
			var err error
			if c.raw != nil {
				err = c.raw.Ping(ctx)
			} else {
				err = errors.Sentinel(ErrNotConnected)
			}
			cancel()
			if err != nil {
				err = errors.Wrap(err, "ws Ping 失败")
			}
			if resCh != nil {
				resCh <- err
			}
			if err != nil {
				_ = c.Close(CloseGoingAway, "heartbeat ping failed")
				return
			}
			continue
		}

		task := q.tasks[0]
		q.tasks = q.tasks[1:]
		if q.capacity > 0 {
			q.notFull.Signal()
		}
		q.mu.Unlock()

		if err := c.writeMessageDirect(task.typ, task.data); err != nil {
			_ = c.Close(CloseGoingAway, "write failed")
			return
		}
	}
}

// writeMessageDirect 由写循环调用，直接写底层（不再入队）
func (c *connImpl) writeMessageDirect(typ MsgType, data []byte) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrNotConnected)
	}
	var wtyp websocket.MessageType
	switch typ {
	case MsgText:
		wtyp = websocket.MessageText
	case MsgBinary:
		wtyp = websocket.MessageBinary
	default:
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "不支持的 MsgType")
	}
	if err := c.raw.Write(context.Background(), wtyp, data); err != nil {
		return errors.Wrap(err, "ws 写入消息失败")
	}
	return nil
}
