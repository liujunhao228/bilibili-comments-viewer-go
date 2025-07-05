package core

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/store"
	"bilibili-comments-viewer-go/logger"
)

// core 包实现了 B 站评论爬取的核心流程，包括主评论、子评论的递归抓取与断点续爬等功能

// FindComment 爬取指定 avid 视频的所有评论（主评论+子评论），支持断点续爬和并发控制
// ctx: 上下文控制
// sem: 并发信号量
// wg: 外部 WaitGroup（可为 nil）
// avid: 视频 avid
// opt: 爬取选项
// resultChan: 评论结果输出通道
func FindComment(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup, avid int, opt *model.Option, resultChan chan<- model.Comment) {
	funcName := runtime.FuncForPC(reflect.ValueOf(FindComment).Pointer()).Name()
	logger.GetLogger().Infof("START %s: avid=%d", funcName, avid)

	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("PANIC in %s: %v\n%s", funcName, r, string(debug.Stack()))
		}
		if wg != nil {
			wg.Done()
		}
		<-sem
		logger.GetLogger().Infof("END %s: avid=%d", funcName, avid)
	}()

	oid := strconv.Itoa(avid)
	logger.GetLogger().Infof("开始爬取视频评论: oid=%s", oid)

	total, err := FetchCount(oid)
	if err != nil {
		logger.GetLogger().Errorf("获取评论总数失败: %v", err)
		return
	}
	logger.GetLogger().Infof("视频 %s 共有 %d 条评论", oid, total)

	if total == 0 {
		logger.GetLogger().Infof("视频 %s 没有评论，跳过爬取", oid)
		return
	}

	// 断点续爬相关变量
	savePath := path.Join(opt.Output, opt.Bvid)
	if opt.CommentOutput != "" {
		savePath = path.Join(opt.CommentOutput, opt.Bvid)
	}
	imgSavePath := opt.ImageOutput
	if imgSavePath == "" {
		imgSavePath = path.Join(savePath, "images")
	}

	var pageWg sync.WaitGroup                            // 控制每一页的并发
	var mu sync.Mutex                                    // 保护计数和 map
	downloadedCount := 0                                 // 已下载评论数
	recordedMap := make(map[int64]bool)                  // 已记录评论去重
	consecutiveEmptyPages := 0                           // 连续空页计数
	offsetStr := ""                                      // 分页 offset
	page := 1                                            // 当前页码
	progressFile := path.Join(savePath, "progress.json") // 断点文件

	// 尝试加载断点
	if prog, err := loadProgress(progressFile); err == nil {
		page = prog.Page
		downloadedCount = prog.DownloadedCount
		logger.GetLogger().Infof("断点续爬: 从第%d页、已爬取%d条评论继续", page, downloadedCount)
	}

