### crawler/blblcd/core/crypto.go
<!-- [START OF FILE: crypto.go] -->
```go
package core

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"

	"github.com/tidwall/gjson"
)

// core 包中 crypto.go 负责 B 站 API 请求的签名、加密、BVID/AVID 转换等工具方法

var (
	mixinKeyEncTab = []int{
		46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
		33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
		61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
		36, 20, 34, 44, 52,
	}
	cache          sync.Map // 缓存 imgKey 和 subKey，减少请求频率
	lastUpdateTime time.Time

	XOR_CODE = int64(23442827791579) // BVID/AVID 转换用常量
	MAX_CODE = int64(2251799813685247)
	CHARTS   = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
)

// SignAndGenerateURL 对 B 站 API 请求 URL 进行签名加密，防止被风控
// urlStr: 原始 URL
// cookie: 登录 cookie
// 返回值: 签名后的 URL 和错误
func SignAndGenerateURL(urlStr string, cookie string) (string, error) {
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	imgKey, subKey := getWbiKeysCached(cookie)
	query := urlObj.Query()
	params := map[string]string{}
	for k, v := range query {
		params[k] = v[0]
	}
	newParams := encWbi(params, imgKey, subKey)
	for k, v := range newParams {
		query.Set(k, v)
	}
	urlObj.RawQuery = query.Encode()
	newUrlStr := urlObj.String()
	return newUrlStr, nil
}

// encWbi 对参数进行加密签名，生成 w_rid
func encWbi(params map[string]string, imgKey, subKey string) map[string]string {
	mixinKey := getMixinKey(imgKey + subKey)
	currTime := strconv.FormatInt(time.Now().Unix(), 10)
	params["wts"] = currTime

	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Remove unwanted characters
	for k, v := range params {
		v = sanitizeString(v)
		params[k] = v
	}

	// Build URL parameters
	query := url.Values{}
	for _, k := range keys {
		query.Set(k, params[k])
	}
	queryStr := query.Encode()

	// Calculate w_rid
	hash := md5.Sum([]byte(queryStr + mixinKey))
	params["w_rid"] = hex.EncodeToString(hash[:])
	return params
}

// getMixinKey 根据 imgKey+subKey 生成混淆 key
func getMixinKey(orig string) string {
	var str strings.Builder
	for _, v := range mixinKeyEncTab {
		if v < len(orig) {
			str.WriteByte(orig[v])
		}
	}
	return str.String()[:32]
}

// sanitizeString 移除参数中的特殊字符，保证签名一致性
func sanitizeString(s string) string {
	unwantedChars := []string{"!", "'", "(", ")", "*"}
	for _, char := range unwantedChars {
		s = strings.ReplaceAll(s, char, "")
	}
	return s
}

// updateCache 定时更新 imgKey 和 subKey 缓存，减少 API 请求
func updateCache(cookie string) {
	if time.Since(lastUpdateTime).Minutes() < 10 {
		return
	}
	imgKey, subKey := getWbiKeys(cookie)
	cache.Store("imgKey", imgKey)
	cache.Store("subKey", subKey)
	lastUpdateTime = time.Now()
}

// getWbiKeysCached 获取缓存的 imgKey 和 subKey
func getWbiKeysCached(cookie string) (string, string) {
	updateCache(cookie)
	imgKeyI, _ := cache.Load("imgKey")
	subKeyI, _ := cache.Load("subKey")
	return imgKeyI.(string), subKeyI.(string)
}

// getWbiKeys 实时请求 B 站接口获取 imgKey 和 subKey
func getWbiKeys(cookie string) (string, string) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.bilibili.com/x/web-interface/nav", nil)
	if err != nil {
		fmt.Printf("Error creating request: %s", err)
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Cookie", string(cookie))

	var resp *http.Response
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = client.Do(req)
		if reqErr != nil {
			fmt.Printf("Error sending request: %s", reqErr)
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
	}, nil)

	if err != nil {
		fmt.Printf("Error sending request: %s", err)
		return "", ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %s", err)
		return "", ""
	}
	json := string(body)
	imgURL := gjson.Get(json, "data.wbi_img.img_url").String()
	subURL := gjson.Get(json, "data.wbi_img.sub_url").String()
	imgKey := strings.Split(strings.Split(imgURL, "/")[len(strings.Split(imgURL, "/"))-1], ".")[0]
	subKey := strings.Split(strings.Split(subURL, "/")[len(strings.Split(subURL, "/"))-1], ".")[0]
	return imgKey, subKey
}

// swapString 交换字符串中指定下标的字符
func swapString(s string, x, y int) string {
	chars := []rune(s)
	chars[x], chars[y] = chars[y], chars[x]
	return string(chars)
}

// Bvid2Avid 将 BVID 转换为 AVID
func Bvid2Avid(bvid string) (avid int64) {
	s := swapString(swapString(bvid, 3, 9), 4, 7)
	bv1 := string([]rune(s)[3:])
	temp := int64(0)
	for _, c := range bv1 {
		idx := strings.IndexRune(CHARTS, c)
		temp = temp*int64(58) + int64(idx)
	}
	avid = (temp & MAX_CODE) ^ XOR_CODE
	return
}

// Avid2Bvid 将 AVID 转换为 BVID
func Avid2Bvid(avid int64) (bvid string) {
	arr := [12]string{"B", "V", "1"}
	bvIdx := len(arr) - 1
	temp := (avid | (MAX_CODE + 1)) ^ XOR_CODE
	for temp > 0 {
		idx := temp % 58
		arr[bvIdx] = string(CHARTS[idx])
		temp /= 58
		bvIdx--
	}
	raw := strings.Join(arr[:], "")
	bvid = swapString(swapString(raw, 3, 9), 4, 7)
	return
}
```

<!-- [END OF FILE: crypto.go] -->

### crawler/blblcd/cli/version.go
<!-- [START OF FILE: version.go] -->
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "查看版本及构建信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(`
author: %s
version: %s
commit: %s
build time: %s

`, Inject.Author, Inject.Version, Inject.Commit, Inject.BuildTime)
	},
}
```

<!-- [END OF FILE: version.go] -->

### crawler/blblcd/cli/root.go
<!-- [START OF FILE: root.go] -->
```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Inject        *Injection
	cookieFile    string
	output        string
	workers       int
	corder        int
	imgDownload   bool
	maxTryCount   int
	commentOutput string
	imageOutput   string
)

var rootCmd = &cobra.Command{
	Use:   "blblcd",
	Short: "A command line tool for downloading Bilibili comments",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Please type `blblcd --help` for more information")
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cookieFile, "cookie", "c", "./cookie.txt", "cookie文件路径")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "./output", "保存目录")
	rootCmd.PersistentFlags().BoolVarP(&imgDownload, "img-download", "i", false, "是否下载评论中的图片")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "w", 5, "最多协程数量")
	rootCmd.PersistentFlags().IntVarP(&maxTryCount, "max-try-count", "u", 3, "当爬取结果为空时请求最大尝试次数")
	rootCmd.PersistentFlags().IntVarP(&corder, "corder", "v", 1, "爬取时评论排序方式，0：按时间，1：按点赞数，2：按回复数")
	rootCmd.PersistentFlags().StringVar(&commentOutput, "comment-output", "", "评论内容保存路径（默认为output目录下的视频BV号文件夹）")
	rootCmd.PersistentFlags().StringVar(&imageOutput, "image-output", "", "评论图片保存路径（默认为评论内容保存路径下的images文件夹）")
}

func Execute(injection *Injection) {
	Inject = injection
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

<!-- [END OF FILE: root.go] -->

### crawler/blblcd/utils/tool.go
<!-- [START OF FILE: tool.go] -->
```go
package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
)

func FileOrPathExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

func ExcutePath() string {
	excutePath, err := os.Executable()
	if err != nil {
		logger.GetLogger().Error(err.Error())
		os.Exit(1)
	}
	return filepath.Dir(excutePath)
}

func ReadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func PresetPath(path string) {
	if !FileOrPathExists(path) {
		os.MkdirAll(path, os.ModePerm)
	}
}

func EncodePaginationOffset(pagination model.PaginationOffset) string {
	paginationJSON, err := json.Marshal(pagination)
	if err != nil {
		fmt.Println("Error marshaling pagination:", err)
		return ""
	}

	return string(paginationJSON)
}

func DecodePaginationOffset(paginationStr string) (*model.PaginationOffset, error) {
	var paginationOffset model.PaginationOffset
	err := json.Unmarshal([]byte(paginationStr), &paginationOffset)
	if err != nil {
		fmt.Println("Error unmarshaling inner JSON:", err)
		return nil, err
	}
	return &paginationOffset, nil
}
```

<!-- [END OF FILE: tool.go] -->

### backend/repair_service.go
<!-- [START OF FILE: repair_service.go] -->
```go
package backend

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
)

// Issue 表示数据库问题
type Issue struct {
	Type          string   `json:"type"`
	Severity      string   `json:"severity"`
	Description   string   `json:"description"`
	Count         int      `json:"count"`
	Fixable       bool     `json:"fixable"`
	Fixed         bool     `json:"fixed"`
	Details       string   `json:"details,omitempty"`
	Category      string   `json:"category,omitempty"`
	AffectedBVids []string `json:"affected_bvids,omitempty"` // 受影响的BV号列表
	Level         string   `json:"level,omitempty"`          // 错误级别
}

// ValidationResult 表示校验结果
type ValidationResult struct {
	Issues  []Issue `json:"issues"`
	Summary struct {
		TotalVideos     int `json:"total_videos"`
		TotalComments   int `json:"total_comments"`
		IssuesFound     int `json:"issues_found"`
		IssuesFixed     int `json:"issues_fixed"`
		IssuesUnfixable int `json:"issues_unfixable"`
	} `json:"summary"`
}

// VideoValidationResult 表示单个视频的校验结果
type VideoValidationResult struct {
	VideoID string           `json:"video_id"`
	Issues  []Issue          `json:"issues"`
	Summary ValidationResult `json:"summary"`
}

// RepairService 修复服务
type RepairService struct {
	db *sql.DB
}

// NewRepairService 创建新的修复服务实例
func NewRepairService() *RepairService {
	return &RepairService{
		db: database.GetDB(),
	}
}

// ValidateDatabase 校验整个数据库
func (rs *RepairService) ValidateDatabase() (*ValidationResult, error) {
	log := logger.GetLogger()
	log.Info("开始校验数据库完整性")

	result := &ValidationResult{}
	var issues []Issue

	// 1. 校验视频信息表
	videoIssues, err := rs.validateVideoTable()
	if err != nil {
		return nil, fmt.Errorf("校验视频表失败: %w", err)
	}
	issues = append(issues, videoIssues...)

	// 2. 校验评论表
	commentIssues, err := rs.validateCommentTable()
	if err != nil {
		return nil, fmt.Errorf("校验评论表失败: %w", err)
	}
	issues = append(issues, commentIssues...)

	// 3. 校验评论关系表
	relationIssues, err := rs.validateCommentRelations()
	if err != nil {
		return nil, fmt.Errorf("校验评论关系表失败: %w", err)
	}
	issues = append(issues, relationIssues...)

	// 4. 校验评论统计表
	statsIssues, err := rs.validateCommentStats()
	if err != nil {
		return nil, fmt.Errorf("校验评论统计表失败: %w", err)
	}
	issues = append(issues, statsIssues...)

	// 5. 计算统计信息
	summary, err := rs.calculateSummary()
	if err != nil {
		return nil, fmt.Errorf("计算统计信息失败: %w", err)
	}

	result.Issues = issues
	result.Summary = *summary

	log.Infof("数据库校验完成，发现 %d 个问题", len(issues))
	return result, nil
}

// ValidateVideoData 校验指定视频的数据
func (rs *RepairService) ValidateVideoData(bvid string) (*VideoValidationResult, error) {
	log := logger.GetLogger()
	log.Infof("开始校验视频数据: %s", bvid)

	result := &VideoValidationResult{
		VideoID: bvid,
	}

	// 1. 检查视频是否存在
	videoExists, err := rs.checkVideoExists(bvid)
	if err != nil {
		return nil, fmt.Errorf("检查视频存在性失败: %w", err)
	}

	if !videoExists {
		result.Issues = append(result.Issues, Issue{
			Type:          ErrorTypeVideoNotFound,
			Severity:      ErrorLevelCritical,
			Level:         ErrorLevelCritical,
			Category:      ErrorTypeDataIntegrity,
			Description:   "视频不存在",
			Count:         1,
			Fixable:       false,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 在数据库中不存在", bvid),
		})
		// 如果视频都不存在，直接返回，不再校验评论和统计
		log.Infof("视频 %s 数据校验完成，发现 %d 个问题", bvid, len(result.Issues))
		return result, nil
	}

	// 2. 校验该视频的评论数据
	commentIssues, err := rs.validateVideoComments(bvid)
	if err != nil {
		return nil, fmt.Errorf("校验视频评论失败: %w", err)
	}
	result.Issues = append(result.Issues, commentIssues...)

	// 3. 校验该视频的评论统计
	statsIssues, err := rs.validateVideoStats(bvid)
	if err != nil {
		return nil, fmt.Errorf("校验视频评论统计失败: %w", err)
	}
	result.Issues = append(result.Issues, statsIssues...)

	// 4. 计算该视频的统计信息
	summary, err := rs.calculateVideoSummary(bvid)
	if err != nil {
		return nil, fmt.Errorf("计算视频统计信息失败: %w", err)
	}

	result.Summary = *summary

	log.Infof("视频 %s 数据校验完成，发现 %d 个问题", bvid, len(result.Issues))
	return result, nil
}

