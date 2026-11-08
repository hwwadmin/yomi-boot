package timeutil

import (
	"testing"
	"time"
)

func TestUnixHelpers(t *testing.T) {
	stamp := time.UnixMilli(1_700_000_000_000)
	if ToUnix(stamp) != stamp.UnixMilli() {
		t.Fatalf("ToUnix = %d", ToUnix(stamp))
	}
	if ToUnixPtr(nil) != nil {
		t.Fatal("nil 时间应返回 nil")
	}
	got := ToUnixPtr(&stamp)
	if got == nil || *got != stamp.UnixMilli() {
		t.Fatalf("ToUnixPtr = %v", got)
	}
}
