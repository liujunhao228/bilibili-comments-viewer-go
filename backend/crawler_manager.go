package backend

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"runtime/debug"
	"time"

	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/blblcd"
	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	blblcdstore "bilibili-comments-viewer-go/crawler/blblcd/store"
	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
	"bilibili-comments-viewer-go/utils"
)

func CrawlAndImport(ctx context.Context, bvid string) error {
	funcName := runtime.FuncForPC(reflect.ValueOf(CrawlAndImport).Pointer()).Name()
	log := logger.GetLogger()

	// 添加上下文超时控制
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// +++ 添加关键日志 +++
	log.Infof("START %s: bvid=%s", funcName, bvid)

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("CrawlAndImport PANIC: %v\n%s", r, string(debug.Stack()))
		}
		log.Infof("END %s: bvid=%s", funcName, bvid)
	}()

	cfg := config.Get()
	log.Infof("开始处理视频: %s (保存模式: %s)", bvid, cfg.Crawler.SaveMode)

	// 获取并保存视频元数据
	log.Infof("获取视频元数据: %s", bvid)
	if videoInfo, err := FetchVideoMetadata(bvid); err == nil {
		dbVideo := &database.Video{
			BVid:  videoInfo.BVID,
			Title: videoInfo.Title,
			Cover: videoInfo.LocalCover,
		}
		if err := database.SaveVideo(dbVideo); err != nil {
			log.Errorf("保存视频信息失败: %v", err)
		} else {
			log.Infof("视频元数据保存成功: %s", bvid)
		}
	} else {
		log.Errorf("获取视频元数据失败: %v", err)
		// 即使元数据获取失败，也创建基础视频记录
		dbVideo := &database.Video{BVid: bvid}
		database.SaveVideo(dbVideo)
	}

	// 爬取评论
	log.Infof("开始爬取评论: %s", bvid)
	comments, err := crawlVideoComments(ctx, bvid)
	if err != nil {
		log.Errorf("评论爬取失败: %v", err)
		return CrawlerError{Message: "评论爬取失败: " + err.Error()}
	}

	// +++ 添加关键日志 +++
	log.Infof("爬取到 %d 条评论 (bvid: %s)", len(comments), bvid)

	// +++ 处理空评论情况 +++
	if len(comments) == 0 {
		log.Warnf("未爬取到评论，跳过处理 (bvid: %s)", bvid)
		return nil
	}

	// 根据保存模式处理评论
	switch cfg.Crawler.SaveMode {
	case SaveModeCSVOnly:
		log.Infof("CSV_ONLY模式处理评论: %s", bvid)
		processCSVOnly(bvid, comments)
	case SaveModeDBOnly:
		log.Infof("DB_ONLY模式导入评论: %s", bvid)
		if err := importCommentsToDB(bvid, comments); err != nil {
			log.Errorf("导入数据库失败: %v", err)
			return err
		} else {
			log.Infof("成功导入 %d 条评论到数据库 (bvid: %s)", len(comments), bvid)
		}
	default: // SaveModeCSVAndDB
		log.Infof("CSV_AND_DB模式处理评论: %s", bvid)
		processCSVAndDB(bvid, comments)
	}

	// +++ 新增：根据配置自动下载评论图片 +++
	if cfg.Crawler.ImgDownload {
		DownloadAllCommentImages(comments)
	}

	log.Infof("视频 %s 的评论处理完成", bvid)
	return nil
}

func CrawlUpVideos(mid int, fetchAll bool) error {
	cfg := config.Get()
	opt := blblcdmodel.NewDefaultOption()

	// 配置爬虫选项
	opt.Cookie = utils.ReadCookie(cfg.Crawler.CookieFile)
	opt.Output = cfg.Crawler.OutputDir
	opt.Mid = mid
	opt.FetchAll = fetchAll
	if cfg.Crawler.UpPages > 0 {
		opt.Pages = cfg.Crawler.UpPages
	}
	if cfg.Crawler.UpOrder != "" {
		opt.Vorder = cfg.Crawler.UpOrder
	}
	if cfg.Crawler.Workers > 0 {
		opt.Workers = cfg.Crawler.Workers
	}
	if cfg.Crawler.MaxTryCount > 0 {
		opt.MaxTryCount = cfg.Crawler.MaxTryCount
	}
	if cfg.Crawler.DelayBaseMs > 0 {
		opt.DelayBaseMs = cfg.Crawler.DelayBaseMs
	}
	if cfg.Crawler.DelayJitterMs > 0 {
		opt.DelayJitterMs = cfg.Crawler.DelayJitterMs
	}

	log.Printf("开始爬取UP主 %d 的视频 (页数: %d, 排序: %s, 协程: %d, 爬取所有: %v)",
		mid, opt.Pages, opt.Vorder, opt.Workers, opt.FetchAll)

	if err := blblcd.CrawlUp(mid, opt); err != nil {
		return CrawlerError{Message: fmt.Sprintf("UP主视频爬取失败: %s", err.Error())}
	}

	// 根据保存模式决定是否导入CSV
	if cfg.Crawler.SaveMode != SaveModeDBOnly {
		processCSVFiles()
	}

	return nil
}

// DownloadAllCommentImages 批量下载所有评论的图片（配置化：是否下载、保存路径，按BV号分目录）
func DownloadAllCommentImages(comments []blblcdmodel.Comment) {
	cfg := config.Get()
	if !cfg.Crawler.ImgDownload {
		return // 配置未开启图片下载
	}
	outputDir := cfg.ImageStorageDir
	for _, cmt := range comments {
		if len(cmt.Pictures) > 0 && cmt.Bvid != "" {
			blblcdstore.WriteImage(cmt.Bvid, cmt.Pictures, outputDir)
		}
	}
}
