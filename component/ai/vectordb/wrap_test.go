// 错误链辅助单测。
//
// @author zwei

package vectordb

import (
	"context"
	"io"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestVectorWrapSentinel 验证哨兵与原因同时保留
func TestVectorWrapSentinel(t *testing.T) {
	t.Parallel()
	only := wrapSentinel(ErrNotFound, nil)
	if !errors.Is(only, ErrNotFound) {
		t.Fatalf("err=%v", only)
	}
	cause := errors.New("upstream boom")
	err := wrapSentinel(ErrUnavailable, cause)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("err=%v", err)
	}
}

// TestVectorWrapCtxErr 覆盖空错误、非上下文错误与重复包装
func TestVectorWrapCtxErr(t *testing.T) {
	t.Parallel()
	if err := wrapCtxErr(nil); err != nil {
		t.Fatalf("nil=%v", err)
	}
	if got := wrapCtxErr(io.EOF); got != io.EOF {
		t.Fatalf("got %v", got)
	}
	once := wrapCtxErr(context.Canceled)
	if !errors.Is(once, context.Canceled) {
		t.Fatalf("canceled=%v", once)
	}
	if twice := wrapCtxErr(once); twice != once {
		t.Fatalf("double wrap changed identity: %v", twice)
	}
	if err := wrapCtxErr(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline=%v", err)
	}
}

// TestMapEngineError 覆盖空值、上下文错误、已知哨兵透传与未知错误收口
func TestMapEngineError(t *testing.T) {
	t.Parallel()
	if err := mapEngineError(nil); err != nil {
		t.Fatalf("nil=%v", err)
	}
	if err := mapEngineError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled=%v", err)
	}
	if err := mapEngineError(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline=%v", err)
	}
	known := []error{ErrInvalidArgument, ErrAlreadyExists, ErrNotFound, ErrClosed, ErrUnavailable}
	for _, sentinel := range known {
		got := mapEngineError(sentinel)
		if got != sentinel {
			t.Fatalf("sentinel %v changed to %v", sentinel, got)
		}
	}
	cause := errors.New("engine down")
	err := mapEngineError(cause)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("err=%v", err)
	}
}