mainLoop:
	for {
		select {
		case <-ctx.Done():
			logger.GetLogger().Infof("收到取消信号，保存断点并退出...")
			saveProgress(progressFile, page, downloadedCount)
			break mainLoop
		default:
		}

		if downloadedCount >= total {
			logger.GetLogger().Infof("已爬取完成，共获取 %d 条评论，目标 %d 条", downloadedCount, total)
			break
		}

		// 内存监控
		if page%10 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			logger.GetLogger().Debugf("Memory: Alloc=%.2fMB, TotalAlloc=%.2fMB",
				float64(m.Alloc)/1024/1024, float64(m.TotalAlloc)/1024/1024)
		}

		if consecutiveEmptyPages >= opt.MaxTryCount {
			logger.GetLogger().Infof("连续 %d 页无新评论，停止爬取", opt.MaxTryCount)
			break
		}

		sem <- struct{}{}
		pageWg.Add(1)

		// 启动 goroutine 并发爬取每一页评论
		go func(pageNum int, offset string) {
			defer func() {
				if r := recover(); r != nil {
					logger.GetLogger().Errorf("Page %d goroutine PANIC: %v\n%s", pageNum, r, string(debug.Stack()))
				}
				pageWg.Done()
				<-sem
			}()

			select {
			case <-ctx.Done():
				logger.GetLogger().Debugf("Page %d canceled", pageNum)
				return
			default:
				// 正常执行
			}

			// 延迟与抖动，防止被风控
			baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
			delay := baseDelay + jitter
			time.Sleep(delay)

			logger.GetLogger().Infof("并发爬取第 %d 页评论 (oid: %s, offset: %s)", pageNum, oid, offset)

			// 计算进度百分比
			if total > 0 {
				progressPercent := float64(downloadedCount) / float64(total) * 100
				logger.GetLogger().Infof("Processing page %d, progress: %.1f%% (%d/%d)",
					pageNum, progressPercent, downloadedCount, total)
			}
			cmtInfo, err := FetchComment(oid, pageNum, opt.Corder, opt.Cookie, offset)
			if err != nil {
				logger.GetLogger().Errorf("请求评论失败，视频%s，第%d页: %v", oid, pageNum, err)
				mu.Lock()
				consecutiveEmptyPages++
				mu.Unlock()
				return
			}

			if cmtInfo.Code != 0 {
				logger.GetLogger().Errorf("请求评论失败，视频%s，第%d页失败: %s", oid, pageNum, cmtInfo.Message)
				mu.Lock()
				consecutiveEmptyPages++
				mu.Unlock()
				return
			}

			logger.GetLogger().Infof("第 %d 页获取到 %d 条主评论", pageNum, len(cmtInfo.Data.Replies))

			// 检查是否到达末尾
			if cmtInfo.Data.Cursor.IsEnd {
				logger.GetLogger().Infof("API返回已到达末尾，停止爬取")
				mu.Lock()
				consecutiveEmptyPages = opt.MaxTryCount
				mu.Unlock()
				return
			}

			var replyCollection []model.ReplyItem
			replyCollection = append(replyCollection, cmtInfo.Data.Replies...)

			// 处理子评论
			for _, k := range cmtInfo.Data.Replies {
				if k.Rcount == 0 {
					continue
				}
				if len(k.Replies) > 0 && len(k.Replies) == k.Rcount {
					replyCollection = append(replyCollection, k.Replies...)
				} else {
					subCmts := FindSubComment(k, opt)
					replyCollection = append(replyCollection, subCmts...)
				}
			}

			// 处理置顶评论
			if len(cmtInfo.Data.TopReplies) != 0 {
				replyCollection = append(replyCollection, cmtInfo.Data.TopReplies...)
				for _, k := range cmtInfo.Data.TopReplies {
					if len(k.Replies) > 0 {
						replyCollection = append(replyCollection, k.Replies...)
					}
				}
			}

			var cmtCollection []model.Comment
			newCommentCount := 0

			// 评论去重与收集
			mu.Lock()
			for _, k := range replyCollection {
				if _, ok := recordedMap[k.Rpid]; !ok {
					cmt := NewCMT(&k)
					recordedMap[cmt.Rpid] = true
					cmtCollection = append(cmtCollection, cmt)
					resultChan <- cmt
					newCommentCount++
				}
			}

			if newCommentCount == 0 {
				consecutiveEmptyPages++
				logger.GetLogger().Debugf("第%d页无新评论，连续空页计数: %d", pageNum, consecutiveEmptyPages)
			} else {
				consecutiveEmptyPages = 0
			}

			downloadedCount += newCommentCount
			mu.Unlock()

			remaining := total - downloadedCount
			if remaining < 0 {
				remaining = 0
			}

			logger.GetLogger().Infof("视频%s，第%d页已爬取%d条新评论，总计%d条，预计剩余%d条",
				oid, pageNum, newCommentCount, downloadedCount, remaining)

			if len(cmtCollection) > 0 {
				store.Save2CSV(opt.Bvid, cmtCollection, savePath, imgSavePath, opt.ImgDownload)
			}

			// 保存进度，便于断点续爬
			saveProgress(progressFile, pageNum+1, downloadedCount)

			if cmtInfo.Data.Cursor.PaginationReply.NextOffset != "" {
				mu.Lock()
				offsetStr = cmtInfo.Data.Cursor.PaginationReply.NextOffset
				mu.Unlock()
				logger.GetLogger().Debugf("更新offset: %s", offsetStr)
			}
		}(page, offsetStr)

		page++
	}

	pageWg.Wait() // 等待所有页 goroutine 完成
	logger.GetLogger().Infof("*****爬取视频：%s评论完成，共获取 %d 条评论*****", oid, downloadedCount)

	_ = removeProgress(progressFile) // 清理断点文件
}

