package httprecord

import (
	"context"
	"testing"
)

func TestPublishContext_BackgroundParent(t *testing.T) {
	ctx, cancel := publishContext(context.Background())
	defer cancel()
	if ctx == nil || ctx.Err() != nil {
		t.Fatal("空父上下文应得到仍有效的投递上下文")
	}
}
