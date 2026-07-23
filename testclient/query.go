// Package testclient 提供接口集成测试用的 HTTP 请求封装
//
// @author zwei
package testclient

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// BuildQuery 将对象字段解析为 URL 查询参数字符串（不含前导 ?）。
// 支持 struct 与 map[string]any；字段名优先取 query 标签，其次 json 标签，最后使用字段名。
// 零值字段会被跳过。
func BuildQuery(v any) (string, error) {
	values, err := toQueryValues(v)
	if err != nil {
		return "", err
	}
	return values.Encode(), nil
}

// AppendQuery 将对象解析为查询参数并拼接到 baseURL，返回完整 URL。
func AppendQuery(baseURL string, v any) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("testclient: 请求地址不能为空")
	}

	values, err := toQueryValues(v)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return baseURL, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("testclient: 解析请求地址失败: %w", err)
	}

	query := u.Query()
	for key, items := range values {
		for _, item := range items {
			query.Add(key, item)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// GetWithQuery 将 query 对象解析为查询参数后发起 GET 请求
func GetWithQuery(baseURL, token string, query any) (body string, ok bool) {
	return defaultClient.GetWithQuery(baseURL, token, query)
}

// GetWithQuery 将 query 对象解析为查询参数后发起 GET 请求
func (c *Client) GetWithQuery(baseURL, token string, query any) (body string, ok bool) {
	fullURL, err := AppendQuery(baseURL, query)
	if err != nil {
		return "testclient: " + err.Error(), false
	}
	return c.Get(fullURL, token)
}

// toQueryValues 将对象转换为 url.Values，支持 struct 与 map[string]any
func toQueryValues(v any) (url.Values, error) {
	if v == nil {
		return url.Values{}, nil
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return url.Values{}, nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		return structToValues(rv)
	case reflect.Map:
		return mapToValues(rv)
	default:
		return nil, fmt.Errorf("testclient: 不支持将 %T 转为查询参数", v)
	}
}

// structToValues 将 struct 反射为 url.Values，嵌入字段会递归展开
func structToValues(rv reflect.Value) (url.Values, error) {
	values := url.Values{}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			fv := rv.Field(i)
			for fv.Kind() == reflect.Pointer {
				if fv.IsNil() {
					fv = reflect.Value{}
					break
				}
				fv = fv.Elem()
			}
			if !fv.IsValid() || fv.Kind() != reflect.Struct {
				continue
			}
			nested, err := structToValues(fv)
			if err != nil {
				return nil, err
			}
			mergeValues(values, nested)
			continue
		}

		name := queryFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		if err := appendValue(values, name, rv.Field(i)); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// mapToValues 将 map[string]any 转换为 url.Values
func mapToValues(rv reflect.Value) (url.Values, error) {
	if rv.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("testclient: 仅支持 map[string]any 转为查询参数")
	}

	values := url.Values{}
	iter := rv.MapRange()
	for iter.Next() {
		key := strings.TrimSpace(iter.Key().String())
		if key == "" {
			continue
		}
		if err := appendValue(values, key, iter.Value()); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// queryFieldName 解析 struct 字段对应的查询参数名
func queryFieldName(field reflect.StructField) string {
	if tag := strings.TrimSpace(field.Tag.Get("query")); tag != "" {
		return strings.Split(tag, ",")[0]
	}
	if tag := strings.TrimSpace(field.Tag.Get("json")); tag != "" {
		return strings.Split(tag, ",")[0]
	}
	return field.Name
}

// appendValue 将单个字段值写入 url.Values，零值字段会被跳过
func appendValue(values url.Values, key string, rv reflect.Value) error {
	if isZeroValue(rv) {
		return nil
	}

	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if isZeroValue(rv.Index(i)) {
				continue
			}
			text, err := valueToString(rv.Index(i))
			if err != nil {
				return err
			}
			values.Add(key, text)
		}
	default:
		text, err := valueToString(rv)
		if err != nil {
			return err
		}
		values.Add(key, text)
	}
	return nil
}

// valueToString 将 reflect.Value 转换为查询参数字符串
func valueToString(rv reflect.Value) (string, error) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.String:
		return rv.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 64), nil
	default:
		if stringer, ok := rv.Interface().(fmt.Stringer); ok {
			return stringer.String(), nil
		}
		return fmt.Sprint(rv.Interface()), nil
	}
}

// isZeroValue 判断 reflect.Value 是否为零值
func isZeroValue(rv reflect.Value) bool {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.String:
		return rv.String() == ""
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// mergeValues 将 src 中的查询参数合并到 dst
func mergeValues(dst, src url.Values) {
	for key, items := range src {
		for _, item := range items {
			dst.Add(key, item)
		}
	}
}
