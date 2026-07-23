// Package snowflaketest 提供 snowflake ID 生成器的测试辅助实现，供服务层测试使用
//
// @author zwei
package snowflaketest

import (
	"gitee.com/hq_project/yomi-boot/errors"
	"math/rand/v2"

	bwmarrin "github.com/bwmarrin/snowflake"
)

// TestGenerator 测试用的雪花 ID 生成器，内部使用真实雪花算法，workerID 随机取 0~100
type TestGenerator struct {
	node *bwmarrin.Node
}

// NewTestGenerator 创建测试用生成器，随机选取 workerID（0~100）创建雪花节点
func NewTestGenerator() *TestGenerator {
	workerID := rand.Int64N(101)
	node, err := bwmarrin.NewNode(workerID)
	if err != nil {
		panic("snowflaketest: 创建雪花节点失败: " + err.Error())
	}
	return &TestGenerator{node: node}
}

// NextID 返回当前时间窗口下的唯一雪花 ID
func (g *TestGenerator) NextID() (int64, error) {
	if g == nil || g.node == nil {
		return 0, errors.New("雪花 ID 生成器未初始化")
	}
	return g.node.Generate().Int64(), nil
}
