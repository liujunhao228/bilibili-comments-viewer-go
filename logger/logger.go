package logger

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	once     sync.Once
	instance *logrus.Logger
)

func InitLogger(logFile, level string, maxSizeMB, maxBackups, maxAge int) *logrus.Logger {
	once.Do(func() {
		instance = logrus.New()

		// 设置日志级别
		logLevel, err := logrus.ParseLevel(level)
		if err != nil {
			logLevel = logrus.InfoLevel
		}
		instance.SetLevel(logLevel)

		// 设置日志格式为JSON
		instance.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})

		// 多输出：文件和终端
		if logFile != "" {
			// 确保日志目录存在
			dir := filepath.Dir(logFile)
			if err := os.MkdirAll(dir, 0755); err != nil {
				instance.Errorf("创建日志目录失败: %v", err)
			}

			// 文件输出（带轮转）
			fileOutput := &lumberjack.Logger{
				Filename:   logFile,
				MaxSize:    maxSizeMB,  // 每个日志文件的最大大小（MB）
				MaxBackups: maxBackups, // 保留旧日志文件的最大个数
				MaxAge:     maxAge,     // 保留旧日志文件的最大天数
				Compress:   true,       // 是否压缩旧日志文件
				LocalTime:  true,       // 使用本地时间
			}

			// 同时输出到文件和控制台
			instance.SetOutput(io.MultiWriter(os.Stdout, fileOutput))
		} else {
			// 只输出到控制台
			instance.SetOutput(os.Stdout)
		}
	})

	return instance
}

func GetLogger() *logrus.Logger {
	if instance == nil {
		panic("logger not initialized")
	}
	return instance
}
