package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
)

// Config 配置结构体，对应wrapper.config.json文件
type Config struct {
	Dest     string `json:"dest"`
	Name     string `json:"name"`
	Force    bool   `json:"force"`
	LogLevel string `json:"log-level"`
	Entry    string `json:"entry"`
	Copy     bool   `json:"copy"`
}

// 全局变量
var (
	source      string
	dest        string
	name        string
	force       bool
	logLevel    string
	entry       string
	enableCopy  bool
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
		fmt.Fprintln(os.Stderr, "  # Basic usage (default dest: user's Aiwb_Application folder)")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe")
		fmt.Fprintln(os.Stderr, "\n  # With custom destination")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -dest /path/to/writable/dir")
		fmt.Fprintln(os.Stderr, "\n  # With subdirectory name")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -name myapp")
		fmt.Fprintln(os.Stderr, "  # or")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -dest /path/to/writable/dir -name myapp")
		fmt.Fprintln(os.Stderr, "\n  # Force sync")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -force")
		fmt.Fprintln(os.Stderr, "\n  # Set log level")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -log-level debug")
		fmt.Fprintln(os.Stderr, "\n  # Execute entry program after sync")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -entry bin/app.exe")
		fmt.Fprintln(os.Stderr, "\n  # Enable file copy (default: false)")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -copy")
		fmt.Fprintln(os.Stderr, "\n  # Run directly from destination directory without copy")
		fmt.Fprintln(os.Stderr, "  ./go_wrapper.exe -copy=false")
	}

	// 获取程序所在目录（源目录）
	var err error
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Failed to get executable path: %v, using current directory instead", err)
		currentDir, err = os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get current directory: %v", err)
		}
	} else {
		currentDir = filepath.Dir(execPath)
	}
	log.Printf("Using source directory: %s", currentDir)

	// 设置默认dest值为用户文件夹下的Aiwb_Application
	if dest == "" {
		usr, err := user.Current()
		if err != nil {
			log.Printf("Warning: Failed to get current user: %v", err)
		} else {
			dest = filepath.Join(usr.HomeDir, "Aiwb_Application")
		}
	}

	// 检查wrapper.config.json文件是否存在
	// 从程序所在位置获取json文件，而不是当前工作目录
	execPath, err = os.Executable()
	if err != nil {
		log.Printf("Warning: Failed to get executable path: %v, using current directory instead", err)
		execPath = currentDir
	}
	execDir := filepath.Dir(execPath)
	configPath := filepath.Join(execDir, "wrapper.config.json")
	configFileExists := false
	if _, err = os.Stat(configPath); err == nil {
		configFileExists = true
	}
	// 使用log.Printf而不是logger.Printf，因为logger还未初始化
	log.Printf("Looking for config file at: %s", configPath)

	// 从配置文件读取默认值
	if configFileExists {
		configData, err := os.ReadFile(configPath)
		if err != nil {
			log.Printf("Warning: Failed to read config file: %v", err)
		} else {
			var config Config
			if err := json.Unmarshal(configData, &config); err != nil {
				log.Printf("Warning: Failed to parse config file: %v", err)
			} else {
				// 设置默认值
				if config.Dest != "" {
					dest = config.Dest
				}
				if config.Name != "" {
					name = config.Name
				}
				force = config.Force
				if config.LogLevel != "" {
					logLevel = config.LogLevel
				}
				if config.Entry != "" {
					entry = config.Entry
				}
				enableCopy = config.Copy
			}
		}
	}

	// 命令行参数解析（优先级高于配置文件）
	flag.StringVar(&dest, "dest", dest, "Target writable directory path (default: user's Aiwb_Application folder)")
	flag.StringVar(&name, "name", name, "Subdirectory name under destination (optional)")
	flag.BoolVar(&force, "force", force, "Force sync, ignore version check")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level (debug, info, warn, error)")
	flag.StringVar(&entry, "entry", entry, "Relative path to entry program to execute after sync")
	flag.BoolVar(&enableCopy, "copy", enableCopy, "Enable file copy (default: false, run directly from destination directory)")
	flag.Parse()

	// 验证必需参数
	if dest == "" {
		log.Fatalf("Error: Destination directory not set and failed to get user home directory")
	}

	// 如果指定了name，则构建完整的目标路径
	if name != "" {
		dest = filepath.Join(dest, name)
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
	logger.Printf("Enable copy: %v", enableCopy)

	// 获取当前目录的版本标识
	version := getVersionFromDir(currentDir)
	logger.Printf("Current version: %s", version)

	// 检查目标目录是否存在，不存在则创建
	if err := os.MkdirAll(dest, 0755); err != nil {
		logger.Fatalf("Failed to create destination directory: %v", err)
	}

	// 检查是否需要同步（仅当enableCopy为true时）
	if enableCopy {
		if !force && !needSync(version) {
			logger.Println("No sync needed, versions match")
			// 继续执行，不返回，以便执行入口程序
		} else {
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
	} else {
		logger.Println("Copy disabled, running directly from destination directory")
	}

	// 执行入口程序（如果指定）
	if entry != "" {
		logger.Printf("Executing entry program: %s", entry)

		// 规范化目标目录路径
		destAbs, err := filepath.Abs(dest)
		if err != nil {
			logger.Printf("Failed to get absolute path for destination directory: %v", err)
		} else {
			// 构建入口程序的完整路径
			var entryPath string
			if enableCopy {
				// 当enableCopy=true时，使用目标目录中的入口程序
				entryPath = filepath.Join(dest, entry)
			} else {
				// 当enableCopy=false时，使用源目录中的入口程序
				entryPath = filepath.Join(currentDir, entry)
			}
			logger.Printf("Entry program full path: %s", entryPath)

			// 检查入口程序是否存在
			if _, err := os.Stat(entryPath); os.IsNotExist(err) {
				logger.Printf("Entry program not found: %s", entryPath)
			} else {
				// 获取入口程序的绝对路径
				absEntryPath, err := filepath.Abs(entryPath)
				if err != nil {
					logger.Printf("Failed to get absolute path for entry program: %v", err)
				} else {
					// 获取源目录的绝对路径
					sourceAbs, err := filepath.Abs(currentDir)
					if err != nil {
						logger.Printf("Failed to get absolute path for source directory: %v", err)
					} else {
						// 验证入口程序是否在相应的目录内，防止路径遍历
						var baseDir string
						if enableCopy {
							baseDir = destAbs
						} else {
							baseDir = sourceAbs
						}

						relPath, err := filepath.Rel(baseDir, absEntryPath)
						if err != nil {
							logger.Printf("Failed to verify entry program path: %v", err)
						} else if strings.HasPrefix(relPath, "..") || relPath == ".." {
							if enableCopy {
								logger.Printf("Entry program path is outside destination directory, refusing to execute: %s", absEntryPath)
							} else {
								logger.Printf("Entry program path is outside source directory, refusing to execute: %s", absEntryPath)
							}
						} else {
							logger.Printf("Entry program absolute path: %s", absEntryPath)

							// 准备命令
							var cmd *exec.Cmd

							// 设置工作目录
							// 对于跨平台支持，直接使用exec.Cmd的Dir字段
							// 这样可以避免使用特定于操作系统的cd命令
							cmd = exec.Command(absEntryPath)
							cmd.Dir = destAbs

							// 传递环境变量（包括0install环境变量）
							cmd.Env = os.Environ()

							// 设置标准输入/输出/错误
							cmd.Stdin = os.Stdin
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr

							// 显示执行信息
							logger.Printf("Changing to directory: %s", destAbs)
							logger.Printf("Executing command: %s", absEntryPath)

							// 执行命令并直接退出，不等待程序完成
							err = cmd.Start()
							if err != nil {
								logger.Printf("Failed to start entry program: %v", err)
								os.Exit(1)
							} else {
								logger.Printf("Started entry program with PID: %d, exiting wrapper", cmd.Process.Pid)
								// 直接退出，不等待子进程完成
								os.Exit(0)
							}
						}
					}
				}
			}
		}
	}

	// 检查目标目录是否为空，如果是空的则删除它
	if err := removeEmptyDir(dest); err != nil {
		logger.Printf("Warning: Failed to remove empty destination directory: %v", err)
	}
}

// removeEmptyDir 检查目录是否为空，如果是空的则删除它，包括嵌套的空目录
func removeEmptyDir(dir string) error {
	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Printf("Directory does not exist: %s", dir)
		return nil // 目录不存在，不需要删除
	}

	// 读取目录内容
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// 递归检查子目录
	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(dir, entry.Name())
			if err := removeEmptyDir(subDir); err != nil {
				logger.Printf("Warning: Failed to remove empty subdirectory: %v", err)
			}
		}
	}

	// 再次读取目录内容，因为子目录可能已经被删除
	entries, err = os.ReadDir(dir)
	if err != nil {
		return err
	}

	// 检查目录是否为空
	if len(entries) == 0 {
		logger.Printf("Removing empty destination directory: %s", dir)
		return os.Remove(dir)
	} else {
		logger.Printf("Destination directory is not empty, keeping it: %s", dir)
		for _, entry := range entries {
			logger.Printf("  - %s", entry.Name())
		}
	}

	return nil
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

	// 规范化路径，避免路径分隔符问题
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	// 遍历源目录
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// 跳过版本文件和 .git 目录
		skip := false
		if relPath == ".version" {
			skip = true
		} else {
			// 检查路径组件中是否包含精确的 ".git"
			components := strings.Split(relPath, string(os.PathSeparator))
			for _, component := range components {
				if component == ".git" {
					skip = true
					break
				}
			}
		}
		if skip {
			return nil
		}

		// 跳过目标目录本身，避免无限递归
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(pathAbs, dstAbs) {
			logger.Println("Skipping destination directory to avoid infinite recursion:", path)
			if info.IsDir() {
				return filepath.SkipDir
			}
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
