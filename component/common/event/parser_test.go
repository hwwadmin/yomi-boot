package event

import (
	"strings"
	"testing"
)

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

	t.Run("eventId at column width", func(t *testing.T) {
		body := `{"eventId":"` + strings.Repeat("名", maxEventIDLen) + `","topic":"t1","payload":"{}"}`
		if _, ok := ParseAndValidate([]byte(body), true); !ok {
			t.Fatal("列宽上限内的 eventId 应通过")
		}
	})

	t.Run("eventId over column width", func(t *testing.T) {
		body := `{"eventId":"` + strings.Repeat("e", maxEventIDLen+1) + `","topic":"t1","payload":"{}"}`
		if _, ok := ParseAndValidate([]byte(body), true); ok {
			t.Fatal("超长 eventId 应抛弃")
		}
	})

	t.Run("topic over column width", func(t *testing.T) {
		body := `{"eventId":"e1","topic":"` + strings.Repeat("题", maxTopicLen+1) + `","payload":"{}"}`
		if _, ok := ParseAndValidate([]byte(body), false); ok {
			t.Fatal("超长 topic 应抛弃")
		}
	})
}
