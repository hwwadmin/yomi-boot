// 流式 Recv/Close 生命周期单测
//
// @author zwei

package llm

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// stubPump 测试用流读取器，可预置增量、错误与阻塞
type stubPump struct {
	mu       sync.Mutex    // mu 保护增量队列
	chunks   []*ChatChunk  // chunks 是待返回的增量队列
	err      error         // err 是队列耗尽后的错误，空则结束
	block    chan struct{} // block 非空时读取先等待关闭
	closed   atomic.Bool   // closed 记录是否已关闭
	closeErr error         // closeErr 是 close 返回的错误
}

// next 弹出下一增量；可先阻塞；队列空时返回预置错误或结束
func (p *stubPump) next() (*ChatChunk, error) {
	if p.block != nil {
		<-p.block
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.chunks) == 0 {
		if p.err != nil {
			return nil, p.err
		}
		return nil, io.EOF
	}
	c := p.chunks[0]
	p.chunks = p.chunks[1:]
	return c, nil
}

// close 标记已关闭并解除读取阻塞；幂等
func (p *stubPump) close() error {
	p.closed.Store(true)
	if p.block != nil {
		select {
		case <-p.block:
		default:
			close(p.block)
		}
	}
	return p.closeErr
}

// TestStreamCloseReturnsPumpError 验证 Close 透传底层关闭错误
func TestStreamCloseReturnsPumpError(t *testing.T) {
	t.Parallel()
	want := errors.New("close boom")
	s := newChatStream(context.Background(), nil, &stubPump{closeErr: want})
	if err := s.Close(); !errors.Is(err, want) {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); !errors.Is(err, want) {
		t.Fatalf("repeat Close: %v", err)
	}
}

// TestStreamConcurrentRecvDoesNotClose 验证并发 Recv 返回非法参数且不关流
func TestStreamConcurrentRecvDoesNotClose(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{})}
	s := newChatStream(context.Background(), nil, p)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := s.Recv()
		done <- err
	}()
	<-started
	time.Sleep(20 * time.Millisecond)
	_, err := s.Recv()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("concurrent Recv: %v", err)
	}
	if p.closed.Load() {
		t.Fatal("concurrent Recv closed stream")
	}
	close(p.block)
	if err := <-done; err != io.EOF && !errors.Is(err, ErrBadResponse) {
		t.Fatalf("first Recv: %v", err)
	}
}

// TestStreamCloseThenRecvEOF 验证 Close 后再 Recv 为未包装结束符
func TestStreamCloseThenRecvEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{chunks: []*ChatChunk{{Delta: "x"}}}
	s := newChatStream(context.Background(), nil, p)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close idempotent: %v", err)
	}
	_, err := s.Recv()
	if err != io.EOF {
		t.Fatalf("got %v", err)
	}
}

// TestStreamCloseUnblocksRecvAsEOF 验证 Close 使进行中 Recv 以结束符返回
func TestStreamCloseUnblocksRecvAsEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{})}
	s := newChatStream(context.Background(), func() {}, p)
	done := make(chan error, 1)
	go func() {
		_, err := s.Recv()
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := <-done
	if err != io.EOF {
		t.Fatalf("in-flight Recv want EOF, got %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("caller Close rewritten as Canceled")
	}
}

// TestStreamCloseMapsPumpReadErrorToEOF 验证 Close 打断读时连接关闭类错误收口为未包装结束符
func TestStreamCloseMapsPumpReadErrorToEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{}), err: errors.New("use of closed network connection")}
	s := newChatStream(context.Background(), func() {}, p)
	done := make(chan error, 1)
	go func() {
		_, err := s.Recv()
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := <-done
	if err != io.EOF {
		t.Fatalf("in-flight Recv want raw EOF, got %v", err)
	}
}

// TestWrapCtxErrIdempotent 验证超时错误只包装一次
func TestWrapCtxErrIdempotent(t *testing.T) {
	t.Parallel()
	once := wrapCtxErr(context.DeadlineExceeded)
	twice := wrapCtxErr(once)
	if twice != once {
		t.Fatalf("second wrap changed identity: %v", twice)
	}
	if !errors.Is(twice, context.DeadlineExceeded) {
		t.Fatalf("lost DeadlineExceeded: %v", twice)
	}
	if strings.Count(twice.Error(), "ai/llm: 请求已取消或超时") != 1 {
		t.Fatalf("wrapped more than once: %q", twice.Error())
	}
}

// TestStreamEmptyTextIsBadResponse 验证流结束无文本且非 Close 时为 ErrBadResponse
func TestStreamEmptyTextIsBadResponse(t *testing.T) {
	t.Parallel()
	s := newChatStream(context.Background(), nil, &stubPump{})
	_, err := s.Recv()
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v", err)
	}
	_, err = s.Recv()
	if err != io.EOF {
		t.Fatalf("after bad response: %v", err)
	}
}

// TestStreamTimeoutThenEOF 验证当次超时非结束符，之后为未包装结束符
func TestStreamTimeoutThenEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{}), err: context.DeadlineExceeded}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	s := newChatStream(ctx, cancel, p)
	time.Sleep(40 * time.Millisecond)
	_, err := s.Recv()
	if errors.Is(err, io.EOF) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	_, err = s.Recv()
	if err != io.EOF {
		t.Fatalf("after timeout: %v", err)
	}
}
