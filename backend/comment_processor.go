package backend

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"

	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/blblcd"
	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
	"bilibili-comments-viewer-go/utils"
)

func crawlVideoComments(ctx context.Context, bvid string) ([]blblcdmodel.Comment, error) {
	funcName := runtime.FuncForPC(reflect.ValueOf(crawlVideoComments).Pointer()).Name()
	log := logger.GetLogger()

	log.Infof("START %s: bvid=%s", funcName, bvid)

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("crawlVideoComments PANIC: %v\n%s", r, string(debug.Stack()))
		}
		log.Infof("END %s: bvid=%s", funcName, bvid)
	}()

	cfg := config.Get()

	// +++ 添加详细日志 +++
	log.Infof("开始爬取视频评论: bvid=%s", bvid)

	opt := &blblcdmodel.Option{
		Cookie:        utils.ReadCookie(cfg.Crawler.CookieFile),
		Bvid:          bvid,
		Output:        cfg.Crawler.OutputDir,
		Workers:       cfg.Crawler.Workers,
		MaxTryCount:   cfg.Crawler.MaxTryCount,
		DelayBaseMs:   cfg.Crawler.DelayBaseMs,
		DelayJitterMs: cfg.Crawler.DelayJitterMs,
	}

	// +++ 记录爬虫配置 +++
	log.Debugf("爬虫配置: workers=%d, maxTryCount=%d", opt.Workers, opt.MaxTryCount)

	comments, err := blblcd.CrawlVideo(ctx, bvid, opt)
	if err != nil {
		log.Errorf("爬取视频评论失败: %v", err)
	} else {
		log.Infof("成功爬取 %d 条评论 (bvid: %s)", len(comments), bvid)
	}

	return comments, err
}

func processCSVOnly(bvid string, comments []blblcdmodel.Comment) {
	cfg := config.Get()
	csvPath := filepath.Join(cfg.Crawler.OutputDir, bvid, bvid+".csv")
	if err := saveCommentsToCSV(comments, csvPath); err != nil {
		logger.GetLogger().Errorf("保存CSV失败: %v", err)
	} else {
		logger.GetLogger().Infof("CSV保存成功: %s", csvPath)
	}
}

func processCSVAndDB(bvid string, comments []blblcdmodel.Comment) {
	cfg := config.Get()
	csvPath := filepath.Join(cfg.Crawler.OutputDir, bvid, bvid+".csv")
	if err := saveCommentsToCSV(comments, csvPath); err != nil {
		logger.GetLogger().Errorf("保存CSV失败: %v", err)
	} else {
		logger.GetLogger().Infof("CSV保存成功: %s", csvPath)
	}

	if err := ImportCommentsFromCSV(bvid, csvPath); err != nil {
		logger.GetLogger().Errorf("导入数据库失败: %v", err)
	}
}

func processCSVFiles() {
	cfg := config.Get()
	files, err := filepath.Glob(filepath.Join(cfg.Crawler.OutputDir, "*/*.csv"))
	if err != nil {
		logger.GetLogger().Infof("查找CSV文件失败: %v", err)
		return
	}

	for _, file := range files {
		bvid := filepath.Base(filepath.Dir(file))
		logger.GetLogger().Infof("开始导入CSV文件: %s, BV: %s", file, bvid)
		if err := ImportCommentsFromCSV(bvid, file); err != nil {
			logger.GetLogger().Errorf("导入CSV文件失败: %v", err)
		} else {
			logger.GetLogger().Infof("成功导入CSV文件: %s", file)
		}
	}
}
