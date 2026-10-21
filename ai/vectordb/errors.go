// 定义向量库哨兵错误。
//
// @author zwei

package vectordb

import "gitee.com/hq_project/yomi-boot/boot/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_VECTORDB_INVALID_ARGUMENT")
	// ErrAlreadyExists CreateCollection 时集合已存在
	ErrAlreadyExists = errors.NewNoStack("AI_VECTORDB_ALREADY_EXISTS")
	// ErrNotFound 集合必须存在但没有（HasCollection 不使用本哨兵）
	ErrNotFound = errors.NewNoStack("AI_VECTORDB_NOT_FOUND")
	// ErrUnavailable 连不上、超时或引擎故障
	ErrUnavailable = errors.NewNoStack("AI_VECTORDB_UNAVAILABLE")
	// ErrClosed Close 之后再调用
	ErrClosed = errors.NewNoStack("AI_VECTORDB_CLOSED")
)
