package captcha

import "testing"

func TestMustNew_ValidConfig(t *testing.T) {
	if svc := MustNew(newSpyStore(), Config{}); svc == nil {
		t.Fatal("MustNew 合法配置不应返回 nil")
	}
}

func TestMustNew_NilStorePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("store 为空时 MustNew 应 panic")
		}
	}()
	MustNew(nil, Config{})
}
