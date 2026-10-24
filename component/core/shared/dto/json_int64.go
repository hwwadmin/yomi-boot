package dto

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// JSONInt64 表示可同时接受 JSON 字符串和数字的 int64 值
type JSONInt64 int64

// UnmarshalJSON 支持从 JSON 字符串或数字反序列化 int64
func (v *JSONInt64) UnmarshalJSON(data []byte) error {
	value, err := parseFlexibleInt64JSON(data)
	if err != nil {
		return err
	}

	*v = JSONInt64(value)
	return nil
}

// MarshalJSON 将 int64 值序列化为 JSON 字符串，避免 JavaScript 大整数精度丢失
func (v JSONInt64) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatInt(int64(v), 10))), nil
}

// Int64 返回底层 int64 值
func (v JSONInt64) Int64() int64 {
	return int64(v)
}

// parseFlexibleInt64JSON 解析灵活的 JSON int64 值，支持字符串和数字两种格式
func parseFlexibleInt64JSON(data []byte) (int64, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, nil
	}

	if trimmed[0] == '"' {
		var raw string
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return 0, errors.Wrap(err, "解析 JSON int64 字符串失败")
		}

		raw = strings.TrimSpace(raw)
		if raw == "" {
			return 0, nil
		}

		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, errors.Wrap(err, "invalid int64 string")
		}

		return value, nil
	}

	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return 0, errors.Wrap(err, "解析 JSON int64 数字失败")
	}

	return number.Int64()
}
