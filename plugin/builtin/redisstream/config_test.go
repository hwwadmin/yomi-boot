package redisstreamplugin

import (
	"testing"
	"time"
)

func TestToConfigSendTimeout(t *testing.T) {
	t.Parallel()
	cfg := BootConfig{
		PublishWorker: PublishWorkerBootConfig{SendTimeoutSeconds: 8},
	}.ToConfig()
	if cfg.PublishWorker.SendTimeoutSeconds != 8 {
		t.Fatalf("SendTimeoutSeconds = %d, want 8", cfg.PublishWorker.SendTimeoutSeconds)
	}
	if got := cfg.PublishWorker.SendTimeout(); got != 8*time.Second {
		t.Fatalf("SendTimeout = %v, want 8s", got)
	}
}
