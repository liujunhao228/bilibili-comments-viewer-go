package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bilibili-comments-viewer-go/backend"
	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
	"bilibili-comments-viewer-go/utils"

	"embed"
	"html/template"
	"io/fs"

	"github.com/gin-gonic/gin"
)

//go:embed frontend/static/*
var staticFS embed.FS

//go:embed frontend/templates/*
var templatesFS embed.FS

func main() {
	// 初始化配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化日志
	logger.InitLogger(
		cfg.Logging.LogFile,
		cfg.Logging.LogLevel,
		cfg.Logging.MaxSizeMB,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAgeDays,
	)
	log := logger.GetLogger()

	// 确保用户数据目录存在
	if err := os.MkdirAll(cfg.UserDataDir, 0755); err != nil {
		log.Fatalf("Failed to create user data directory: %v", err)
	}

	// 确保爬虫输出目录存在
	if err := os.MkdirAll(cfg.Crawler.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create crawler output directory: %v", err)
	}

	// 确保图片目录存在
	if err := os.MkdirAll(cfg.ImageStorageDir, 0755); err != nil {
		log.Fatalf("Failed to create image storage directory: %v", err)
	}

	// 初始化数据库
	err = database.InitDB(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.CloseDB()

	// 创建Gin路由器
	router := gin.Default()

	// 内嵌静态文件服务
	staticServer, _ := fs.Sub(staticFS, "frontend/static")
	router.StaticFS("/static", http.FS(staticServer))

	// 内嵌模板
	tmplServer, _ := fs.Sub(templatesFS, "frontend/templates")
	router.SetHTMLTemplate(template.Must(template.ParseFS(tmplServer, "*.html")))

	// API路由
	api := router.Group("/api")
	{
		api.GET("/videos", getVideos)
		api.GET("/video/:bvid", getVideoDetails)
		api.GET("/comments/:bvid", getComments)
		api.POST("/crawl/:bvid", crawlVideo)
		api.POST("/crawl/up/:mid", crawlUpVideos)

		// 新增评论回复接口
		api.GET("/comment/replies/:comment_id", getCommentReplies)

		// 修复模块路由
		api.GET("/repair/validate", validateDatabase)
		api.GET("/repair/validate/:bvid", validateVideoData)
		api.POST("/repair/fix", repairDatabase)
		api.POST("/repair/fix/:bvid", repairVideoData)
	}

	// 本地图片服务
	router.GET("/local_images/*filename", serveLocalImage)

	// 代理图片
	router.GET("/proxy_image", proxyImage)

	// 前端路由
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	router.GET("/repair", func(c *gin.Context) {
		c.HTML(http.StatusOK, "repair.html", nil)
	})

	router.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// 启动服务器（支持优雅退出）
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.DefaultPort),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 信号捕获
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Printf("收到退出信号，正在优雅关闭服务器...")
		ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelTimeout()
		if err := server.Shutdown(ctxTimeout); err != nil {
			log.Fatalf("优雅关闭服务器失败: %v", err)
		}
		log.Printf("服务器已优雅退出")
	}()

	log.Printf("Starting server on port %d", cfg.DefaultPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// 实现爬取视频评论的API
func crawlVideo(c *gin.Context) {
	bvid := c.Param("bvid")
	if bvid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bvid parameter"})
		return
	}

	log := logger.GetLogger()
	log.Infof("收到爬取请求: bvid=%s", bvid)

	// 暂时不传 ctx，待 backend 层完成 context 改造后再恢复
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic] crawlVideo goroutine: %v", r)
			}
		}()
		if err := backend.CrawlAndImport(context.Background(), bvid); err != nil {
			log.Errorf("视频 %s 爬取失败: %v", bvid, err)
		} else {
			log.Infof("视频 %s 爬取完成", bvid)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":  "started",
		"message": "Crawling started for video " + bvid,
	})
}

// 爬取UP主所有视频的评论
func crawlUpVideos(c *gin.Context) {
	mid := c.Param("mid")
	if mid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mid parameter"})
		return
	}

	// 获取是否爬取所有视频参数
	fetchAll := c.DefaultQuery("all", "false") == "true"

	midInt, err := utils.StringToInt(mid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mid parameter"})
		return
	}

	log.Printf("收到UP主爬取请求: MID=%d, 爬取所有=%v", midInt, fetchAll)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic] crawlUpVideos goroutine: %v", r)
			}
		}()
		if err := backend.CrawlUpVideos(midInt, fetchAll); err != nil {
			log.Printf("UP主爬取失败: %v", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":  "started",
		"message": fmt.Sprintf("Crawling started for UP %s (all: %v)", mid, fetchAll),
	})
}

