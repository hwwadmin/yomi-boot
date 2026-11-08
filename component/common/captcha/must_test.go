package captcha

import "testing"

func TestMustNew_NilStorePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("store 为空时 MustNew 应 panic")
		}
	}()
	MustNew(nil, Config{})
}
