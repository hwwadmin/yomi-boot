package dto

import (
	"encoding/json"
	"testing"
)

func TestJSONInt64_Flexible(t *testing.T) {
	var value JSONInt64
	for _, raw := range []string{`null`, `""`, `"  "`, `"42"`, `42`} {
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
	}
	if value.Int64() != 42 {
		t.Fatalf("Int64 = %d", value.Int64())
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != `"42"` {
		t.Fatalf("marshal = %s err=%v", encoded, err)
	}
	if err := json.Unmarshal([]byte(`"abc"`), &value); err == nil {
		t.Fatal("非法字符串应失败")
	}
	if err := json.Unmarshal([]byte(`true`), &value); err == nil {
		t.Fatal("布尔值应失败")
	}
	if err := json.Unmarshal([]byte(`"12.5"`), &value); err == nil {
		t.Fatal("非整数字符串应失败")
	}
}

func TestJSONInt64Slice_RoundTrip(t *testing.T) {
	var items JSONInt64Slice
	if err := json.Unmarshal([]byte(`null`), &items); err != nil || items != nil {
		t.Fatalf("null = %#v err=%v", items, err)
	}
	if err := json.Unmarshal([]byte(`["1", 2, null, ""]`), &items); err != nil {
		t.Fatalf("mixed: %v", err)
	}
	got := items.Int64Slice()
	if len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("slice = %v", got)
	}
	encoded, err := json.Marshal(items)
	if err != nil || string(encoded) != `["1","2","0","0"]` {
		t.Fatalf("marshal = %s err=%v", encoded, err)
	}
	if encoded, err = json.Marshal(JSONInt64Slice(nil)); err != nil || string(encoded) != "null" {
		t.Fatalf("nil marshal = %s err=%v", encoded, err)
	}
	if NewJSONInt64Slice(nil) != nil {
		t.Fatal("nil 输入应返回 nil")
	}
	built := NewJSONInt64Slice([]int64{7, 8})
	if len(built.Int64Slice()) != 2 || built.Int64Slice()[0] != 7 {
		t.Fatalf("built = %v", built.Int64Slice())
	}
	if err := json.Unmarshal([]byte(`{"a":1}`), &items); err == nil {
		t.Fatal("对象不应解析为数组")
	}
	if err := json.Unmarshal([]byte(`["bad"]`), &items); err == nil {
		t.Fatal("非法元素应失败")
	}
}

func TestValiPage(t *testing.T) {
	var page *PageInput
	page.ValiPage()

	page = &PageInput{}
	page.ValiPage()
	if page.Page != DefaultPage || page.PageSize != DefaultPageSize {
		t.Fatalf("defaults = %+v", page)
	}
	page.PageSize = MaxPageSize + 5
	page.ValiPage()
	if page.PageSize != MaxPageSize {
		t.Fatalf("pageSize = %d", page.PageSize)
	}
}
