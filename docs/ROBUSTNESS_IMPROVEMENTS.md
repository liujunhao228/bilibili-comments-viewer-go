# 爬虫健壮性改进总结

本文档总结了为 B 站评论爬虫系统实施的健壮性改进措施。

## 1. Panic 捕获和日志记录

### 改进内容
- 在所有关键函数入口/出口添加了详细的日志记录
- 实现了完整的 panic 捕获机制，包括堆栈信息记录
- 使用 `runtime.FuncForPC` 获取函数名称用于日志标识

### 修改的文件
- `crawler/blblcd/core/fetch.go`
- `crawler/blblcd/core/comment.go`
- `crawler/blblcd/core/crypto.go`
- `backend/crawler_manager.go`
- `backend/comment_processor.go`
- `crawler/blblcd/crawler.go`

### 示例代码
```go
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
    // ... 函数逻辑
}
```

## 2. Goroutine 安全退出

### 改进内容
- 完善了并发控制机制
- 确保所有 goroutine 都能正常退出，不泄漏资源
- 添加了上下文取消检查

### 示例代码
```go
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
    // ... 页面处理逻辑
}(page, offsetStr)
```

## 3. 增强重试机制

### 改进内容
- 改进了重试逻辑，增加了指数退避策略
- 添加了随机抖动，避免重试风暴
- 优化了重试延迟计算

### 修改的文件
- `crawler/bili_info/util/retry.go`

### 示例代码
```go
func RetryDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
    retryDelay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
    if retryDelay > maxDelay {
        retryDelay = maxDelay
    }
    // 添加随机抖动
    jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
    return retryDelay + jitter
}
```

## 4. 边界条件处理

### 改进内容
- 在 `NewCMT` 转换函数中添加了空指针检查
- 在 `Bvid2Avid` 函数中添加了输入验证
- 防止因无效输入导致的崩溃

### 示例代码
```go
// NewCMT 函数中的边界检查
if item == nil {
    logger.GetLogger().Warn("Invalid ReplyItem received: nil pointer")
    return model.Comment{}
}

// Bvid2Avid 函数中的边界检查
if len(bvid) < 4 {
    logger.GetLogger().Errorf("Invalid BVID: %s", bvid)
    return 0
}
```

## 5. HTTP 客户端增强

### 改进内容
- 增加了请求超时时间（从 15 秒增加到 30 秒）
- 添加了内置重试机制
- 增强了错误处理和日志记录

### 示例代码
```go
client := resty.New()
client.SetTimeout(30 * time.Second) // 增加超时时间
client.SetRetryCount(3)
client.SetRetryWaitTime(2 * time.Second)
```

## 6. 上下文超时控制

### 改进内容
- 在 `CrawlAndImport` 函数中添加了 30 分钟的超时控制
- 确保长时间运行的任务能够被正确终止

### 示例代码
```go
// 添加上下文超时控制
ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
defer cancel()
```

## 7. 内存监控

### 改进内容
- 每处理 10 页评论时记录内存使用情况
- 监控内存分配和总分配量
- 帮助识别内存泄漏问题

### 示例代码
```go
// 内存监控
if page%10 == 0 {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    logger.GetLogger().Debugf("Memory: Alloc=%.2fMB, TotalAlloc=%.2fMB", 
        float64(m.Alloc)/1024/1024, float64(m.TotalAlloc)/1024/1024)
}
```

## 8. 进度日志增强

### 改进内容
- 添加了详细的进度百分比计算
- 记录当前处理页数和总页数
- 提供更好的用户体验

### 示例代码
```go
// 计算进度百分比
if total > 0 {
    progressPercent := float64(downloadedCount) / float64(total) * 100
    logger.GetLogger().Infof("Processing page %d, progress: %.1f%% (%d/%d)", 
        pageNum, progressPercent, downloadedCount, total)
}
```

## 9. 资源清理

### 改进内容
- 确保通道正确关闭
- 添加了资源清理逻辑
- 防止资源泄漏

### 示例代码
```go
// 关闭通道并收集所有评论
close(resultChan)
defer func() {
    for range resultChan {} // 清空通道
}()

for comment := range resultChan {
    comments = append(comments, comment)
}
```

## 改进效果

这些改进显著增强了爬虫系统的健壮性：

1. **稳定性提升**：panic 能被捕获并记录，系统不会因单个错误而崩溃
2. **资源管理**：goroutine 能安全退出，不会泄漏资源
3. **网络可靠性**：完善的超时和重试机制提高了网络请求的成功率
4. **可观测性**：详细的日志记录便于问题排查和性能监控
5. **边界安全**：输入验证防止了因无效数据导致的崩溃
6. **性能监控**：内存监控帮助识别性能瓶颈

## 使用建议

1. **日志级别**：生产环境建议使用 `info` 级别，调试时使用 `debug` 级别
2. **超时配置**：根据网络环境调整超时时间
3. **重试策略**：根据目标网站的限流策略调整重试参数
4. **内存监控**：定期检查内存使用情况，及时发现内存泄漏

## 注意事项

1. 这些改进会增加一定的性能开销，但相比稳定性的提升是值得的
2. 日志文件会增长较快，需要定期清理
3. 在生产环境中需要监控 panic 的发生频率
4. 建议定期运行测试以验证改进的有效性 