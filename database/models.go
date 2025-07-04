package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bilibili-comments-viewer-go/logger"

	_ "modernc.org/sqlite"
)

// 全局数据库连接
var db *sql.DB

// InitDB 初始化数据库连接
func InitDB(dbPath string) error {
	// 确保数据库目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("创建数据库目录失败: %w", err)
	}

	// 打开数据库连接
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// PRAGMA优化设置
	pragmaStmts := []string{
		"PRAGMA synchronous = OFF;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA temp_store = MEMORY;",
	}
	for _, stmt := range pragmaStmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("设置PRAGMA失败: %w", err)
		}
	}

	// 创建表
	if err := createTables(); err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	logger.GetLogger().Infof("数据库初始化成功: %s", dbPath)
	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() {
	if db != nil {
		db.Close()
		logger.GetLogger().Info("数据库连接已关闭")
	}
}

// GetDB 获取数据库连接
func GetDB() *sql.DB {
	return db
}

// createTables 创建所需的数据库表
func createTables() error {
	// 创建视频信息表
	videoTableSQL := `
	CREATE TABLE IF NOT EXISTS video_info (
		bvid TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		cover TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(videoTableSQL); err != nil {
		return fmt.Errorf("创建视频表失败: %w", err)
	}

	// 创建评论表 - 移除replies字段
	commentTableSQL := `
	CREATE TABLE IF NOT EXISTS bilibili_comments (
		unique_id TEXT PRIMARY KEY,
		bvid TEXT NOT NULL,
		rpid INTEGER NOT NULL,
		content TEXT,
		pictures TEXT,
		oid INTEGER,
		mid INTEGER,
		parent TEXT,
		fans_grade INTEGER,
		ctime INTEGER,  -- 存储整型时间戳
		like_count INTEGER,
		upname TEXT,
		sex TEXT,
		following BOOLEAN,
		level INTEGER,
		location TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_bvid ON bilibili_comments(bvid);
	CREATE INDEX IF NOT EXISTS idx_rpid ON bilibili_comments(rpid);
	CREATE INDEX IF NOT EXISTS idx_mid ON bilibili_comments(mid);
	CREATE INDEX IF NOT EXISTS idx_bvid_ctime ON bilibili_comments(bvid, ctime);`

	if _, err := db.Exec(commentTableSQL); err != nil {
		return fmt.Errorf("创建评论表失败: %w", err)
	}

	// 创建评论关系表
	relationTableSQL := `
	CREATE TABLE IF NOT EXISTS comment_relations (
		parent_id TEXT NOT NULL,
		child_id TEXT NOT NULL,
		PRIMARY KEY (parent_id, child_id),
		FOREIGN KEY (parent_id) REFERENCES bilibili_comments(unique_id),
		FOREIGN KEY (child_id) REFERENCES bilibili_comments(unique_id)
	);
	
	CREATE INDEX IF NOT EXISTS idx_parent_child ON comment_relations(parent_id, child_id);`

	if _, err := db.Exec(relationTableSQL); err != nil {
		return fmt.Errorf("创建评论关系表失败: %w", err)
	}

	// 创建评论统计表
	statsTableSQL := `
	CREATE TABLE IF NOT EXISTS comment_stats (
		bvid TEXT PRIMARY KEY,
		comment_count INTEGER NOT NULL DEFAULT 0,
		last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (bvid) REFERENCES video_info(bvid)
	);`

	if _, err := db.Exec(statsTableSQL); err != nil {
		return fmt.Errorf("创建评论统计表失败: %w", err)
	}

	logger.GetLogger().Info("数据库表创建成功")
	return nil
}

// SaveVideo 保存视频信息到数据库
func SaveVideo(video *Video) error {
	// 直接存储文件名（不需要修改路径）
	_, err := db.Exec(`
        INSERT OR REPLACE INTO video_info (bvid, title, cover)
        VALUES (?, ?, ?)`,
		video.BVid, video.Title, video.Cover,
	)

	if err != nil {
		return fmt.Errorf("保存视频信息失败: %w", err)
	}
	return nil
}

// SaveComment 保存评论到数据库
func SaveComment(comment *Comment) error {
	// 生成唯一ID (bvid + "_" + rpid)
	uniqueID := fmt.Sprintf("%s_%d", comment.BVid, comment.Rpid)

	// 将图片数组转换为分号分隔的字符串
	pictures := ""
	if len(comment.Pictures) > 0 {
		var picURLs []string
		for _, pic := range comment.Pictures {
			picURLs = append(picURLs, pic.ImgSrc)
		}
		pictures = strings.Join(picURLs, ";")
	}

	// 将时间转换为Unix时间戳
	ctime := comment.Ctime.Unix()

	_, err := db.Exec(`
		INSERT OR REPLACE INTO bilibili_comments 
		(unique_id, bvid, rpid, content, pictures, oid, mid, parent, fans_grade, 
		 ctime, like_count, upname, sex, following, level, location)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uniqueID,
		comment.BVid,
		comment.Rpid,
		comment.Content,
		pictures,
		comment.Oid,
		comment.Mid,
		comment.Parent,
		comment.FansGrade,
		ctime, // 使用整型时间戳
		comment.LikeCount,
		comment.Upname,
		comment.Sex,
		comment.Following,
		comment.Level,
		comment.Location,
	)

	if err != nil {
		return fmt.Errorf("保存评论失败 (Rpid: %d): %w", comment.Rpid, err)
	}

	return nil
}

