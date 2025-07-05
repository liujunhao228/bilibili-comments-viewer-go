package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"bilibili-comments-viewer-go/crawler/blblcd/model"

	"bilibili-comments-viewer-go/logger"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"

	"github.com/go-resty/resty/v2"
)

// core 包中 comment.go 负责与 B 站评论相关的 API 请求与数据结构处理

// UserAgent/Origin/Host 用于 HTTP 请求头，模拟浏览器环境
var (
	UserAgent string = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0"
	Origin    string = "https://www.bilibili.com"
	Host      string = "https://www.bilibili.com"
)

// FetchCount 获取指定 oid（视频 avid）下的评论总数
func FetchCount(oid string) (count int, err error) {
	url := fmt.Sprintf("https://api.bilibili.com/x/v2/reply/count?type=1&oid=%s", oid)
	client := resty.New()
	data := model.CommentsCountResponse{}
	resp, err := client.R().
		SetResult(&data).
		SetHeader("Accept", "application/json").
		Get(url)
	if err != nil {
		logger.GetLogger().Error("Erro:" + err.Error())
		return
	}
	if resp.IsError() {
		logger.GetLogger().Error("Erro:" + resp.String())
		return
	}

	count = data.Data.Count
	return
}

// FetchComment 获取指定 oid（视频 avid）下主评论列表，支持分页和 offset
// oid: 视频 avid
// next: 页码
// order: 排序方式
// cookie: 登录 cookie
// offsetStr: 分页 offset
// 返回值: 评论响应结构体和错误
func FetchComment(oid string, next int, order int, cookie string, offsetStr string) (data model.CommentResponse, err error) {
	funcName := runtime.FuncForPC(reflect.ValueOf(FetchComment).Pointer()).Name()
	logger.GetLogger().Debugf("START %s: oid=%s, page=%d", funcName, oid, next)

	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("API请求 PANIC: %v\n%s", r, string(debug.Stack()))
			err = fmt.Errorf("API请求时发生panic: %v", r)
		}
		logger.GetLogger().Debugf("END %s: oid=%s, page=%d", funcName, oid, next)
	}()

	// +++ 添加详细日志 +++
	logger.GetLogger().Debugf("请求评论API: oid=%s, page=%d, offset=%s", oid, next, offsetStr)
	client := resty.New()
	client.SetTimeout(30 * time.Second) // 增加超时时间
	client.SetRetryCount(3)
	client.SetRetryWaitTime(2 * time.Second)

	var fmtOffsetStr string
	if offsetStr == "" {
		fmtOffsetStr = `{"offset":""}`
	} else {
		fmtOffsetStr = fmt.Sprintf(`{"offset":%q}`, offsetStr)
	}

	params := url.Values{}
	params.Set("oid", oid)
	params.Set("type", "1")
	params.Set("mode", "3")
	params.Set("plat", "1")
	params.Set("web_location", "1315875")
	params.Set("pagination_str", fmtOffsetStr)

	url := "https://api.bilibili.com/x/v2/reply/wbi/main?" + params.Encode()
	newUrl, err := SignAndGenerateURL(url, cookie)
	if err != nil {
		logger.GetLogger().Errorf("生成签名URL失败: %v", err)
		return data, fmt.Errorf("生成签名URL失败: %v", err)
	}

	var resp *resty.Response
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = client.R().
			SetResult(&data).
			SetHeader("Accept", "application/json").
			SetHeader("User-Agent", UserAgent).
			SetHeader("Origin", Origin).
			SetHeader("Referer", "https://www.bilibili.com/").
			Get(newUrl)
		if reqErr != nil {
			logger.GetLogger().Errorf("请求评论API失败: %v", reqErr)
			return reqErr
		}
		if resp.StatusCode() >= 500 {
			return fmt.Errorf("服务器错误: %d", resp.StatusCode())
		}
		if resp.StatusCode() >= 400 {
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", resp.StatusCode())}
		}
		return nil
	}, func(err error) bool {
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		return true
	}, logger.GetLogger())

	if err != nil {
		logger.GetLogger().Errorf("请求评论API失败: %v", err)
		return data, fmt.Errorf("请求评论API失败: %v", err)
	}

	// +++ 添加详细日志 +++
	logger.GetLogger().Debugf("评论API响应: 状态码=%d, 响应体=%s", resp.StatusCode(), resp.String())

	if resp.IsError() {
		logger.GetLogger().Error("API响应错误:" + resp.String())
		return data, fmt.Errorf("API响应错误: %d", resp.StatusCode())
	}

	// 验证响应数据
	if data.Code != 0 {
		logger.GetLogger().Errorf("API返回错误码: %d, 消息: %s", data.Code, data.Message)
		return data, fmt.Errorf("API返回错误: %s", data.Message)
	}

	// 验证数据结构
	if data.Data.Replies == nil {
		data.Data.Replies = []model.ReplyItem{}
	}
	if data.Data.TopReplies == nil {
		data.Data.TopReplies = []model.ReplyItem{}
	}

	logger.GetLogger().Debugf("成功获取评论数据: 主评论%d条, 置顶评论%d条",
		len(data.Data.Replies), len(data.Data.TopReplies))

	return data, nil
}

