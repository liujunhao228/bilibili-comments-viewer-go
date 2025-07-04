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
