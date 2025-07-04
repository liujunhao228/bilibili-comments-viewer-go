package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/fetch"
	"bilibili-comments-viewer-go/crawler/bili_info/model"
	"bilibili-comments-viewer-go/crawler/bili_info/store"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"
)

// 程序版本信息
const (
	Version   = "1.0.0"
	BuildTime = "2023-10-15"
)

func main() {
	// 1. 解析命令行参数
	cfg := parseFlags()

	// 2. 显示帮助信息（如果需要）
	if cfg.Help {
		printHelp()
		return
	}

	// 3. 创建必要的目录
	setupDirectories(cfg)

	// 4. 加载Cookies
	cookies := util.LoadCookies(config.CookieFile)
	if cookies == "" {
		logger.GetLogger().Fatal("❌ Cookie加载失败，请检查cookie.txt文件")
	}

	// 5. 获取要处理的BV号列表
	bvList := getBVList(cfg)
	if len(bvList) == 0 {
		logger.GetLogger().Fatal("❌ 未找到有效BV号")
	}

	logger.GetLogger().Infof("✅ 找到 %d 个视频需要处理", len(bvList))

	// 6. 创建API客户端
	apiClient := fetch.NewAPIClient(cookies)

	// 7. 处理视频
	processVideos(context.Background(), apiClient, bvList, cfg)
}

// 解析命令行参数
func parseFlags() *config.Config {
	cfg := &config.Config{}

	flag.StringVar(&cfg.Output, "output", "", "输出文件名\n  单个模式: 根据BVID自动生成\n  批量模式: videos_info.csv")
	flag.StringVar(&cfg.Output, "o", "", "输出文件名(简写)")

	flag.StringVar(&cfg.BVIDs, "bvid", "", "指定一个或多个BV号(逗号分隔)")
	flag.StringVar(&cfg.BVIDs, "b", "", "指定BV号(简写)")

	flag.StringVar(&cfg.Input, "input", "", "从.txt文件指定BV号")
	flag.StringVar(&cfg.Input, "i", "", "从文件指定BV号(简写)")

	flag.StringVar(&cfg.ImageDir, "image-dir", config.ImageBaseDir, "图片存储目录")
	flag.StringVar(&cfg.ImageDir, "d", config.ImageBaseDir, "图片目录(简写)")

	flag.BoolVar(&cfg.NoCover, "no-cover", false, "不保存封面图片")
	flag.BoolVar(&cfg.NoCover, "n", false, "不保存封面(简写)")

	flag.BoolVar(&cfg.Help, "help", false, "显示帮助信息")
	flag.BoolVar(&cfg.Help, "h", false, "帮助信息(简写)")

	// 自定义帮助信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "B站视频信息爬虫工具 v%s (构建时间: %s)\n", Version, BuildTime)
		fmt.Fprintf(os.Stderr, "使用方式: %s [参数]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "参数:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s -b BV1xx411x7xx -o video_info.csv\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i bv_list.txt -d my_images\n", os.Args[0])
	}

	flag.Parse()
	return cfg
}

// 显示帮助信息
func printHelp() {
	flag.Usage()
}

// 创建必要的目录
func setupDirectories(cfg *config.Config) {
	if !cfg.NoCover {
		if err := util.CreateDirIfNotExist(cfg.ImageDir); err != nil {
			logger.GetLogger().Fatalf("❌ 创建图片目录失败: %v", err)
		}
		logger.GetLogger().Infof("📁 图片将保存到: %s", cfg.ImageDir)
	}
}

// 获取BV号列表
func getBVList(cfg *config.Config) []string {
	bvSet := make(map[string]bool)

	// 从命令行参数获取BV号
	if cfg.BVIDs != "" {
		bvs := strings.Split(cfg.BVIDs, ",")
		for _, bv := range bvs {
			bvid := strings.TrimSpace(bv)
			if util.IsValidBVID(bvid) {
				bvSet[bvid] = true
			}
		}
	}

	// 从输入文件读取BV号
	if cfg.Input != "" {
		bvids, err := util.ReadBVIdsFromFile(cfg.Input)
		if err == nil {
			for _, bvid := range bvids {
				if util.IsValidBVID(bvid) {
					bvSet[bvid] = true
				}
			}
		} else {
			logger.GetLogger().Warnf("⚠️ 无法打开输入文件: %v", err)
		}
	}

	// 转为切片
	bvList := make([]string, 0, len(bvSet))
	for bvid := range bvSet {
		bvList = append(bvList, bvid)
	}
	return bvList
}

// 处理视频信息
func processVideos(ctx context.Context, apiClient *fetch.APIClient, bvList []string, cfg *config.Config) {
	// 创建任务通道和结果通道
	jobs := make(chan string, len(bvList))
	results := make(chan *model.VideoInfo, len(bvList))

	// 创建worker池
	var wg sync.WaitGroup
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		// 使用fetch包中的Worker函数
		go fetch.Worker(ctx, &wg, apiClient, jobs, results, cfg)
	}

	// 分发任务
	for _, bvid := range bvList {
		jobs <- bvid
	}
	close(jobs)

	// 等待所有worker完成
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.GetLogger().Errorf("[panic] processVideos results goroutine: %v", r)
			}
		}()
		wg.Wait()
		close(results)
	}()

	// 收集结果
	var videoInfos []*model.VideoInfo
	for info := range results {
		videoInfos = append(videoInfos, info)
	}

	// 保存结果
	saveResults(videoInfos, cfg)
}

// 保存结果
func saveResults(videoInfos []*model.VideoInfo, cfg *config.Config) {
	if len(videoInfos) == 0 {
		logger.GetLogger().Warn("⚠️ 没有可保存的视频信息")
		return
	}

	// 单个视频处理
	if len(videoInfos) == 1 {
		outputFile := cfg.Output
		if outputFile == "" {
			outputFile = store.GenerateOutputFileName(videoInfos[0])
		}

		if err := store.WriteSingleResult(outputFile, videoInfos[0]); err != nil {
			logger.GetLogger().Fatalf("❌ 保存结果失败: %v", err)
		}
		logger.GetLogger().Infof("✅ 结果已保存到: %s", outputFile)
	} else {
		// 批量处理
		outputFile := cfg.Output
		if outputFile == "" {
			outputFile = "videos_info.csv"
		}

		if err := store.WriteResultsToFile(videoInfos, outputFile); err != nil {
			logger.GetLogger().Fatalf("❌ 保存结果失败: %v", err)
		}
		logger.GetLogger().Infof("✅ %d 个视频信息已保存到: %s", len(videoInfos), outputFile)
	}
}
