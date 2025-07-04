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
	Short: "获取视频评论，支持单个和多个视频",
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