// 获取视频列表
func getVideos(c *gin.Context) {
	// 获取分页参数
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("pageSize", "10")
	searchTerm := c.DefaultQuery("search", "")

	pageInt, err := utils.StringToInt(page)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter"})
		return
	}

	pageSizeInt, err := utils.StringToInt(pageSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pageSize parameter"})
		return
	}

	videos, total, err := database.GetVideosPaginated(pageInt, pageSizeInt, searchTerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get videos"})
		return
	}

	if videos == nil {
		videos = []database.Video{} // 确保返回空数组而不是nil
	}
	c.JSON(http.StatusOK, gin.H{
		"videos":    videos,
		"total":     total,
		"page":      pageInt,
		"page_size": pageSizeInt,
	})
}

// 获取视频详情
func getVideoDetails(c *gin.Context) {
	bvid := c.Param("bvid")
	if bvid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bvid parameter"})
		return
	}

	video, err := database.GetVideoByBVid(bvid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get video details"})
		return
	}

	if video == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
		return
	}

	c.JSON(http.StatusOK, video)
}

// 获取评论
func getComments(c *gin.Context) {
	bvid := c.Param("bvid")
	if bvid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bvid parameter"})
		return
	}

	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("pageSize", "20")
	keyword := c.DefaultQuery("keyword", "") // 获取关键词参数

	pageInt, err := utils.StringToInt(page)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter"})
		return
	}

	pageSizeInt, err := utils.StringToInt(pageSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pageSize parameter"})
		return
	}

	// 默认只获取顶级评论
	comments, total, err := database.GetCommentsByBVid(bvid, pageInt, pageSizeInt, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get comments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comments": comments,
		"total":    total,
		"page":     pageInt,
		"pageSize": pageSizeInt,
	})
}

// 获取评论的回复
func getCommentReplies(c *gin.Context) {
	commentID := c.Param("comment_id")

	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("pageSize", "5") // 默认每页5条回复

	pageInt, _ := utils.StringToInt(page)
	pageSizeInt, _ := utils.StringToInt(pageSize)

	replies, total, err := database.GetCommentReplies(commentID, pageInt, pageSizeInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get comment replies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"replies":  replies,
		"total":    total,
		"page":     pageInt,
		"pageSize": pageSizeInt,
	})
}

