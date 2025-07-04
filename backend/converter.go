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
