// blblcd 包提供了 B 站评论爬取相关的核心功能
package blblcd

import (
	"context"
	"sync"

	"bilibili-comments-viewer-go/crawler/blblcd/core"
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
)

// CrawlVideo 爬取指定 bvid 视频的所有评论
// ctx: 上下文，用于控制取消等
// bvid: 视频的 BVID
// opt: 爬取选项，包括并发数等
// 返回值: 评论列表和错误信息
func CrawlVideo(ctx context.Context, bvid string, opt *model.Option) ([]model.Comment, error) {
	// 记录开始爬取日志
	logger.GetLogger().Infof("开始爬取视频: bvid=%s", bvid)

	// 用于收集评论的通道，带缓冲区
	resultChan := make(chan model.Comment, 1000)
	var comments []model.Comment
	// 控制并发的信号量，容量为 opt.Workers
	sem := make(chan struct{}, opt.Workers)

	// 创建独立的 WaitGroup 用于等待 FindComment 完成
	var findWg sync.WaitGroup
	findWg.Add(1)

	// 启动 goroutine 进行评论查找
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.GetLogger().Errorf("[panic] CrawlVideo goroutine: %v", r)
			}
			findWg.Done()
		}()
		// 将 bvid 转换为 avid
		avid := core.Bvid2Avid(bvid)
		// 调用核心查找评论逻辑，wg 传 nil 由内部管理
		core.FindComment(ctx, sem, nil, int(avid), opt, resultChan)
	}()

	// 等待评论查找完成
	findWg.Wait()

	// 关闭通道并收集所有评论
	close(resultChan)
	for comment := range resultChan {
		comments = append(comments, comment)
	}

	// 记录爬取完成日志
	logger.GetLogger().Infof("视频 %s 爬取完成, 共获取 %d 条评论", bvid, len(comments))

	return comments, nil
}

// CrawlUp 爬取指定 up 主（用户）的所有视频评论
// mid: up 主的 mid
// opt: 爬取选项
// 返回值: 错误信息
func CrawlUp(mid int, opt *model.Option) error {
	// 控制并发的信号量，容量为 opt.Workers
	sem := make(chan struct{}, opt.Workers)
	// 设置 up 主 mid
	opt.Mid = mid
	// 调用核心查找 up 主视频评论逻辑
	core.FindUser(sem, opt)
	return nil
}
