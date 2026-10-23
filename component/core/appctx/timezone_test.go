package appctx

import (
	"testing"
	"time"
)

// TestLockTimezone 验证锁定时区后会改写进程 time.Local
func TestLockTimezone(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	if err := LockTimezone("Asia/Shanghai"); err != nil {
		t.Fatalf("LockTimezone(Asia/Shanghai): %v", err)
	}
	if got := time.Local.String(); got != "Asia/Shanghai" {
		t.Fatalf("time.Local = %s, want Asia/Shanghai", got)
	}

	if err := LockTimezone(""); err != nil {
		t.Fatalf("LockTimezone(empty): %v", err)
	}
	if got := time.Local.String(); got != DefaultTimezone {
		t.Fatalf("time.Local = %s, want %s", got, DefaultTimezone)
	}
}

// TestLockTimezone_RejectsInvalid 验证拒绝 Local 与非法时区
func TestLockTimezone_RejectsInvalid(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	for _, tz := range []string{"Local", "local", "Mars/Base"} {
		if err := LockTimezone(tz); err == nil {
			t.Fatalf("LockTimezone(%q) expected error", tz)
		}
	}
	if time.Local != original {
		t.Fatal("failed LockTimezone must not change time.Local")
	}
}
