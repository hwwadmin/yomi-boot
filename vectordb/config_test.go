// Package vectordb 构造配置校验测试。
//
// @author zwei
package vectordb

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestNewValidate 验证 New 对非法配置 Fail Fast
func TestNewValidate(t *testing.T) {
	t.Parallel()

	t.Run("unsupported provider", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: "qdrant", Dimension: 8})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("dimension required", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: ProviderMemory})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("timeout negative", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: ProviderMemory, Dimension: 8, Timeout: -1})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("milvus requires address", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: ProviderMilvus, Dimension: 8})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("bad metric", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: ProviderMemory, Dimension: 8, Metric: "dot"})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("metric default cosine", func(t *testing.T) {
		t.Parallel()
		s, err := New(Config{Provider: ProviderMemory, Dimension: 8, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
	})
}
