// Package llm 流式读取与关闭的生命周期。
//
// @author zwei
package llm

import (
	"context"
	stderrors "errors"
	"io"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
)

// chunkPump 协议流拉取下一增量；结束返回 io.EOF
type chunkPump interface {
	// next 读取下一增量；流结束返回 io.EOF；读失败返回其它错误
	next() (*ChatChunk, error)
	// close 关闭底层流，须幂等
	close() error
}

// chatStream 实现 Stream
type chatStream struct {
	ctx            context.Context    // ctx 是带墙钟超时的请求上下文
	cancel         context.CancelFunc // cancel 释放 WithTimeout 定时器；可空
	pump           chunkPump          // pump 是协议流读取器
	recvMu         sync.Mutex         // recvMu 保证 Recv 串行；并发 Recv 走 TryLock
	stateMu        sync.Mutex         // stateMu 保护 closedByCaller / terminated / gotText
	closedByCaller bool               // closedByCaller 表示调用方已 Close
	terminated     bool               // terminated 表示流已结束，后续 Recv 一律 EOF
	gotText        bool               // gotText 表示已产出过非空增量
}

var _ Stream = (*chatStream)(nil)

// newChatStream 包装协议流；cancel 用于释放超时定时器，可为 nil
func newChatStream(ctx context.Context, cancel context.CancelFunc, pump chunkPump) *chatStream {
	return &chatStream{ctx: ctx, cancel: cancel, pump: pump}
}

// Recv 读取下一增量
//
// 并发 Recv 返回 ErrInvalidArgument 且不关流。调用方 Close 且非超时则未包装 io.EOF。
// 当次超时或取消返回上下文错误（可包装）。流结束无文本且非 Close 时返回 ErrBadResponse。
func (s *chatStream) Recv() (*ChatChunk, error) {
	// 并发读取直接拒绝，不关闭流
	if !s.recvMu.TryLock() {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	defer s.recvMu.Unlock()

	s.stateMu.Lock()
	if s.terminated {
		s.stateMu.Unlock()
		return nil, terminateEOF()
	}
	closed := s.closedByCaller
	s.stateMu.Unlock()

	// 调用方已关闭则后续一律未包装结束
	if closed {
		s.markTerminated()
		_ = s.pump.close()
		return nil, terminateEOF()
	}
	if err := s.ctx.Err(); err != nil {
		s.stateMu.Lock()
		closed = s.closedByCaller
		s.stateMu.Unlock()
		s.markTerminated()
		_ = s.pump.close()
		// 调用方 Close 触发的取消不得改写成超时
		if closed && stderrors.Is(err, context.Canceled) && !stderrors.Is(err, context.DeadlineExceeded) {
			return nil, terminateEOF()
		}
		return nil, wrapCtxErr(err)
	}

	chunk, err := s.pump.next()
	if err != nil {
		s.stateMu.Lock()
		gotText := s.gotText
		closedByCaller := s.closedByCaller
		s.stateMu.Unlock()
		s.markTerminated()
		_ = s.pump.close()
		// Close 打断读时，连接关闭类错误收口为未包装 EOF；超时仍按超时
		if closedByCaller && !stderrors.Is(err, context.DeadlineExceeded) {
			return nil, terminateEOF()
		}
		if stderrors.Is(err, io.EOF) {
			if !gotText {
				return nil, wrapSentinel(ErrBadResponse, errors.New("ai/llm: 流结束且无 assistant 文本"))
			}
			return nil, terminateEOF()
		}
		if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
			return nil, wrapCtxErr(err)
		}
		return nil, err
	}
	if chunk != nil && chunk.Delta != "" {
		// 记录已产出文本，避免空流转成协议异常
		s.stateMu.Lock()
		s.gotText = true
		s.stateMu.Unlock()
	}
	return chunk, nil
}

// Close 释放资源；幂等；成功返回 nil；可与 Recv 并发
func (s *chatStream) Close() error {
	s.stateMu.Lock()
	s.closedByCaller = true
	s.stateMu.Unlock()
	_ = s.pump.close()
	// 取消超时上下文，打断进行中的读取
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// markTerminated 标记流已结束并释放超时定时器，后续 Recv 一律 EOF
func (s *chatStream) markTerminated() {
	s.stateMu.Lock()
	s.terminated = true
	s.stateMu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
