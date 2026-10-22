// Package snowflake 提供基于 bwmarrin/snowflake 的雪花 ID 生成器封装
//
// @author zwei
package snowflake

import (
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	bwmarrin "github.com/bwmarrin/snowflake"
)

// Generator 表示雪花 ID 生成器接口，供领域层依赖
type Generator interface {
	// NextID 返回当前时间窗口下的唯一 ID
	NextID() (int64, error)
}

// nodeGenerator bwmarrin.Node 的 Generator 实现
type nodeGenerator struct {
	node *bwmarrin.Node
}

var _ Generator = (*nodeGenerator)(nil)

// NewGenerator 基于指定的 worker ID 创建雪花 ID 生成器（0~1023）
func NewGenerator(workerID int64) (Generator, error) {
	node, err := bwmarrin.NewNode(workerID)
	if err != nil {
		return nil, errors.Wrap(err, "创建雪花 ID 生成器失败")
	}
	return &nodeGenerator{node: node}, nil
}

// NextID 返回当前时间窗口下的唯一 ID
func (g *nodeGenerator) NextID() (int64, error) {
	return g.node.Generate().Int64(), nil
}
