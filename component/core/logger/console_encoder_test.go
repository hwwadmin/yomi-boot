package logger

import (
	"strings"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/shared"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestEncodeEntry_ReusesLineBuffer(t *testing.T) {
	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.EncodeTime = colorTimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encCfg.EncodeCaller = colorCallerEncoder
	enc := newTracingConsoleEncoder(encCfg)

	when := time.Date(2026, 11, 6, 15, 40, 0, 0, time.UTC)
	caller := zapcore.EntryCaller{Defined: true, File: "app.go", Line: 10}

	firstTP := "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	secondTP := "00-cccccccccccccccccccccccccccccccc-dddddddddddddddd-01"
	first, err := enc.EncodeEntry(zapcore.Entry{
		Time: when, Level: zapcore.InfoLevel, Message: "first-line", Caller: caller,
	}, []zapcore.Field{zap.String(shared.Traceparent, firstTP)})
	if err != nil {
		t.Fatalf("encode first: %v", err)
	}
	if !strings.Contains(first.String(), "first-line") || !strings.Contains(first.String(), firstTP) {
		t.Fatalf("第一行 = %q", first.String())
	}
	first.Free()

	second, err := enc.EncodeEntry(zapcore.Entry{
		Time: when, Level: zapcore.InfoLevel, Message: "second-line", Caller: caller,
	}, []zapcore.Field{zap.String(shared.Traceparent, secondTP)})
	if err != nil {
		t.Fatalf("encode second: %v", err)
	}
	defer second.Free()
	got := second.String()
	if strings.Contains(got, "first-line") || strings.Contains(got, firstTP) {
		t.Fatalf("复用的缓冲还留着上一行: %q", got)
	}
	if !strings.Contains(got, "second-line") || !strings.Contains(got, secondTP) {
		t.Fatalf("第二行 = %q", got)
	}
}
