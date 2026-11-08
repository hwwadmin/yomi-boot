package httprecord

import "testing"

func TestPublishContext_NilParent(t *testing.T) {
	ctx, cancel := publishContext(nil)
	defer cancel()
	if ctx == nil || ctx.Err() != nil {
		t.Fatal("空父上下文应得到仍有效的投递上下文")
	}
}