// FetchSubComment 获取指定主评论（rpid）下的子评论列表，支持分页
// oid: 视频 avid
// rpid: 主评论 id
// next: 页码
// cookie: 登录 cookie
// 返回值: 评论响应结构体和错误
func FetchSubComment(oid string, rpid int64, next int, cookie string) (data model.CommentResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("子评论API请求 PANIC: %v\n%s", r, string(debug.Stack()))
			err = fmt.Errorf("子评论API请求时发生panic: %v", r)
		}
	}()

	logger.GetLogger().Debugf("请求子评论API: oid=%s, rpid=%d, page=%d", oid, rpid, next)

	client := http.Client{
		Timeout: 30 * time.Second, // 增加超时时间
	}
	payload := strings.NewReader("")

	params := url.Values{}
	params.Set("oid", oid)
	params.Set("type", "1")
	params.Set("root", fmt.Sprint(rpid))
	params.Set("ps", "20")
	params.Set("pn", fmt.Sprint(next))

	url := "https://api.bilibili.com/x/v2/reply/reply?" + params.Encode()
	newUrl, err := SignAndGenerateURL(url, cookie)
	if err != nil {
		logger.GetLogger().Errorf("生成子评论签名URL失败: %v", err)
		return data, fmt.Errorf("生成子评论签名URL失败: %v", err)
	}

	req, err := http.NewRequest("GET", newUrl, payload)
	if err != nil {
		logger.GetLogger().Errorf("创建子评论请求失败: %v", err)
		return data, fmt.Errorf("创建子评论请求失败: %v", err)
	}

	req.Header.Add("User-agent", UserAgent)
	req.Header.Add("Origin", Origin)
	req.Header.Add("Host", Host)
	req.Header.Add("Referer", "https://www.bilibili.com/")
	req.Header.Add("Sec-Fetch-Dest", "empty")
	req.Header.Add("Sec-Fetch-Mode", "cors")
	req.Header.Add("Sec-Fetch-Site", "same-site")
	req.Header.Add("Cookie", cookie)

	var res *http.Response
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		res, reqErr = client.Do(req)
		if reqErr != nil {
			return reqErr
		}
		if res.StatusCode >= 500 {
			// 5xx 服务器错误，重试
			return fmt.Errorf("服务器错误: %d", res.StatusCode)
		}
		if res.StatusCode >= 400 {
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", res.StatusCode)}
		}
		return nil
	}, func(err error) bool {
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		return true
	}, logger.GetLogger())

	if err != nil {
		logger.GetLogger().Errorf("请求子评论失败: %v", err)
		return data, fmt.Errorf("请求子评论失败: %v", err)
	}
	defer res.Body.Close()

	dataStr, err := io.ReadAll(res.Body)
	if err != nil {
		logger.GetLogger().Errorf("读取子评论响应失败: %v", err)
		return data, fmt.Errorf("读取子评论响应失败: %v", err)
	}

	if err := json.Unmarshal(dataStr, &data); err != nil {
		logger.GetLogger().Errorf("解析子评论JSON失败: %v", err)
		return data, fmt.Errorf("解析子评论JSON失败: %v", err)
	}

	// 验证响应数据
	if data.Code != 0 {
		logger.GetLogger().Errorf("子评论API返回错误码: %d, 消息: %s", data.Code, data.Message)
		return data, fmt.Errorf("子评论API返回错误: %s", data.Message)
	}

	// 验证数据结构
	if data.Data.Replies == nil {
		data.Data.Replies = []model.ReplyItem{}
	}
	if data.Data.TopReplies == nil {
		data.Data.TopReplies = []model.ReplyItem{}
	}

	logger.GetLogger().Infof("成功获取子评论数据: oid=%s, rpid=%d, 第%d页, 回复%d条",
		oid, rpid, next, len(data.Data.Replies))

	return data, nil
}
