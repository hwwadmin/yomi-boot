// 定义检索增强数据能力哨兵错误。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_RAG_INVALID_ARGUMENT")
	// ErrDimensionMismatch 向量化输出维度与本实例锁定的维度不一致
	ErrDimensionMismatch = errors.NewNoStack("AI_RAG_DIMENSION_MISMATCH")
	// ErrBadResponse 向量化实现或上游返回的数据不符合本包契约
	ErrBadResponse = errors.NewNoStack("AI_RAG_BAD_RESPONSE")
)