// FindSubComment 递归爬取某条主评论下的所有子评论
// cmt: 主评论项
// opt: 爬取选项
// 返回值: 子评论集合
func FindSubComment(cmt model.ReplyItem, opt *model.Option) []model.ReplyItem {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("FindSubComment PANIC: %v\n%s", r, string(debug.Stack()))
		}
	}()

	oid := strconv.Itoa(cmt.Oid)
	round := 1
	replyCollection := []model.ReplyItem{}
	consecutiveEmptyPages := 0 // 连续空页计数

	logger.GetLogger().Infof("开始爬取评论 %d 的子评论，预计 %d 条", cmt.Rpid, cmt.Rcount)

	for {
		// 检查连续空页次数
		if consecutiveEmptyPages >= opt.MaxTryCount {
			logger.GetLogger().Infof("连续 %d 页无子评论，停止爬取评论 %d", opt.MaxTryCount, cmt.Rpid)
			break
		}

		// 延迟逻辑（配置化）
		baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
		delay := baseDelay + jitter
		time.Sleep(delay)

		logger.GetLogger().Infof("爬取评论 %d 的子评论第 %d 页", cmt.Rpid, round)
		cmtInfo, err := FetchSubComment(oid, cmt.Rpid, round, opt.Cookie)
		if err != nil {
			logger.GetLogger().Errorf("请求子评论失败，父评论%d，第%d页: %v", cmt.Rpid, round, err)
			consecutiveEmptyPages++
			round++
			continue
		}

		round++
		if cmtInfo.Code != 0 {
			logger.GetLogger().Errorf("请求子评论失败，父评论%d，第%d页失败: %s", cmt.Rpid, round-1, cmtInfo.Message)
			consecutiveEmptyPages++
			continue
		}

		if len(cmtInfo.Data.Replies) > 0 {
			replyCollection = append(replyCollection, cmtInfo.Data.Replies...)

			// 处理嵌套回复
			for _, k := range cmtInfo.Data.Replies {
				if len(k.Replies) > 0 {
					replyCollection = append(replyCollection, k.Replies...)
				}
			}

			// 处理置顶回复
			if len(cmtInfo.Data.TopReplies) != 0 {
				replyCollection = append(replyCollection, cmtInfo.Data.TopReplies...)
				for _, k := range cmtInfo.Data.TopReplies {
					if len(k.Replies) > 0 {
						replyCollection = append(replyCollection, k.Replies...)
					}
				}
			}

			consecutiveEmptyPages = 0 // 重置连续空页计数
			logger.GetLogger().Debugf("评论 %d 第 %d 页获取到 %d 条子评论", cmt.Rpid, round-1, len(cmtInfo.Data.Replies))
		} else {
			consecutiveEmptyPages++
			logger.GetLogger().Debugf("评论 %d 第 %d 页无子评论，连续空页计数: %d", cmt.Rpid, round-1, consecutiveEmptyPages)
		}
	}

	logger.GetLogger().Infof("评论 %d 子评论爬取完成，共获取 %d 条", cmt.Rpid, len(replyCollection))
	return replyCollection
}

