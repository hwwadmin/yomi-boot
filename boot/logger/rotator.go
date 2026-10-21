// Package logger 提供文件日志滚动能力
//
// @author zwei
package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

var historyNamePattern = regexp.MustCompile(`^(\d{8})-(\d{7})\.log(?:\.gz)?$`)

// FileRotator 按大小或自然日滚动的日志写入器
// 活跃文件：{dir}/{logType}.log
// 历史文件：{dir}/{YYYYMMDD}-{seq7}.log[.gz]
type FileRotator struct {
	dir           string
	logType       string
	maxSizeBytes  int64
	retentionDays int
	maxBackups    int
	maxTotalBytes int64
	compress      bool
	location      *time.Location
	rootDir       string // 用于 max_total_mb 统计的 logs 根目录

	mu      sync.Mutex
	file    *os.File
	size    int64
	curDate string
	nextSeq int
	closed  bool

	// cleanupMu 防止并发清理叠跑；清理不持 mu，避免阻塞 Write
	cleanupMu sync.Mutex
}

// RotatorOptions 创建 FileRotator 的参数
type RotatorOptions struct {
	RootDir       string
	LogType       string
	MaxSizeMB     int
	RetentionDays int
	MaxBackups    int
	MaxTotalMB    int
	Compress      bool
	Timezone      string // 为空时使用进程时区（由 app.timezone 在启动时锁定）
}

// NewFileRotator 创建并打开活跃日志文件
func NewFileRotator(opts RotatorOptions) (*FileRotator, error) {
	if strings.TrimSpace(opts.LogType) == "" {
		return nil, errors.New("logType 不能为空")
	}
	if opts.MaxSizeMB <= 0 || opts.RetentionDays <= 0 || opts.MaxBackups <= 0 || opts.MaxTotalMB <= 0 {
		return nil, errors.New("滚动参数必须 > 0")
	}
	loc := time.Local
	if tzName := strings.TrimSpace(opts.Timezone); tzName != "" {
		var err error
		loc, err = time.LoadLocation(tzName)
		if err != nil {
			return nil, errors.Wrap(err, "加载日志时区失败")
		}
	}
	root := filepath.Clean(opts.RootDir)
	dir := filepath.Join(root, opts.LogType)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Wrap(err, "创建日志目录失败")
	}

	r := &FileRotator{
		dir:           dir,
		logType:       opts.LogType,
		maxSizeBytes:  int64(opts.MaxSizeMB) * 1024 * 1024,
		retentionDays: opts.RetentionDays,
		maxBackups:    opts.MaxBackups,
		maxTotalBytes: int64(opts.MaxTotalMB) * 1024 * 1024,
		compress:      opts.Compress,
		location:      loc,
		rootDir:       root,
	}
	if err := r.openActive(); err != nil {
		return nil, err
	}
	return r, nil
}

// Write 实现 io.Writer，必要时先滚动再写入
func (r *FileRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.file == nil {
		return 0, errors.New("日志文件已关闭")
	}
	if err := r.rotateIfNeededLocked(int64(len(p))); err != nil {
		return 0, err
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	if err != nil {
		return n, errors.Wrap(err, "写入日志文件失败")
	}
	return n, nil
}

// Sync 刷新文件缓冲
func (r *FileRotator) Sync() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	if err := r.file.Sync(); err != nil {
		return errors.Wrap(err, "同步日志文件失败")
	}
	return nil
}

// Close 关闭活跃文件
func (r *FileRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	if err != nil {
		return errors.Wrap(err, "关闭日志文件失败")
	}
	return nil
}

// activePath 返回当前类型活跃日志文件路径
func (r *FileRotator) activePath() string {
	return filepath.Join(r.dir, r.logType+".log")
}

// openActive 打开（或创建）活跃日志文件并初始化日期序号
func (r *FileRotator) openActive() error {
	path := r.activePath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.Wrap(err, "打开日志文件失败")
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return errors.Wrap(err, "读取日志文件信息失败")
	}
	now := time.Now().In(r.location)
	r.curDate = now.Format("20060102")
	r.nextSeq = r.scanNextSeq(r.curDate)
	r.file = f
	r.size = info.Size()
	return nil
}

// rotateIfNeededLocked 在已持锁前提下判断是否需按大小或跨日滚动
func (r *FileRotator) rotateIfNeededLocked(incoming int64) error {
	now := time.Now().In(r.location)
	today := now.Format("20060102")
	needRotate := r.size+incoming >= r.maxSizeBytes || today != r.curDate
	if !needRotate {
		return nil
	}
	return r.rotateLocked(today)
}

