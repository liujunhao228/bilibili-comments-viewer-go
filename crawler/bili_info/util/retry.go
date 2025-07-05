package util

import (
	"math"
	"math/rand"
	"time"

	mainconfig "bilibili-comments-viewer-go/config"
)

// RetryDelay 指数退避延时（支持自定义参数）
func RetryDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	retryDelay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if retryDelay > maxDelay {
		retryDelay = maxDelay
	}
	// 添加随机抖动
	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
	return retryDelay + jitter
}

// 兼容老接口
func DefaultRetryDelay(attempt int) time.Duration {
	return RetryDelay(attempt, 2*time.Second, 60*time.Second)
}

// PermanentError 用于标记不可重试的错误
// 例如4xx等业务错误
// 在Retry中遇到该类型错误会立即终止重试
type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string {
	return e.Err.Error()
}

// Retry 通用重试封装（支持自定义延迟参数）
// attempts: 最大重试次数
// fn: 执行的函数，返回 error
// isRetryable: 判断 error 是否可重试
// logger: 日志对象，需实现 Warnf(string, ...interface{})
// baseDelay: 初始延迟
// maxDelay: 最大延迟
func RetryWithBackoff(attempts int, baseDelay, maxDelay time.Duration, fn func() error, isRetryable func(error) bool, logger interface{ Warnf(string, ...interface{}) }) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		// 读取全局配置的jitter（毫秒）
		jitterMs := 800 // 默认值，兼容老配置
		if cfg := mainconfig.Get(); cfg != nil && cfg.Crawler.DelayJitterMs > 0 {
			jitterMs = cfg.Crawler.DelayJitterMs
		}
		delay := RetryDelay(i, baseDelay, maxDelay) + time.Duration(rand.Int63n(int64(jitterMs)))*time.Millisecond
		logger.Warnf("请求失败: %v，将在 %v 后重试 (%d/%d)", err, delay, i+1, attempts)
		time.Sleep(delay)
	}
	return err
}

// 兼容老接口，默认参数
func Retry(attempts int, fn func() error, isRetryable func(error) bool, logger interface{ Warnf(string, ...interface{}) }) error {
	return RetryWithBackoff(attempts, 2*time.Second, 60*time.Second, fn, isRetryable, logger)
}
