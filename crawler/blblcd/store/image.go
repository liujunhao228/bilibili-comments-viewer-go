package store

import (
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/utils"
	"bilibili-comments-viewer-go/logger"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"

	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DownloadImg 下载单张图片到指定目录，文件名为原始图片名
func DownloadImg(wg *sync.WaitGroup, sem chan struct{}, imageUrl string, output string, fileName string) {
	if !utils.FileOrPathExists(output) {
		os.MkdirAll(output, os.ModePerm)
	}
	var response *http.Response
	err := util.Retry(config.MaxRetries, func() error {
		var reqErr error
		response, reqErr = http.Get(imageUrl)
		if reqErr != nil {
			logger.GetLogger().Errorf("获取图片错误:%s", reqErr)
			return reqErr
		}
		if response.StatusCode >= 500 {
			return util.PermanentError{Err: fmt.Errorf("服务器错误: %d", response.StatusCode)}
		}
		if response.StatusCode >= 400 {
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", response.StatusCode)}
		}
		return nil
	}, func(err error) bool {
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		return true
	}, logger.GetLogger())

	if err != nil {
		logger.GetLogger().Errorf("获取图片错误:%s", err)
		wg.Done()
		<-sem
		return
	}
	if response.StatusCode != http.StatusOK {
		logger.GetLogger().Errorf("访问图片响应错误:%s", response.Status)
		response.Body.Close()
		wg.Done()
		<-sem
		return
	}
	outFile, err := os.Create(filepath.Join(output, fileName))
	if err != nil {
		logger.GetLogger().Errorf("创建文件错误:%s", err)
		response.Body.Close()
		wg.Done()
		<-sem
		return
	}
	defer func() {
		response.Body.Close()
		outFile.Close()
		if err := recover(); err != nil {
			logger.GetLogger().Errorf("写入图片失败:%s", imageUrl)
			logger.GetLogger().Error(err)
		}
		wg.Done()
		<-sem
	}()
	_, err = io.Copy(outFile, response.Body)
	if err != nil {
		logger.GetLogger().Errorf("报错图片错误:%s", err)
		return
	}
	logger.GetLogger().Info("图片下载成功：" + imageUrl)
}

// WriteImage 批量下载评论图片，按BV号分目录，文件名为原始图片名
func WriteImage(bvid string, pics []model.Picture, output string) {
	wg := sync.WaitGroup{}
	sem := make(chan struct{}, 5)
	bvidDir := filepath.Join(output, bvid)
	for _, pic := range pics {
		wg.Add(1)
		sem <- struct{}{}
		imgName := pic.Img_src
		// 取URL最后一段为文件名
		parts := strings.Split(imgName, "/")
		fileName := parts[len(parts)-1]
		go DownloadImg(&wg, sem, pic.Img_src, bvidDir, fileName)
	}
	wg.Wait()
}
