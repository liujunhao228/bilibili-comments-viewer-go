package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
)

// core 包中 video.go 负责 up 主视频列表的抓取

// FetchVideoList 获取指定 up 主（mid）的投稿视频列表
// mid: up 主的 mid
// page: 页码
// order: 排序方式（如时间、播放量等）
// cookie: 登录 cookie
// 返回值: 视频列表响应结构体和错误信息
func FetchVideoList(mid int, page int, order string, cookie string) (videoList model.VideoListResponse, err error) {
	defer func() {
		if err := recover(); err != nil {
			logger.GetLogger().Errorf("爬取up主视频列表失败,mid:%d", mid)
			logger.GetLogger().Error(err)
		}
	}()
	// 构造 API 请求参数
	api := "https://api.bilibili.com/x/space/wbi/arc/search?"
	params := url.Values{}
	params.Set("mid", fmt.Sprint(mid))
	params.Set("order", order)
	params.Set("platform", "web")
	params.Set("pn", fmt.Sprint(page))
	params.Set("ps", "30")
	params.Set("tid", "0")

	client := http.Client{}
	// 对 API 进行签名加密，防止被风控
	crypedApi, _ := SignAndGenerateURL(api+params.Encode(), cookie)

	// 构造 HTTP 请求
	req, _ := http.NewRequest("GET", crypedApi, strings.NewReader(""))

	req.Header.Add("Origin", "https://space.bilibili.com")
	req.Header.Add("Host", Host)
	req.Header.Add("Referer", Origin)
	req.Header.Add("User-agent", UserAgent)
	req.Header.Add("Cookie", cookie)

	var resp *http.Response
	// 使用重试机制，提升健壮性
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = client.Do(req)
		if reqErr != nil {
			return reqErr
		}
		if resp.StatusCode >= 500 {
			return util.PermanentError{Err: fmt.Errorf("服务器错误: %d", resp.StatusCode)}
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
		logger.GetLogger().Error("parse json error:" + err.Error())
		return
	}
	defer resp.Body.Close()

	// 读取响应体并解析 JSON
	jsonByte, _ := io.ReadAll(resp.Body)
	logger.GetLogger().Info(resp.Status)
	json.Unmarshal(jsonByte, &videoList)
	logger.GetLogger().Infof("爬取up主视频列表成功,mid:%d，第%d页", mid, page)
	return
}
