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