// BatchSaveComments 批量保存评论
func BatchSaveComments(comments []*Comment) error {
	if len(comments) == 0 {
		logger.GetLogger().Warn("警告: 尝试保存空评论列表")
		return nil
	}

	const chunkSize = 1000      // 每组事务处理的评论数
	const batchInsertSize = 100 // 每条SQL插入的最大评论数
	startTime := time.Now()
	logger.GetLogger().Infof("开始批量保存 %d 条评论...", len(comments))

	successCount := 0
	errorCount := 0

	total := len(comments)
	for chunkStart := 0; chunkStart < total; chunkStart += chunkSize {
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > total {
			chunkEnd = total
		}
		chunk := comments[chunkStart:chunkEnd]

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("开始事务失败: %w", err)
		}

		for batchStart := 0; batchStart < len(chunk); batchStart += batchInsertSize {
			batchEnd := batchStart + batchInsertSize
			if batchEnd > len(chunk) {
				batchEnd = len(chunk)
			}
			batch := chunk[batchStart:batchEnd]

			// 构造多值插入SQL
			valueStrings := make([]string, 0, len(batch))
			valueArgs := make([]interface{}, 0, len(batch)*16)
			for _, comment := range batch {
				uniqueID := fmt.Sprintf("%s_%d", comment.BVid, comment.Rpid)
				comment.UniqueID = uniqueID
				pictures := ""
				if len(comment.Pictures) > 0 {
					var picURLs []string
					for _, pic := range comment.Pictures {
						picURLs = append(picURLs, pic.ImgSrc)
					}
					pictures = strings.Join(picURLs, ";")
				}
				ctime := comment.Ctime.Unix()
				valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
				valueArgs = append(valueArgs,
					comment.UniqueID,
					comment.BVid,
					comment.Rpid,
					comment.Content,
					pictures,
					comment.Oid,
					comment.Mid,
					comment.Parent,
					comment.FansGrade,
					ctime,
					comment.LikeCount,
					comment.Upname,
					comment.Sex,
					comment.Following,
					comment.Level,
					comment.Location,
				)
			}
			insertSQL := "INSERT OR REPLACE INTO bilibili_comments " +
				"(unique_id, bvid, rpid, content, pictures, oid, mid, parent, fans_grade, " +
				"ctime, like_count, upname, sex, following, level, location) VALUES " +
				strings.Join(valueStrings, ",")
			_, err := tx.Exec(insertSQL, valueArgs...)
			if err != nil {
				errorCount += len(batch)
				logger.GetLogger().Errorf("批量插入评论失败 (index: %d-%d): %v", chunkStart+batchStart, chunkStart+batchEnd-1, err)
			} else {
				successCount += len(batch)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		logger.GetLogger().Infof("已处理 %d/%d 条评论 (成功: %d, 失败: %d)", chunkEnd, total, successCount, errorCount)
	}

	logger.GetLogger().Infof("批量保存完成! 总计: %d, 成功: %d, 失败: %d, 耗时: %.2f秒",
		len(comments), successCount, errorCount, time.Since(startTime).Seconds())

	if errorCount > 0 {
		return fmt.Errorf("部分评论保存失败 (%d/%d)", errorCount, len(comments))
	}

	return nil
}

// SaveCommentRelations 保存评论关系
func SaveCommentRelations(parentID string, childIDs []string) error {
	if len(childIDs) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO comment_relations (parent_id, child_id)
		VALUES (?, ?)`)

	if err != nil {
		return fmt.Errorf("准备关系插入语句失败: %w", err)
	}
	defer stmt.Close()

	for _, childID := range childIDs {
		_, err = stmt.Exec(parentID, childID)
		if err != nil {
			return fmt.Errorf("插入关系失败: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// UpdateCommentStats 更新评论统计信息
func UpdateCommentStats(bvid string) error {
	_, err := db.Exec(`
        INSERT OR REPLACE INTO comment_stats (bvid, comment_count)
        SELECT ?, COUNT(*) 
        FROM bilibili_comments 
        WHERE bvid = ? AND parent = '0'`,
		bvid, bvid,
	)
	return err
}

// 获取分页视频列表
func GetVideosPaginated(page, perPage int, searchTerm string) ([]Video, int, error) {
	offset := (page - 1) * perPage
	var videos []Video
	var total int

	// 获取总数
	countQuery := "SELECT COUNT(*) FROM video_info"
	if searchTerm != "" {
		countQuery += " WHERE title LIKE ?"
	}

	var args []interface{}
	if searchTerm != "" {
		args = append(args, "%"+searchTerm+"%")
	}

	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("获取视频总数失败: %w", err)
	}

	// 修改查询：加入评论统计信息
	query := `
        SELECT v.bvid, v.title, v.cover, IFNULL(s.comment_count, 0) 
        FROM video_info v
        LEFT JOIN comment_stats s ON v.bvid = s.bvid
    `
	if searchTerm != "" {
		query += " WHERE v.title LIKE ?"
	}
	query += " ORDER BY v.created_at DESC LIMIT ? OFFSET ?"

	if searchTerm != "" {
		args = append(args, "%"+searchTerm+"%", perPage, offset)
	} else {
		args = []interface{}{perPage, offset}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询视频失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v Video
		var commentCount int
		if err := rows.Scan(&v.BVid, &v.Title, &v.Cover, &commentCount); err != nil {
			return nil, 0, fmt.Errorf("扫描视频行失败: %w", err)
		}
		v.CommentCount = commentCount // 设置评论数
		v.Cover = strings.ReplaceAll(v.Cover, `\`, `/`)
		videos = append(videos, v)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历视频行失败: %w", err)
	}

	return videos, total, nil
}

// GetVideoByBVid 通过BV号获取视频详情
func GetVideoByBVid(bvid string) (*Video, error) {
	row := db.QueryRow(`
		SELECT v.bvid, v.title, v.cover, IFNULL(s.comment_count, 0) 
		FROM video_info v
		LEFT JOIN comment_stats s ON v.bvid = s.bvid
		WHERE v.bvid = ?`, bvid)

	var v Video
	var commentCount int
	if err := row.Scan(&v.BVid, &v.Title, &v.Cover, &commentCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询视频详情失败: %w", err)
	}

	v.CommentCount = commentCount
	return &v, nil
}

// GetCommentsByBVid 获取指定视频的评论
func GetCommentsByBVid(bvid string, page, pageSize int, keyword string) ([]Comment, int, error) {
	offset := (page - 1) * pageSize
	var comments []Comment
	var total int

	// 从统计表获取总数
	err := db.QueryRow("SELECT comment_count FROM comment_stats WHERE bvid = ?", bvid).Scan(&total)
	if err != nil {
		if err == sql.ErrNoRows {
			// 如果没有统计记录，回退到实时计数
			err = db.QueryRow("SELECT COUNT(*) FROM bilibili_comments WHERE bvid = ?", bvid).Scan(&total)
			if err != nil {
				return nil, 0, fmt.Errorf("获取评论总数失败: %w", err)
			}
		} else {
			return nil, 0, fmt.Errorf("获取评论统计失败: %w", err)
		}
	}

	// 查询评论
	query := `
		SELECT unique_id, bvid, rpid, content, pictures, oid, mid, parent, 
			fans_grade, ctime, like_count, upname, sex, following, level, location
		FROM bilibili_comments
        WHERE bvid = ? AND parent = '0'`

	var args []interface{}
	args = append(args, bvid)

	if keyword != "" {
		query += " AND content LIKE ?"
		args = append(args, "%"+keyword+"%")
	}

	query += " ORDER BY like_count DESC, ctime DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询评论失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c Comment
		var ctime int64 // 整型时间戳
		var picturesStr string

		err := rows.Scan(
			&c.UniqueID, &c.BVid, &c.Rpid, &c.Content, &picturesStr,
			&c.Oid, &c.Mid, &c.Parent, &c.FansGrade, &ctime,
			&c.LikeCount, &c.Upname, &c.Sex, &c.Following, &c.Level,
			&c.Location,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("扫描评论行失败: %w", err)
		}

		// 将时间戳转换为时间对象
		c.Ctime = time.Unix(ctime, 0)
		c.FormattedTime = c.Ctime.Format("2006-01-02 15:04:05")

		// 解析图片字符串
		if picturesStr != "" {
			picURLs := strings.Split(picturesStr, ";")
			for _, url := range picURLs {
				if url != "" {
					c.Pictures = append(c.Pictures, Picture{ImgSrc: url})
				}
			}
		}

		comments = append(comments, c)
	}

	return comments, total, nil
}

// GetCommentReplies 获取评论的回复
func GetCommentReplies(parentID string, page, pageSize int) ([]Comment, int, error) {
	offset := (page - 1) * pageSize

	// 1. 先获取回复总数
	var total int
	countQuery := `SELECT COUNT(*) 
                   FROM comment_relations 
                   WHERE parent_id = ?`
	err := db.QueryRow(countQuery, parentID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("获取回复总数失败: %w", err)
	}

	// 2. 如果总数为0，直接返回空列表
	if total == 0 {
		return []Comment{}, 0, nil
	}

	// 3. 查询回复列表
	query := `
        SELECT c.unique_id, c.bvid, c.rpid, c.content, c.pictures, c.oid, c.mid, 
               c.parent, c.fans_grade, c.ctime, c.like_count, c.upname, 
               c.sex, c.following, c.level, c.location
        FROM bilibili_comments c
        JOIN comment_relations r ON c.unique_id = r.child_id
        WHERE r.parent_id = ?
        ORDER BY c.like_count DESC, c.ctime DESC
        LIMIT ? OFFSET ?`

	rows, err := db.Query(query, parentID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("查询回复失败: %w", err)
	}
	defer rows.Close()

	// 4. 声明并初始化replies切片
	var replies []Comment
	for rows.Next() {
		var c Comment
		var ctime int64
		var picturesStr string

		err := rows.Scan(
			&c.UniqueID, &c.BVid, &c.Rpid, &c.Content, &picturesStr,
			&c.Oid, &c.Mid, &c.Parent, &c.FansGrade, &ctime,
			&c.LikeCount, &c.Upname, &c.Sex, &c.Following, &c.Level,
			&c.Location,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("扫描回复行失败: %w", err)
		}

		// 将时间戳转换为时间对象
		c.Ctime = time.Unix(ctime, 0)
		c.FormattedTime = c.Ctime.Format("2006-01-02 15:04:05")

		// 解析图片字符串
		if picturesStr != "" {
			picURLs := strings.Split(picturesStr, ";")
			for _, url := range picURLs {
				if url != "" {
					c.Pictures = append(c.Pictures, Picture{ImgSrc: url})
				}
			}
		}

		replies = append(replies, c)
	}

	return replies, total, nil
}

// ImportVideoData 导入视频数据
func ImportVideoData(bvid string, videoData map[string]string) error {
	video := &Video{
		BVid:  bvid,
		Title: videoData["title"],
		Cover: videoData["cover"],
	}
	return SaveVideo(video)
}

// ImportCommentsData 导入评论数据
func ImportCommentsData(bvid string, comments []*Comment) error {
	// 批量保存评论
	if err := BatchSaveComments(comments); err != nil {
		return fmt.Errorf("批量保存评论失败: %w", err)
	}

	// 保存评论关系
	for _, comment := range comments {
		if len(comment.Replies) > 0 {
			if err := SaveCommentRelations(comment.UniqueID, comment.Replies); err != nil {
				logger.GetLogger().Errorf("保存评论关系失败: %v", err)
			}
		}
	}

	// 更新评论统计信息
	if err := UpdateCommentStats(bvid); err != nil {
		logger.GetLogger().Errorf("更新评论统计失败: %v", err)
	}

	return nil
}

// 图片结构
type Picture struct {
	ImgSrc string `json:"img_src"`
}

// 视频结构体
type Video struct {
	BVid         string `json:"bvid"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	CommentCount int    `json:"comment_count,omitempty"`
	Description  string `json:"description,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	ViewCount    int    `json:"view_count,omitempty"`
}

// 评论结构体
type Comment struct {
	UniqueID      string    `json:"unique_id"`
	BVid          string    `json:"bvid"`
	Rpid          int64     `json:"rpid"`
	Content       string    `json:"content"`
	Pictures      []Picture `json:"pictures"`
	Oid           int       `json:"oid"`
	Mid           int       `json:"mid"`
	Parent        string    `json:"parent"`
	FansGrade     int       `json:"fans_grade"`
	Ctime         time.Time `json:"ctime"`
	LikeCount     int       `json:"like_count"`
	Upname        string    `json:"upname"`
	Sex           string    `json:"sex"`
	Following     bool      `json:"following"`
	Level         int       `json:"level"`
	Location      string    `json:"location"`
	Replies       []string  `json:"replies,omitempty"` // 现在只存储回复ID
	FormattedTime string    `json:"formatted_time,omitempty"`
}

// RebuildAllCommentRelations 重建指定bvid下所有评论的父子关系
func RebuildAllCommentRelations(bvid string) error {
	db := GetDB()
	// 1. 删除该bvid下所有评论关系
	_, err := db.Exec(`
		DELETE FROM comment_relations
		WHERE parent_id IN (SELECT unique_id FROM bilibili_comments WHERE bvid = ?)
		   OR child_id IN (SELECT unique_id FROM bilibili_comments WHERE bvid = ?)
	`, bvid, bvid)
	if err != nil {
		return fmt.Errorf("删除旧评论关系失败: %w", err)
	}

	// 2. 查询所有有parent的评论（parent != '0'）
	rows, err := db.Query(`SELECT unique_id, parent FROM bilibili_comments WHERE bvid = ? AND parent != '0'`, bvid)
	if err != nil {
		return fmt.Errorf("查询评论失败: %w", err)
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO comment_relations (parent_id, child_id) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("准备插入语句失败: %w", err)
	}
	defer stmt.Close()

	for rows.Next() {
		var childID, parentID string
		if err := rows.Scan(&childID, &parentID); err != nil {
			tx.Rollback()
			return fmt.Errorf("扫描评论失败: %w", err)
		}
		_, err := stmt.Exec(parentID, childID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("插入评论关系失败: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		tx.Rollback()
		return fmt.Errorf("遍历评论失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
