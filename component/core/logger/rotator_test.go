package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestFileRotator_CompressHistoryOnSizeRotate 验证按大小滚动产生历史文件时，
// compress=true 会启动异步 goroutine 把历史 .log 压成 .log.gz（成功后删除原文件，POSIX 下断言）
func TestFileRotator_CompressHistoryOnSizeRotate(t *testing.T) {
	dir := t.TempDir()
	rot, err := NewFileRotator(RotatorOptions{
		RootDir:       dir,
		LogType:       "info",
		MaxSizeMB:     1,
		RetentionDays: 1,
		MaxBackups:    3,
		MaxTotalMB:    8,
		Compress:      true,
	})
	if err != nil {
		t.Fatalf("NewFileRotator: %v", err)
	}
	t.Cleanup(func() { _ = rot.Close() })

	if _, err := rot.Write([]byte("rotate me")); err != nil {
		t.Fatalf("写入: %v", err)
	}
	// MaxSizeMB 最小 1MB，与现有用例一致直接把 size 顶到上限，让下一笔写入触发按大小滚动
	rot.mu.Lock()
	historyPath := filepath.Join(rot.dir, fmt.Sprintf("%s-%07d.log", rot.curDate, rot.nextSeq))
	rot.size = rot.maxSizeBytes
	rot.mu.Unlock()
	if _, err := rot.Write([]byte("trigger")); err != nil {
		t.Fatalf("触发滚动: %v", err)
	}

	// 压缩在异步 goroutine 里执行：用 deadline 轮询等 .log.gz 产物出现且内容完整可解
	// （gzip 流能完整解出原文 ⟺ goroutine 已跑完 copy+close），不靠裸 sleep 定成败
	deadline := time.Now().Add(2 * time.Second)
	var readErr error
	for {
		content, gzErr := readGzipFile(historyPath + ".gz")
		if gzErr == nil && string(content) == "rotate me" {
			readErr = nil
			break
		}
		readErr = gzErr
		if time.Now().After(deadline) {
			t.Fatalf("压缩 goroutine 未完成: %v", readErr)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// compressHistoryFile 成功后会在读句柄关闭之后删除原文件（defer 内先 Close 再 Remove）
	for {
		if _, statErr := os.Stat(historyPath); os.IsNotExist(statErr) {
			break
		} else if statErr != nil {
			t.Fatalf("stat 历史文件: %v", statErr)
		}
		if time.Now().After(deadline) {
			t.Fatalf("压缩完成后原历史文件未被删除: %s", historyPath)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readGzipFile 读取并解压 gzip 文件；文件不存在或流不完整时返回错误
func readGzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
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
	savedDir := rot.dir
	rot.dir = filepath.Join(dir, "missing-seq")
	if rot.scanNextSeq("20990101") != 1 {
		t.Fatal("目录不可读时应从 1 开始")
	}
	rot.dir = savedDir
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
