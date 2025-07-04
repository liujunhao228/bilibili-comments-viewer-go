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
