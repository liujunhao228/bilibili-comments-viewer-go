package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/bili_info/fetch"
	"bilibili-comments-viewer-go/crawler/bili_info/model"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"
)

func FetchVideoMetadata(bvid string) (*model.VideoInfo, error) {
	cfg := config.Get()

	// 加载cookies
	cookies := util.LoadCookies(cfg.Crawler.CookieFile)
	if cookies == "" {
		return nil, fmt.Errorf("Cookie加载失败")
	}

	// 创建API客户端
	apiClient := fetch.NewAPIClient(cookies)

	// 获取视频信息
	info, err := apiClient.GetVideoInfo(context.Background(), bvid)
	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %v", err)
	}

	// 下载封面
	if !cfg.Crawler.NoCover && info.Cover != "" {
		fileName := fmt.Sprintf("%s.jpg", info.BVID)
		localPath := filepath.Join(cfg.ImageStorageDir, "cover", fileName)
		coverDir := filepath.Join(cfg.ImageStorageDir, "cover")

		if err := os.MkdirAll(coverDir, 0755); err != nil {
			logger.GetLogger().Errorf("创建封面目录失败: %v", err)
		} else {
			resp, err := http.Get(info.Cover)
			if err != nil {
				logger.GetLogger().Errorf("封面下载失败: %v", err)
			} else {
				defer resp.Body.Close()

				file, err := os.Create(localPath)
				if err != nil {
					logger.GetLogger().Errorf("创建封面文件失败: %v", err)
				} else {
					defer file.Close()
					if _, err = io.Copy(file, resp.Body); err != nil {
						logger.GetLogger().Errorf("保存封面失败: %v", err)
					} else {
						info.LocalCover = filepath.Join("cover", fileName)
					}
				}
			}
		}
	}
	return info, nil
}
