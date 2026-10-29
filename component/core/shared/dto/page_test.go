// dto.PageOutput 分页载荷序列化行为单测
//
// @author zwei

package dto

import (
	"encoding/json"
	"testing"
)

// pageOut 测试用业务出参：内嵌分页载荷
type pageOut struct {
	PageOutput[pageItem]
}

type pageItem struct {
	ID int64 `json:"id"`
}

// TestPageOutput_MarshalUnderData 验证内嵌 PageOutput 扁平化为 {items,total}
func TestPageOutput_MarshalUnderData(t *testing.T) {
	out := pageOut{PageOutput: NewPageOutput([]pageItem{{ID: 1}}, 3)}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["PageOutput"]; ok {
		t.Fatalf("内嵌字段应扁平化而非以类型名为键: %s", data)
	}
	if string(raw["items"]) != `[{"id":1}]` || string(raw["total"]) != "3" {
		t.Fatalf("期望 {items,total}，实际 %s", data)
	}
}

// TestNewPageOutput_NormalizeNilItems 验证 nil items 归一为空切片，序列化为 [] 而非 null
func TestNewPageOutput_NormalizeNilItems(t *testing.T) {
	page := NewPageOutput[pageItem](nil, 0)

	data, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"items":[],"total":0}` {
		t.Fatalf("nil items 应序列化为 []，实际 %s", data)
	}
}
