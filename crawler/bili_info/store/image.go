package store

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"
)

func DownloadImage(url, bvid, imageDir string) (string, error) {
	videoDir := filepath.Join(imageDir, bvid)
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	// 生成唯一文件名
	hash := md5.Sum([]byte(url))
	filename := fmt.Sprintf("%x.jpg", hash)
	filePath := filepath.Join(videoDir, filename)

	var resp *http.Response
	err := util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = http.Get(url)
		if reqErr != nil {
			return reqErr
		}
		if resp.StatusCode >= 500 {
			return errors.New(fmt.Sprintf("服务器错误: %d", resp.StatusCode))
		}
		if resp.StatusCode >= 400 {
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", resp.StatusCode)}
		}
		return nil
	}, func(err error) bool {
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		return true
	}, logger.GetLogger())

	if err != nil {
		return "", fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP错误: %s", resp.Status)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}

	return filePath, nil
}