// NewCMT 将 ReplyItem 转换为 Comment 结构体
func NewCMT(item *model.ReplyItem) model.Comment {
	// 边界条件处理
	if item == nil {
		logger.GetLogger().Warn("Invalid ReplyItem received: nil pointer")
		return model.Comment{}
	}

	// Oid 校验
	var bvid string
	if item.Oid > 0 {
		bvid = Avid2Bvid(int64(item.Oid))
	}
	// 防御性：Bvid 必须以 BV 开头且长度为12
	if !strings.HasPrefix(bvid, "BV") || len(bvid) != 12 {
		bvid = ""
	}
	return model.Comment{
		Uname:         item.Member.Uname,
		Sex:           item.Member.Sex,
		Content:       item.Content.Message,
		Rpid:          item.Rpid,
		Oid:           item.Oid,
		Bvid:          bvid,
		Mid:           item.Mid,
		Parent:        item.Parent,
		Ctime:         item.Ctime,
		Like:          item.Like,
		Following:     item.ReplyControl.Following,
		Current_level: item.Member.LevelInfo.CurrentLevel,
		Pictures:      item.Content.Pictures,
		Location:      strings.Replace(item.ReplyControl.Location, "IP属地：", "", -1),
	}
}

// FindUser 爬取指定 up 主的所有视频评论
// sem: 并发信号量
// opt: 爬取选项（需包含 mid）
func FindUser(sem chan struct{}, opt *model.Option) {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("FindUser PANIC: %v\n%s", r, string(debug.Stack()))
		}
	}()

	var wg sync.WaitGroup
	round := opt.Skip + 1
	var videoCollection []model.VideoItem

	for ; round < opt.Pages+opt.Skip; round++ {
		// 延迟逻辑（配置化）
		baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
		delay := baseDelay + jitter
		time.Sleep(delay)
		logger.GetLogger().Infof("爬取视频列表第%d页", round)
		tempVideoInfo, err := FetchVideoList(opt.Mid, round, opt.Vorder, opt.Cookie)
		if err != nil {
			logger.GetLogger().Errorf("请求up主视频列表失败，第%d页失败", round)
			logger.GetLogger().Error(err)
			continue
		}
		if tempVideoInfo.Code != 0 {
			logger.GetLogger().Errorf("请求up主视频列表失败，第%d页失败", round)
			logger.GetLogger().Error(tempVideoInfo.Message)
			continue
		}
		if len(tempVideoInfo.Data.List.Vlist) != 0 {
			videoCollection = append(videoCollection, tempVideoInfo.Data.List.Vlist...)
		} else {
			break
		}
	}

	logger.GetLogger().Infof("%d查找到了%d条视频", opt.Mid, len(videoCollection))
	for _, k := range videoCollection {
		time.Sleep(3 * time.Second)
		logger.GetLogger().Infof("------启动爬取%d------", k.Aid)
		wg.Add(1)
		sem <- struct{}{}

		// 创建结果通道
		resultChan := make(chan model.Comment, 1000)

		// 传递结果通道作为第5个参数
		go func(aid int) {
			defer wg.Done()
			FindComment(context.Background(), sem, &wg, aid, opt, resultChan)
			close(resultChan)
		}(k.Aid)
	}
	wg.Wait()
}

// 断点续爬相关结构体与方法
// progress 记录断点信息
// page: 当前页码
// downloadedCount: 已下载评论数
type progress struct {
	Page            int `json:"page"`
	DownloadedCount int `json:"downloaded_count"`
}

// saveProgress 保存断点信息到文件
func saveProgress(filename string, page, downloadedCount int) error {
	p := progress{Page: page, DownloadedCount: downloadedCount}
	b, _ := json.Marshal(p)
	return os.WriteFile(filename, b, 0644)
}

// loadProgress 加载断点信息
func loadProgress(filename string) (progress, error) {
	var p progress
	b, err := os.ReadFile(filename)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}

// removeProgress 删除断点文件
func removeProgress(filename string) error {
	return os.Remove(filename)
}
