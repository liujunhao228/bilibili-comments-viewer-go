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
			logger.GetLogger().Error("写入CSV错误:", err)
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
			logger.GetLogger().Errorf("打开csv文件错误，oid:%d", cmts[0].Oid)
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
				logger.GetLogger().Errorf("追加评论至csv文件错误，oid:%d", cmt.Oid)
			}
		}

		logger.GetLogger().Infof("追加评论至csv文件成功，oid:%d", cmts[0].Oid)

	} else {
		file, err := os.Create(csv_path)
		if err != nil {
			logger.GetLogger().Errorf("创建csv文件错误，oid:%d", cmts[0].Oid)
			return
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()
		headers := "bvid,upname,sex,content,pictures,rpid,oid,mid,parent,fans_grade,ctime,like,following,level,location"
		headerErr := writer.Write(strings.Split(headers, ","))
		if headerErr != nil {
			logger.GetLogger().Errorf("写入csv文件字段错误，oid:%d", cmts[0].Oid)
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
				logger.GetLogger().Errorf("写入csv文件错误，oid:%d", cmt.Oid)
				return
			}
		}
		logger.GetLogger().Infof("写入csv文件成功，oid:%d", cmts[0].Oid)
	}

}
