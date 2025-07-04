package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"bilibili-comments-viewer-go/backend"
	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
)

func main() {
	// 初始化配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化日志
	logger.InitLogger(
		cfg.Logging.LogFile,
		cfg.Logging.LogLevel,
		cfg.Logging.MaxSizeMB,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAgeDays,
	)
	log := logger.GetLogger()

	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test/test_crawler.go <bvid>")
		fmt.Println("Example: go run test/test_crawler.go BV1xx411c7mD")
		os.Exit(1)
	}

	bvid := os.Args[1]
	log.Infof("开始测试爬虫修复效果，目标视频: %s", bvid)

	// 初始化数据库
	log.Infof("初始化数据库: %s", cfg.DatabasePath)
	err = database.InitDB(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.CloseDB()
	log.Infof("数据库初始化成功")

	// 创建上下文，设置超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 开始爬取
	startTime := time.Now()
	err = backend.CrawlAndImport(ctx, bvid)
	endTime := time.Now()

	if err != nil {
		log.Errorf("爬取失败: %v", err)
		os.Exit(1)
	}

	duration := endTime.Sub(startTime)
	log.Infof("爬取完成，耗时: %v", duration)
	log.Infof("测试完成，请检查数据库中的评论数量")
}
