package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileRotator_ClosedFileWriteAndSync(t *testing.T) {
	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       dir,
		LogType:       "info",
		MaxSizeMB:     1,
		RetentionDays: 1,
		MaxBackups:    1,
		MaxTotalMB:    8,
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	t.Cleanup(func() { _ = rot.Close() })

	if err := rot.file.Close(); err != nil {
		t.Fatalf("关闭活跃文件: %v", err)
	}
	if _, err := rot.Write([]byte("x")); err == nil {
		t.Fatal("写入已关闭文件应失败")
	}
	if err := rot.Sync(); err == nil {
		t.Fatal("同步已关闭文件应失败")
	}
}

func TestFileRotator_ReopenFailsWhenDirRemoved(t *testing.T) {
	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       dir,
		LogType:       "error",
		MaxSizeMB:     1,
		RetentionDays: 1,
		MaxBackups:    1,
		MaxTotalMB:    8,
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	if err := rot.file.Close(); err != nil {
		t.Fatalf("关闭活跃文件: %v", err)
	}
	rot.file = nil
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("删除目录: %v", err)
	}
	if _, err := rot.Write([]byte("x")); err == nil {
		t.Fatal("目录被删后重新打开应失败")
	}
}

func TestFileRotator_ValidationReopenAndCleanup(t *testing.T) {
	if _, err := NewFileRotator(RotatorOptions{}); err == nil {
		t.Fatal("空日志类型应失败")
	}
	if _, err := NewFileRotator(RotatorOptions{LogType: "info"}); err == nil {
		t.Fatal("非正滚动参数应失败")
	}
	if _, err := NewFileRotator(RotatorOptions{
		RootDir: t.TempDir(), LogType: "info", MaxSizeMB: 1, RetentionDays: 1, MaxBackups: 1, MaxTotalMB: 1, Timezone: "Not/AZone",
	}); err == nil {
		t.Fatal("非法时区应失败")
	}

	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir: dir, LogType: "info", MaxSizeMB: 1, RetentionDays: 1, MaxBackups: 1, MaxTotalMB: 1, Compress: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	_ = rot.file.Close()
	rot.file = nil
	rot.mu.Unlock()
	if _, err := rot.Write([]byte("reopen")); err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	_ = rot.file.Close()
	rot.file = nil
	rot.mu.Unlock()
	if err := rot.Sync(); err != nil {
		t.Fatal(err)
	}
	rot.mu.Lock()
	active := rot.activePath()
	if rot.file != nil {
		_ = rot.file.Close()
		rot.file = nil
	}
	_ = os.Remove(active)
	rot.size = rot.maxSizeBytes
	rot.mu.Unlock()
	if _, err := rot.Write([]byte("rotated")); err != nil {
		t.Fatal(err)
	}
	var rotateErr error
	rot.reopenAfterRotateFailure(nil)
	rot.reopenAfterRotateFailure(&rotateErr)

	historyDir := filepath.Join(dir, "info")
	_ = os.MkdirAll(filepath.Join(historyDir, "nested"), 0o755)
	_ = os.WriteFile(filepath.Join(historyDir, "19990101-0000001.log"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-0000001.log"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-0000002.log"), []byte("b"), 0o644)
	_ = os.WriteFile(filepath.Join(historyDir, "20990101-note.log"), []byte("bad"), 0o644)
	if rot.scanNextSeq("20990101") < 3 {
		t.Fatal("历史序号应跳过已有文件")
	}
	missing := *rot
	missing.dir = filepath.Join(dir, "missing-seq")
	if missing.scanNextSeq("20990101") != 1 {
		t.Fatal("目录不可读时应从 1 开始")
	}
	rot.maxTotalBytes = 1
	if err := rot.cleanup(); err != nil {
		t.Fatal(err)
	}

	plain := filepath.Join(historyDir, "20990102-0000001.log")
	if err := os.WriteFile(plain, []byte("compress me"), 0o644); err != nil {
		t.Fatal(err)
	}
	compressHistoryFile(plain)
	compressHistoryFile(plain + ".gz")
	compressHistoryFile(filepath.Join(historyDir, "absent.log"))
	if err := rot.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := rot.Write([]byte("closed")); err == nil {
		t.Fatal("关闭后写入应失败")
	}
}
