// 工具定义与函数适配测试。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// validSchema 返回一个含必填 city 的合法参数 Schema
func validSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"city": {"type": "string"}},
		"required": ["city"]
	}`)
}

// validSpec 返回一个合法工具定义
func validSpec(name string) Spec {
	return Spec{
		Name:        name,
		Description: "查询指定城市的天气",
		Parameters:  validSchema(),
		Metadata:    map[string]string{"risk": "read"},
	}
}

// requireCode 断言错误为 *Error 且错误码匹配
func requireCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，得到 nil", code)
	}
	var toolErr *Error
	if !errors.As(err, &toolErr) {
		t.Fatalf("错误类型不是 *Error: %T %v", err, err)
	}
	if toolErr.Code != code {
		t.Fatalf("错误码不符: got %s want %s (%v)", toolErr.Code, code, err)
	}
	return toolErr
}

// TestValidateSpec 覆盖工具定义契约校验
func TestValidateSpec(t *testing.T) {
	t.Parallel()
	valid := validSpec("get_weather")
	cases := []struct {
		name string
		spec Spec
		code string
	}{
		{"合法", valid, ""},
		{"名字两端空白", Spec{Name: " get_weather ", Description: "d", Parameters: validSchema()}, ""},
		{"空名", Spec{Name: "  ", Description: "d", Parameters: validSchema()}, CodeInvalidArgument},
		{"超长名", Spec{Name: strings.Repeat("a", MaxToolNameBytes+1), Description: "d", Parameters: validSchema()}, CodeInvalidArgument},
		{"非法字符", Spec{Name: "get.weather", Description: "d", Parameters: validSchema()}, CodeInvalidArgument},
		{"空描述", Spec{Name: "get_weather", Description: "  ", Parameters: validSchema()}, CodeInvalidArgument},
		{"空 Schema", Spec{Name: "get_weather", Description: "d"}, CodeInvalidArgument},
		{"Schema 非对象", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`[]`)}, CodeInvalidArgument},
		{"Schema 非法 JSON", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{`)}, CodeInvalidArgument},
		{"根类型非 object", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"array"}`)}, CodeInvalidArgument},
		{"properties 类型错误", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object","properties":[]}`)}, CodeInvalidArgument},
		{"required 类型错误", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object","required":"city"}`)}, CodeInvalidArgument},
		{"required 空键", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object","required":[""]}`)}, CodeInvalidArgument},
		{"required 重复", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object","required":["city","city"]}`)}, CodeInvalidArgument},
		{"$ref 不误杀", Spec{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"$ref":"#/$defs/City"}},"required":["city"],"$defs":{"City":{"type":"string"}}}`)}, ""},
		{"Metadata 空键", Spec{Name: "get_weather", Description: "d", Parameters: validSchema(), Metadata: map[string]string{" ": "v"}}, CodeInvalidArgument},
		{"Metadata 修剪后重复", Spec{Name: "get_weather", Description: "d", Parameters: validSchema(), Metadata: map[string]string{"risk": "read", " risk ": "write"}}, CodeInvalidArgument},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSpec(c.spec)
			if c.code == "" {
				if err != nil {
					t.Fatalf("合法定义被拒: %v", err)
				}
				return
			}
			requireCode(t, err, c.code)
		})
	}
}

// TestNormalizeSpec 覆盖规范化与 required 提取
func TestNormalizeSpec(t *testing.T) {
	t.Parallel()
	spec := validSpec("  get_weather  ")
	spec.Description = "  查询天气  "
	spec.Metadata = map[string]string{" risk ": "read"}
	normalized, required, err := normalizeSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Name != "get_weather" || normalized.Description != "查询天气" {
		t.Fatalf("名字或描述未修剪: %+v", normalized)
	}
	if normalized.Metadata["risk"] != "read" {
		t.Fatalf("Metadata 键未修剪: %+v", normalized.Metadata)
	}
	if len(required) != 1 || required[0] != "city" {
		t.Fatalf("required 不符: %+v", required)
	}
}

// TestCloneSpecIsDeep 覆盖定义深拷贝
func TestCloneSpecIsDeep(t *testing.T) {
	t.Parallel()
	spec := validSpec("get_weather")
	cloned := cloneSpec(spec)
	cloned.Parameters[0] = '['
	cloned.Metadata["risk"] = "write"
	if spec.Parameters[0] != '{' {
		t.Fatal("Parameters 未深拷贝")
	}
	if spec.Metadata["risk"] != "read" {
		t.Fatal("Metadata 未深拷贝")
	}
}

// TestValidateArguments 覆盖参数契约的可执行形式
func TestValidateArguments(t *testing.T) {
	t.Parallel()
	spec := validSpec("get_weather")
	if err := ValidateArguments(spec, json.RawMessage(`{"city":"北京"}`)); err != nil {
		t.Fatalf("合法参数被拒: %v", err)
	}
	for _, args := range []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`{}`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"city":`),
	} {
		if err := ValidateArguments(spec, args); err == nil {
			t.Fatalf("非法参数应被拒: %s", args)
		} else {
			requireCode(t, err, CodeInvalidArgument)
		}
	}
	invalidSpec := spec
	invalidSpec.Parameters = json.RawMessage(`{"type":"array"}`)
	err := ValidateArguments(invalidSpec, json.RawMessage(`{}`))
	requireCode(t, err, CodeInvalidArgument)
}

// TestNewFunc 覆盖函数工具构造与执行
func TestNewFunc(t *testing.T) {
	t.Parallel()
	if _, err := NewFunc(validSpec("get_weather"), nil); err == nil {
		t.Fatal("nil 函数应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
	if _, err := NewFunc(Spec{}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}); err == nil {
		t.Fatal("非法定义应被拒")
	}

	current, err := NewFunc(validSpec("get_weather"), func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if string(args) != `{"city":"北京"}` {
			t.Fatalf("参数不符: %s", args)
		}
		return json.RawMessage(`{"weather":"晴"}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := current.Spec()
	spec.Metadata["risk"] = "write"
	if current.Spec().Metadata["risk"] != "read" {
		t.Fatal("Spec 应返回副本")
	}
	content, err := current.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
	if err != nil || string(content) != `{"weather":"晴"}` {
		t.Fatalf("执行不符: %s %v", content, err)
	}
}
