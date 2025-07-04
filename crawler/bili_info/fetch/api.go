package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/model"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"
)

// API 客户端结构体
type APIClient struct {
	HTTPClient *http.Client
	Cookies    string
}

// 创建新的 API 客户端
func NewAPIClient(cookies string) *APIClient {
	return &APIClient{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Cookies: cookies,
	}
}

// 获取视频信息
func (c *APIClient) GetVideoInfo(ctx context.Context, bvid string) (*model.VideoInfo, error) {
	// 1. 构建请求参数
	params := url.Values{"bvid": []string{bvid}}

	// 2. 生成带WBI签名的查询字符串
	query := util.GenerateWBIParams(params)
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?%s", query)

	// 3. 创建带上下文的请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 4. 设置请求头
	util.SetHeaders(req, c.Cookies)

	// 5. 发送请求（带重试机制）
	var resp *http.Response
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = c.HTTPClient.Do(req)
		if reqErr != nil {
			return reqErr
		}
		if resp.StatusCode >= 500 {
			// 5xx 服务器错误，重试
			resp.Body.Close()
			return fmt.Errorf("服务器错误: %d", resp.StatusCode)
		}
		if resp.StatusCode >= 400 {
			// 4xx 直接失败，不重试
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", resp.StatusCode)}
		}
		return nil
	}, func(err error) bool {
		// 只要不是PermanentError都可重试
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		// 这里可扩展为更细致的错误类型判断
		return true
	}, logger.GetLogger())

	if err != nil {
		return nil, fmt.Errorf("API请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 6. 解析API响应
	var apiResp model.VideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %v", err)
	}

	// 7. 检查API错误码
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API错误: %d - %s", apiResp.Code, apiResp.Message)
	}

	// 8. 返回视频信息
	return &model.VideoInfo{
		BVID:  bvid,
		Title: apiResp.Data.Title,
		Cover: apiResp.Data.Pic,
	}, nil
}