// 拆分：校验单个视频的评论统计
func (rs *RepairService) validateVideoStats(bvid string) ([]Issue, error) {
	var issues []Issue
	var commentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&commentCount)
	if err != nil {
		return nil, fmt.Errorf("查询评论数失败: %w", err)
	}
	var statsCount int
	err = rs.db.QueryRow("SELECT comment_count FROM comment_stats WHERE bvid = ?", bvid).Scan(&statsCount)
	if err == sql.ErrNoRows {
		// 没有统计记录
		issues = append(issues, Issue{
			Type:          ErrorTypeMissingStats,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataConsistency,
			Description:   "缺少评论统计信息",
			Count:         1,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 缺少评论统计信息", bvid),
		})
	} else if err != nil {
		return nil, fmt.Errorf("查询评论统计失败: %w", err)
	} else if statsCount != commentCount {
		// 统计不一致
		issues = append(issues, Issue{
			Type:          ErrorTypeInconsistentStats,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataConsistency,
			Description:   fmt.Sprintf("评论统计不一致（实际: %d, 统计: %d）", commentCount, statsCount),
			Count:         1,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 评论统计不一致（实际: %d, 统计: %d）", bvid, commentCount, statsCount),
		})
	}
	return issues, nil
}

// RepairDatabase 修复数据库问题
func (rs *RepairService) RepairDatabase() (*ValidationResult, error) {
	log := logger.GetLogger()
	log.Info("开始修复数据库问题")

	// 1. 先进行校验，获取所有问题
	validationResult, err := rs.ValidateDatabase()
	if err != nil {
		return nil, fmt.Errorf("校验失败: %w", err)
	}

	// 2. 修复可修复的问题
	fixedCount := rs.fixAllIssues(validationResult.Issues)

	// 3. 更新统计信息
	validationResult.Summary.IssuesFixed = fixedCount

	log.Infof("数据库修复完成，修复了 %d 个问题", fixedCount)
	return validationResult, nil
}

// 拆分：批量修复所有问题
func (rs *RepairService) fixAllIssues(issues []Issue) int {
	log := logger.GetLogger()
	fixedCount := 0
	for i := range issues {
		issue := &issues[i]
		if issue.Fixable && !issue.Fixed {
			if err := rs.fixIssue(issue); err != nil {
				log.Errorf("修复问题失败 [%s]: %v", issue.Type, err)
				continue
			}
			issue.Fixed = true
			fixedCount++
			log.Infof("成功修复问题: %s", issue.Description)
		}
	}
	return fixedCount
}

// RepairVideoData 修复指定视频的数据问题
func (rs *RepairService) RepairVideoData(bvid string) (*VideoValidationResult, error) {
	log := logger.GetLogger()
	log.Infof("开始修复视频数据: %s", bvid)

	// 1. 先进行校验，获取该视频的所有问题
	validationResult, err := rs.ValidateVideoData(bvid)
	if err != nil {
		return nil, fmt.Errorf("校验失败: %w", err)
	}

	// 2. 修复可修复的问题
	fixedCount := rs.fixAllVideoIssues(bvid, validationResult.Issues)

	// 3. 更新统计信息
	validationResult.Summary.Summary.IssuesFixed = fixedCount

	log.Infof("视频 %s 数据修复完成，修复了 %d 个问题", bvid, fixedCount)
	return validationResult, nil
}

// 拆分：批量修复单个视频的所有问题
func (rs *RepairService) fixAllVideoIssues(bvid string, issues []Issue) int {
	log := logger.GetLogger()
	fixedCount := 0
	for i := range issues {
		issue := &issues[i]
		if issue.Fixable && !issue.Fixed {
			if err := rs.fixVideoIssue(bvid, issue); err != nil {
				log.Errorf("修复视频问题失败 [%s]: %v", issue.Type, err)
				continue
			}
			issue.Fixed = true
			fixedCount++
			log.Infof("成功修复视频问题: %s", issue.Description)
		}
	}
	return fixedCount
}

// 私有方法：校验视频表
func (rs *RepairService) validateVideoTable() ([]Issue, error) {
	var issues []Issue

	emptyTitleIssues, err := rs.validateEmptyVideoTitles()
	if err != nil {
		return nil, err
	}
	issues = append(issues, emptyTitleIssues...)

	duplicateBvidIssues, err := rs.validateDuplicateBVids()
	if err != nil {
		return nil, err
	}
	issues = append(issues, duplicateBvidIssues...)

	missingCommentsIssues, err := rs.validateVideosMissingComments()
	if err != nil {
		return nil, err
	}
	issues = append(issues, missingCommentsIssues...)

	return issues, nil
}

// 拆分：检查空标题
func (rs *RepairService) validateEmptyVideoTitles() ([]Issue, error) {
	var issues []Issue
	var count int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM video_info WHERE title IS NULL OR title = ''").Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		rows, err := rs.db.Query("SELECT bvid FROM video_info WHERE title IS NULL OR title = '' LIMIT 10")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var affectedBVids []string
		for rows.Next() {
			var bvid string
			if err := rows.Scan(&bvid); err != nil {
				continue
			}
			affectedBVids = append(affectedBVids, bvid)
		}
		issues = append(issues, Issue{
			Type:          ErrorTypeEmptyVideoTitle,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataValidation,
			Description:   fmt.Sprintf("发现 %d 个空标题视频", count),
			Count:         count,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: affectedBVids,
			Details:       fmt.Sprintf("影响视频数量: %d，示例BV号: %v", count, affectedBVids),
		})
	}
	return issues, nil
}

// 拆分：检查重复BVid
func (rs *RepairService) validateDuplicateBVids() ([]Issue, error) {
	var issues []Issue
	var duplicateCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT bvid, COUNT(*) as cnt 
			FROM video_info 
			GROUP BY bvid 
			HAVING cnt > 1
		)`).Scan(&duplicateCount)
	if err != nil {
		return nil, err
	}
	if duplicateCount > 0 {
		rows, err := rs.db.Query(`
			SELECT bvid FROM (
				SELECT bvid, COUNT(*) as cnt 
				FROM video_info 
				GROUP BY bvid 
				HAVING cnt > 1
			) LIMIT 10`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var affectedBVids []string
		for rows.Next() {
			var bvid string
			if err := rows.Scan(&bvid); err != nil {
				continue
			}
			affectedBVids = append(affectedBVids, bvid)
		}
		issues = append(issues, Issue{
			Type:          ErrorTypeDuplicateBvid,
			Severity:      ErrorLevelHigh,
			Level:         ErrorLevelHigh,
			Category:      ErrorTypeDataIntegrity,
			Description:   fmt.Sprintf("发现 %d 个重复BVid", duplicateCount),
			Count:         duplicateCount,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: affectedBVids,
			Details:       fmt.Sprintf("重复BV号数量: %d，示例BV号: %v", duplicateCount, affectedBVids),
		})
	}
	return issues, nil
}

// 拆分：检查视频存在但缺少评论数据
func (rs *RepairService) validateVideosMissingComments() ([]Issue, error) {
	var issues []Issue
	var missingCommentsCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM video_info v
		LEFT JOIN bilibili_comments c ON v.bvid = c.bvid
		WHERE c.bvid IS NULL`).Scan(&missingCommentsCount)
	if err != nil {
		return nil, err
	}
	if missingCommentsCount > 0 {
		rows, err := rs.db.Query(`
			SELECT v.bvid FROM video_info v
			LEFT JOIN bilibili_comments c ON v.bvid = c.bvid
			WHERE c.bvid IS NULL LIMIT 10`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var affectedBVids []string
		for rows.Next() {
			var bvid string
			if err := rows.Scan(&bvid); err != nil {
				continue
			}
			affectedBVids = append(affectedBVids, bvid)
		}
		issues = append(issues, Issue{
			Type:          ErrorTypeVideoMissingComments,
			Severity:      ErrorLevelHigh,
			Level:         ErrorLevelHigh,
			Category:      ErrorTypeDataConsistency,
			Description:   fmt.Sprintf("发现 %d 个视频缺少评论数据", missingCommentsCount),
			Count:         missingCommentsCount,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: affectedBVids,
			Details:       fmt.Sprintf("缺少评论的视频数量: %d，示例BV号: %v", missingCommentsCount, affectedBVids),
		})
	}
	return issues, nil
}

// 私有方法：校验评论表
func (rs *RepairService) validateCommentTable() ([]Issue, error) {
	var issues []Issue

	orphanIssues, err := rs.validateOrphanComments()
	if err != nil {
		return nil, err
	}
	issues = append(issues, orphanIssues...)

	duplicateIssues, err := rs.validateDuplicateComments()
	if err != nil {
		return nil, err
	}
	issues = append(issues, duplicateIssues...)

	emptyContentIssues, err := rs.validateEmptyCommentContent()
	if err != nil {
		return nil, err
	}
	issues = append(issues, emptyContentIssues...)

	invalidTimestampIssues, err := rs.validateInvalidTimestamps()
	if err != nil {
		return nil, err
	}
	issues = append(issues, invalidTimestampIssues...)

	return issues, nil
}

