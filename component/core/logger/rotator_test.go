package logger

import (
	"os"
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
