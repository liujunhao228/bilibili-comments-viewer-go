package store

import (
	"encoding/csv"
	"fmt"
	"os"

	"bilibili-comments-viewer-go/crawler/bili_info/model"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"
)

func WriteSingleResult(outputFile string, video *model.VideoInfo) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入标题
	if err := writer.Write([]string{"BVID", "Title", "Cover", "LocalCover"}); err != nil {
		return err
	}

	// 写入数据
	return writer.Write([]string{video.BVID, video.Title, video.Cover, video.LocalCover})
}

func WriteResultsToFile(results []*model.VideoInfo, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		logger.GetLogger().Errorf("创建文件失败: %v", err)
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入标题
	if err := writer.Write([]string{"BVID", "Title", "Cover", "LocalCover"}); err != nil {
		logger.GetLogger().Errorf("写入标题失败: %v", err)
		return err
	}

	// 写入数据
	for _, info := range results {
		if err := writer.Write([]string{info.BVID, info.Title, info.Cover, info.LocalCover}); err != nil {
			logger.GetLogger().Errorf("写入数据失败: %v", err)
			return err
		}
	}

	return nil
}

// GenerateOutputFileName 生成输出文件名
func GenerateOutputFileName(video *model.VideoInfo) string {
	safeTitle := util.SanitizeFileName(video.Title)
	return fmt.Sprintf("【%s】%s.csv", video.BVID, safeTitle)
}
