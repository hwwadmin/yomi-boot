// 泛型工具适配与参数解码测试。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type typedArgs struct {
	City string `json:"city"`
}

type typedResult struct {
	City    string `json:"city"`
	Weather string `json:"weather"`
}

// TestNewTyped 覆盖解码、编码与错误分类
func TestNewTyped(t *testing.T) {
	t.Parallel()
	if _, err := NewTyped[typedArgs, typedResult](validSpec("get_weather"), nil); err == nil {
		t.Fatal("nil 函数应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}

	current, err := NewTyped(validSpec("get_weather"), func(_ context.Context, in typedArgs) (typedResult, error) {
		if in.City != "北京" {
			t.Fatalf("解码不符: %+v", in)
		}
		return typedResult{City: in.City, Weather: "晴"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := current.Invoke(context.Background(), Call{
		Name:      "get_weather",
		Arguments: json.RawMessage(`{"city":"北京","unknown":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"city":"北京","weather":"晴"}` {
		t.Fatalf("编码不符: %s", content)
	}
}

// TestNewTypedDecodeError 覆盖参数类型不匹配
func TestNewTypedDecodeError(t *testing.T) {
	t.Parallel()
	current, err := NewTyped(validSpec("get_weather"), func(_ context.Context, in typedArgs) (typedResult, error) {
		t.Fatal("类型不匹配时不应执行")
		return typedResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = current.Invoke(context.Background(), Call{
		Name:      "get_weather",
		Arguments: json.RawMessage(`{"city":123}`),
	})
	requireCode(t, err, CodeInvalidArgument)
}

// TestNewTypedMarshalError 覆盖返回值编码失败
func TestNewTypedMarshalError(t *testing.T) {
	t.Parallel()
	current, err := NewTyped(Spec{Name: "bad", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, struct{}) (chan int, error) {
			return make(chan int), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	_, err = current.Invoke(context.Background(), Call{Name: "bad", Arguments: json.RawMessage(`{}`)})
	requireCode(t, err, CodeBadResponse)
}

// TestDecodeArgs 覆盖归一、对象校验与类型解码
func TestDecodeArgs(t *testing.T) {
	t.Parallel()
	empty, err := DecodeArgs[typedArgs](nil)
	if err != nil || empty.City != "" {
		t.Fatalf("空参数应解码为零值: %+v %v", empty, err)
	}
	null, err := DecodeArgs[typedArgs](json.RawMessage(`null`))
	if err != nil || null.City != "" {
		t.Fatalf("null 应归一为 {}: %+v %v", null, err)
	}
	parsed, err := DecodeArgs[typedArgs](json.RawMessage(`{"city":"北京"}`))
	if err != nil || parsed.City != "北京" {
		t.Fatalf("解码不符: %+v %v", parsed, err)
	}
	if _, err := DecodeArgs[typedArgs](json.RawMessage(`[]`)); err == nil {
		t.Fatal("非对象应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
	if _, err := DecodeArgs[typedArgs](json.RawMessage(`{"city":123}`)); err == nil {
		t.Fatal("类型不匹配应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
}
