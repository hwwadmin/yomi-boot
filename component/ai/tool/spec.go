// 定义工具描述与规格校验。
//
// @author zwei

package tool

import (
	"bytes"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// MaxToolNameBytes 是工具名字节上限，与主流模型服务对齐
const MaxToolNameBytes = 64

// Spec 工具定义；Name、Description、Parameters 面向模型，Metadata 仅宿主可见
type Spec struct {
	Name        string            `json:"name"`        // Name 是模型调用时使用的工具名
	Description string            `json:"description"` // Description 是模型理解工具用途的描述
	Parameters  json.RawMessage   `json:"parameters"`  // Parameters 是根类型为 object 的 JSON Schema
	Metadata    map[string]string `json:"-"`           // Metadata 是宿主标签，不发给模型
}

// ValidateSpec 校验工具定义；供自定义 Tool 实现与注册表复用，非法返回 CodeInvalidArgument
func ValidateSpec(spec Spec) error {
	_, _, err := normalizeSpec(spec)
	return err
}

// ValidateArguments 校验参数形态与根级 required；供自定义 Tool 实现与直接调用方复用。
// 不做类型、嵌套与业务校验，也不应用任何大小上限。
func ValidateArguments(spec Spec, args json.RawMessage) error {
	name := strings.TrimSpace(spec.Name)
	_, required, err := parseParameters(spec.Parameters)
	if err != nil {
		return newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", err)
	}
	_, object, err := normalizeArguments(args, 0, name, "")
	if err != nil {
		return err
	}
	return validateRequired(required, object, name, "")
}

// normalizeSpec 校验并规范化工具定义，同时返回根级 required 键
func normalizeSpec(spec Spec) (Spec, []string, error) {
	name := strings.TrimSpace(spec.Name)
	if err := validateName(name); err != nil {
		return Spec{}, nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", err)
	}
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		return Spec{}, nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", errors.New("ai/tool: 工具描述为空"))
	}
	parameters, required, err := parseParameters(spec.Parameters)
	if err != nil {
		return Spec{}, nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", err)
	}
	metadata, err := normalizeMetadata(spec.Metadata)
	if err != nil {
		return Spec{}, nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", err)
	}
	return Spec{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Metadata:    metadata,
	}, required, nil
}

// validateName 校验工具名为 ASCII 字母、数字、下划线与短横线，长度不超过上限
func validateName(name string) error {
	if name == "" {
		return errors.New("ai/tool: 工具名为空")
	}
	if len(name) > MaxToolNameBytes {
		return errors.Newf("ai/tool: 工具名超过 %d 字节", MaxToolNameBytes)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_', c == '-':
		default:
			return errors.Newf("ai/tool: 工具名包含非法字符 %q", c)
		}
	}
	return nil
}

// parseParameters 校验参数 Schema 形态并返回拷贝与根级 required 键
//
// 只校验根节点为 object、required 为不重复的非空字符串数组；
// 不解析 $ref/allOf，也不校验嵌套类型、枚举与范围，这些由 provider 与工具实现负责。
func parseParameters(raw json.RawMessage) (json.RawMessage, []string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, errors.New("ai/tool: 参数 Schema 为空")
	}
	// 先确保根节点是 JSON 对象，避免 json.Unmarshal 把 null 静默解成零值
	if trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, nil, errors.New("ai/tool: 参数 Schema 必须是合法 JSON 对象")
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return nil, nil, errors.Wrap(err, "ai/tool: 参数 Schema 解析失败")
	}
	if schema.Type != "object" {
		return nil, nil, errors.New("ai/tool: 参数 Schema 根类型必须是 object")
	}
	seen := make(map[string]struct{}, len(schema.Required))
	for _, key := range schema.Required {
		if strings.TrimSpace(key) == "" {
			return nil, nil, errors.New("ai/tool: 参数 Schema required 含空键")
		}
		if _, ok := seen[key]; ok {
			return nil, nil, errors.Newf("ai/tool: 参数 Schema required 键 %q 重复", key)
		}
		seen[key] = struct{}{}
	}
	parameters := append(json.RawMessage(nil), trimmed...)
	required := append([]string(nil), schema.Required...)
	return parameters, required, nil
}

// normalizeMetadata 修剪元数据键并拒绝空键与修剪后重名
func normalizeMetadata(metadata map[string]string) (map[string]string, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string, len(metadata))
	for key, value := range metadata {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return nil, errors.New("ai/tool: Metadata 键为空")
		}
		if _, ok := normalized[trimmed]; ok {
			return nil, errors.Newf("ai/tool: Metadata 键 %q 重复", trimmed)
		}
		normalized[trimmed] = value
	}
	return normalized, nil
}

// cloneSpec 深拷贝工具定义，避免调用方修改返回值污染注册表
func cloneSpec(spec Spec) Spec {
	out := spec
	out.Parameters = append(json.RawMessage(nil), spec.Parameters...)
	if len(spec.Metadata) == 0 {
		out.Metadata = nil
		return out
	}
	out.Metadata = make(map[string]string, len(spec.Metadata))
	for key, value := range spec.Metadata {
		out.Metadata[key] = value
	}
	return out
}
