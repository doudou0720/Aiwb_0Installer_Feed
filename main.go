package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 全局变量
var (
	source      string
	dest        string
	force       bool
	logLevel    string
	logger      *log.Logger
	currentDir  string
	versionFile string
)

// 日志级别
const (
	LogDebug = "debug"
	LogInfo  = "info"
	LogWarn  = "warn"
	LogError = "error"
)

func init() {
	// 设置自定义Usage信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintln(os.Stderr, "  # Basic usage")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -dest /path/to/writable/dir")
		fmt.Fprintln(os.Stderr, "\n  # Force sync")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -dest /path/to/writable/dir -force")
		fmt.Fprintln(os.Stderr, "\n  # Set log level")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -dest /path/to/writable/dir -log-level debug")
	}

	// 获取当前目录（源目录）
	var err error
	currentDir, err = os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// 命令行参数解析
	flag.StringVar(&dest, "dest", "", "Target writable directory path (required)")
	flag.BoolVar(&force, "force", false, "Force sync, ignore version check")
	flag.StringVar(&logLevel, "log-level", LogInfo, "Log level (debug, info, warn, error)")
	flag.Parse()

	// 验证必需参数
	if dest == "" {
		flag.Usage()
		os.Exit(1)
	}

	// 初始化日志
	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	// 版本文件路径
	versionFile = filepath.Join(dest, ".version")
}

func main() {
	logger.Printf("Starting 0install cache sync tool")
	logger.Printf("Source directory: %s", currentDir)
	logger.Printf("Destination directory: %s", dest)
	logger.Printf("Force sync: %v", force)
	logger.Printf("Log level: %s", logLevel)

	// 获取当前目录的版本标识
	version := getVersionFromDir(currentDir)
	logger.Printf("Current version: %s", version)

	// 检查目标目录是否存在，不存在则创建
	if err := os.MkdirAll(dest, 0755); err != nil {
		logger.Fatalf("Failed to create destination directory: %v", err)
	}

	// 检查是否需要同步
	if !force && !needSync(version) {
		logger.Println("No sync needed, versions match")
		return
	}

	// 执行同步
	logger.Println("Starting sync process...")
	if err := syncDir(currentDir, dest); err != nil {
		logger.Fatalf("Sync failed: %v", err)
	}

	// 更新版本文件
	if err := updateVersionFile(version); err != nil {
		logger.Fatalf("Failed to update version file: %v", err)
	}

	logger.Println("Sync completed successfully")
}

// getVersionFromDir 从目录名称中提取版本标识
func getVersionFromDir(dir string) string {
	baseName := filepath.Base(dir)
	// 检查是否符合sha256new_前缀格式
	if strings.HasPrefix(baseName, "sha256new_") {
		return baseName
	}
	return baseName
}

// needSync 检查是否需要同步
func needSync(version string) bool {
	// 检查版本文件是否存在
	if _, err := os.Stat(versionFile); os.IsNotExist(err) {
		logger.Println("Version file not found, need sync")
		return true
	}

	// 读取版本文件内容
	content, err := os.ReadFile(versionFile)
	if err != nil {
		logger.Printf("Failed to read version file: %v, need sync", err)
		return true
	}

	// 比对版本
	storedVersion := strings.TrimSpace(string(content))
	if storedVersion != version {
		logger.Printf("Version mismatch: stored=%s, current=%s, need sync", storedVersion, version)
		return true
	}

	return false
}

// updateVersionFile 更新版本文件
func updateVersionFile(version string) error {
	return os.WriteFile(versionFile, []byte(version), 0644)
}

// syncDir 同步目录
func syncDir(src, dst string) error {
	// 创建工作池
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	done := make(chan struct{})

	// 遍历源目录
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// 跳过版本文件
		if relPath == ".version" {
			return nil
		}

		// 目标路径
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// 创建目录
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return err
			}
		} else {
			// 复制文件
			wg.Add(1)
			go func(srcFile, dstFile string, mode os.FileMode) {
				defer wg.Done()
				if err := copyFile(srcFile, dstFile, mode); err != nil {
					select {
					case errChan <- err:
					default:
					}
				}
			}(path, dstPath, info.Mode())
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 等待所有复制任务完成
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或错误
	select {
	case <-done:
		return nil
	case err := <-errChan:
		return err
	}
}

// copyFile 复制文件
func copyFile(src, dst string, mode os.FileMode) error {
	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// 复制内容
	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// 设置文件权限
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set file mode: %w", err)
	}

	return nil
}