// rotateLocked 将活跃文件 rename 为历史名并创建新活跃文件（调用方须已持锁）
func (r *FileRotator) rotateLocked(today string) error {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	active := r.activePath()
	info, err := os.Stat(active)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "统计活跃日志失败")
		}
	} else if info.Size() > 0 {
		// 历史文件日期用滚动前的 curDate（跨日时内容仍属旧日）
		historyDate := r.curDate
		seq := r.nextSeq
		r.nextSeq++
		historyName := fmt.Sprintf("%s-%07d.log", historyDate, seq)
		historyPath := filepath.Join(r.dir, historyName)
		if err := os.Rename(active, historyPath); err != nil {
			return errors.Wrap(err, "滚动日志 rename 失败")
		}
		if r.compress {
			go compressHistoryFile(historyPath)
		}
	}

	if today != r.curDate {
		r.curDate = today
		r.nextSeq = r.scanNextSeq(today)
	}

	// 锁内只做 rename / 开新文件；目录遍历与删除放到异步清理，避免阻塞业务 Write
	r.scheduleCleanup()

	f, err := os.OpenFile(active, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.Wrap(err, "创建新活跃日志失败")
	}
	r.file = f
	r.size = 0
	return nil
}

// scheduleCleanup 异步执行历史清理；已有清理在跑则跳过，避免叠跑
func (r *FileRotator) scheduleCleanup() {
	go func() {
		if !r.cleanupMu.TryLock() {
			return
		}
		defer r.cleanupMu.Unlock()
		if err := r.cleanup(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "日志清理失败: %v\n", err)
		}
	}()
}

// scanNextSeq 扫描目录得到指定日期下一可用历史序号（从 1 起）
func (r *FileRotator) scanNextSeq(date string) int {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return 1
	}
	maxSeq := 0
	prefix := date + "-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		m := historyNamePattern.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		seq, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1
}

// historyFile 表示待清理评估的历史日志文件元数据
type historyFile struct {
	path    string
	modTime time.Time
	size    int64
}

// cleanup 按 retention / max_backups / max_total_mb 优先级清理历史文件（不持 Write 锁）
func (r *FileRotator) cleanup() error {
	// 1) 按 retention 删除过期
	cutoff := time.Now().In(r.location).AddDate(0, 0, -r.retentionDays)
	cutoffDate := cutoff.Format("20060102")

	var files []historyFile
	_ = filepath.Walk(r.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == "access.log" || base == "info.log" || base == "error.log" || base == "debug.log" {
			return nil
		}
		m := historyNamePattern.FindStringSubmatch(base)
		if m == nil {
			return nil
		}
		if m[1] < cutoffDate {
			_ = os.Remove(path)
			return nil
		}
		files = append(files, historyFile{path: path, modTime: info.ModTime(), size: info.Size()})
		return nil
	})

	// 2) 按 max_backups：每个子目录独立保留最新 N 个历史
	byDir := map[string][]historyFile{}
	for _, f := range files {
		dir := filepath.Dir(f.path)
		byDir[dir] = append(byDir[dir], f)
	}
	files = files[:0]
	for dir, list := range byDir {
		sort.Slice(list, func(i, j int) bool {
			return list[i].modTime.After(list[j].modTime)
		})
		for i, f := range list {
			if i >= r.maxBackups {
				_ = os.Remove(f.path)
				continue
			}
			files = append(files, f)
			_ = dir
		}
	}

	// 3) 按 max_total_mb：整个 root 历史合计，删最旧
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	var total int64
	_ = filepath.Walk(r.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	for total > r.maxTotalBytes && len(files) > 0 {
		victim := files[0]
		files = files[1:]
		if err := os.Remove(victim.path); err == nil {
			total -= victim.size
		}
	}
	return nil
}

// compressHistoryFile 异步将历史 .log gzip 为 .log.gz 并删除原文件
func compressHistoryFile(path string) {
	if strings.HasSuffix(path, ".gz") {
		return
	}
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = in.Close() }()

	outPath := path + ".gz"
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	zw := gzip.NewWriter(out)
	_, copyErr := io.Copy(zw, in)
	closeErr := zw.Close()
	outCloseErr := out.Close()
	if copyErr != nil || closeErr != nil || outCloseErr != nil {
		_ = os.Remove(outPath)
		return
	}
	_ = os.Remove(path)
}
