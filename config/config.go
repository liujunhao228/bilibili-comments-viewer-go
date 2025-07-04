package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var cfg *Config

func Get() *Config {
	return cfg
}

type Config struct {
	AppName         string   `mapstructure:"app_name"`
	DefaultPort     int      `mapstructure:"default_port"`
	UserDataDir     string   `mapstructure:"user_data_dir"`
	DatabasePath    string   `mapstructure:"database_path"`
	SampleDataDir   string   `mapstructure:"sample_data_dir"`
	ImageStorageDir string   `mapstructure:"image_storage_dir"`
	FrontendDir     string   `mapstructure:"frontend_dir"`

	AllowedImageDomains []string `mapstructure:"allowed_image_domains"`

	// 新增日志配置
	Logging struct {
		LogFile    string `mapstructure:"log_file"`
		LogLevel   string `mapstructure:"log_level"`
		MaxSizeMB  int    `mapstructure:"max_size_mb"`
		MaxBackups int    `mapstructure:"max_backups"`
		MaxAgeDays int    `mapstructure:"max_age_days"`
	} `mapstructure:"logging"`

	Crawler struct {
		CookieFile    string `mapstructure:"cookie_file"`
		NoCover       bool   `mapstructure:"no_cover"`
		OutputDir     string `mapstructure:"output_dir"`
		Workers       int    `mapstructure:"workers"`
		MaxTryCount   int    `mapstructure:"max_try_count"`
		ImgDownload   bool   `mapstructure:"img_download"`
		UpPages       int    `mapstructure:"up_pages"`
		UpOrder       string `mapstructure:"up_order"`
		SaveMode      string `mapstructure:"save_mode"`
		DelayBaseMs   int    `mapstructure:"delay_base_ms"`
		DelayJitterMs int    `mapstructure:"delay_jitter_ms"`
	} `mapstructure:"crawler"`
}

// 增强路径规范化函数
func normalizePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// 处理变量扩展 ({{user_data_dir}})
	if strings.Contains(path, "{{user_data_dir}}") {
		userDataDir, _ := normalizePath(viper.GetString("user_data_dir"))
		path = strings.ReplaceAll(path, "{{user_data_dir}}", userDataDir)
	}

	// 扩展 ~ 开头的路径
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			// 主目录获取失败时使用当前目录
			wd, _ := os.Getwd()
			home = wd
		}
		path = filepath.Join(home, path[1:])
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve relative path %s: %w", path, err)
		}
		path = absPath
	}

	// 特别处理图片目录路径 - 确保使用正斜杠
	path = strings.ReplaceAll(path, "\\", "/")

	// 清理路径格式
	return filepath.Clean(path), nil
}

// 确保目录存在
func ensureDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(path, 0755)
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.SetDefault("crawler.up_pages", 10)
	viper.SetDefault("crawler.up_order", "pubdate")

	// 获取主目录（跨平台兼容）
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 主目录获取失败时使用当前目录
		wd, _ := os.Getwd()
		homeDir = wd
		fmt.Printf("Warning: using current directory as home: %s\n", homeDir)
	}

	// 设置默认值
	viper.SetDefault("app_name", "BilibiliCommentsViewer")
	viper.SetDefault("default_port", 5000)
	viper.SetDefault("user_data_dir", filepath.Join(homeDir, ".bilibili-comments-viewer"))

	// 使用变量语法处理依赖路径
	viper.SetDefault("database_path", "{{user_data_dir}}/bilibili.db")
	viper.SetDefault("image_storage_dir", "{{user_data_dir}}/images")
	viper.SetDefault("sample_data_dir", "./sample_data")
	viper.SetDefault("frontend_dir", "./frontend")

	// 设置日志默认值
	viper.SetDefault("logging.log_file", "{{user_data_dir}}/logs/app.log")
	viper.SetDefault("logging.log_level", "info")
	viper.SetDefault("logging.max_size_mb", 10)
	viper.SetDefault("logging.max_backups", 5)
	viper.SetDefault("logging.max_age_days", 30)

	// 设置爬虫配置默认值
	viper.SetDefault("crawler.cookie_file", "{{user_data_dir}}/cookie.txt")
	viper.SetDefault("crawler.no_cover", false)
	viper.SetDefault("crawler.output_dir", "{{user_data_dir}}/crawler_output")
	viper.SetDefault("crawler.workers", 5)
	viper.SetDefault("crawler.max_try_count", 3)
	viper.SetDefault("crawler.img_download", false)
	viper.SetDefault("crawler.save_mode", "csv_and_db")
	viper.SetDefault("crawler.delay_base_ms", 3000)
	viper.SetDefault("crawler.delay_jitter_ms", 2000)

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 自动加载环境变量（支持~扩展）
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 解析配置到结构体
	var configObj Config
	if err := viper.Unmarshal(&configObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 路径规范化处理
	pathsToNormalize := []*string{
		&configObj.UserDataDir,
		&configObj.DatabasePath,
		&configObj.SampleDataDir,
		&configObj.ImageStorageDir,
		&configObj.FrontendDir,
		&configObj.Crawler.CookieFile,
		&configObj.Crawler.OutputDir,
		&configObj.Logging.LogFile,
	}

	for _, pathPtr := range pathsToNormalize {
		normalized, err := normalizePath(*pathPtr)
		if err != nil {
			return nil, fmt.Errorf("path normalization error: %w", err)
		}
		*pathPtr = normalized
	}

	// 确保关键目录存在
	if err := ensureDir(configObj.UserDataDir); err != nil {
		return nil, fmt.Errorf("failed to create user data dir: %w", err)
	}
	if err := ensureDir(filepath.Dir(configObj.DatabasePath)); err != nil {
		return nil, fmt.Errorf("failed to create database dir: %w", err)
	}
	if err := ensureDir(configObj.ImageStorageDir); err != nil {
		return nil, fmt.Errorf("failed to create image storage dir: %w", err)
	}
	if err := ensureDir(configObj.Crawler.OutputDir); err != nil {
		return nil, fmt.Errorf("failed to create crawler output dir: %w", err)
	}

	// 打印配置信息
	fmt.Printf("应用程序数据目录: %s\n", configObj.UserDataDir)
	fmt.Printf("数据库路径: %s\n", configObj.DatabasePath)
	fmt.Printf("样本数据目录: %s\n", configObj.SampleDataDir)
	fmt.Printf("图片存储目录: %s\n", configObj.ImageStorageDir)

	// 打印日志配置信息
	fmt.Printf("日志文件: %s\n", configObj.Logging.LogFile)
	fmt.Printf("日志级别: %s\n", configObj.Logging.LogLevel)

	// 打印爬虫配置
	fmt.Printf("爬虫配置:\n")
	fmt.Printf("  Cookie文件: %s\n", configObj.Crawler.CookieFile)
	fmt.Printf("  输出目录: %s\n", configObj.Crawler.OutputDir)
	fmt.Printf("  工作线程数: %d\n", configObj.Crawler.Workers)
	fmt.Printf("  最大重试次数: %d\n", configObj.Crawler.MaxTryCount)
	fmt.Printf("  下载图片: %t\n", configObj.Crawler.ImgDownload)

	// 设置全局配置
	cfg = &configObj

	return cfg, nil
}
