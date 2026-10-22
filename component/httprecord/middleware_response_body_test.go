package httprecord

import (
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"gitee.com/hq_project/yomi-boot/core/utils/utf8util"
)

func newTestBodyWriter(maxSize int) *bodyRecordWriter {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	return newBodyRecordWriter(c.Writer, maxSize)
}

func TestBodyRecordWriter_UTF8Boundary(t *testing.T) {
	w := newTestBodyWriter(5)
	payload := []byte("报错")
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if !w.truncated {
		t.Fatal("want truncated")
	}
	got := w.recordedBody()
	if !utf8.ValidString(got) {
		t.Fatalf("recorded body is not valid UTF-8: %q %x", got, got)
	}
	if got != "报" {
		t.Fatalf("got %q want 报", got)
	}
}

func TestBodyRecordWriter_SplitRuneThenTruncate(t *testing.T) {
	w := newTestBodyWriter(5)
	// 报=E6 8A A5，错=E9 94 99；第一次写满「报」+ 错的首字节，第二次再截断
	if _, err := w.Write([]byte("报\xE9")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("\x94\x99")); err != nil {
		t.Fatal(err)
	}
	if !w.truncated {
		t.Fatal("want truncated")
	}
	got := w.recordedBody()
	if !utf8.ValidString(got) {
		t.Fatalf("recorded body is not valid UTF-8: %x", got)
	}
	if got != "报" {
		t.Fatalf("got %q want 报", got)
	}
}

func TestBodyRecordWriter_Unlimited(t *testing.T) {
	w := newTestBodyWriter(utf8util.Unlimited)
	raw := []byte("有基于WebSocket的消息推送的API吗")
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if w.truncated {
		t.Fatal("unlimited should not truncate")
	}
	if got := w.recordedBody(); got != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestBodyRecordWriter_ClientStillReceivesFullBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newBodyRecordWriter(c.Writer, 3)
	payload := []byte("你好")
	n, err := w.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Fatalf("Write n=%d want %d", n, len(payload))
	}
	if rec.Body.String() != "你好" {
		t.Fatalf("client got %q want 你好", rec.Body.String())
	}
	if got := w.recordedBody(); got != "你" {
		t.Fatalf("record got %q want 你", got)
	}
}