// 拆分：检查孤立评论
func (rs *RepairService) validateOrphanComments() ([]Issue, error) {
	var issues []Issue
	var orphanCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM bilibili_comments c
		LEFT JOIN video_info v ON c.bvid = v.bvid
		WHERE v.bvid IS NULL`).Scan(&orphanCount)
	if err != nil {
		return nil, err
	}
	if orphanCount > 0 {
		issues = append(issues, Issue{
			Type:        "orphan_comments",
			Severity:    "high",
			Description: fmt.Sprintf("发现 %d 条孤立评论（没有对应视频）", orphanCount),
			Count:       orphanCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查重复评论
func (rs *RepairService) validateDuplicateComments() ([]Issue, error) {
	var issues []Issue
	var duplicateCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT unique_id, COUNT(*) as cnt 
			FROM bilibili_comments 
			GROUP BY unique_id 
			HAVING cnt > 1
		)`).Scan(&duplicateCount)
	if err != nil {
		return nil, err
	}
	if duplicateCount > 0 {
		issues = append(issues, Issue{
			Type:        "duplicate_comments",
			Severity:    "medium",
			Description: fmt.Sprintf("发现 %d 条重复评论", duplicateCount),
			Count:       duplicateCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查空内容评论
func (rs *RepairService) validateEmptyCommentContent() ([]Issue, error) {
	var issues []Issue
	var emptyContentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE content IS NULL OR content = ''").Scan(&emptyContentCount)
	if err != nil {
		return nil, err
	}
	if emptyContentCount > 0 {
		issues = append(issues, Issue{
			Type:        "empty_comment_content",
			Severity:    "low",
			Description: fmt.Sprintf("发现 %d 条空内容评论", emptyContentCount),
			Count:       emptyContentCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查异常时间戳
func (rs *RepairService) validateInvalidTimestamps() ([]Issue, error) {
	var issues []Issue
	var invalidTimestampCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM bilibili_comments 
		WHERE ctime < 0 OR ctime > ?`, time.Now().Unix()+86400).Scan(&invalidTimestampCount)
	if err != nil {
		return nil, err
	}
	if invalidTimestampCount > 0 {
		issues = append(issues, Issue{
			Type:        "invalid_timestamp",
			Severity:    "medium",
			Description: fmt.Sprintf("发现 %d 条异常时间戳评论", invalidTimestampCount),
			Count:       invalidTimestampCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 私有方法：校验评论关系表
func (rs *RepairService) validateCommentRelations() ([]Issue, error) {
	var issues []Issue

	invalidParentIssues, err := rs.validateInvalidParentReferences()
	if err != nil {
		return nil, err
	}
	issues = append(issues, invalidParentIssues...)

	invalidChildIssues, err := rs.validateInvalidChildReferences()
	if err != nil {
		return nil, err
	}
	issues = append(issues, invalidChildIssues...)

	selfReferenceIssues, err := rs.validateSelfReferences()
	if err != nil {
		return nil, err
	}
	issues = append(issues, selfReferenceIssues...)

	parentNotExistIssues, err := rs.validateParentNotExist()
	if err != nil {
		return nil, err
	}
	issues = append(issues, parentNotExistIssues...)

	missingRelationIssues, err := rs.validateMissingCommentRelations()
	if err != nil {
		return nil, err
	}
	issues = append(issues, missingRelationIssues...)

	return issues, nil
}

// 拆分：检查无效父评论引用
func (rs *RepairService) validateInvalidParentReferences() ([]Issue, error) {
	var issues []Issue
	var invalidParentCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM comment_relations r
		LEFT JOIN bilibili_comments c ON r.parent_id = c.unique_id
		WHERE c.unique_id IS NULL`).Scan(&invalidParentCount)
	if err != nil {
		return nil, err
	}
	if invalidParentCount > 0 {
		issues = append(issues, Issue{
			Type:        "invalid_parent_reference",
			Severity:    "high",
			Description: fmt.Sprintf("发现 %d 个无效父评论引用", invalidParentCount),
			Count:       invalidParentCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查无效子评论引用
func (rs *RepairService) validateInvalidChildReferences() ([]Issue, error) {
	var issues []Issue
	var invalidChildCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM comment_relations r
		LEFT JOIN bilibili_comments c ON r.child_id = c.unique_id
		WHERE c.unique_id IS NULL`).Scan(&invalidChildCount)
	if err != nil {
		return nil, err
	}
	if invalidChildCount > 0 {
		issues = append(issues, Issue{
			Type:        "invalid_child_reference",
			Severity:    "high",
			Description: fmt.Sprintf("发现 %d 个无效子评论引用", invalidChildCount),
			Count:       invalidChildCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查自引用关系
func (rs *RepairService) validateSelfReferences() ([]Issue, error) {
	var issues []Issue
	var selfReferenceCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM comment_relations 
		WHERE parent_id = child_id`).Scan(&selfReferenceCount)
	if err != nil {
		return nil, err
	}
	if selfReferenceCount > 0 {
		issues = append(issues, Issue{
			Type:        "self_reference",
			Severity:    "medium",
			Description: fmt.Sprintf("发现 %d 个自引用关系", selfReferenceCount),
			Count:       selfReferenceCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查评论 parent 字段指向不存在的评论
func (rs *RepairService) validateParentNotExist() ([]Issue, error) {
	var issues []Issue
	var parentNotExistCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM bilibili_comments c
		WHERE c.parent != '0' AND c.parent NOT IN (SELECT unique_id FROM bilibili_comments)
	`).Scan(&parentNotExistCount)
	if err != nil {
		return nil, err
	}
	if parentNotExistCount > 0 {
		issues = append(issues, Issue{
			Type:        ErrorTypeParentNotExist,
			Severity:    ErrorLevelHigh,
			Description: fmt.Sprintf("发现 %d 条评论的 parent 字段指向不存在的父评论", parentNotExistCount),
			Count:       parentNotExistCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 拆分：检查评论关系缺失
func (rs *RepairService) validateMissingCommentRelations() ([]Issue, error) {
	var issues []Issue
	var missingRelationCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM bilibili_comments c
		WHERE c.parent != '0' AND c.parent IN (SELECT unique_id FROM bilibili_comments)
		AND NOT EXISTS (
			SELECT 1 FROM comment_relations r WHERE r.child_id = c.unique_id AND r.parent_id = c.parent
		)
	`).Scan(&missingRelationCount)
	if err != nil {
		return nil, err
	}
	if missingRelationCount > 0 {
		issues = append(issues, Issue{
			Type:        ErrorTypeMissingCommentRelations,
			Severity:    ErrorLevelHigh,
			Description: fmt.Sprintf("发现 %d 条评论缺失父子关系（parent 存在但无关系）", missingRelationCount),
			Count:       missingRelationCount,
			Fixable:     true,
			Fixed:       false,
		})
	}
	return issues, nil
}

// 私有方法：校验评论统计表
func (rs *RepairService) validateCommentStats() ([]Issue, error) {
	var issues []Issue

	inconsistentStatsIssues, err := rs.validateInconsistentStats()
	if err != nil {
		return nil, err
	}
	issues = append(issues, inconsistentStatsIssues...)

	missingStatsIssues, err := rs.validateMissingStats()
	if err != nil {
		return nil, err
	}
	issues = append(issues, missingStatsIssues...)

	return issues, nil
}

// 拆分：检查统计不一致
func (rs *RepairService) validateInconsistentStats() ([]Issue, error) {
	var issues []Issue
	var inconsistentCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM comment_stats s
		LEFT JOIN (
			SELECT bvid, COUNT(*) as actual_count 
			FROM bilibili_comments 
			GROUP BY bvid
		) c ON s.bvid = c.bvid
		WHERE s.comment_count != c.actual_count OR c.actual_count IS NULL`).Scan(&inconsistentCount)
	if err != nil {
		return nil, err
	}
	if inconsistentCount > 0 {
		rows, err := rs.db.Query(`
			SELECT s.bvid FROM comment_stats s
			LEFT JOIN (
				SELECT bvid, COUNT(*) as actual_count 
				FROM bilibili_comments 
				GROUP BY bvid
			) c ON s.bvid = c.bvid
			WHERE s.comment_count != c.actual_count OR c.actual_count IS NULL LIMIT 10`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var affectedBVids []string
		for rows.Next() {
			var bvid string
			if err := rows.Scan(&bvid); err != nil {
				continue
			}
			affectedBVids = append(affectedBVids, bvid)
		}
		issues = append(issues, Issue{
			Type:          ErrorTypeInconsistentStats,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataConsistency,
			Description:   fmt.Sprintf("发现 %d 个统计不一致", inconsistentCount),
			Count:         inconsistentCount,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: affectedBVids,
			Details:       fmt.Sprintf("统计不一致的视频数量: %d，示例BV号: %v", inconsistentCount, affectedBVids),
		})
	}
	return issues, nil
}

// 拆分：检查缺失统计
func (rs *RepairService) validateMissingStats() ([]Issue, error) {
	var issues []Issue
	var missingStatsCount int
	err := rs.db.QueryRow(`
		SELECT COUNT(*) FROM video_info v
		LEFT JOIN comment_stats s ON v.bvid = s.bvid
		WHERE s.bvid IS NULL`).Scan(&missingStatsCount)
	if err != nil {
		return nil, err
	}
	if missingStatsCount > 0 {
		rows, err := rs.db.Query(`
			SELECT v.bvid FROM video_info v
			LEFT JOIN comment_stats s ON v.bvid = s.bvid
			WHERE s.bvid IS NULL LIMIT 10`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var affectedBVids []string
		for rows.Next() {
			var bvid string
			if err := rows.Scan(&bvid); err != nil {
				continue
			}
			affectedBVids = append(affectedBVids, bvid)
		}
		issues = append(issues, Issue{
			Type:          ErrorTypeMissingStats,
			Severity:      ErrorLevelLow,
			Level:         ErrorLevelLow,
			Category:      ErrorTypeDataConsistency,
			Description:   fmt.Sprintf("发现 %d 个缺失统计", missingStatsCount),
			Count:         missingStatsCount,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: affectedBVids,
			Details:       fmt.Sprintf("缺失统计的视频数量: %d，示例BV号: %v", missingStatsCount, affectedBVids),
		})
	}
	return issues, nil
}

// 私有方法：计算统计信息
func (rs *RepairService) calculateSummary() (*struct {
	TotalVideos     int `json:"total_videos"`
	TotalComments   int `json:"total_comments"`
	IssuesFound     int `json:"issues_found"`
	IssuesFixed     int `json:"issues_fixed"`
	IssuesUnfixable int `json:"issues_unfixable"`
}, error) {
	summary := &struct {
		TotalVideos     int `json:"total_videos"`
		TotalComments   int `json:"total_comments"`
		IssuesFound     int `json:"issues_found"`
		IssuesFixed     int `json:"issues_fixed"`
		IssuesUnfixable int `json:"issues_unfixable"`
	}{}

	// 获取视频总数
	err := rs.db.QueryRow("SELECT COUNT(*) FROM video_info").Scan(&summary.TotalVideos)
	if err != nil {
		return nil, err
	}

	// 获取评论总数
	err = rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments").Scan(&summary.TotalComments)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

// 私有方法：检查视频是否存在
func (rs *RepairService) checkVideoExists(bvid string) (bool, error) {
	var exists int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM video_info WHERE bvid = ?", bvid).Scan(&exists)
	return exists > 0, err
}

// 私有方法：校验视频评论
func (rs *RepairService) validateVideoComments(bvid string) ([]Issue, error) {
	var issues []Issue

	// 检查该视频是否有评论数据
	var commentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&commentCount)
	if err != nil {
		return nil, err
	}

	// 如果视频没有评论数据，添加问题
	if commentCount == 0 {
		issues = append(issues, Issue{
			Type:          ErrorTypeVideoMissingComments,
			Severity:      ErrorLevelHigh,
			Level:         ErrorLevelHigh,
			Category:      ErrorTypeDataConsistency,
			Description:   "视频缺少评论数据",
			Count:         1,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 缺少评论数据", bvid),
		})
	}

	// 检查该视频的评论统计
	var statsCount int
	err = rs.db.QueryRow("SELECT comment_count FROM comment_stats WHERE bvid = ?", bvid).Scan(&statsCount)
	if err == sql.ErrNoRows {
		// 没有统计记录
		issues = append(issues, Issue{
			Type:          ErrorTypeMissingStats,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataConsistency,
			Description:   "缺少评论统计信息",
			Count:         1,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 缺少评论统计信息", bvid),
		})
	} else if err != nil {
		return nil, err
	} else if statsCount != commentCount {
		// 统计不一致
		issues = append(issues, Issue{
			Type:          ErrorTypeInconsistentStats,
			Severity:      ErrorLevelMedium,
			Level:         ErrorLevelMedium,
			Category:      ErrorTypeDataConsistency,
			Description:   fmt.Sprintf("评论统计不一致（实际: %d, 统计: %d）", commentCount, statsCount),
			Count:         1,
			Fixable:       true,
			Fixed:         false,
			AffectedBVids: []string{bvid},
			Details:       fmt.Sprintf("视频 %s 评论统计不一致（实际: %d, 统计: %d）", bvid, commentCount, statsCount),
		})
	}

	return issues, nil
}

// 私有方法：计算视频统计信息
func (rs *RepairService) calculateVideoSummary(bvid string) (*ValidationResult, error) {
	summary := &ValidationResult{}

	// 获取该视频的评论数
	var commentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&commentCount)
	if err != nil {
		return nil, err
	}

	summary.Summary.TotalVideos = 1
	summary.Summary.TotalComments = commentCount

	return summary, nil
}

// 私有方法：修复问题
func (rs *RepairService) fixIssue(issue *Issue) error {
	switch issue.Type {
	case ErrorTypeEmptyVideoTitle:
		return rs.fixEmptyVideoTitles()
	case ErrorTypeDuplicateBvid:
		return rs.fixDuplicateBVids()
	case ErrorTypeVideoMissingComments:
		return rs.fixVideoMissingComments()
	case ErrorTypeOrphanComments:
		return rs.fixOrphanComments()
	case ErrorTypeDuplicateComments:
		return rs.fixDuplicateComments()
	case ErrorTypeEmptyCommentContent:
		return rs.fixEmptyCommentContent()
	case ErrorTypeInvalidTimestamp:
		return rs.fixInvalidTimestamps()
	case ErrorTypeInvalidParentRef:
		return rs.fixInvalidParentReferences()
	case ErrorTypeInvalidChildRef:
		return rs.fixInvalidChildReferences()
	case ErrorTypeSelfReference:
		return rs.fixSelfReferences()
	case ErrorTypeInconsistentStats:
		return rs.fixInconsistentStats()
	case ErrorTypeMissingStats:
		return rs.fixMissingStats()
	case ErrorTypeMissingCommentRelations:
		return rs.fixAllCommentRelations()
	case ErrorTypeParentNotExist:
		return rs.fixParentNotExist()
	default:
		return fmt.Errorf("未知的问题类型: %s", issue.Type)
	}
}

// 私有方法：修复视频问题
func (rs *RepairService) fixVideoIssue(bvid string, issue *Issue) error {
	switch issue.Type {
	case ErrorTypeVideoMissingComments:
		return rs.fixVideoMissingCommentsSingle(bvid)
	case ErrorTypeMissingStats:
		return rs.fixVideoMissingStats(bvid)
	case ErrorTypeInconsistentStats:
		return rs.fixVideoInconsistentStats(bvid)
	default:
		return fmt.Errorf("未知的视频问题类型: %s", issue.Type)
	}
}

// 修复空视频标题
func (rs *RepairService) fixEmptyVideoTitles() error {
	_, err := rs.db.Exec("UPDATE video_info SET title = '未知标题' WHERE title IS NULL OR title = ''")
	return err
}

// 修复重复BVid
func (rs *RepairService) fixDuplicateBVids() error {
	_, err := rs.db.Exec(`
		DELETE FROM video_info 
		WHERE rowid NOT IN (
			SELECT MIN(rowid) 
			FROM video_info 
			GROUP BY bvid
		)`)
	return err
}

// 修复视频缺少评论数据
func (rs *RepairService) fixVideoMissingComments() error {
	// 删除在video_info表中存在但在其他表中没有相关数据的视频
	_, err := rs.db.Exec(`
		DELETE FROM video_info 
		WHERE bvid NOT IN (
			SELECT DISTINCT bvid FROM bilibili_comments
		) AND bvid NOT IN (
			SELECT DISTINCT bvid FROM comment_stats
		)`)
	return err
}

// 修复孤立评论
func (rs *RepairService) fixOrphanComments() error {
	_, err := rs.db.Exec(`
		DELETE FROM bilibili_comments 
		WHERE bvid NOT IN (SELECT bvid FROM video_info)`)
	return err
}

// 修复重复评论
func (rs *RepairService) fixDuplicateComments() error {
	_, err := rs.db.Exec(`
		DELETE FROM bilibili_comments 
		WHERE rowid NOT IN (
			SELECT MIN(rowid) 
			FROM bilibili_comments 
			GROUP BY unique_id
		)`)
	return err
}

// 修复空评论内容
func (rs *RepairService) fixEmptyCommentContent() error {
	_, err := rs.db.Exec("UPDATE bilibili_comments SET content = '[内容已删除]' WHERE content IS NULL OR content = ''")
	return err
}

// 修复异常时间戳
func (rs *RepairService) fixInvalidTimestamps() error {
	now := time.Now().Unix()
	_, err := rs.db.Exec(`
		UPDATE bilibili_comments 
		SET ctime = ? 
		WHERE ctime < 0 OR ctime > ?`, now, now+86400)
	return err
}

// 修复无效父评论引用
func (rs *RepairService) fixInvalidParentReferences() error {
	_, err := rs.db.Exec(`
		DELETE FROM comment_relations 
		WHERE parent_id NOT IN (SELECT unique_id FROM bilibili_comments)`)
	return err
}

// 修复无效子评论引用
func (rs *RepairService) fixInvalidChildReferences() error {
	_, err := rs.db.Exec(`
		DELETE FROM comment_relations 
		WHERE child_id NOT IN (SELECT unique_id FROM bilibili_comments)`)
	return err
}

// 修复自引用关系
func (rs *RepairService) fixSelfReferences() error {
	_, err := rs.db.Exec("DELETE FROM comment_relations WHERE parent_id = child_id")
	return err
}

// 修复统计不一致
func (rs *RepairService) fixInconsistentStats() error {
	_, err := rs.db.Exec(`
		UPDATE comment_stats 
		SET comment_count = (
			SELECT COUNT(*) 
			FROM bilibili_comments 
			WHERE bilibili_comments.bvid = comment_stats.bvid
		)`)
	return err
}

// 修复缺失统计
func (rs *RepairService) fixMissingStats() error {
	_, err := rs.db.Exec(`
		INSERT INTO comment_stats (bvid, comment_count)
		SELECT v.bvid, COALESCE(c.comment_count, 0)
		FROM video_info v
		LEFT JOIN (
			SELECT bvid, COUNT(*) as comment_count
			FROM bilibili_comments
			GROUP BY bvid
		) c ON v.bvid = c.bvid
		WHERE NOT EXISTS (
			SELECT 1 FROM comment_stats s WHERE s.bvid = v.bvid
		)`)
	return err
}

// 修复视频缺失统计
func (rs *RepairService) fixVideoMissingStats(bvid string) error {
	var commentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&commentCount)
	if err != nil {
		return err
	}

	_, err = rs.db.Exec(`
		INSERT OR REPLACE INTO comment_stats (bvid, comment_count)
		VALUES (?, ?)`, bvid, commentCount)
	return err
}

// 修复视频统计不一致
func (rs *RepairService) fixVideoInconsistentStats(bvid string) error {
	var commentCount int
	err := rs.db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&commentCount)
	if err != nil {
		return err
	}

	_, err = rs.db.Exec("UPDATE comment_stats SET comment_count = ? WHERE bvid = ?", commentCount, bvid)
	return err
}

// 修复单个视频缺少评论数据
func (rs *RepairService) fixVideoMissingCommentsSingle(bvid string) error {
	// 新增：尝试自动爬取评论并导入数据库
	log := logger.GetLogger()
	log.Infof("尝试自动爬取并导入评论: %s", bvid)
	if err := CrawlAndImport(context.Background(), bvid); err != nil {
		log.Errorf("自动爬取评论失败: %v，执行兜底删除", err)
		// 兜底：如爬取失败，删除该视频记录
		_, delErr := rs.db.Exec("DELETE FROM video_info WHERE bvid = ?", bvid)
		if delErr != nil {
			return fmt.Errorf("评论爬取失败且删除视频记录失败: %v, %v", err, delErr)
		}
		return fmt.Errorf("评论爬取失败，已删除视频记录: %v", err)
	}
	log.Infof("评论爬取并导入成功: %s", bvid)
	return nil
}

// 修复 parent_not_exist：为 parent 字段指向不存在评论的评论插入占位父评论
func (rs *RepairService) fixParentNotExist() error {
	// 查询所有 parent 字段指向不存在评论的 unique_id 及其子评论的 bvid
	rows, err := rs.db.Query(`
		SELECT DISTINCT c.parent, c.bvid FROM bilibili_comments c
		WHERE c.parent != '0' AND c.parent NOT IN (SELECT unique_id FROM bilibili_comments)
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type parentInfo struct{ parentID, bvid string }
	var missingParents []parentInfo
	for rows.Next() {
		var parentID, bvid string
		if err := rows.Scan(&parentID, &bvid); err != nil {
			return err
		}
		missingParents = append(missingParents, parentInfo{parentID, bvid})
	}

	for _, info := range missingParents {
		// 插入占位评论，bvid 填充为子评论的 bvid
		_, err := rs.db.Exec(`
			INSERT INTO bilibili_comments (
				unique_id, bvid, rpid, content, pictures, oid, mid, parent, fans_grade, ctime, like_count, upname, sex, following, level, location
			) VALUES (?, ?, 0, '[该评论内容缺失]', '', 0, 0, '0', 0, ?, 0, '', '', 0, 0, '')
			ON CONFLICT(unique_id) DO NOTHING
		`, info.parentID, info.bvid, time.Now().Unix())
		if err != nil {
			return err
		}
	}
	return nil
}

// fixIssue 拆分：全量重建所有视频的评论关系
func (rs *RepairService) fixAllCommentRelations() error {
	videos, _, err := database.GetVideosPaginated(1, 1000000, "")
	if err != nil {
		return err
	}
	for _, v := range videos {
		if err := database.RebuildAllCommentRelations(v.BVid); err != nil {
			return err
		}
	}
	return nil
}
```

<!-- [END OF FILE: repair_service.go] -->

### crawler/blblcd/core/comment.go
<!-- [START OF FILE: comment.go] -->
```go
package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Errorf("====爬取主评论,oid:%s，第%d页失败=====", oid, next)
			logger.GetLogger().Error(r)
			err = fmt.Errorf("爬取评论时发生panic: %v", r)
		}
	}()

	// +++ 添加详细日志 +++
	logger.GetLogger().Debugf("请求评论API: oid=%s, page=%d, offset=%s", oid, next, offsetStr)
	client := resty.New()
	client.SetTimeout(15 * time.Second) // 增加超时时间

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
		if err := recover(); err != nil {
			logger.GetLogger().Errorf("xxxxx爬取子评论,oid:%s，第%d页失败xxxxx", oid, next)
			logger.GetLogger().Error(err)
		}
	}()

	logger.GetLogger().Debugf("请求子评论API: oid=%s, rpid=%d, page=%d", oid, rpid, next)

	client := http.Client{
		Timeout: 15 * time.Second, // 增加超时时间
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
```

<!-- [END OF FILE: comment.go] -->

### crawler/blblcd/crawler.go
<!-- [START OF FILE: crawler.go] -->
```go
// blblcd 包提供了 B 站评论爬取相关的核心功能
package blblcd

import (
	"context"
	"sync"

	"bilibili-comments-viewer-go/crawler/blblcd/core"
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
)

// CrawlVideo 爬取指定 bvid 视频的所有评论
// ctx: 上下文，用于控制取消等
// bvid: 视频的 BVID
// opt: 爬取选项，包括并发数等
// 返回值: 评论列表和错误信息
func CrawlVideo(ctx context.Context, bvid string, opt *model.Option) ([]model.Comment, error) {
	// 记录开始爬取日志
	logger.GetLogger().Infof("开始爬取视频: bvid=%s", bvid)

	// 用于收集评论的通道，带缓冲区
	resultChan := make(chan model.Comment, 1000)
	var comments []model.Comment
	// 控制并发的信号量，容量为 opt.Workers
	sem := make(chan struct{}, opt.Workers)

	// 创建独立的 WaitGroup 用于等待 FindComment 完成
	var findWg sync.WaitGroup
	findWg.Add(1)

	// 启动 goroutine 进行评论查找
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.GetLogger().Errorf("[panic] CrawlVideo goroutine: %v", r)
			}
			findWg.Done()
		}()
		// 将 bvid 转换为 avid
		avid := core.Bvid2Avid(bvid)
		// 调用核心查找评论逻辑，wg 传 nil 由内部管理
		core.FindComment(ctx, sem, nil, int(avid), opt, resultChan)
	}()

	// 等待评论查找完成
	findWg.Wait()

	// 关闭通道并收集所有评论
	close(resultChan)
	for comment := range resultChan {
		comments = append(comments, comment)
	}

	// 记录爬取完成日志
	logger.GetLogger().Infof("视频 %s 爬取完成, 共获取 %d 条评论", bvid, len(comments))

	return comments, nil
}

// CrawlUp 爬取指定 up 主（用户）的所有视频评论
// mid: up 主的 mid
// opt: 爬取选项
// 返回值: 错误信息
func CrawlUp(mid int, opt *model.Option) error {
	// 控制并发的信号量，容量为 opt.Workers
	sem := make(chan struct{}, opt.Workers)
	// 设置 up 主 mid
	opt.Mid = mid
	// 调用核心查找 up 主视频评论逻辑
	core.FindUser(sem, opt)
	return nil
}
```

<!-- [END OF FILE: crawler.go] -->

### crawler/blblcd/core/video.go
<!-- [START OF FILE: video.go] -->
```go
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
```

<!-- [END OF FILE: video.go] -->

### crawler/blblcd/model/pagination.go
<!-- [START OF FILE: pagination.go] -->
```go
package model

type PaginationOffsetData struct {
	Pn int `json:"pn"`
}

type PaginationOffset struct {
	Type      int                  `json:"type"`
	Direction int                  `json:"direction"`
	Data      PaginationOffsetData `json:"data"`
	SessionId string               `json:"session_id"`
}

type Pagination struct {
	Offset string `json:"offset"`
}
```

<!-- [END OF FILE: pagination.go] -->

### crawler/blblcd/cli/video.go
<!-- [START OF FILE: video.go] -->
```go
package cli

import (
	"context"
	"fmt"
	"sync"

	"bilibili-comments-viewer-go/crawler/blblcd/core"
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/utils"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(videoCmd)
}

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "鑾峰彇瑙嗛璇勮锛屾敮鎸佸崟涓拰澶氫釜瑙嗛",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("please provide bvid")
			return
		}
		cookie, err := utils.ReadTextFile(cookieFile)
		if err != nil {
			fmt.Println(err)
			return
		}

		for i := range args {
			bvid := args[i]
			opt := model.Option{
				Bvid:          bvid,
				Corder:        corder,
				Cookie:        cookie,
				Output:        output,
				ImgDownload:   imgDownload,
				MaxTryCount:   maxTryCount,
				CommentOutput: commentOutput,
				ImageOutput:   imageOutput,
			}
			fmt.Printf("bvid: %s\n", bvid)
			sem := make(chan struct{}, workers)
			var wg sync.WaitGroup
			commentChan := make(chan model.Comment, 1000)
			wg.Add(1)
			go core.FindComment(context.Background(), sem, &wg, int(core.Bvid2Avid(bvid)), &opt, commentChan)
			wg.Wait()
			close(commentChan)
		}

	},
}
```

<!-- [END OF FILE: video.go] -->

### backend/types.go
<!-- [START OF FILE: types.go] -->
```go
package backend

import (
	"time"
)

// 保存模式常量
const (
	SaveModeCSVOnly  = "csv_only"
	SaveModeDBOnly   = "db_only"
	SaveModeCSVAndDB = "csv_and_db"
)

// 错误级别常量
const (
	ErrorLevelCritical = "critical" // 严重错误：影响系统正常运行
	ErrorLevelHigh     = "high"     // 高级错误：影响数据完整性
	ErrorLevelMedium   = "medium"   // 中级错误：影响数据质量
	ErrorLevelLow      = "low"      // 低级错误：轻微问题
	ErrorLevelInfo     = "info"     // 信息：提示性信息
)

// 错误类型分类
const (
	// 数据完整性错误
	ErrorTypeDataIntegrity    = "data_integrity"    // 数据完整性
	ErrorTypeDataConsistency  = "data_consistency"  // 数据一致性
	ErrorTypeDataValidation   = "data_validation"   // 数据验证
	ErrorTypeDataRelationship = "data_relationship" // 数据关系

	// 系统错误
	ErrorTypeSystemError   = "system_error"   // 系统错误
	ErrorTypeNetworkError  = "network_error"  // 网络错误
	ErrorTypeDatabaseError = "database_error" // 数据库错误
	ErrorTypeAPIError      = "api_error"      // API错误

	// 业务逻辑错误
	ErrorTypeBusinessLogic = "business_logic" // 业务逻辑
	ErrorTypeUserInput     = "user_input"     // 用户输入
	ErrorTypeConfiguration = "configuration"  // 配置错误
)

// 具体错误类型
const (
	// 视频相关错误
	ErrorTypeVideoNotFound        = "video_not_found"        // 视频不存在
	ErrorTypeEmptyVideoTitle      = "empty_video_title"      // 空视频标题
	ErrorTypeDuplicateBvid        = "duplicate_bvid"         // 重复BV号
	ErrorTypeVideoMissingComments = "video_missing_comments" // 视频缺少评论

	// 评论相关错误
	ErrorTypeOrphanComments      = "orphan_comments"       // 孤立评论
	ErrorTypeDuplicateComments   = "duplicate_comments"    // 重复评论
	ErrorTypeEmptyCommentContent = "empty_comment_content" // 空评论内容
	ErrorTypeInvalidTimestamp    = "invalid_timestamp"     // 异常时间戳

	// 关系相关错误
	ErrorTypeInvalidParentRef        = "invalid_parent_reference"  // 无效父评论引用
	ErrorTypeInvalidChildRef         = "invalid_child_reference"   // 无效子评论引用
	ErrorTypeSelfReference           = "self_reference"            // 自引用关系
	ErrorTypeMissingCommentRelations = "missing_comment_relations" // 评论关系缺失
	ErrorTypeParentNotExist          = "parent_not_exist"          // 父评论不存在（子评论指向不存在的父评论）

	// 统计相关错误
	ErrorTypeInconsistentStats = "inconsistent_stats" // 统计不一致
	ErrorTypeMissingStats      = "missing_stats"      // 缺失统计
)

// CrawlerError 爬虫错误
type CrawlerError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Level     string `json:"level"`
	Category  string `json:"category"`
	Details   string `json:"details,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Fixable   bool   `json:"fixable"`
	Retryable bool   `json:"retryable"`
}

func (e CrawlerError) Error() string {
	return e.Message
}

// NewCrawlerError 创建新的爬虫错误
func NewCrawlerError(message, errorType, level, category string) *CrawlerError {
	return &CrawlerError{
		Message:   message,
		Type:      errorType,
		Level:     level,
		Category:  category,
		Timestamp: time.Now().Unix(),
		Fixable:   isErrorFixable(errorType),
		Retryable: isErrorRetryable(errorType),
	}
}

// NewCrawlerErrorWithDetails 创建带详细信息的爬虫错误
func NewCrawlerErrorWithDetails(message, errorType, level, category, details string) *CrawlerError {
	return &CrawlerError{
		Message:   message,
		Type:      errorType,
		Level:     level,
		Category:  category,
		Details:   details,
		Timestamp: time.Now().Unix(),
		Fixable:   isErrorFixable(errorType),
		Retryable: isErrorRetryable(errorType),
	}
}

// isErrorFixable 判断错误是否可修复
func isErrorFixable(errorType string) bool {
	fixableErrors := map[string]bool{
		ErrorTypeEmptyVideoTitle:         true,
		ErrorTypeDuplicateBvid:           true,
		ErrorTypeVideoMissingComments:    true,
		ErrorTypeOrphanComments:          true,
		ErrorTypeDuplicateComments:       true,
		ErrorTypeEmptyCommentContent:     true,
		ErrorTypeInvalidTimestamp:        true,
		ErrorTypeInvalidParentRef:        true,
		ErrorTypeInvalidChildRef:         true,
		ErrorTypeSelfReference:           true,
		ErrorTypeInconsistentStats:       true,
		ErrorTypeMissingStats:            true,
		ErrorTypeMissingCommentRelations: true,
		ErrorTypeParentNotExist:          true,
		ErrorTypeVideoNotFound:           false, // 视频不存在无法修复
	}

	if fixable, exists := fixableErrors[errorType]; exists {
		return fixable
	}
	return false
}

// isErrorRetryable 判断错误是否可重试
func isErrorRetryable(errorType string) bool {
	retryableErrors := map[string]bool{
		ErrorTypeNetworkError:    true,
		ErrorTypeAPIError:        true,
		ErrorTypeSystemError:     true,
		ErrorTypeDatabaseError:   true,
		ErrorTypeDataValidation:  false,
		ErrorTypeDataIntegrity:   false,
		ErrorTypeDataConsistency: false,
		ErrorTypeBusinessLogic:   false,
		ErrorTypeUserInput:       false,
		ErrorTypeConfiguration:   false,
	}

	if retryable, exists := retryableErrors[errorType]; exists {
		return retryable
	}
	return false
}

// GetErrorLevelName 获取错误级别中文名称
func GetErrorLevelName(level string) string {
	levelNames := map[string]string{
		ErrorLevelCritical: "严重",
		ErrorLevelHigh:     "高",
		ErrorLevelMedium:   "中",
		ErrorLevelLow:      "低",
		ErrorLevelInfo:     "信息",
	}

	if name, exists := levelNames[level]; exists {
		return name
	}
	return level
}

// GetErrorTypeName 获取错误类型中文名称
func GetErrorTypeName(errorType string) string {
	typeNames := map[string]string{
		ErrorTypeVideoNotFound:           "视频不存在",
		ErrorTypeEmptyVideoTitle:         "空视频标题",
		ErrorTypeDuplicateBvid:           "重复BV号",
		ErrorTypeVideoMissingComments:    "视频缺少评论数据",
		ErrorTypeOrphanComments:          "孤立评论",
		ErrorTypeDuplicateComments:       "重复评论",
		ErrorTypeEmptyCommentContent:     "空评论内容",
		ErrorTypeInvalidTimestamp:        "异常时间戳",
		ErrorTypeInvalidParentRef:        "无效父评论引用",
		ErrorTypeInvalidChildRef:         "无效子评论引用",
		ErrorTypeSelfReference:           "自引用关系",
		ErrorTypeInconsistentStats:       "统计不一致",
		ErrorTypeMissingStats:            "缺失统计",
		ErrorTypeMissingCommentRelations: "评论关系缺失",
		ErrorTypeParentNotExist:          "父评论不存在（子评论指向不存在的父评论）",
		ErrorTypeDataIntegrity:           "数据完整性错误",
		ErrorTypeDataConsistency:         "数据一致性错误",
		ErrorTypeDataValidation:          "数据验证错误",
		ErrorTypeDataRelationship:        "数据关系错误",
		ErrorTypeSystemError:             "系统错误",
		ErrorTypeNetworkError:            "网络错误",
		ErrorTypeDatabaseError:           "数据库错误",
		ErrorTypeAPIError:                "API错误",
		ErrorTypeBusinessLogic:           "业务逻辑错误",
		ErrorTypeUserInput:               "用户输入错误",
		ErrorTypeConfiguration:           "配置错误",
	}

	if name, exists := typeNames[errorType]; exists {
		return name
	}
	return errorType
}

// GetErrorCategoryName 获取错误分类中文名称
func GetErrorCategoryName(category string) string {
	categoryNames := map[string]string{
		ErrorTypeDataIntegrity:    "数据完整性",
		ErrorTypeDataConsistency:  "数据一致性",
		ErrorTypeDataValidation:   "数据验证",
		ErrorTypeDataRelationship: "数据关系",
		ErrorTypeSystemError:      "系统错误",
		ErrorTypeNetworkError:     "网络错误",
		ErrorTypeDatabaseError:    "数据库错误",
		ErrorTypeAPIError:         "API错误",
		ErrorTypeBusinessLogic:    "业务逻辑",
		ErrorTypeUserInput:        "用户输入",
		ErrorTypeConfiguration:    "配置错误",
	}

	if name, exists := categoryNames[category]; exists {
		return name
	}
	return category
}
```

<!-- [END OF FILE: types.go] -->

### backend/comment_processor.go
<!-- [START OF FILE: comment_processor.go] -->
```go
package backend

import (
	"context"
	"path/filepath"

	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/blblcd"
	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/logger"
	"bilibili-comments-viewer-go/utils"
)

func crawlVideoComments(ctx context.Context, bvid string) ([]blblcdmodel.Comment, error) {
	cfg := config.Get()
	log := logger.GetLogger()

	// +++ 添加详细日志 +++
	log.Infof("开始爬取视频评论: bvid=%s", bvid)

	opt := &blblcdmodel.Option{
		Cookie:        utils.ReadCookie(cfg.Crawler.CookieFile),
		Bvid:          bvid,
		Output:        cfg.Crawler.OutputDir,
		Workers:       cfg.Crawler.Workers,
		MaxTryCount:   cfg.Crawler.MaxTryCount,
		DelayBaseMs:   cfg.Crawler.DelayBaseMs,
		DelayJitterMs: cfg.Crawler.DelayJitterMs,
	}

	// +++ 记录爬虫配置 +++
	log.Debugf("爬虫配置: workers=%d, maxTryCount=%d", opt.Workers, opt.MaxTryCount)

	comments, err := blblcd.CrawlVideo(ctx, bvid, opt)
	if err != nil {
		log.Errorf("爬取视频评论失败: %v", err)
	} else {
		log.Infof("成功爬取 %d 条评论 (bvid: %s)", len(comments), bvid)
	}

	return comments, err
}

func processCSVOnly(bvid string, comments []blblcdmodel.Comment) {
	cfg := config.Get()
	csvPath := filepath.Join(cfg.Crawler.OutputDir, bvid, bvid+".csv")
	if err := saveCommentsToCSV(comments, csvPath); err != nil {
		logger.GetLogger().Errorf("保存CSV失败: %v", err)
	} else {
		logger.GetLogger().Infof("CSV保存成功: %s", csvPath)
	}
}

func processCSVAndDB(bvid string, comments []blblcdmodel.Comment) {
	cfg := config.Get()
	csvPath := filepath.Join(cfg.Crawler.OutputDir, bvid, bvid+".csv")
	if err := saveCommentsToCSV(comments, csvPath); err != nil {
		logger.GetLogger().Errorf("保存CSV失败: %v", err)
	} else {
		logger.GetLogger().Infof("CSV保存成功: %s", csvPath)
	}

	if err := ImportCommentsFromCSV(bvid, csvPath); err != nil {
		logger.GetLogger().Errorf("导入数据库失败: %v", err)
	}
}

func processCSVFiles() {
	cfg := config.Get()
	files, err := filepath.Glob(filepath.Join(cfg.Crawler.OutputDir, "*/*.csv"))
	if err != nil {
		logger.GetLogger().Infof("查找CSV文件失败: %v", err)
		return
	}

	for _, file := range files {
		bvid := filepath.Base(filepath.Dir(file))
		logger.GetLogger().Infof("开始导入CSV文件: %s, BV: %s", file, bvid)
		if err := ImportCommentsFromCSV(bvid, file); err != nil {
			logger.GetLogger().Errorf("导入CSV文件失败: %v", err)
		} else {
			logger.GetLogger().Infof("成功导入CSV文件: %s", file)
		}
	}
}
```

<!-- [END OF FILE: comment_processor.go] -->

### crawler/blblcd/model/comment.go
<!-- [START OF FILE: comment.go] -->
```go
package model

type CommentsCountResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TTL     int    `json:"ttl"`
	Data    struct {
		Count int `json:"count"`
	} `json:"data"`
}

type Cursor struct {
	IsBegin         bool `json:"is_begin"`
	Prev            int  `json:"prev"`
	Next            int  `json:"next"`
	IsEnd           bool `json:"is_end"`
	PaginationReply struct {
		NextOffset string `json:"next_offset"`
	} `json:"pagination_reply"`
	SessionID   string `json:"session_id"`
	Mode        int    `json:"mode"`
	ModeText    string `json:"mode_text"`
	AllCount    int    `json:"all_count"`
	SupportMode []int  `json:"support_mode"`
	Name        string `json:"name"`
}
type Comment struct {
    Uname         string    //姓名
    Sex           string    //性别
    Content       string    //评论内容
    Rpid          int64     //评论id
    Oid           int       //评论区id
    Bvid          string    //视频bv
    Mid           int       //发送者id
    Parent        int       //父评论ID
    Fansgrade     int       //是否粉丝标签
    Ctime         int       //评论时间戳
    Like          int       //喜欢数
    Following     bool      //是否关注
    Current_level int       //当前等级
    Location      string    //位置
    Pictures      []Picture // 图片
    Replies       []ReplyItem // 添加回复字段
}

type Picture struct {
	Img_src string `json:"img_src"`
}
type ReplyItem struct {
	Rpid      int64  `json:"rpid"`
	Oid       int    `json:"oid"`
	Type      int    `json:"type"`
	Mid       int    `json:"mid"`
	Root      int    `json:"root"`
	Parent    int    `json:"parent"`
	Dialog    int    `json:"dialog"`
	Count     int    `json:"count"`
	Rcount    int    `json:"rcount"`
	State     int    `json:"state"`
	Fansgrade int    `json:"fansgrade"`
	Attr      int    `json:"attr"`
	Ctime     int    `json:"ctime"`
	MidStr    string `json:"mid_str"`
	OidStr    string `json:"oid_str"`
	RpidStr   string `json:"rpid_str"`
	Like      int    `json:"like"`
	Action    int    `json:"action"`
	Member    struct {
		Mid            string `json:"mid"`
		Uname          string `json:"uname"`
		Sex            string `json:"sex"`
		Sign           string `json:"sign"`
		Avatar         string `json:"avatar"`
		Rank           string `json:"rank"`
		FaceNftNew     int    `json:"face_nft_new"`
		IsSeniorMember int    `json:"is_senior_member"`
		LevelInfo      struct {
			CurrentLevel int `json:"current_level"`
			CurrentMin   int `json:"current_min"`
			CurrentExp   int `json:"current_exp"`
			NextExp      int `json:"next_exp"`
		} `json:"level_info"`
		Vip struct {
			VipType       int    `json:"vipType"`
			VipDueDate    int64  `json:"vipDueDate"`
			DueRemark     string `json:"dueRemark"`
			AccessStatus  int    `json:"accessStatus"`
			VipStatus     int    `json:"vipStatus"`
			VipStatusWarn string `json:"vipStatusWarn"`
		} `json:"vip"`
		FansDetail any `json:"fans_detail"`
	} `json:"member"`
	Content struct {
		Message  string    `json:"message"`
		Pictures []Picture `json:"pictures"`
		Members  []any     `json:"members"`
		Emote    struct {
			NAMING_FAILED struct {
				ID        int    `json:"id"`
				PackageID int    `json:"package_id"`
				State     int    `json:"state"`
				Type      int    `json:"type"`
				Attr      int    `json:"attr"`
				Text      string `json:"text"`
				URL       string `json:"url"`
				Meta      struct {
					Size int `json:"size"`
				} `json:"meta"`
				Mtime     int    `json:"mtime"`
				JumpTitle string `json:"jump_title"`
			} `json:"[吃瓜]"`
		} `json:"emote"`
		JumpURL struct {
		} `json:"jump_url"`
		MaxLine int `json:"max_line"`
	} `json:"content"`
	Replies      []ReplyItem `json:"replies"`
	Invisible    bool        `json:"invisible"`
	ReplyControl struct {
		Following bool   `json:"following"`
		MaxLine   int    `json:"max_line"`
		TimeDesc  string `json:"time_desc"`
		Location  string `json:"location"`
	} `json:"reply_control"`
}

type CommentResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TTL     int    `json:"ttl"`
	Data    struct {
		Cursor Cursor `json:"cursor"`
		Page   struct {
			Num    int `json:"num"`
			Size   int `json:"size"`
			Count  int `json:"count"`
			Acount int `json:"acount"`
		} `json:"page"`
		Replies    []ReplyItem `json:"replies"`
		TopReplies []ReplyItem `json:"top_replies"`
	} `json:"data"`
}
```

<!-- [END OF FILE: comment.go] -->

### crawler/blblcd/core/fetch.go
<!-- [START OF FILE: fetch.go] -->
```go
package core

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/store"
	"bilibili-comments-viewer-go/logger"
)

// core 包实现了 B 站评论爬取的核心流程，包括主评论、子评论的递归抓取与断点续爬等功能

// FindComment 爬取指定 avid 视频的所有评论（主评论+子评论），支持断点续爬和并发控制
// ctx: 上下文控制
// sem: 并发信号量
// wg: 外部 WaitGroup（可为 nil）
// avid: 视频 avid
// opt: 爬取选项
// resultChan: 评论结果输出通道
func FindComment(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup, avid int, opt *model.Option, resultChan chan<- model.Comment) {
	defer func() {
		if err := recover(); err != nil {
			logger.GetLogger().Errorf("爬取视频：%d失败", avid)
			logger.GetLogger().Error(err)
		}
		<-sem
		if wg != nil {
			wg.Done()
		}
	}()

	oid := strconv.Itoa(avid)
	logger.GetLogger().Infof("开始爬取视频评论: oid=%s", oid)

	total, err := FetchCount(oid)
	if err != nil {
		logger.GetLogger().Errorf("获取评论总数失败: %v", err)
		return
	}
	logger.GetLogger().Infof("视频 %s 共有 %d 条评论", oid, total)

	if total == 0 {
		logger.GetLogger().Infof("视频 %s 没有评论，跳过爬取", oid)
		return
	}

	// 断点续爬相关变量
	savePath := path.Join(opt.Output, opt.Bvid)
	if opt.CommentOutput != "" {
		savePath = path.Join(opt.CommentOutput, opt.Bvid)
	}
	imgSavePath := opt.ImageOutput
	if imgSavePath == "" {
		imgSavePath = path.Join(savePath, "images")
	}

	var pageWg sync.WaitGroup                            // 控制每一页的并发
	var mu sync.Mutex                                    // 保护计数和 map
	downloadedCount := 0                                 // 已下载评论数
	recordedMap := make(map[int64]bool)                  // 已记录评论去重
	consecutiveEmptyPages := 0                           // 连续空页计数
	offsetStr := ""                                      // 分页 offset
	page := 1                                            // 当前页码
	progressFile := path.Join(savePath, "progress.json") // 断点文件

	// 尝试加载断点
	if prog, err := loadProgress(progressFile); err == nil {
		page = prog.Page
		downloadedCount = prog.DownloadedCount
		logger.GetLogger().Infof("断点续爬: 从第%d页、已爬取%d条评论继续", page, downloadedCount)
	}

mainLoop:
	for {
		select {
		case <-ctx.Done():
			logger.GetLogger().Infof("收到取消信号，保存断点并退出...")
			saveProgress(progressFile, page, downloadedCount)
			break mainLoop
		default:
		}

		if downloadedCount >= total {
			logger.GetLogger().Infof("已爬取完成，共获取 %d 条评论，目标 %d 条", downloadedCount, total)
			break
		}

		if consecutiveEmptyPages >= opt.MaxTryCount {
			logger.GetLogger().Infof("连续 %d 页无新评论，停止爬取", opt.MaxTryCount)
			break
		}

		sem <- struct{}{}
		pageWg.Add(1)

		// 启动 goroutine 并发爬取每一页评论
		go func(pageNum int, offset string) {
			defer func() {
				if r := recover(); r != nil {
					logger.GetLogger().Errorf("爬取第%d页评论时发生panic: %v", pageNum, r)
				}
				<-sem
				pageWg.Done()
			}()

			select {
			case <-ctx.Done():
				logger.GetLogger().Infof("[goroutine] 收到取消信号，退出第%d页", pageNum)
				return
			default:
			}

			// 延迟与抖动，防止被风控
			baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
			delay := baseDelay + jitter
			time.Sleep(delay)

			logger.GetLogger().Infof("并发爬取第 %d 页评论 (oid: %s, offset: %s)", pageNum, oid, offset)
			cmtInfo, err := FetchComment(oid, pageNum, opt.Corder, opt.Cookie, offset)
			if err != nil {
				logger.GetLogger().Errorf("请求评论失败，视频%s，第%d页: %v", oid, pageNum, err)
				mu.Lock()
				consecutiveEmptyPages++
				mu.Unlock()
				return
			}

			if cmtInfo.Code != 0 {
				logger.GetLogger().Errorf("请求评论失败，视频%s，第%d页失败: %s", oid, pageNum, cmtInfo.Message)
				mu.Lock()
				consecutiveEmptyPages++
				mu.Unlock()
				return
			}

			logger.GetLogger().Infof("第 %d 页获取到 %d 条主评论", pageNum, len(cmtInfo.Data.Replies))

			// 检查是否到达末尾
			if cmtInfo.Data.Cursor.IsEnd {
				logger.GetLogger().Infof("API返回已到达末尾，停止爬取")
				mu.Lock()
				consecutiveEmptyPages = opt.MaxTryCount
				mu.Unlock()
				return
			}

			var replyCollection []model.ReplyItem
			replyCollection = append(replyCollection, cmtInfo.Data.Replies...)

			// 处理子评论
			for _, k := range cmtInfo.Data.Replies {
				if k.Rcount == 0 {
					continue
				}
				if len(k.Replies) > 0 && len(k.Replies) == k.Rcount {
					replyCollection = append(replyCollection, k.Replies...)
				} else {
					subCmts := FindSubComment(k, opt)
					replyCollection = append(replyCollection, subCmts...)
				}
			}

			// 处理置顶评论
			if len(cmtInfo.Data.TopReplies) != 0 {
				replyCollection = append(replyCollection, cmtInfo.Data.TopReplies...)
				for _, k := range cmtInfo.Data.TopReplies {
					if len(k.Replies) > 0 {
						replyCollection = append(replyCollection, k.Replies...)
					}
				}
			}

			var cmtCollection []model.Comment
			newCommentCount := 0

			// 评论去重与收集
			mu.Lock()
			for _, k := range replyCollection {
				if _, ok := recordedMap[k.Rpid]; !ok {
					cmt := NewCMT(&k)
					recordedMap[cmt.Rpid] = true
					cmtCollection = append(cmtCollection, cmt)
					resultChan <- cmt
					newCommentCount++
				}
			}

			if newCommentCount == 0 {
				consecutiveEmptyPages++
				logger.GetLogger().Debugf("第%d页无新评论，连续空页计数: %d", pageNum, consecutiveEmptyPages)
			} else {
				consecutiveEmptyPages = 0
			}

			downloadedCount += newCommentCount
			mu.Unlock()

			remaining := total - downloadedCount
			if remaining < 0 {
				remaining = 0
			}

			logger.GetLogger().Infof("视频%s，第%d页已爬取%d条新评论，总计%d条，预计剩余%d条",
				oid, pageNum, newCommentCount, downloadedCount, remaining)

			if len(cmtCollection) > 0 {
				store.Save2CSV(opt.Bvid, cmtCollection, savePath, imgSavePath, opt.ImgDownload)
			}

			// 保存进度，便于断点续爬
			saveProgress(progressFile, pageNum+1, downloadedCount)

			if cmtInfo.Data.Cursor.PaginationReply.NextOffset != "" {
				mu.Lock()
				offsetStr = cmtInfo.Data.Cursor.PaginationReply.NextOffset
				mu.Unlock()
				logger.GetLogger().Debugf("更新offset: %s", offsetStr)
			}
		}(page, offsetStr)

		page++
	}

	pageWg.Wait() // 等待所有页 goroutine 完成
	logger.GetLogger().Infof("*****爬取视频：%s评论完成，共获取 %d 条评论*****", oid, downloadedCount)

	_ = removeProgress(progressFile) // 清理断点文件
}

// FindSubComment 递归爬取某条主评论下的所有子评论
// cmt: 主评论项
// opt: 爬取选项
// 返回值: 子评论集合
func FindSubComment(cmt model.ReplyItem, opt *model.Option) []model.ReplyItem {
	oid := strconv.Itoa(cmt.Oid)
	round := 1
	replyCollection := []model.ReplyItem{}
	consecutiveEmptyPages := 0 // 连续空页计数

	logger.GetLogger().Infof("开始爬取评论 %d 的子评论，预计 %d 条", cmt.Rpid, cmt.Rcount)

	for {
		// 检查连续空页次数
		if consecutiveEmptyPages >= opt.MaxTryCount {
			logger.GetLogger().Infof("连续 %d 页无子评论，停止爬取评论 %d", opt.MaxTryCount, cmt.Rpid)
			break
		}

		// 延迟逻辑（配置化）
		baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
		delay := baseDelay + jitter
		time.Sleep(delay)

		logger.GetLogger().Infof("爬取评论 %d 的子评论第 %d 页", cmt.Rpid, round)
		cmtInfo, err := FetchSubComment(oid, cmt.Rpid, round, opt.Cookie)
		if err != nil {
			logger.GetLogger().Errorf("请求子评论失败，父评论%d，第%d页: %v", cmt.Rpid, round, err)
			consecutiveEmptyPages++
			round++
			continue
		}

		round++
		if cmtInfo.Code != 0 {
			logger.GetLogger().Errorf("请求子评论失败，父评论%d，第%d页失败: %s", cmt.Rpid, round-1, cmtInfo.Message)
			consecutiveEmptyPages++
			continue
		}

		if len(cmtInfo.Data.Replies) > 0 {
			replyCollection = append(replyCollection, cmtInfo.Data.Replies...)

			// 处理嵌套回复
			for _, k := range cmtInfo.Data.Replies {
				if len(k.Replies) > 0 {
					replyCollection = append(replyCollection, k.Replies...)
				}
			}

			// 处理置顶回复
			if len(cmtInfo.Data.TopReplies) != 0 {
				replyCollection = append(replyCollection, cmtInfo.Data.TopReplies...)
				for _, k := range cmtInfo.Data.TopReplies {
					if len(k.Replies) > 0 {
						replyCollection = append(replyCollection, k.Replies...)
					}
				}
			}

			consecutiveEmptyPages = 0 // 重置连续空页计数
			logger.GetLogger().Debugf("评论 %d 第 %d 页获取到 %d 条子评论", cmt.Rpid, round-1, len(cmtInfo.Data.Replies))
		} else {
			consecutiveEmptyPages++
			logger.GetLogger().Debugf("评论 %d 第 %d 页无子评论，连续空页计数: %d", cmt.Rpid, round-1, consecutiveEmptyPages)
		}
	}

	logger.GetLogger().Infof("评论 %d 子评论爬取完成，共获取 %d 条", cmt.Rpid, len(replyCollection))
	return replyCollection
}

// NewCMT 将 ReplyItem 转换为 Comment 结构体
func NewCMT(item *model.ReplyItem) model.Comment {
	// Oid 校验
	var bvid string
	if item.Oid > 0 {
		bvid = Avid2Bvid(int64(item.Oid))
	}
	// 防御性：Bvid 必须以 BV 开头且长度为12
	if !strings.HasPrefix(bvid, "BV") || len(bvid) != 12 {
		bvid = ""
	}
	return model.Comment{
		Uname:         item.Member.Uname,
		Sex:           item.Member.Sex,
		Content:       item.Content.Message,
		Rpid:          item.Rpid,
		Oid:           item.Oid,
		Bvid:          bvid,
		Mid:           item.Mid,
		Parent:        item.Parent,
		Ctime:         item.Ctime,
		Like:          item.Like,
		Following:     item.ReplyControl.Following,
		Current_level: item.Member.LevelInfo.CurrentLevel,
		Pictures:      item.Content.Pictures,
		Location:      strings.Replace(item.ReplyControl.Location, "IP属地：", "", -1),
	}
}

// FindUser 爬取指定 up 主的所有视频评论
// sem: 并发信号量
// opt: 爬取选项（需包含 mid）
func FindUser(sem chan struct{}, opt *model.Option) {
	defer func() {
		if err := recover(); err != nil {
			logger.GetLogger().Errorf("爬取up：%d失败", opt.Mid)
			logger.GetLogger().Error(err)
		}
	}()

	var wg sync.WaitGroup
	round := opt.Skip + 1
	var videoCollection []model.VideoItem

	for ; round < opt.Pages+opt.Skip; round++ {
		// 延迟逻辑（配置化）
		baseDelay := time.Duration(opt.DelayBaseMs) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(opt.DelayJitterMs))) * time.Millisecond
		delay := baseDelay + jitter
		time.Sleep(delay)
		logger.GetLogger().Infof("爬取视频列表第%d页", round)
		tempVideoInfo, err := FetchVideoList(opt.Mid, round, opt.Vorder, opt.Cookie)
		if err != nil {
			logger.GetLogger().Errorf("请求up主视频列表失败，第%d页失败", round)
			logger.GetLogger().Error(err)
			continue
		}
		if tempVideoInfo.Code != 0 {
			logger.GetLogger().Errorf("请求up主视频列表失败，第%d页失败", round)
			logger.GetLogger().Error(tempVideoInfo.Message)
			continue
		}
		if len(tempVideoInfo.Data.List.Vlist) != 0 {
			videoCollection = append(videoCollection, tempVideoInfo.Data.List.Vlist...)
		} else {
			break
		}
	}

	logger.GetLogger().Infof("%d查找到了%d条视频", opt.Mid, len(videoCollection))
	for _, k := range videoCollection {
		time.Sleep(3 * time.Second)
		logger.GetLogger().Infof("------启动爬取%d------", k.Aid)
		wg.Add(1)
		sem <- struct{}{}

		// 创建结果通道
		resultChan := make(chan model.Comment, 1000)

		// 传递结果通道作为第5个参数
		go func(aid int) {
			defer wg.Done()
			FindComment(context.Background(), sem, &wg, aid, opt, resultChan)
			close(resultChan)
		}(k.Aid)
	}
	wg.Wait()
}

// 断点续爬相关结构体与方法
// progress 记录断点信息
// page: 当前页码
// downloadedCount: 已下载评论数
type progress struct {
	Page            int `json:"page"`
	DownloadedCount int `json:"downloaded_count"`
}

// saveProgress 保存断点信息到文件
func saveProgress(filename string, page, downloadedCount int) error {
	p := progress{Page: page, DownloadedCount: downloadedCount}
	b, _ := json.Marshal(p)
	return os.WriteFile(filename, b, 0644)
}

// loadProgress 加载断点信息
func loadProgress(filename string) (progress, error) {
	var p progress
	b, err := os.ReadFile(filename)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}

// removeProgress 删除断点文件
func removeProgress(filename string) error {
	return os.Remove(filename)
}
```

<!-- [END OF FILE: fetch.go] -->

### backend/csv_importer.go
<!-- [START OF FILE: csv_importer.go] -->
```go
package backend

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bilibili-comments-viewer-go/logger"

	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/database"
)

func saveCommentsToCSV(comments []blblcdmodel.Comment, csvPath string) error {
	if err := os.MkdirAll(filepath.Dir(csvPath), 0755); err != nil {
		return fmt.Errorf("创建CSV目录失败: %w", err)
	}

	file, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("创建CSV文件失败: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{"bvid", "upname", "sex", "content", "pictures", "rpid", "oid", "mid",
		"parent", "fans_grade", "ctime", "like_count", "following", "level", "location"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("写入CSV标题失败: %w", err)
	}

	for _, comment := range comments {
		record := []string{
			comment.Bvid,
			comment.Uname,
			comment.Sex,
			comment.Content,
			"", // 图片留空
			strconv.FormatInt(comment.Rpid, 10),
			strconv.Itoa(comment.Oid),
			strconv.Itoa(comment.Mid),
			strconv.Itoa(comment.Parent),
			strconv.Itoa(comment.Fansgrade),
			strconv.Itoa(comment.Ctime),
			strconv.Itoa(comment.Like),
			strconv.FormatBool(comment.Following),
			strconv.Itoa(comment.Current_level),
			comment.Location,
		}

		if err := writer.Write(record); err != nil {
			logger.GetLogger().Errorf("写入评论失败: %v", err)
		}
	}

	return nil
}

func ImportCommentsFromCSV(bvid, filePath string) error {
	// 打开CSV文件
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 创建CSV reader
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 允许可变字段数

	// 读取头部
	header, err := reader.Read()
	if err != nil {
		return err
	}

	// 创建字段映射
	fieldMap := make(map[string]int)
	for i, field := range header {
		fieldMap[strings.ToLower(field)] = i
	}

	// 第一遍：构建映射关系
	rpidToUniqueID := make(map[int]string)
	allComments := make([]map[string]string, 0)

	// 读取所有行
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for _, record := range records {
		comment := make(map[string]string)
		for field, index := range fieldMap {
			if index < len(record) {
				comment[field] = strings.TrimSpace(record[index])
			}
		}

		bvidVal := comment["bvid"]
		rpidVal := comment["rpid"]

		if bvidVal == "" || rpidVal == "" {
			continue
		}

		// 生成唯一ID
		uniqueID := bvidVal + "_" + rpidVal

		// 存储映射关系
		if rpid, err := strconv.Atoi(rpidVal); err == nil {
			rpidToUniqueID[rpid] = uniqueID
			allComments = append(allComments, comment)
		}
	}

	// 第二遍：构建父子关系
	parentChildMap := make(map[string][]string)
	for _, comment := range allComments {
		childRpid, err := strconv.Atoi(comment["rpid"])
		if err != nil {
			continue
		}
		childUniqueID := rpidToUniqueID[childRpid]

		parentVal := comment["parent"]
		if parentVal == "" {
			parentVal = "0"
		}

		parentRpid, err := strconv.Atoi(parentVal)
		if err != nil {
			continue
		}

		if parentRpid == 0 {
			parentChildMap[childUniqueID] = []string{}
		} else {
			parentUniqueID := rpidToUniqueID[parentRpid]
			if parentUniqueID != "" {
				if _, exists := parentChildMap[parentUniqueID]; !exists {
					parentChildMap[parentUniqueID] = []string{}
				}
				parentChildMap[parentUniqueID] = append(parentChildMap[parentUniqueID], childUniqueID)
			}
		}
	}

	// 第三遍：准备导入数据
	commentsToImport := make([]*database.Comment, 0)

	for _, comment := range allComments {
		bvidVal := comment["bvid"]
		rpidVal := comment["rpid"]

		if bvidVal == "" || rpidVal == "" {
			continue
		}

		uniqueID := bvidVal + "_" + rpidVal

		// 处理父关系
		parentVal := comment["parent"]
		parentID := "0"
		if parentVal != "" && parentVal != "0" {
			if parentRpid, err := strconv.Atoi(parentVal); err == nil {
				if parentUniqueID, exists := rpidToUniqueID[parentRpid]; exists {
					parentID = parentUniqueID
				}
			}
		}

		// 处理回复列表
		repliesList := parentChildMap[uniqueID]

		// 创建评论对象
		dbComment := &database.Comment{
			UniqueID: uniqueID,
			BVid:     bvidVal,
			Content:  comment["content"],
			Pictures: []database.Picture{},
			Parent:   parentID,
			Replies:  repliesList, // 直接使用字符串切片
		}

		// 解析rpid
		if rpid, err := strconv.ParseInt(rpidVal, 10, 64); err == nil {
			dbComment.Rpid = rpid
		}

		// 解析oid
		if oid, err := strconv.Atoi(comment["oid"]); err == nil {
			dbComment.Oid = oid
		}

		// 解析mid
		if mid, err := strconv.Atoi(comment["mid"]); err == nil {
			dbComment.Mid = mid
		}

		// 解析粉丝等级
		if fansGrade, err := strconv.Atoi(comment["fans_grade"]); err == nil {
			dbComment.FansGrade = fansGrade
		}

		// 解析时间戳 - 转换为 time.Time
		if ctimeStr, ok := comment["ctime"]; ok {
			if ctime, err := strconv.ParseInt(ctimeStr, 10, 64); err == nil {
				dbComment.Ctime = time.Unix(ctime, 0)
			}
		}

		// 解析点赞数
		if likeCount, err := strconv.Atoi(comment["like_count"]); err == nil {
			dbComment.LikeCount = likeCount
		}

		// 解析等级
		if level, err := strconv.Atoi(comment["level"]); err == nil {
			dbComment.Level = level
		}

		// 解析关注状态
		following := strings.ToLower(comment["following"])
		dbComment.Following = following == "true" || following == "1" || following == "yes"

		// 其他字段
		dbComment.Upname = comment["upname"]
		dbComment.Sex = comment["sex"]
		dbComment.Location = comment["location"]

		// 处理图片
		if pics, ok := comment["pictures"]; ok && pics != "" {
			picList := strings.Split(pics, ";")
			for _, pic := range picList {
				if pic != "" {
					dbComment.Pictures = append(dbComment.Pictures, database.Picture{ImgSrc: pic})
				}
			}
		}

		commentsToImport = append(commentsToImport, dbComment)
	}

	// 使用新的批量导入接口
	if err := database.ImportCommentsData(bvid, commentsToImport); err != nil {
		return err
	}

	return nil
}
```

<!-- [END OF FILE: csv_importer.go] -->

### crawler/blblcd/model/videolist.go
<!-- [START OF FILE: videolist.go] -->
```go
package model

type VideoItem struct {
	Comment          int    `json:"comment"`
	Typeid           int    `json:"typeid"`
	Play             int    `json:"play"`
	Pic              string `json:"pic"`
	Subtitle         string `json:"subtitle"`
	Description      string `json:"description"`
	Copyright        string `json:"copyright"`
	Title            string `json:"title"`
	Review           int    `json:"review"`
	Author           string `json:"author"`
	Mid              int    `json:"mid"`
	Created          int    `json:"created"`
	Length           string `json:"length"`
	VideoReview      int    `json:"video_review"`
	Aid              int    `json:"aid"`
	Bvid             string `json:"bvid"`
	HideClick        bool   `json:"hide_click"`
	IsPay            int    `json:"is_pay"`
	IsUnionVideo     int    `json:"is_union_video"`
	IsSteinsGate     int    `json:"is_steins_gate"`
	IsLivePlayback   int    `json:"is_live_playback"`
	IsLessonVideo    int    `json:"is_lesson_video"`
	IsLessonFinished int    `json:"is_lesson_finished"`
	LessonUpdateInfo string `json:"lesson_update_info"`
	JumpURL          string `json:"jump_url"`
	Meta             any    `json:"meta"`
	IsAvoided        int    `json:"is_avoided"`
	SeasonID         int    `json:"season_id"`
	Attribute        int    `json:"attribute"`
	IsChargingArc    bool   `json:"is_charging_arc"`
	Vt               int    `json:"vt"`
	EnableVt         int    `json:"enable_vt"`
	VtDisplay        string `json:"vt_display"`
	PlaybackPosition int    `json:"playback_position"`
}

type VideoListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TTL     int    `json:"ttl"`
	Data    struct {
		List struct {
			Vlist []VideoItem `json:"vlist"`
			Slist []any       `json:"slist"`
		} `json:"list"`
		Page struct {
			Pn    int `json:"pn"`
			Ps    int `json:"ps"`
			Count int `json:"count"`
		} `json:"page"`
		EpisodicButton struct {
			Text string `json:"text"`
			URI  string `json:"uri"`
		} `json:"episodic_button"`
		IsRisk      bool `json:"is_risk"`
		GaiaResType int  `json:"gaia_res_type"`
		GaiaData    any  `json:"gaia_data"`
	} `json:"data"`
}
```

<!-- [END OF FILE: videolist.go] -->

### crawler/blblcd/cli/up.go
<!-- [START OF FILE: up.go] -->
```go
package cli

import (
	"fmt"
	"strconv"

	"bilibili-comments-viewer-go/crawler/blblcd/core"
	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/utils"

	"github.com/spf13/cobra"
)

var (
	pages  int
	skip   int
	vorder string
)

func init() {
	upCmd.Flags().IntVarP(&pages, "pages", "p", 3, "获取的页数")
	upCmd.Flags().IntVarP(&skip, "skip", "s", 0, "跳过视频的页数")
	upCmd.Flags().StringVarP(&vorder, "vorder", "t", "pubdate", "爬取up主视频列表时排序方式，最新发布：pubdate最多播放：click最多收藏：stow")

	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "批量获取UP主视频列表的评论",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("please provide mid")
			return
		}
		mid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fmt.Println(err)
			return
		}

		utils.PresetPath(output)
		cookie, err := utils.ReadTextFile(cookieFile)
		if err != nil {
			fmt.Println(err)
			return
		}

		opt := model.Option{
			Mid:           int(mid),
			Pages:         pages,
			Skip:          skip,
			Vorder:        vorder,
			Bvid:          "",
			Corder:        corder,
			Cookie:        cookie,
			Output:        output,
			ImgDownload:   imgDownload,
			MaxTryCount:   maxTryCount,
			CommentOutput: commentOutput,
			ImageOutput:   imageOutput,
		}
		sem := make(chan struct{}, workers)
		core.FindUser(sem, &opt)

	},
}
```

<!-- [END OF FILE: up.go] -->

### crawler/blblcd/store/image.go
<!-- [START OF FILE: image.go] -->
```go
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
```

<!-- [END OF FILE: image.go] -->

### crawler/blblcd/store/csv.go
<!-- [START OF FILE: csv.go] -->
```go
package store

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/crawler/blblcd/utils"
	"bilibili-comments-viewer-go/logger"
)

func parseInt64(num int64) string {
	return fmt.Sprint(num)
}

func parseInt(num int) string {
	return strconv.Itoa(num)
}

func CMT2Record(cmt model.Comment) (record []string) {
	picURLs := ""
	for _, pic := range cmt.Pictures {
		picURLs += pic.Img_src + ";"
	}
	return []string{
		cmt.Bvid, cmt.Uname, cmt.Sex, cmt.Content, picURLs,
		parseInt64(cmt.Rpid), parseInt(cmt.Oid), parseInt(cmt.Mid),
		parseInt(cmt.Parent), parseInt(cmt.Fansgrade), parseInt(cmt.Ctime),
		parseInt(cmt.Like), fmt.Sprint(cmt.Following), parseInt(cmt.Current_level), cmt.Location,
	}
}

func Save2CSV(filename string, cmts []model.Comment, output string, imgOutput string, downloadIMG bool) {
	defer func() {
		if err := recover(); err != nil {
			logger.GetLogger().Error("鍐欏叆CSV閿欒:", err)
		}
	}()
	utils.PresetPath(output)
	if len(cmts) == 0 {
		return
	}
	csv_path := filepath.Join(output, filename+".csv")
	if utils.FileOrPathExists(csv_path) {
		file, err := os.OpenFile(csv_path, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.GetLogger().Errorf("鎵撳紑csv鏂囦欢閿欒锛宱id:%d", cmts[0].Oid)
			return
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		for _, cmt := range cmts {
			if cmt.Uname == "" {
				continue
			}
			if downloadIMG {
				if len(cmt.Pictures) != 0 {
					go WriteImage(cmt.Uname, cmt.Pictures, output+"/"+"images")
				}
			}

			record := CMT2Record(cmt)
			err = writer.Write(record)
			if err != nil {
				logger.GetLogger().Errorf("杩藉姞璇勮鑷砪sv鏂囦欢閿欒锛宱id:%d", cmt.Oid)
			}
		}

		logger.GetLogger().Infof("杩藉姞璇勮鑷砪sv鏂囦欢鎴愬姛锛宱id:%d", cmts[0].Oid)

	} else {
		file, err := os.Create(csv_path)
		if err != nil {
			logger.GetLogger().Errorf("鍒涘缓csv鏂囦欢閿欒锛宱id:%d", cmts[0].Oid)
			return
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()
		headers := "bvid,upname,sex,content,pictures,rpid,oid,mid,parent,fans_grade,ctime,like,following,level,location"
		headerErr := writer.Write(strings.Split(headers, ","))
		if headerErr != nil {
			logger.GetLogger().Errorf("鍐欏叆csv鏂囦欢瀛楁閿欒锛宱id:%d", cmts[0].Oid)
			return
		}

		for _, cmt := range cmts {
			if cmt.Uname == "" {
				continue
			}
			if downloadIMG {
				if len(cmt.Pictures) != 0 {
					go WriteImage(cmt.Uname, cmt.Pictures, imgOutput)
				}
			}

			record := CMT2Record(cmt)
			err := writer.Write(record)
			if err != nil {
				logger.GetLogger().Errorf("鍐欏叆csv鏂囦欢閿欒锛宱id:%d", cmt.Oid)
				return
			}
		}
		logger.GetLogger().Infof("鍐欏叆csv鏂囦欢鎴愬姛锛宱id:%d", cmts[0].Oid)
	}

}
```

<!-- [END OF FILE: csv.go] -->

### backend/metadata_service.go
<!-- [START OF FILE: metadata_service.go] -->
```go
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
```

<!-- [END OF FILE: metadata_service.go] -->

### backend/crawler_manager.go
<!-- [START OF FILE: crawler_manager.go] -->
```go
package backend

import (
	"context"
	"fmt"
	"log"

	"bilibili-comments-viewer-go/config"
	"bilibili-comments-viewer-go/crawler/blblcd"
	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	blblcdstore "bilibili-comments-viewer-go/crawler/blblcd/store"
	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
	"bilibili-comments-viewer-go/utils"
)

func CrawlAndImport(ctx context.Context, bvid string) error {
	cfg := config.Get()
	log := logger.GetLogger()

	// +++ 添加关键日志 +++
	log.Infof("开始处理视频: %s (保存模式: %s)", bvid, cfg.Crawler.SaveMode)

	// 获取并保存视频元数据
	log.Infof("获取视频元数据: %s", bvid)
	if videoInfo, err := FetchVideoMetadata(bvid); err == nil {
		dbVideo := &database.Video{
			BVid:  videoInfo.BVID,
			Title: videoInfo.Title,
			Cover: videoInfo.LocalCover,
		}
		if err := database.SaveVideo(dbVideo); err != nil {
			log.Errorf("保存视频信息失败: %v", err)
		} else {
			log.Infof("视频元数据保存成功: %s", bvid)
		}
	} else {
		log.Errorf("获取视频元数据失败: %v", err)
		// 即使元数据获取失败，也创建基础视频记录
		dbVideo := &database.Video{BVid: bvid}
		database.SaveVideo(dbVideo)
	}

	// 爬取评论
	log.Infof("开始爬取评论: %s", bvid)
	comments, err := crawlVideoComments(ctx, bvid)
	if err != nil {
		log.Errorf("评论爬取失败: %v", err)
		return CrawlerError{Message: "评论爬取失败: " + err.Error()}
	}

	// +++ 添加关键日志 +++
	log.Infof("爬取到 %d 条评论 (bvid: %s)", len(comments), bvid)

	// +++ 处理空评论情况 +++
	if len(comments) == 0 {
		log.Warnf("未爬取到评论，跳过处理 (bvid: %s)", bvid)
		return nil
	}

	// 根据保存模式处理评论
	switch cfg.Crawler.SaveMode {
	case SaveModeCSVOnly:
		log.Infof("CSV_ONLY模式处理评论: %s", bvid)
		processCSVOnly(bvid, comments)
	case SaveModeDBOnly:
		log.Infof("DB_ONLY模式导入评论: %s", bvid)
		if err := importCommentsToDB(bvid, comments); err != nil {
			log.Errorf("导入数据库失败: %v", err)
			return err
		} else {
			log.Infof("成功导入 %d 条评论到数据库 (bvid: %s)", len(comments), bvid)
		}
	default: // SaveModeCSVAndDB
		log.Infof("CSV_AND_DB模式处理评论: %s", bvid)
		processCSVAndDB(bvid, comments)
	}

	// +++ 新增：根据配置自动下载评论图片 +++
	if cfg.Crawler.ImgDownload {
		DownloadAllCommentImages(comments)
	}

	log.Infof("视频 %s 的评论处理完成", bvid)
	return nil
}

func CrawlUpVideos(mid int, fetchAll bool) error {
	cfg := config.Get()
	opt := blblcdmodel.NewDefaultOption()

	// 配置爬虫选项
	opt.Cookie = utils.ReadCookie(cfg.Crawler.CookieFile)
	opt.Output = cfg.Crawler.OutputDir
	opt.Mid = mid
	opt.FetchAll = fetchAll
	if cfg.Crawler.UpPages > 0 {
		opt.Pages = cfg.Crawler.UpPages
	}
	if cfg.Crawler.UpOrder != "" {
		opt.Vorder = cfg.Crawler.UpOrder
	}
	if cfg.Crawler.Workers > 0 {
		opt.Workers = cfg.Crawler.Workers
	}
	if cfg.Crawler.MaxTryCount > 0 {
		opt.MaxTryCount = cfg.Crawler.MaxTryCount
	}
	if cfg.Crawler.DelayBaseMs > 0 {
		opt.DelayBaseMs = cfg.Crawler.DelayBaseMs
	}
	if cfg.Crawler.DelayJitterMs > 0 {
		opt.DelayJitterMs = cfg.Crawler.DelayJitterMs
	}

	log.Printf("开始爬取UP主 %d 的视频 (页数: %d, 排序: %s, 协程: %d, 爬取所有: %v)",
		mid, opt.Pages, opt.Vorder, opt.Workers, opt.FetchAll)

	if err := blblcd.CrawlUp(mid, opt); err != nil {
		return CrawlerError{Message: fmt.Sprintf("UP主视频爬取失败: %s", err.Error())}
	}

	// 根据保存模式决定是否导入CSV
	if cfg.Crawler.SaveMode != SaveModeDBOnly {
		processCSVFiles()
	}

	return nil
}

// DownloadAllCommentImages 批量下载所有评论的图片（配置化：是否下载、保存路径，按BV号分目录）
func DownloadAllCommentImages(comments []blblcdmodel.Comment) {
	cfg := config.Get()
	if !cfg.Crawler.ImgDownload {
		return // 配置未开启图片下载
	}
	outputDir := cfg.ImageStorageDir
	for _, cmt := range comments {
		if len(cmt.Pictures) > 0 && cmt.Bvid != "" {
			blblcdstore.WriteImage(cmt.Bvid, cmt.Pictures, outputDir)
		}
	}
}
```

<!-- [END OF FILE: crawler_manager.go] -->

### crawler/blblcd/cli/inject.go
<!-- [START OF FILE: inject.go] -->
```go
package cli

type Injection struct {
	Version   string
	BuildTime string
	Commit    string
	Author    string
}
```

<!-- [END OF FILE: inject.go] -->

### crawler/blblcd/model/error.go
<!-- [START OF FILE: error.go] -->
```go
package model

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TTL     int    `json:"ttl"`
}
```

<!-- [END OF FILE: error.go] -->

### crawler/blblcd/model/option.go
<!-- [START OF FILE: option.go] -->
```go
package model

type Option struct {
	Cookie        string
	Mid           int
	Pages         int
	Skip          int
	Vorder        string
	Bvid          string
	Corder        int
	Output        string
	ImgDownload   bool
	MaxTryCount   int
	CommentOutput string
	ImageOutput   string
	Workers       int
	FetchAll      bool // 新增：是否爬取所有视频
	DelayBaseMs   int  // 新增
	DelayJitterMs int  // 新增
}

// 新增构造函数确保默认值
func NewDefaultOption() *Option {
	return &Option{
		Pages:         10,        // 默认获取10页视频
		Vorder:        "pubdate", // 默认按最新发布排序
		Workers:       5,         // 默认5个协程
		MaxTryCount:   3,         // 默认重试3次
		FetchAll:      false,     // 默认不爬取所有视频
		DelayBaseMs:   3000,      // 新增默认值
		DelayJitterMs: 2000,      // 新增默认值
	}
}
```

<!-- [END OF FILE: option.go] -->

### backend/converter.go
<!-- [START OF FILE: converter.go] -->
```go
package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	blblcdmodel "bilibili-comments-viewer-go/crawler/blblcd/model"
	"bilibili-comments-viewer-go/database"
	"bilibili-comments-viewer-go/logger"
)

func importCommentsToDB(bvid string, comments []blblcdmodel.Comment) error {
	log := logger.GetLogger()
	log.Infof("正在导入 %d 条评论到数据库 (bvid: %s)", len(comments), bvid)

	// +++ 添加空评论检查 +++
	if len(comments) == 0 {
		log.Warn("警告: 尝试导入空评论列表")
		return nil
	}

	var dbComments []*database.Comment
	for i, comment := range comments {
		dbComment := convertToDBComment(&comment)
		dbComments = append(dbComments, dbComment)

		if i%100 == 0 {
			logger.GetLogger().Infof("已转换 %d/%d 条评论", i+1, len(comments))
		}
	}

	logger.GetLogger().Infof("开始批量保存 %d 条评论...", len(dbComments))

	if err := database.BatchSaveComments(dbComments); err != nil {
		return fmt.Errorf("批量保存评论失败: %w", err)
	}

	logger.GetLogger().Infof("评论数据保存完成，开始处理回复关系...")

	for _, dbComment := range dbComments {
		if len(dbComment.Replies) > 0 {
			logger.GetLogger().Infof("为评论 %s 保存 %d 条回复关系", dbComment.UniqueID, len(dbComment.Replies))
			if err := database.SaveCommentRelations(dbComment.UniqueID, dbComment.Replies); err != nil {
				logger.GetLogger().Errorf("保存评论关系失败: %v", err)
			}
		}
	}

	logger.GetLogger().Infof("开始更新评论统计...")
	if err := database.UpdateCommentStats(bvid); err != nil {
		logger.GetLogger().Errorf("更新评论统计失败: %v", err)
	}

	logger.GetLogger().Infof("重建所有评论关系...")
	if err := database.RebuildAllCommentRelations(bvid); err != nil {
		logger.GetLogger().Errorf("重建评论关系失败: %v", err)
	}

	return nil
}

func convertToDBComment(comment *blblcdmodel.Comment) *database.Comment {
	// 校验 Bvid 和 Rpid
	if strings.HasPrefix(comment.Bvid, "http") || strings.Contains(comment.Bvid, "/") {
		logger.GetLogger().Warnf("异常Bvid: %s, comment: %+v", comment.Bvid, comment)
		return nil
	}
	if comment.Rpid <= 0 {
		logger.GetLogger().Warnf("异常Rpid: %d, comment: %+v", comment.Rpid, comment)
		return nil
	}

	// 确保rpid不为0
	if comment.Rpid == 0 {
		logger.GetLogger().Infof("警告: 发现rpid=0的评论: %+v", comment)
		comment.Rpid = time.Now().UnixNano() // 生成临时ID
	}

	uniqueID := comment.Bvid + "_" + strconv.FormatInt(comment.Rpid, 10)

	// 处理图片
	var pictures []database.Picture
	for _, pic := range comment.Pictures {
		pictures = append(pictures, database.Picture{ImgSrc: pic.Img_src})
	}

	// 处理回复关系
	var replies []string
	for _, reply := range comment.Replies {
		replyID := comment.Bvid + "_" + strconv.FormatInt(reply.Rpid, 10)
		replies = append(replies, replyID)
	}

	// 处理父评论ID
	parentID := "0"
	if comment.Parent != 0 {
		parentID = comment.Bvid + "_" + strconv.FormatInt(int64(comment.Parent), 10)
	}

	return &database.Comment{
		UniqueID:  uniqueID,
		BVid:      comment.Bvid,
		Rpid:      comment.Rpid,
		Content:   comment.Content,
		Pictures:  pictures, // 修复：添加图片数据
		Oid:       comment.Oid,
		Mid:       comment.Mid,
		Parent:    parentID,
		FansGrade: comment.Fansgrade,
		Ctime:     time.Unix(int64(comment.Ctime), 0),
		LikeCount: comment.Like,
		Upname:    comment.Uname,
		Sex:       comment.Sex,
		Following: comment.Following,
		Level:     comment.Current_level,
		Location:  comment.Location,
		Replies:   replies, // 修复：添加回复关系
	}
}
```

<!-- [END OF FILE: converter.go] -->

