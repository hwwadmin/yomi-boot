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

// Spec 工具定义；Name、Description、Parameters 面向模型，Fill、Metadata 仅宿主可见
type Spec struct {
	Name        string            `json:"name"`        // Name 是模型调用时使用的工具名
	Description string            `json:"description"` // Description 是模型理解工具用途的描述
	Parameters  json.RawMessage   `json:"parameters"`  // Parameters 是根类型为 object 的 JSON Schema
	Fill        FillSpec          `json:"-"`           // Fill 是宿主侧填槽规则，不发给模型
	Metadata    map[string]string `json:"-"`           // Metadata 是宿主标签，不发给模型
}

// FillSpec 描述填槽语义；与 Parameters 一起构成模型提示与运行时校验的完整契约
type FillSpec struct {
	Required       []string // Required 是缺槽候选键；simple tool 用默认检查，resolved tool 由 Validate 判定
	IdentifierKeys []string // IdentifierKeys 是必须能在证据原文中找到的标识符键
	Guidance       string   // Guidance 是给填槽模型的动作指导
	Example        string   // Example 是期望 JSON 示例，只示范形状
	MissingPrompt  string   // MissingPrompt 是缺槽时的默认追问文案
	EvidenceOnly   bool     // EvidenceOnly 为 true 时跳过模型，直接从证据映射
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
	fill, err := normalizeFillSpec(spec.Fill)
	if err != nil {
		return Spec{}, nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, name, "", err)
	}
	return Spec{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Fill:        fill,
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

// normalizeFillSpec 修剪填槽键与文案，并校验示例是 JSON 对象
func normalizeFillSpec(fill FillSpec) (FillSpec, error) {
	required, err := normalizeKeyList(fill.Required, "Required")
	if err != nil {
		return FillSpec{}, err
	}
	identifierKeys, err := normalizeKeyList(fill.IdentifierKeys, "IdentifierKeys")
	if err != nil {
		return FillSpec{}, err
	}
	example := strings.TrimSpace(fill.Example)
	if example != "" && (example[0] != '{' || !json.Valid([]byte(example))) {
		return FillSpec{}, errors.New("ai/tool: Fill.Example 必须是合法 JSON 对象")
	}
	return FillSpec{
		Required:       required,
		IdentifierKeys: identifierKeys,
		Guidance:       strings.TrimSpace(fill.Guidance),
		Example:        example,
		MissingPrompt:  strings.TrimSpace(fill.MissingPrompt),
		EvidenceOnly:   fill.EvidenceOnly,
	}, nil
}

// normalizeKeyList 修剪键并拒绝空键与重复键
func normalizeKeyList(keys []string, field string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return nil, errors.Newf("ai/tool: Fill.%s 含空键", field)
		}
		if _, ok := seen[trimmed]; ok {
			return nil, errors.Newf("ai/tool: Fill.%s 键 %q 重复", field, trimmed)
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out, nil
}

// fillSpecEmpty 判断填槽规则是否全部为零值
func fillSpecEmpty(fill FillSpec) bool {
	return len(fill.Required) == 0 &&
		len(fill.IdentifierKeys) == 0 &&
		fill.Guidance == "" &&
		fill.Example == "" &&
		fill.MissingPrompt == "" &&
		!fill.EvidenceOnly
}

// cloneSpec 深拷贝工具定义，避免调用方修改返回值污染注册表
func cloneSpec(spec Spec) Spec {
	out := spec
	out.Parameters = append(json.RawMessage(nil), spec.Parameters...)
	out.Fill.Required = append([]string(nil), spec.Fill.Required...)
	out.Fill.IdentifierKeys = append([]string(nil), spec.Fill.IdentifierKeys...)
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