// 实现本地图片服务
func serveLocalImage(c *gin.Context) {
	cfg := config.Get()
	filename := c.Param("filename")

	// 1. 路径标准化处理
	// 替换所有反斜杠为正斜杠，并清除开头的斜杠
	filename = strings.ReplaceAll(filename, `\`, `/`)
	filename = strings.TrimPrefix(filename, "/")

	// 2. 安全检查 - 防止路径遍历攻击
	if strings.Contains(filename, "..") || strings.Contains(filename, "//") {
		c.String(http.StatusBadRequest, "无效的文件名")
		return
	}

	// 3. 构造实际图片路径（注意：cfg.ImageStorageDir已是绝对路径）
	imagePath := filepath.Join(cfg.ImageStorageDir, filename)

	// 4. 检查文件是否存在
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		// 如果文件不存在，返回默认图片
		log.Printf("图片不存在: %s", imagePath)

		// 构造默认图片路径
		defaultPath := filepath.Join(cfg.FrontendDir, "static/images/default-cover.jpg")

		// 确保默认图片存在
		if _, err := os.Stat(defaultPath); err == nil {
			c.Header("Cache-Control", "public, max-age=31536000") // 1年缓存
			c.File(defaultPath)
			return
		}

		// 如果默认图片也不存在，返回404
		c.String(http.StatusNotFound, "未找到默认图片")
		return
	}

	// 5. 检查是否真的是图片文件 (基础文件扩展名检查)
	if !isImageFile(filename) {
		c.String(http.StatusForbidden, "不支持的图像类型")
		return
	}

	// 6. 设置缓存头并返回图片
	c.Header("Cache-Control", "public, max-age=31536000") // 1年缓存
	c.File(imagePath)
}

// 辅助函数：检查是否为图片文件
func isImageFile(filename string) bool {
	// 获取小写的文件扩展名
	ext := strings.ToLower(filepath.Ext(filename))

	// 支持的图片扩展名列表
	supported := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	for _, e := range supported {
		if ext == e {
			return true
		}
	}
	return false
}

// 远程图片代理逻辑
func proxyImage(c *gin.Context) {
	// 从查询参数获取图片URL
	imageURL := c.Query("url")
	if imageURL == "" {
		c.String(http.StatusBadRequest, "Missing image URL")
		return
	}

	// 安全过滤：只允许特定域名
	if !isAllowedImageDomain(imageURL) {
		c.String(http.StatusForbidden, "Image domain not allowed")
		return
	}

	// 创建带超时的HTTP客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 创建请求并设置必要的请求头
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		log.Printf("Proxy image request creation failed: %v", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}

	// 设置请求头模拟浏览器访问
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Proxy image fetch failed: %v", err)
		c.String(http.StatusBadGateway, "Failed to fetch image")
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		log.Printf("Proxy image non-200 status: %d for URL: %s", resp.StatusCode, imageURL)
		c.String(mapHTTPStatus(resp.StatusCode), "Image server error: "+resp.Status)
		return
	}

	// 获取Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream" // 默认类型
	}

	// 验证是否为图片类型
	if !strings.HasPrefix(contentType, "image/") {
		log.Printf("Non-image content type: %s for URL: %s", contentType, imageURL)
		c.String(http.StatusUnsupportedMediaType, "Not an image")
		return
	}

	// 设置响应头
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=31536000") // 1年缓存
	c.Header("Content-Length", strconv.Itoa(int(resp.ContentLength)))

	// 流式传输图片数据
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		log.Printf("Error streaming image: %v", err)
	}
}

// 辅助函数：映射HTTP状态码
func mapHTTPStatus(statusCode int) int {
	if statusCode >= 400 && statusCode < 500 {
		return http.StatusBadRequest
	}
	if statusCode >= 500 {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// 检查是否允许的图片域名
func isAllowedImageDomain(url string) bool {
	// 允许本地图片路径
	if strings.HasPrefix(url, "cover/") || strings.Contains(url, "images/") {
		return true
	}

	cfg := config.Get()
	for _, domain := range cfg.AllowedImageDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}

// 校验数据库完整性
func validateDatabase(c *gin.Context) {
	log := logger.GetLogger()
	log.Info("收到数据库完整性校验请求")

	repairService := backend.NewRepairService()
	result, err := repairService.ValidateDatabase()
	if err != nil {
		log.Errorf("数据库校验失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Database validation failed",
			"message": err.Error(),
			"type":    backend.ErrorTypeDatabaseError,
			"level":   backend.ErrorLevelCritical,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"result":  result,
		"message": fmt.Sprintf("数据库校验完成，发现 %d 个问题", len(result.Issues)),
	})
}

// 校验指定视频的数据
func validateVideoData(c *gin.Context) {
	bvid := c.Param("bvid")
	if bvid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bvid parameter"})
		return
	}

	log := logger.GetLogger()
	log.Infof("收到视频数据校验请求: bvid=%s", bvid)

	repairService := backend.NewRepairService()
	result, err := repairService.ValidateVideoData(bvid)
	if err != nil {
		log.Errorf("视频数据校验失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Video data validation failed",
			"message": err.Error(),
			"type":    backend.ErrorTypeDatabaseError,
			"level":   backend.ErrorLevelHigh,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"result":  result,
		"message": fmt.Sprintf("视频 %s 数据校验完成，发现 %d 个问题", bvid, len(result.Issues)),
	})
}

// 修复整个数据库
func repairDatabase(c *gin.Context) {
	log := logger.GetLogger()
	log.Info("收到数据库修复请求")

	repairService := backend.NewRepairService()
	result, err := repairService.RepairDatabase()
	if err != nil {
		log.Errorf("数据库修复失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Database repair failed",
			"message": err.Error(),
			"type":    backend.ErrorTypeDatabaseError,
			"level":   backend.ErrorLevelCritical,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"result":  result,
		"message": fmt.Sprintf("数据库修复完成，修复了 %d 个问题", result.Summary.IssuesFixed),
	})
}

// 修复指定视频的数据
func repairVideoData(c *gin.Context) {
	bvid := c.Param("bvid")
	if bvid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bvid parameter"})
		return
	}

	log := logger.GetLogger()
	log.Infof("收到视频数据修复请求: bvid=%s", bvid)

	repairService := backend.NewRepairService()
	result, err := repairService.RepairVideoData(bvid)
	if err != nil {
		log.Errorf("视频数据修复失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Video data repair failed",
			"message": err.Error(),
			"type":    backend.ErrorTypeDatabaseError,
			"level":   backend.ErrorLevelHigh,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"result":  result,
		"message": fmt.Sprintf("视频 %s 数据修复完成，修复了 %d 个问题", bvid, result.Summary.Summary.IssuesFixed),
	})
}
