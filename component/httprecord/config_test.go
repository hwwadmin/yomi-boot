package httprecord

import (
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/utils/utf8util"
)

func TestConfigValidate_RejectsNegativeBodySize(t *testing.T) {
	t.Parallel()
	neg := -1
	cfg := Config{MaxRequestBodySize: &neg}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative max_request_body_size")
	}
	cfg = Config{MaxResponseBodySize: &neg}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative max_response_body_size")
	}
}

func TestConfigBodySizeOrDefault(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	if got := cfg.MaxRequestBodySizeOrDefault(); got != defaultHTTPRecordMaxRequestBodySize {
		t.Fatalf("nil request size=%d want %d", got, defaultHTTPRecordMaxRequestBodySize)
	}
	if got := cfg.MaxResponseBodySizeOrDefault(); got != defaultHTTPRecordMaxResponseBodySize {
		t.Fatalf("nil response size=%d want %d", got, defaultHTTPRecordMaxResponseBodySize)
	}

	zero := 0
	cfg.MaxRequestBodySize = &zero
	cfg.MaxResponseBodySize = &zero
	if got := cfg.MaxRequestBodySizeOrDefault(); got != utf8util.Unlimited {
		t.Fatalf("explicit 0 request size=%d want unlimited", got)
	}
	if got := cfg.MaxResponseBodySizeOrDefault(); got != utf8util.Unlimited {
		t.Fatalf("explicit 0 response size=%d want unlimited", got)
	}

	limit := 1024
	cfg.MaxRequestBodySize = &limit
	cfg.MaxResponseBodySize = &limit
	if got := cfg.MaxRequestBodySizeOrDefault(); got != 1024 {
		t.Fatalf("request size=%d want 1024", got)
	}
	if got := cfg.MaxResponseBodySizeOrDefault(); got != 1024 {
		t.Fatalf("response size=%d want 1024", got)
	}
}
