// Package dto 提供跨模块复用的传输结构定义与 JSON 辅助类型
//
// @author zwei
package dto

import (
	"bytes"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// JSONInt64Slice 表示可同时接受字符串和数字元素的 int64 切片
type JSONInt64Slice []JSONInt64

// UnmarshalJSON 支持从 JSON 数组中反序列化字符串或数字形式的 int64
func (s *JSONInt64Slice) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = nil
		return nil
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(trimmed, &rawItems); err != nil {
		return errors.Wrap(err, "解析 JSON int64 数组失败")
	}

	items := make(JSONInt64Slice, 0, len(rawItems))
	for _, rawItem := range rawItems {
		value, err := parseFlexibleInt64JSON(rawItem)
		if err != nil {
			return err
		}

		items = append(items, JSONInt64(value))
	}

	*s = items
	return nil
}

// Int64Slice 返回底层 int64 切片副本
func (s JSONInt64Slice) Int64Slice() []int64 {
	items := make([]int64, 0, len(s))
	for _, item := range s {
		items = append(items, item.Int64())
	}

	return items
}

// MarshalJSON 将 int64 切片序列化为 JSON 字符串数组，避免 JavaScript 大整数精度丢失
func (s JSONInt64Slice) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}

	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, v := range s {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := v.MarshalJSON()
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// NewJSONInt64Slice 将 []int64 转为 JSONInt64Slice
func NewJSONInt64Slice(values []int64) JSONInt64Slice {
	if values == nil {
		return nil
	}

	items := make(JSONInt64Slice, 0, len(values))
	for _, v := range values {
		items = append(items, JSONInt64(v))
	}

	return items
}
