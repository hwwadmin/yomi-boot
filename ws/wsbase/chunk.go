// Package wsbase 实现可配置分片的 WriteReader
//
// @author zwei
package wsbase

import (
	"context"
	"io"

	"gitee.com/hq_project/yomi-boot/errors"
)

// WriteOption 是 WriteReader 的单次调用选项
type WriteOption func(*writeOptions)

// writeOptions 汇总 WriteReader 单次选项
type writeOptions struct {
	chunkSize int // chunkSize 是本次分片字节数
}

// WithChunkSize 指定本次 WriteReader 的分片大小（字节）；覆盖实例默认
func WithChunkSize(n int) WriteOption {
	return func(o *writeOptions) {
		o.chunkSize = n
	}
}

// WriteReader 从 r 按分片读取并发送；每片为一条完整消息（P2）
func (c *connImpl) WriteReader(ctx context.Context, opcode Opcode, r io.Reader, opts ...WriteOption) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if r == nil {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteReader r 不能为 nil")
	}
	if opcode != OpcodeText && opcode != OpcodeBinary {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "WriteReader opcode 仅支持 Text/Binary")
	}

	o := writeOptions{chunkSize: c.chunkSize}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if err := validateChunkSize(o.chunkSize); err != nil {
		return err
	}
	chunkSize := resolveChunkSize(o.chunkSize)

	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := r.Read(buf)
		if n > 0 {
			payload := make([]byte, n)
			copy(payload, buf[:n])
			if err := c.raw.WriteMessage(opcode, payload); err != nil {
				return errors.Wrap(err, "ws 分片写入失败")
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return errors.Wrap(readErr, "ws 读取分片源失败")
		}
	}
}
