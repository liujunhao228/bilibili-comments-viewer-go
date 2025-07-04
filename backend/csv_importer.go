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
