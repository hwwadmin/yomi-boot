package db

import (
	"testing"
	"time"
)

func TestMaintainInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		idle time.Duration
		want time.Duration
	}{
		{name: "过短夹到5s", idle: 2 * time.Second, want: 5 * time.Second},
		{name: "一半落在区间内", idle: 40 * time.Second, want: 20 * time.Second},
		{name: "过长夹到1m", idle: 10 * time.Minute, want: time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := maintainInterval(tc.idle); got != tc.want {
				t.Fatalf("maintainInterval(%v)=%v, want %v", tc.idle, got, tc.want)
			}
		})
	}
}

func TestIdleKeeperStopIdempotent(t *testing.T) {
	t.Parallel()

	var k *idleKeeper
	k.Stop() // nil 安全

	k = startIdleKeeper(nil, 2, time.Second)
	if k != nil {
		t.Fatal("sqlDB=nil 不应启动 keeper")
	}

	k = startIdleKeeper(nil, 0, time.Second)
	if k != nil {
		t.Fatal("minIdle=0 不应启动 keeper")
	}
}
