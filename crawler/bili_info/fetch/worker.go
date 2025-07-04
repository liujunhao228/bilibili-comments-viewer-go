package fetch

import (
	"context"
	"math/rand"
	"sync"
	"time"

	mainconfig "bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/model"
	"bilibili-comments-viewer-go/crawler/bili_info/store"
	"bilibili-comments-viewer-go/logger"
)

func Worker(ctx context.Context, wg *sync.WaitGroup, apiClient *APIClient,
	jobs <-chan string, results chan<- *model.VideoInfo, cfg *config.Config) {

	defer wg.Done()

	for bvid := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
			// 随机延迟防止请求过快
			baseDelay := time.Duration(mainconfig.Get().Crawler.DelayBaseMs) * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(mainconfig.Get().Crawler.DelayJitterMs))) * time.Millisecond
			delay := baseDelay + jitter
			time.Sleep(delay)

			// 获取视频信息
			info, err := apiClient.GetVideoInfo(ctx, bvid)
			if err != nil {
				logger.GetLogger().Warnf("⚠️ 视频 %s 获取失败: %v", bvid, err)
				continue
			}

			// 下载封面图片（如果需要）
			if !cfg.NoCover && info.Cover != "" {
				localPath, err := store.DownloadImage(info.Cover, info.BVID, cfg.ImageDir)
				if err != nil {
					logger.GetLogger().Warnf("⚠️ 视频 %s 封面下载失败: %v", bvid, err)
					info.LocalCover = info.Cover // 保留原始URL
				} else {
					info.LocalCover = localPath
					logger.GetLogger().Infof("✅ 下载封面成功: %s → %s", info.Cover, localPath)
				}
			}

			results <- info
			logger.GetLogger().Infof("✅ 成功处理视频: %s - %s", info.BVID, info.Title)
		}
	}
}
