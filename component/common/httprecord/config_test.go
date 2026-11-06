package httprecord

import (
	"testing"
)

func TestConfigValidate_RejectsNonPositiveBodySize(t *testing.T) {
	t.Parallel()
	for _, n := range []int{-1, 0} {
		req := n
		resp := n
		if err := (Config{MaxRequestBodySize: &req}).Validate(); err == nil {
			t.Fatalf("max_request_body_size=%d 应拒绝", n)
		}
		if err := (Config{MaxResponseBodySize: &resp}).Validate(); err == nil {
			t.Fatalf("max_response_body_size=%d 应拒绝", n)
		}
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
	if got := cfg.MaxRequestBodySizeOrDefault(); got != defaultHTTPRecordMaxRequestBodySize {
		t.Fatalf("explicit 0 request size=%d want default %d", got, defaultHTTPRecordMaxRequestBodySize)
	}
	if got := cfg.MaxResponseBodySizeOrDefault(); got != defaultHTTPRecordMaxResponseBodySize {
		t.Fatalf("explicit 0 response size=%d want default %d", got, defaultHTTPRecordMaxResponseBodySize)
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
