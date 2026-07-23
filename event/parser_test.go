package event

import "testing"

// TestParseAndValidate 验证事件 JSON 解析与 eventId 必填校验
func TestParseAndValidate(t *testing.T) {
	t.Run("valid with eventId", func(t *testing.T) {
		evt, ok := ParseAndValidate([]byte(`{"eventId":"e1","topic":"t1","payload":"{}"}`), true)
		if !ok {
			t.Fatal("expected valid")
		}
		if evt.EventID != "e1" || evt.Topic != "t1" {
			t.Fatalf("unexpected event: %+v", evt)
		}
	})

	t.Run("missing eventId when required", func(t *testing.T) {
		_, ok := ParseAndValidate([]byte(`{"topic":"t1","payload":"{}"}`), true)
		if ok {
			t.Fatal("expected invalid")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, ok := ParseAndValidate([]byte(`{`), true)
		if ok {
			t.Fatal("expected invalid")
		}
	})
}
